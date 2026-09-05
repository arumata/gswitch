package runtime

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"testing"
)

// Capture emitted evdev records in a regular file; never create host input devices.
func TestCustomConversionModifiers(t *testing.T) {
	for _, swap := range []bool{false, true} {
		for _, side := range []struct {
			name        string
			ctrl, shift uint16
		}{
			{"left", KEY_LEFTCTRL, KEY_LEFTSHIFT}, {"right", KEY_RIGHTCTRL, KEY_RIGHTSHIFT},
		} {
			for _, mode := range []string{"word", "line", "selection", "case"} {
				for _, modifierFirst := range []bool{false, true} {
					t.Run(fmt.Sprintf("swap=%t/%s/%s/modifierFirst=%t", swap, side.name, mode, modifierFirst), func(t *testing.T) {
						checkCustomConversionModifiers(t, modifierScenario{swap, side.ctrl, side.shift, mode, modifierFirst})
					})
				}
			}
		}
	}
}

type modifierScenario struct {
	swap          bool
	ctrl, shift   uint16
	mode          string
	modifierFirst bool
}

func checkCustomConversionModifiers(t *testing.T, tc modifierScenario) {
	t.Helper()
	output, err := os.CreateTemp(t.TempDir(), "events")
	if err != nil {
		t.Fatal(err)
	}
	defer output.Close()
	var selected []selectionTransform
	s := &Switcher{
		config:    &Config{ConvertKey: testKeyPause, SwapConversionModifiers: tc.swap},
		converter: NewConverter(), vKeyboard: &VirtualKeyboard{fd: output}, inputReader: &InputReader{},
		selectionHandler: func(transform selectionTransform) { selected = append(selected, transform) },
	}
	s.converter.ConvKey = testKeyPause
	send := func(code uint16, value int32) { s.processKeyEvent(&InputEvent{Type: EV_KEY, Code: code, Value: value}) }
	for _, code := range []uint16{testKeyA, KEY_SPACE, testKeyA} {
		send(code, K_DOWN)
		send(code, K_UP)
	}
	lineMod, selectionMod := tc.shift, tc.ctrl
	if tc.swap {
		lineMod, selectionMod = tc.ctrl, tc.shift
	}
	var mods []uint16
	switch tc.mode {
	case "line":
		mods = []uint16{lineMod}
	case "selection":
		mods = []uint16{selectionMod}
	case "case":
		mods = []uint16{tc.ctrl, tc.shift}
	}
	for _, mod := range mods {
		send(mod, K_DOWN)
	}
	send(testKeyPause, K_DOWN)
	send(testKeyPause, K_REPEAT)
	assertWaiting := func() {
		t.Helper()
		stat, err := output.Stat()
		if err != nil {
			t.Fatal(err)
		}
		if stat.Size() != 0 || len(selected) != 0 {
			t.Fatal("conversion ran before gesture release")
		}
	}
	assertWaiting()
	if tc.modifierFirst {
		for _, mod := range mods {
			send(mod, K_UP)
			assertWaiting()
		}
		send(testKeyPause, K_UP)
	} else {
		send(testKeyPause, K_UP)
		for i, mod := range mods {
			if i == 0 {
				assertWaiting()
			}
			send(mod, K_UP)
		}
	}
	if tc.mode == "selection" || tc.mode == "case" {
		want := selectionConvertLayout
		if tc.mode == "case" {
			want = selectionSwapCase
		}
		if len(selected) != 1 || selected[0] != want {
			t.Fatalf("selection calls = %v, want [%v]", selected, want)
		}
		stat, err := output.Stat()
		if err != nil {
			t.Fatal(err)
		}
		if stat.Size() != 0 {
			t.Fatal("selection also emitted buffered text")
		}
		return
	}
	if len(selected) != 0 {
		t.Fatalf("unexpected selection: %v", selected)
	}
	backspaces := capturedBackspaces(t, output, tc.ctrl)
	want := 1
	if tc.mode == "line" {
		want = 3
	}
	if backspaces != want {
		t.Fatalf("backspaces = %d, want %d", backspaces, want)
	}
	action := ActionConvertWord
	if tc.mode == "line" {
		action = ActionConvertAll
	}
	if !s.converter.CanUndo(action) {
		t.Fatal("conversion did not arm undo")
	}
	for _, mod := range mods {
		send(mod, K_DOWN)
	}
	send(testKeyPause, K_DOWN)
	send(testKeyPause, K_UP)
	for _, mod := range mods {
		send(mod, K_UP)
	}
	if s.converter.CanUndo(action) || capturedBackspaces(t, output, tc.ctrl) != 2*want {
		t.Fatal("repeated gesture did not complete undo")
	}
}

func capturedBackspaces(t *testing.T, output *os.File, ctrl uint16) int {
	t.Helper()
	if _, err := output.Seek(0, 0); err != nil {
		t.Fatal(err)
	}
	backspaces := 0
	for {
		var event InputEvent
		err := binary.Read(output, binary.LittleEndian, &event)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if event.Type != EV_KEY {
			continue
		}
		if event.Code == KEY_BACKSPACE && event.Value == K_DOWN {
			backspaces++
		}
		if event.Code == testKeyPause || event.Code == ctrl {
			t.Fatalf("trigger leaked into replay: %+v", event)
		}
	}
	return backspaces
}
