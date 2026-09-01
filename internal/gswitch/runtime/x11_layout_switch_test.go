package runtime

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

type fakeLayoutGroupReader struct {
	groups []uint8
}

func (reader *fakeLayoutGroupReader) CurrentGroup() (uint8, error) {
	if len(reader.groups) == 0 {
		return 0, nil
	}
	group := reader.groups[0]
	if len(reader.groups) > 1 {
		reader.groups = reader.groups[1:]
	}
	return group, nil
}

func TestTriggerAndConfirmLayoutSwitchAcceptsObservedTransition(t *testing.T) {
	reader := &fakeLayoutGroupReader{groups: []uint8{0, 1}}
	var emitted []KeyEvent

	err := triggerAndConfirmLayoutSwitch(
		reader,
		[]uint16{186},
		func(event KeyEvent) error {
			emitted = append(emitted, event)
			return nil
		},
		2,
		2,
		60*time.Millisecond,
		func(time.Duration) {},
	)
	if err != nil {
		t.Fatalf("triggerAndConfirmLayoutSwitch() error = %v", err)
	}

	want := []KeyEvent{{Code: 186, Value: K_DOWN}, {Code: 186, Value: K_UP}}
	if !reflect.DeepEqual(emitted, want) {
		t.Fatalf("emitted = %#v, want %#v", emitted, want)
	}
}

func TestTriggerAndConfirmLayoutSwitchDoesNotRetryDelayedTransition(t *testing.T) {
	reader := &fakeLayoutGroupReader{groups: []uint8{0, 0, 1}}
	var emitted []KeyEvent

	err := triggerAndConfirmLayoutSwitch(
		reader,
		[]uint16{186},
		func(event KeyEvent) error {
			emitted = append(emitted, event)
			return nil
		},
		2,
		2,
		60*time.Millisecond,
		func(time.Duration) {},
	)
	if err != nil {
		t.Fatalf("triggerAndConfirmLayoutSwitch() error = %v", err)
	}
	if len(emitted) != 2 {
		t.Fatalf("emitted %d events, want one key pair", len(emitted))
	}
}

func TestTriggerAndConfirmLayoutSwitchRetriesMissedTransitionOnce(t *testing.T) {
	reader := &fakeLayoutGroupReader{groups: []uint8{0, 0, 0, 1}}
	var emitted []KeyEvent

	err := triggerAndConfirmLayoutSwitch(
		reader,
		[]uint16{186},
		func(event KeyEvent) error {
			emitted = append(emitted, event)
			return nil
		},
		2,
		2,
		60*time.Millisecond,
		func(time.Duration) {},
	)
	if err != nil {
		t.Fatalf("triggerAndConfirmLayoutSwitch() error = %v", err)
	}
	if len(emitted) != 4 {
		t.Fatalf("emitted %d events, want two key pairs", len(emitted))
	}
}

func TestTriggerAndConfirmLayoutSwitchFailsBeforeTextMutation(t *testing.T) {
	reader := &fakeLayoutGroupReader{groups: []uint8{0, 0, 0, 0, 0}}
	var emitted []KeyEvent

	err := triggerAndConfirmLayoutSwitch(
		reader,
		[]uint16{186},
		func(event KeyEvent) error {
			emitted = append(emitted, event)
			return nil
		},
		2,
		2,
		60*time.Millisecond,
		func(time.Duration) {},
	)
	if err == nil || !strings.Contains(err.Error(), "layout group did not change") {
		t.Fatalf("error = %v, want group timeout", err)
	}
	for _, event := range emitted {
		if event.Code != 186 {
			t.Fatalf("timeout emitted text mutation event: %#v", event)
		}
	}
}

func TestXKBStateRequestEncoding(t *testing.T) {
	const opcode = 135
	if got, want := xkbUseExtensionRequest(opcode), []byte{135, 0, 2, 0, 1, 0, 0, 0}; !reflect.DeepEqual(got, want) {
		t.Fatalf("xkbUseExtensionRequest() = %v, want %v", got, want)
	}
	if got, want := xkbGetStateRequest(opcode), []byte{135, 4, 2, 0, 0, 1, 0, 0}; !reflect.DeepEqual(got, want) {
		t.Fatalf("xkbGetStateRequest() = %v, want %v", got, want)
	}
}
