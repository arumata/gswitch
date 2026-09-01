package detect

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestEnsureGNOMEX11Launch7BindingAppendsAndVerifies(t *testing.T) {
	originalRunner := runGsettingsForLaunch7
	t.Cleanup(func() { runGsettingsForLaunch7 = originalRunner })

	var calls [][]string
	responses := []struct {
		output string
		err    error
	}{
		{output: "['<Super>space', 'XF86Keyboard']\n"},
		{},
		{output: "['<Super>space', 'XF86Keyboard', 'XF86Launch7']\n"},
	}
	runGsettingsForLaunch7 = func(_ *SessionEnv, args ...string) ([]byte, error) {
		calls = append(calls, append([]string(nil), args...))
		response := responses[len(calls)-1]
		return []byte(response.output), response.err
	}

	changed, err := EnsureGNOMEX11Launch7Binding(gnomeX11TestEnv())
	if err != nil {
		t.Fatalf("EnsureGNOMEX11Launch7Binding() error = %v", err)
	}
	if !changed {
		t.Fatal("EnsureGNOMEX11Launch7Binding() changed = false, want true")
	}

	wantCalls := [][]string{
		{"get", gnomeWMKeybindingsSchema, gnomeSwitchInputSourceKey},
		{"set", gnomeWMKeybindingsSchema, gnomeSwitchInputSourceKey, "['<Super>space', 'XF86Keyboard', 'XF86Launch7']"},
		{"get", gnomeWMKeybindingsSchema, gnomeSwitchInputSourceKey},
	}
	if !reflect.DeepEqual(calls, wantCalls) {
		t.Fatalf("gsettings calls = %#v, want %#v", calls, wantCalls)
	}
}

func TestEnsureGNOMEX11Launch7BindingIsIdempotent(t *testing.T) {
	originalRunner := runGsettingsForLaunch7
	t.Cleanup(func() { runGsettingsForLaunch7 = originalRunner })

	var calls int
	runGsettingsForLaunch7 = func(_ *SessionEnv, _ ...string) ([]byte, error) {
		calls++
		return []byte("['<Super>space', 'XF86Launch7']\n"), nil
	}

	changed, err := EnsureGNOMEX11Launch7Binding(gnomeX11TestEnv())
	if err != nil {
		t.Fatalf("EnsureGNOMEX11Launch7Binding() error = %v", err)
	}
	if changed {
		t.Fatal("EnsureGNOMEX11Launch7Binding() changed = true, want false")
	}
	if calls != 1 {
		t.Fatalf("gsettings calls = %d, want 1", calls)
	}
}

func TestEnsureGNOMEX11Launch7BindingSkipsOtherSessions(t *testing.T) {
	originalRunner := runGsettingsForLaunch7
	t.Cleanup(func() { runGsettingsForLaunch7 = originalRunner })

	runGsettingsForLaunch7 = func(_ *SessionEnv, args ...string) ([]byte, error) {
		t.Fatalf("unexpected gsettings call: %v", args)
		return nil, nil
	}

	tests := []struct {
		name string
		env  *SessionEnv
	}{
		{name: "GNOME Wayland", env: &SessionEnv{XDGCurrentDesktop: "GNOME", SessionType: "wayland", WaylandDisplay: "wayland-0"}},
		{name: "KDE X11", env: &SessionEnv{XDGCurrentDesktop: "KDE", SessionType: "x11", Display: ":0"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			changed, err := EnsureGNOMEX11Launch7Binding(tt.env)
			if err != nil {
				t.Fatalf("EnsureGNOMEX11Launch7Binding() error = %v", err)
			}
			if changed {
				t.Fatal("EnsureGNOMEX11Launch7Binding() changed = true, want false")
			}
		})
	}
}

func TestEnsureGNOMEX11Launch7BindingRollsBackFailedVerification(t *testing.T) {
	originalRunner := runGsettingsForLaunch7
	t.Cleanup(func() { runGsettingsForLaunch7 = originalRunner })

	var calls [][]string
	responses := []struct {
		output string
		err    error
	}{
		{output: "['<Super>space']\n"},
		{},
		{output: "['<Super>space']\n"},
		{},
	}
	runGsettingsForLaunch7 = func(_ *SessionEnv, args ...string) ([]byte, error) {
		calls = append(calls, append([]string(nil), args...))
		response := responses[len(calls)-1]
		return []byte(response.output), response.err
	}

	changed, err := EnsureGNOMEX11Launch7Binding(gnomeX11TestEnv())
	if err == nil || !strings.Contains(err.Error(), "verify XF86Launch7 binding") {
		t.Fatalf("EnsureGNOMEX11Launch7Binding() error = %v, want verification error", err)
	}
	if changed {
		t.Fatal("EnsureGNOMEX11Launch7Binding() changed = true, want false")
	}

	wantRollback := []string{"set", gnomeWMKeybindingsSchema, gnomeSwitchInputSourceKey, "['<Super>space']"}
	if !reflect.DeepEqual(calls[len(calls)-1], wantRollback) {
		t.Fatalf("rollback call = %#v, want %#v", calls[len(calls)-1], wantRollback)
	}
}

func TestEnsureGNOMEX11Launch7BindingReportsSetFailure(t *testing.T) {
	originalRunner := runGsettingsForLaunch7
	t.Cleanup(func() { runGsettingsForLaunch7 = originalRunner })

	setErr := errors.New("write denied")
	var calls int
	runGsettingsForLaunch7 = func(_ *SessionEnv, _ ...string) ([]byte, error) {
		calls++
		if calls == 1 {
			return []byte("['<Super>space']\n"), nil
		}
		return nil, setErr
	}

	changed, err := EnsureGNOMEX11Launch7Binding(gnomeX11TestEnv())
	if !errors.Is(err, setErr) {
		t.Fatalf("EnsureGNOMEX11Launch7Binding() error = %v, want %v", err, setErr)
	}
	if changed {
		t.Fatal("EnsureGNOMEX11Launch7Binding() changed = true, want false")
	}
	if calls != 2 {
		t.Fatalf("gsettings calls = %d, want 2", calls)
	}
}

func gnomeX11TestEnv() *SessionEnv {
	return &SessionEnv{
		XDGCurrentDesktop: "ubuntu:GNOME",
		SessionType:       "x11",
		Display:           ":0",
	}
}
