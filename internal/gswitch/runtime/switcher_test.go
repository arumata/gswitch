package runtime

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

// fastWaitConfig returns a configuration with very short intervals for testing.
func fastWaitConfig() WaitConfig {
	return WaitConfig{
		InitialInterval: 10 * time.Millisecond,
		MaxInterval:     50 * time.Millisecond,
		LogInterval:     100 * time.Millisecond,
	}
}

func testSessionEnv() *SessionEnv {
	return &SessionEnv{User: "testuser", Display: ":0"}
}

// TestWaitForSessionImmediateSuccess tests that waitForSession returns immediately
// when GetActiveSessionEnv succeeds on first call.
func TestWaitForSessionImmediateSuccess(t *testing.T) {
	orig := getActiveSessionEnv
	defer func() { getActiveSessionEnv = orig }()

	getActiveSessionEnv = func() (*SessionEnv, error) {
		return testSessionEnv(), nil
	}

	s := &Switcher{debug: true}
	ctx := context.Background()

	start := time.Now()
	env, err := s.waitForSessionWithConfig(ctx, fastWaitConfig())
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("waitForSession() error = %v", err)
	}
	if env == nil {
		t.Fatal("waitForSession() returned nil env")
	}
	if env.User != "testuser" {
		t.Errorf("User = %q, want %q", env.User, "testuser")
	}
	if elapsed > 500*time.Millisecond {
		t.Errorf("waitForSession() took %v, expected immediate return", elapsed)
	}
}

// TestWaitForSessionContextCancellation tests that waitForSession returns
// ctx.Err() when context is canceled.
func TestWaitForSessionContextCancellation(t *testing.T) {
	orig := getActiveSessionEnv
	defer func() { getActiveSessionEnv = orig }()

	getActiveSessionEnv = func() (*SessionEnv, error) {
		return nil, ErrNoActiveSession
	}

	s := &Switcher{debug: true}
	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	start := time.Now()
	env, err := s.waitForSessionWithConfig(ctx, fastWaitConfig())
	elapsed := time.Since(start)

	if !errors.Is(err, context.Canceled) {
		t.Errorf("waitForSession() error = %v, want context.Canceled", err)
	}
	if env != nil {
		t.Errorf("waitForSession() env = %v, want nil", env)
	}
	if elapsed > 500*time.Millisecond {
		t.Errorf("waitForSession() took %v after cancellation, expected faster", elapsed)
	}
}

// TestWaitForSessionBackoff tests that waitForSession uses exponential backoff.
func TestWaitForSessionBackoff(t *testing.T) {
	orig := getActiveSessionEnv
	defer func() { getActiveSessionEnv = orig }()

	var attempts atomic.Int32
	getActiveSessionEnv = func() (*SessionEnv, error) {
		if attempts.Add(1) >= 3 {
			return testSessionEnv(), nil
		}
		return nil, ErrNoActiveSession
	}

	s := &Switcher{debug: true}
	ctx := context.Background()

	cfg := fastWaitConfig()
	start := time.Now()
	env, err := s.waitForSessionWithConfig(ctx, cfg)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("waitForSession() error = %v", err)
	}
	if env == nil {
		t.Fatal("waitForSession() returned nil env")
	}
	if elapsed < 20*time.Millisecond {
		t.Errorf("waitForSession() took %v, expected at least 20ms (backoff)", elapsed)
	}
	if elapsed > 500*time.Millisecond {
		t.Errorf("waitForSession() took %v, expected less than 500ms", elapsed)
	}
	if attempts.Load() < 3 {
		t.Errorf("waitForSession() made %d attempts, expected at least 3", attempts.Load())
	}
}

// TestWaitForSessionRealError tests that waitForSession returns real errors
// (not ErrNoActiveSession).
func TestWaitForSessionRealError(t *testing.T) {
	orig := getActiveSessionEnv
	defer func() { getActiveSessionEnv = orig }()

	getActiveSessionEnv = func() (*SessionEnv, error) {
		return nil, errors.New("loginctl failed")
	}

	s := &Switcher{debug: true}
	ctx := context.Background()
	env, err := s.waitForSessionWithConfig(ctx, fastWaitConfig())

	if err == nil {
		t.Fatal("waitForSession() should return error when loginctl fails")
	}
	if errors.Is(err, ErrNoActiveSession) {
		t.Errorf("waitForSession() should not return ErrNoActiveSession for real errors")
	}
	if env != nil {
		t.Errorf("waitForSession() env = %v, want nil on error", env)
	}
}

// TestWaitForSessionWithTimeout tests waitForSession with a timeout context.
func TestWaitForSessionWithTimeout(t *testing.T) {
	orig := getActiveSessionEnv
	defer func() { getActiveSessionEnv = orig }()

	getActiveSessionEnv = func() (*SessionEnv, error) {
		return nil, ErrNoActiveSession
	}

	s := &Switcher{debug: true}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	start := time.Now()
	env, err := s.waitForSessionWithConfig(ctx, fastWaitConfig())
	elapsed := time.Since(start)

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("waitForSession() error = %v, want context.DeadlineExceeded", err)
	}
	if env != nil {
		t.Errorf("waitForSession() env = %v, want nil", env)
	}
	if elapsed > 500*time.Millisecond {
		t.Errorf("waitForSession() took %v, expected timeout within 500ms", elapsed)
	}
}

func TestSwitcherStopSignalHandlerIdempotent(_ *testing.T) {
	s := &Switcher{}
	s.setupSignalHandler()
	s.stopSignalHandler()
	s.stopSignalHandler()
}

func TestPrepareSessionEnvironmentNonRootUsesCurrentEnvironment(t *testing.T) {
	orig := getActiveSessionEnv
	defer func() { getActiveSessionEnv = orig }()

	var calls atomic.Int32
	getActiveSessionEnv = func() (*SessionEnv, error) {
		calls.Add(1)
		return testSessionEnv(), nil
	}

	s := &Switcher{ctx: context.Background(), debug: true}
	env, err := s.prepareSessionEnvironment(true, false)
	if err != nil {
		t.Fatalf("prepareSessionEnvironment() error = %v", err)
	}
	if env != nil {
		t.Fatalf("prepareSessionEnvironment() env = %v, want nil", env)
	}
	if calls.Load() != 0 {
		t.Fatalf("GetActiveSessionEnv called %d times, want 0", calls.Load())
	}
}

func TestAddDeviceWithRetryEventuallySucceeds(t *testing.T) {
	var calls atomic.Int32
	want := &Device{Path: "/dev/input/event-test"}
	add := func(_ string) (*Device, error) {
		if calls.Add(1) < 3 {
			return nil, fmt.Errorf("open device: %w", syscall.EACCES)
		}
		return want, nil
	}

	got, err := addDeviceWithRetry(context.Background(), want.Path, 4, time.Millisecond, add)
	if err != nil {
		t.Fatalf("addDeviceWithRetry() error = %v", err)
	}
	if got != want {
		t.Fatalf("addDeviceWithRetry() = %v, want %v", got, want)
	}
	if calls.Load() != 3 {
		t.Fatalf("add attempts = %d, want 3", calls.Load())
	}
}

func TestAddDeviceWithRetryDoesNotRetryPermanentError(t *testing.T) {
	var calls atomic.Int32
	wantErr := errors.New("not a keyboard")
	add := func(_ string) (*Device, error) {
		calls.Add(1)
		return nil, wantErr
	}

	_, err := addDeviceWithRetry(context.Background(), "event-test", 4, time.Millisecond, add)
	if !errors.Is(err, wantErr) {
		t.Fatalf("addDeviceWithRetry() error = %v, want %v", err, wantErr)
	}
	if calls.Load() != 1 {
		t.Fatalf("add attempts = %d, want 1", calls.Load())
	}
}

func TestAddDeviceWithRetryStopsOnCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	add := func(_ string) (*Device, error) {
		cancel()
		return nil, fmt.Errorf("open device: %w", os.ErrNotExist)
	}

	_, err := addDeviceWithRetry(ctx, "event-test", 4, time.Second, add)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("addDeviceWithRetry() error = %v, want context.Canceled", err)
	}
}
