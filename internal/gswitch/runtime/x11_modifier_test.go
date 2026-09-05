package runtime

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/jezek/xgb/xproto"
)

type fakeModifierStateReader struct {
	masks []uint16
	err   error
}

func (reader *fakeModifierStateReader) ModifierMask() (uint16, error) {
	if reader.err != nil {
		return 0, reader.err
	}
	mask := reader.masks[0]
	if len(reader.masks) > 1 {
		reader.masks = reader.masks[1:]
	}
	return mask, nil
}

func TestWaitForSelectionModifiersReleasedObservesX11Barrier(t *testing.T) {
	reader := &fakeModifierStateReader{masks: []uint16{
		xproto.ModMaskControl | xproto.ModMaskShift,
		xproto.ModMaskControl,
		0,
	}}
	var pauses []time.Duration

	err := waitForSelectionModifiersReleased(reader, 3, func(delay time.Duration) {
		pauses = append(pauses, delay)
	})
	if err != nil {
		t.Fatalf("waitForSelectionModifiersReleased() error = %v", err)
	}
	want := []time.Duration{modifierReleasePollDelay, modifierReleasePollDelay}
	if !reflect.DeepEqual(pauses, want) {
		t.Fatalf("pauses = %v, want %v", pauses, want)
	}
}

func TestWaitForSelectionModifiersReleasedFailsClosed(t *testing.T) {
	t.Run("timeout", func(t *testing.T) {
		reader := &fakeModifierStateReader{masks: []uint16{xproto.ModMaskControl}}
		if err := waitForSelectionModifiersReleased(reader, 2, func(time.Duration) {}); err == nil {
			t.Fatal("waitForSelectionModifiersReleased() error = nil, want timeout")
		}
	})

	t.Run("query error", func(t *testing.T) {
		queryErr := errors.New("query failed")
		reader := &fakeModifierStateReader{err: queryErr}
		if err := waitForSelectionModifiersReleased(reader, 2, func(time.Duration) {}); !errors.Is(err, queryErr) {
			t.Fatalf("waitForSelectionModifiersReleased() error = %v, want %v", err, queryErr)
		}
	})
}
