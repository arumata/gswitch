package runtime

import (
	"errors"
	"reflect"
	"testing"
	"time"
)

type sequenceLayoutStateReader struct {
	states []string
	err    error
	index  int
}

func (reader *sequenceLayoutStateReader) CurrentState() (string, error) {
	if reader.err != nil {
		return "", reader.err
	}
	if reader.index >= len(reader.states) {
		return reader.states[len(reader.states)-1], nil
	}
	state := reader.states[reader.index]
	reader.index++
	return state, nil
}

func TestTriggerAndConfirmGNOMEWaylandLayoutSwitchRetries(t *testing.T) {
	reader := &sequenceLayoutStateReader{
		states: []string{"us", "us", "us", "ua"},
	}
	emitted := make([]KeyEvent, 0, 8)

	err := triggerAndConfirmGNOMEWaylandLayoutSwitch(
		reader,
		[]uint16{KEY_LEFTMETA, KEY_SPACE},
		func(event KeyEvent) error {
			emitted = append(emitted, event)
			return nil
		},
		2,
		2,
		0,
		func(time.Duration) {},
	)
	if err != nil {
		t.Fatalf("triggerAndConfirmGNOMEWaylandLayoutSwitch() error = %v", err)
	}
	if len(emitted) != 8 {
		t.Fatalf("emitted events = %d, want 8 for two attempts", len(emitted))
	}
}

func TestTriggerAndConfirmGNOMEWaylandLayoutSwitchRejectsNoChange(t *testing.T) {
	reader := &sequenceLayoutStateReader{states: []string{"us"}}

	err := triggerAndConfirmGNOMEWaylandLayoutSwitch(
		reader,
		[]uint16{KEY_LEFTMETA, KEY_SPACE},
		func(KeyEvent) error { return nil },
		2,
		2,
		0,
		func(time.Duration) {},
	)
	if err == nil {
		t.Fatal("triggerAndConfirmGNOMEWaylandLayoutSwitch() error = nil, want unchanged source error")
	}
}

func TestTriggerAndConfirmGNOMEWaylandLayoutSwitchReturnsReaderError(t *testing.T) {
	want := errors.New("gsettings unavailable")
	reader := &sequenceLayoutStateReader{err: want}

	err := triggerAndConfirmGNOMEWaylandLayoutSwitch(
		reader,
		[]uint16{KEY_LEFTMETA, KEY_SPACE},
		func(KeyEvent) error { return nil },
		2,
		2,
		0,
		func(time.Duration) {},
	)
	if !errors.Is(err, want) {
		t.Fatalf("triggerAndConfirmGNOMEWaylandLayoutSwitch() error = %v, want %v", err, want)
	}
}

func TestShouldVerifyGNOMEWaylandLayoutSwitch(t *testing.T) {
	result := &DetectionResult{Source: SourceGNOME, RawValue: "<Super>space"}
	wayland := &SessionEnv{
		XDGCurrentDesktop: "GNOME",
		SessionType:       "wayland",
		WaylandDisplay:    "wayland-0",
	}
	if !shouldVerifyGNOMEWaylandLayoutSwitch(result, wayland) {
		t.Fatal("GNOME Wayland detection must enable input-source acknowledgement")
	}
	if shouldVerifyGNOMEWaylandLayoutSwitch(result, testSessionEnv()) {
		t.Fatal("GNOME X11 must not enable Wayland input-source acknowledgement")
	}
	if shouldVerifyGNOMEWaylandLayoutSwitch(
		&DetectionResult{Source: SourceXKB, RawValue: "<Super>space"},
		wayland,
	) {
		t.Fatal("non-GNOME detection must not enable GNOME Wayland acknowledgement")
	}
}

func TestGNOMEWaylandLayoutStateReaderReadsMRUSources(t *testing.T) {
	original := runGsettingsForGNOMEWayland
	t.Cleanup(func() { runGsettingsForGNOMEWayland = original })
	env := &SessionEnv{User: "test"}
	var gotArgs []string
	runGsettingsForGNOMEWayland = func(gotEnv *SessionEnv, args ...string) ([]byte, error) {
		if gotEnv != env {
			t.Fatalf("RunGsettings() env = %p, want %p", gotEnv, env)
		}
		gotArgs = append([]string(nil), args...)
		return []byte("  [('xkb', 'ua'), ('xkb', 'us')]\n"), nil
	}

	state, err := (gnomeWaylandLayoutStateReader{env: env}).CurrentState()
	if err != nil {
		t.Fatalf("CurrentState() error = %v", err)
	}
	if state != "[('xkb', 'ua'), ('xkb', 'us')]" {
		t.Fatalf("CurrentState() = %q", state)
	}
	wantArgs := []string{
		"get",
		"org.gnome.desktop.input-sources",
		"mru-sources",
	}
	if !reflect.DeepEqual(gotArgs, wantArgs) {
		t.Fatalf("RunGsettings() args = %#v, want %#v", gotArgs, wantArgs)
	}
}
