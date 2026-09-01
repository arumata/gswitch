package runtime

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	original := os.Stdout
	os.Stdout = writer
	t.Cleanup(func() { os.Stdout = original })

	fn()
	if err := writer.Close(); err != nil {
		t.Fatalf("close stdout writer: %v", err)
	}
	os.Stdout = original

	output, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("read captured stdout: %v", err)
	}
	if err := reader.Close(); err != nil {
		t.Fatalf("close stdout reader: %v", err)
	}
	return string(output)
}

func TestProcessKeyEventDebugOutputOmitsKeystrokeContent(t *testing.T) {
	s := &Switcher{
		config:    &Config{},
		converter: NewConverter(),
		debug:     true,
	}
	s.converter.SetDebugLogger(s.logDebug)

	output := captureStdout(t, func() {
		s.processKeyEvent(&InputEvent{Code: testKeyA, Value: K_DOWN})
	})

	for _, sensitive := range []string{"input A down", "Buffer: A_DOWN"} {
		if strings.Contains(output, sensitive) {
			t.Fatalf("debug output exposes keystroke content %q: %q", sensitive, output)
		}
	}
	if !strings.Contains(output, "buffer length=1") {
		t.Fatalf("debug output = %q, expected non-sensitive buffer length", output)
	}
}

func TestTextDebugSummaryOmitsContent(t *testing.T) {
	secret := "private selected text"
	got := textDebugSummary("selected text", secret)
	if strings.Contains(got, secret) {
		t.Fatalf("textDebugSummary() exposed content: %q", got)
	}
	if got != "selected text length=21 runes" {
		t.Fatalf("textDebugSummary() = %q", got)
	}
}

func TestSwapCase(t *testing.T) {
	tests := []struct {
		name string
		text string
		want string
	}{
		{name: "latin", text: "Hello, WORLD! 123", want: "hELLO, world! 123"},
		{name: "cyrillic", text: "Привет, МИР! ёЖ", want: "пРИВЕТ, мир! Ёж"},
		{name: "symbols", text: "123 — +_", want: "123 — +_"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := swapCase(tt.text); got != tt.want {
				t.Fatalf("swapCase(%q) = %q, want %q", tt.text, got, tt.want)
			}
		})
	}
}

func TestProcessKeyEventRoutesSelectionTransforms(t *testing.T) {
	tests := []struct {
		name    string
		convKey uint16
		events  []InputEvent
		want    selectionTransform
	}{
		{
			name:    "custom key layout conversion",
			convKey: testKeyPause,
			events: []InputEvent{
				{Code: KEY_LEFTCTRL, Value: K_DOWN},
				{Code: testKeyPause, Value: K_DOWN},
			},
			want: selectionConvertLayout,
		},
		{
			name:    "custom key swap case",
			convKey: testKeyPause,
			events: []InputEvent{
				{Code: KEY_LEFTCTRL, Value: K_DOWN},
				{Code: testKeyLeftShift, Value: K_DOWN},
				{Code: testKeyPause, Value: K_DOWN},
			},
			want: selectionSwapCase,
		},
		{
			name: "double shift layout conversion",
			events: []InputEvent{
				{Code: KEY_LEFTCTRL, Value: K_DOWN},
				{Code: testKeyLeftShift, Value: K_DOWN},
				{Code: testKeyLeftShift, Value: K_UP},
				{Code: testKeyLeftShift, Value: K_DOWN},
				{Code: testKeyLeftShift, Value: K_UP},
			},
			want: selectionConvertLayout,
		},
		{
			name: "double shift swap case",
			events: []InputEvent{
				{Code: KEY_LEFTCTRL, Value: K_DOWN},
				{Code: testKeyLeftShift, Value: K_DOWN},
				{Code: testKeyRightShift, Value: K_DOWN},
				{Code: testKeyRightShift, Value: K_UP},
				{Code: testKeyRightShift, Value: K_DOWN},
				{Code: testKeyRightShift, Value: K_UP},
				{Code: testKeyLeftShift, Value: K_UP},
			},
			want: selectionSwapCase,
		},
		{
			name: "double shift swap case with ctrl released first",
			events: []InputEvent{
				{Code: KEY_LEFTCTRL, Value: K_DOWN},
				{Code: testKeyLeftShift, Value: K_DOWN},
				{Code: testKeyRightShift, Value: K_DOWN},
				{Code: testKeyRightShift, Value: K_UP},
				{Code: testKeyRightShift, Value: K_DOWN},
				{Code: testKeyRightShift, Value: K_UP},
				{Code: KEY_LEFTCTRL, Value: K_UP},
				{Code: testKeyLeftShift, Value: K_UP},
			},
			want: selectionSwapCase,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got []selectionTransform
			s := &Switcher{
				config:    &Config{ConvertKey: tt.convKey},
				converter: NewConverter(),
				selectionHandler: func(transform selectionTransform) {
					got = append(got, transform)
				},
			}
			s.converter.ConvKey = tt.convKey

			for i := range tt.events {
				s.processKeyEvent(&tt.events[i])
			}

			if len(got) != 1 {
				t.Fatalf("selection handler calls = %d, want 1", len(got))
			}
			if got[0] != tt.want {
				t.Fatalf("selection transform = %v, want %v", got[0], tt.want)
			}
		})
	}
}

func TestProcessKeyEventDoesNotReuseCanceledCtrlShiftGesture(t *testing.T) {
	var calls int
	s := &Switcher{
		config:    &Config{},
		converter: NewConverter(),
		selectionHandler: func(selectionTransform) {
			calls++
		},
	}

	events := []InputEvent{
		{Code: KEY_LEFTCTRL, Value: K_DOWN},
		{Code: testKeyLeftShift, Value: K_DOWN},
		{Code: testKeyLeftShift, Value: K_UP},
		{Code: KEY_LEFTCTRL, Value: K_UP},
		{Code: testKeyLeftShift, Value: K_DOWN},
		{Code: testKeyRightShift, Value: K_DOWN},
		{Code: testKeyRightShift, Value: K_UP},
		{Code: testKeyRightShift, Value: K_DOWN},
		{Code: testKeyRightShift, Value: K_UP},
		{Code: testKeyLeftShift, Value: K_UP},
	}
	for i := range events {
		s.processKeyEvent(&events[i])
	}

	if calls != 0 {
		t.Fatalf("selection handler calls = %d, want 0", calls)
	}
}

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

func TestPrepareStableGNOMEX11LayoutSwitchRunsForAutoDetection(t *testing.T) {
	originalEnsure := ensureGNOMEX11Launch7Binding
	t.Cleanup(func() { ensureGNOMEX11Launch7Binding = originalEnsure })

	env := testSessionEnv()
	var gotEnv *SessionEnv
	ensureGNOMEX11Launch7Binding = func(candidate *SessionEnv) (bool, error) {
		gotEnv = candidate
		return true, nil
	}

	s := &Switcher{sessionEnv: env, debug: true}
	if err := s.prepareStableGNOMEX11LayoutSwitch(true); err != nil {
		t.Fatalf("prepareStableGNOMEX11LayoutSwitch() error = %v", err)
	}
	if gotEnv != env {
		t.Fatalf("ensure env = %p, want %p", gotEnv, env)
	}
}

func TestPrepareStableGNOMEX11LayoutSwitchSkipsExplicitConfig(t *testing.T) {
	originalEnsure := ensureGNOMEX11Launch7Binding
	t.Cleanup(func() { ensureGNOMEX11Launch7Binding = originalEnsure })

	ensureGNOMEX11Launch7Binding = func(candidate *SessionEnv) (bool, error) {
		t.Fatalf("unexpected ensure call with env %#v", candidate)
		return false, nil
	}

	s := &Switcher{sessionEnv: testSessionEnv(), debug: true}
	if err := s.prepareStableGNOMEX11LayoutSwitch(false); err != nil {
		t.Fatalf("prepareStableGNOMEX11LayoutSwitch() error = %v", err)
	}
}

func TestPrepareStableGNOMEX11LayoutSwitchFailsClosed(t *testing.T) {
	originalEnsure := ensureGNOMEX11Launch7Binding
	t.Cleanup(func() { ensureGNOMEX11Launch7Binding = originalEnsure })

	ensureErr := errors.New("gsettings unavailable")
	ensureGNOMEX11Launch7Binding = func(*SessionEnv) (bool, error) {
		return false, ensureErr
	}

	s := &Switcher{sessionEnv: testSessionEnv(), debug: true}
	err := s.prepareStableGNOMEX11LayoutSwitch(true)
	if !errors.Is(err, ensureErr) {
		t.Fatalf("prepareStableGNOMEX11LayoutSwitch() error = %v, want %v", err, ensureErr)
	}
}

func TestShouldVerifyGNOMEX11LayoutSwitch(t *testing.T) {
	result := &DetectionResult{Source: SourceGNOME, RawValue: "XF86Launch7"}
	if !shouldVerifyGNOMEX11LayoutSwitch(result, testSessionEnv()) {
		t.Fatal("GNOME X11 Launch7 detection must enable group acknowledgement")
	}
	if shouldVerifyGNOMEX11LayoutSwitch(
		result,
		&SessionEnv{XDGCurrentDesktop: "GNOME", SessionType: "wayland", WaylandDisplay: "wayland-0"},
	) {
		t.Fatal("GNOME Wayland must not enable X11 group acknowledgement")
	}
	if shouldVerifyGNOMEX11LayoutSwitch(
		&DetectionResult{Source: SourceXKB, RawValue: "XF86Launch7"},
		testSessionEnv(),
	) {
		t.Fatal("non-GNOME detection must not enable GNOME X11 acknowledgement")
	}
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
