package runtime

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

// Key codes for tests
const (
	testKeyA          = 30
	testKeyB          = 48
	testKeyC          = 46
	testKeySpace      = 57
	testKeyEnter      = 28
	testKeyBackspace  = 14
	testKeyLeftShift  = 42
	testKeyRightShift = 54
	testKeyPause      = 119 // Example convert key
)

func TestConverter_NewConverter(t *testing.T) {
	c := NewConverter()
	if c == nil {
		t.Fatal("NewConverter returned nil")
	}
	if len(c.buffer) != 0 {
		t.Errorf("expected empty buffer, got %d elements", len(c.buffer))
	}
}

func TestConverter_Push_RegularKeys(t *testing.T) {
	c := NewConverter()

	// Push a regular key down
	modified := c.Push(testKeyA, K_DOWN)
	if !modified {
		t.Error("Push should return true for regular key")
	}
	if c.BufferLen() != 1 {
		t.Errorf("expected buffer length 1, got %d", c.BufferLen())
	}

	// Push key up should not add to buffer for regular keys
	modified = c.Push(testKeyA, K_UP)
	if modified {
		t.Error("Push should return false for regular key up")
	}

	// Repeat should NOT add to buffer (prevents duplicate chars on key hold)
	modified = c.Push(testKeyA, K_REPEAT)
	if modified {
		t.Error("Push should return false for regular key repeat")
	}
	if c.BufferLen() != 1 {
		t.Errorf("expected buffer length 1 after repeat, got %d", c.BufferLen())
	}
}

func TestConverter_Push_ShiftKeys(t *testing.T) {
	c := NewConverter()

	// Shift down
	modified := c.Push(testKeyLeftShift, K_DOWN)
	if !modified {
		t.Error("Push should return true for shift down")
	}

	// Shift up
	modified = c.Push(testKeyLeftShift, K_UP)
	if !modified {
		t.Error("Push should return true for shift up")
	}

	// Shift repeat should be ignored
	c.ClearBuffer()
	modified = c.Push(testKeyLeftShift, K_REPEAT)
	if modified {
		t.Error("Push should return false for shift repeat")
	}
}

func TestConverter_Push_Backspace(t *testing.T) {
	c := NewConverter()

	// Add some keys
	c.Push(testKeyA, K_DOWN)
	c.Push(testKeyB, K_DOWN)
	c.Push(testKeyC, K_DOWN)

	if c.BufferLen() != 3 {
		t.Errorf("expected buffer length 3, got %d", c.BufferLen())
	}

	// Backspace should remove last key
	c.Push(testKeyBackspace, K_DOWN)
	if c.BufferLen() != 2 {
		t.Errorf("expected buffer length 2 after backspace, got %d", c.BufferLen())
	}
}

func TestConverter_Push_KillerKeys(t *testing.T) {
	c := NewConverter()

	// Add some keys
	c.Push(testKeyA, K_DOWN)
	c.Push(testKeyB, K_DOWN)

	if c.BufferLen() != 2 {
		t.Errorf("expected buffer length 2, got %d", c.BufferLen())
	}

	// Tab (killer key) should clear buffer
	c.Push(15, K_DOWN) // KEY_TAB = 15
	if c.BufferLen() != 0 {
		t.Errorf("expected buffer to be cleared, got %d", c.BufferLen())
	}
}

func TestConverter_Push_BufferLimit(t *testing.T) {
	c := NewConverter()

	// Fill buffer beyond MaxKeyBufSize
	for range MaxKeyBufSize + 100 {
		c.Push(testKeyA, K_DOWN)
	}

	// Buffer should be trimmed
	if c.BufferLen() > MaxKeyBufSize {
		t.Errorf("buffer exceeded MaxKeyBufSize: got %d", c.BufferLen())
	}
}

func TestConverter_HasText(t *testing.T) {
	c := NewConverter()

	// Empty buffer
	if c.HasText() {
		t.Error("empty buffer should have no text")
	}

	// Only shift keys
	c.Push(testKeyLeftShift, K_DOWN)
	c.Push(testKeyLeftShift, K_UP)
	if c.HasText() {
		t.Error("buffer with only shift keys should have no text")
	}

	// With regular key
	c.Push(testKeyA, K_DOWN)
	if !c.HasText() {
		t.Error("buffer with regular key should have text")
	}
}

func TestConverter_ClearBuffer(t *testing.T) {
	c := NewConverter()

	c.Push(testKeyA, K_DOWN)
	c.Push(testKeyB, K_DOWN)

	c.ClearBuffer()
	if c.BufferLen() != 0 {
		t.Errorf("buffer should be empty after clear, got %d", c.BufferLen())
	}
}

func TestConverter_Process_DoubleShift_ConvertWord(t *testing.T) {
	c := NewConverter()
	c.ConvKey = 0 // double-shift mode

	// Type some text
	c.Push(testKeyA, K_DOWN)
	c.Push(testKeyB, K_DOWN)
	c.Push(testKeyC, K_DOWN)

	// Double-shift pattern: shift down, up, down, up (same shift)
	c.Push(testKeyLeftShift, K_DOWN)
	c.Push(testKeyLeftShift, K_UP)
	c.Push(testKeyLeftShift, K_DOWN)
	c.Push(testKeyLeftShift, K_UP)

	action := c.Process()
	if action != ActionConvertWord {
		t.Errorf("expected ActionConvertWord, got %v", action)
	}
}

func TestConverter_Process_DoubleShift_ConvertAll(t *testing.T) {
	c := NewConverter()
	c.ConvKey = 0 // double-shift mode

	// Type some text
	c.Push(testKeyA, K_DOWN)
	c.Push(testKeyB, K_DOWN)
	c.Push(testKeyC, K_DOWN)

	// Hold one shift, double-tap the other
	// Pattern: shift1 down, shift2 down, shift2 up, shift2 down, shift2 up, shift1 up
	c.Push(testKeyLeftShift, K_DOWN)
	c.Push(testKeyRightShift, K_DOWN)
	c.Push(testKeyRightShift, K_UP)
	c.Push(testKeyRightShift, K_DOWN)
	c.Push(testKeyRightShift, K_UP)
	c.Push(testKeyLeftShift, K_UP)

	action := c.Process()
	if action != ActionConvertAll {
		t.Errorf("expected ActionConvertAll, got %v", action)
	}
}

func TestConverter_Process_DoubleShift_NoText(t *testing.T) {
	c := NewConverter()
	c.ConvKey = 0 // double-shift mode

	// Double-shift without text: no buffer conversion, but the trigger
	// itself must be reported so Ctrl+DoubleShift can convert the selection
	c.Push(testKeyLeftShift, K_DOWN)
	c.Push(testKeyLeftShift, K_UP)
	c.Push(testKeyLeftShift, K_DOWN)
	c.Push(testKeyLeftShift, K_UP)

	action := c.Process()
	if action != ActionDoubleShiftNoText {
		t.Errorf("expected ActionDoubleShiftNoText for double-shift without text, got %v", action)
	}
	if len(c.buffer) != 0 {
		t.Errorf("expected buffer to be trimmed, got %d events", len(c.buffer))
	}
}

func TestConverter_Process_DoubleShift_NoText_StrayShiftPrefix(t *testing.T) {
	c := NewConverter()
	c.ConvKey = 0 // double-shift mode

	// A single aborted shift tap left in the buffer must not mask the trigger
	c.Push(testKeyLeftShift, K_DOWN)
	c.Push(testKeyLeftShift, K_UP)
	if got := c.Process(); got != ActionNone {
		t.Fatalf("expected ActionNone after single shift tap, got %v", got)
	}

	c.Push(testKeyLeftShift, K_DOWN)
	c.Push(testKeyLeftShift, K_UP)
	c.Push(testKeyLeftShift, K_DOWN)
	c.Push(testKeyLeftShift, K_UP)

	action := c.Process()
	if action != ActionDoubleShiftNoText {
		t.Errorf("expected ActionDoubleShiftNoText with stray shift prefix, got %v", action)
	}
}

func TestConverter_Process_CustomKey_ConvertWord(t *testing.T) {
	c := NewConverter()
	c.ConvKey = testKeyPause // custom convert key

	// Type some text
	c.Push(testKeyA, K_DOWN)
	c.Push(testKeyB, K_DOWN)

	// Press convert key without shift
	c.Push(testKeyPause, K_DOWN)
	c.Push(testKeyPause, K_UP)

	action := c.Process()
	if action != ActionConvertWord {
		t.Errorf("expected ActionConvertWord, got %v", action)
	}
}

func TestConverter_Process_CustomKey_ConvertAll(t *testing.T) {
	c := NewConverter()
	c.ConvKey = testKeyPause // custom convert key

	// Type some text
	c.Push(testKeyA, K_DOWN)
	c.Push(testKeyB, K_DOWN)

	// Press convert key with shift
	c.Push(testKeyLeftShift, K_DOWN)
	c.Push(testKeyPause, K_DOWN)
	c.Push(testKeyPause, K_UP)
	c.Push(testKeyLeftShift, K_UP)

	action := c.Process()
	if action != ActionConvertAll {
		t.Errorf("expected ActionConvertAll, got %v", action)
	}
}

func TestConverter_Process_CustomKey_ShiftHeldFromTyping_DoesNotDropShiftUp(t *testing.T) {
	c := NewConverter()
	c.ConvKey = testKeyPause
	c.LSKeys = []uint16{125}

	// Simulate typing with Shift held, then pressing ConvKey while Shift is still held.
	// Conversion may trigger before we observe shift-up; we must still ensure replay releases shift
	// to avoid "stuck shift" and garbage output.
	c.Push(testKeyLeftShift, K_DOWN)
	c.Push(testKeyA, K_DOWN)
	c.Push(testKeyPause, K_DOWN)
	c.Push(testKeyPause, K_UP)

	action := c.Process()
	if action != ActionConvertWord {
		t.Fatalf("expected ActionConvertWord, got %v", action)
	}

	// ConvKey events should be trimmed out
	for _, ev := range c.GetBuffer() {
		if ev.Code == testKeyPause {
			t.Fatalf("expected convkey events to be trimmed from buffer, found: %+v", ev)
		}
	}

	// Shift up must remain to avoid shift being stuck during replay
	events := c.Convert(action)
	for _, ev := range events {
		if ev.Code == testKeyPause {
			t.Fatalf("expected convkey not to be replayed, found: %+v", ev)
		}
	}

	// Even though we never pushed shift-up, Convert() should ensure shift is released in output.
	hasShiftUpInOutput := false
	for _, ev := range events {
		if ev.Code == testKeyLeftShift && ev.Value == K_UP {
			hasShiftUpInOutput = true
			break
		}
	}
	if !hasShiftUpInOutput {
		t.Fatal("expected Convert() to release shift in output events")
	}
}

func TestConverter_Convert_GeneratesBackspaces(t *testing.T) {
	c := NewConverter()
	c.LSKeys = []uint16{125} // layout switch key

	// Type "abc"
	c.Push(testKeyA, K_DOWN)
	c.Push(testKeyB, K_DOWN)
	c.Push(testKeyC, K_DOWN)

	events := c.Convert(ActionConvertWord)

	// Should have: layout switch + backspaces + replayed keys
	// Layout switch: 1 down + 1 up = 2
	// Backspaces for 3 chars: 3 * 2 = 6
	// Replay 3 keys: 3 * 2 = 6
	// Total: 14

	if len(events) < 14 {
		t.Errorf("expected at least 14 events, got %d", len(events))
	}

	// First events should be layout switch
	if events[0].Code != 125 || events[0].Value != K_DOWN {
		t.Error("first event should be layout switch key down")
	}
}

func TestConverter_Convert_WordVsAll(t *testing.T) {
	c := NewConverter()
	c.LSKeys = []uint16{125}

	// Type "hello world" (with space)
	for _, key := range []uint16{35, 18, 38, 38, 24} { // h e l
		c.Push(key, K_DOWN)
	}
	c.Push(testKeySpace, K_DOWN)
	for _, key := range []uint16{17, 24, 19, 38, 32} { // w o r l d
		c.Push(key, K_DOWN)
	}

	// ConvertWord should only convert "world"
	wordEvents := c.Convert(ActionConvertWord)

	c.ClearBuffer()
	// Refill buffer
	for _, key := range []uint16{35, 18, 38, 38, 24} {
		c.Push(key, K_DOWN)
	}
	c.Push(testKeySpace, K_DOWN)
	for _, key := range []uint16{17, 24, 19, 38, 32} {
		c.Push(key, K_DOWN)
	}

	// ConvertAll should convert everything
	allEvents := c.Convert(ActionConvertAll)

	// ConvertAll should have more events than ConvertWord
	if len(allEvents) <= len(wordEvents) {
		t.Errorf("ConvertAll should produce more events than ConvertWord: %d vs %d",
			len(allEvents), len(wordEvents))
	}
}

func TestConverterDebugOutputOmitsBufferContent(t *testing.T) {
	c := NewConverter()
	var messages []string
	c.SetDebugLogger(func(format string, args ...any) {
		messages = append(messages, fmt.Sprintf(format, args...))
	})

	c.Push(testKeyA, K_DOWN)
	c.Push(testKeyLeftShift, K_DOWN)
	c.Push(testKeyLeftShift, K_UP)
	c.Push(testKeyLeftShift, K_DOWN)
	c.Push(testKeyLeftShift, K_UP)
	if got := c.Process(); got != ActionConvertWord {
		t.Fatalf("Process() = %v, want ActionConvertWord", got)
	}
	c.Convert(ActionConvertWord)

	output := strings.Join(messages, "\n")
	for _, sensitive := range []string{"buffer before trim:", "buffer after trim:", "replay sequence (", "A_DOWN"} {
		if strings.Contains(output, sensitive) {
			t.Fatalf("converter debug output exposes buffer content %q: %q", sensitive, output)
		}
	}
}

func TestConverter_BackspaceRemovesShiftPairs(t *testing.T) {
	c := NewConverter()

	// Type shifted character: shift down, a, shift up
	c.Push(testKeyLeftShift, K_DOWN)
	c.Push(testKeyA, K_DOWN)
	c.Push(testKeyLeftShift, K_UP)

	initialLen := c.BufferLen()

	// Backspace should remove the key and trailing shift pair
	c.Push(testKeyBackspace, K_DOWN)

	// Should have removed the key and possibly shift pair
	if c.BufferLen() >= initialLen {
		t.Error("backspace should reduce buffer length")
	}
}

func TestConverter_DifferentShiftKeys(t *testing.T) {
	c := NewConverter()
	c.ConvKey = 0

	// Type text
	c.Push(testKeyA, K_DOWN)

	// Double-tap with mixed shifts should NOT trigger (needs same shift)
	c.Push(testKeyLeftShift, K_DOWN)
	c.Push(testKeyLeftShift, K_UP)
	c.Push(testKeyRightShift, K_DOWN)
	c.Push(testKeyRightShift, K_UP)

	action := c.Process()
	if action == ActionConvertWord {
		t.Error("mixed shift double-tap should not trigger convert")
	}
}

func TestConverter_RepeatedTriggerUndoesLastConversion(t *testing.T) {
	tests := []struct {
		name    string
		convKey uint16
		want    Action
		trigger []KeyEvent
	}{
		{
			name: "double shift word",
			want: ActionConvertWord,
			trigger: []KeyEvent{
				{Code: testKeyLeftShift, Value: K_DOWN},
				{Code: testKeyLeftShift, Value: K_UP},
				{Code: testKeyLeftShift, Value: K_DOWN},
				{Code: testKeyLeftShift, Value: K_UP},
			},
		},
		{
			name: "double shift phrase",
			want: ActionConvertAll,
			trigger: []KeyEvent{
				{Code: testKeyLeftShift, Value: K_DOWN},
				{Code: testKeyRightShift, Value: K_DOWN},
				{Code: testKeyRightShift, Value: K_UP},
				{Code: testKeyRightShift, Value: K_DOWN},
				{Code: testKeyRightShift, Value: K_UP},
				{Code: testKeyLeftShift, Value: K_UP},
			},
		},
		{
			name:    "custom key word",
			convKey: testKeyPause,
			want:    ActionConvertWord,
			trigger: []KeyEvent{
				{Code: testKeyPause, Value: K_DOWN},
				{Code: testKeyPause, Value: K_UP},
			},
		},
		{
			name:    "custom key phrase",
			convKey: testKeyPause,
			want:    ActionConvertAll,
			trigger: []KeyEvent{
				{Code: testKeyLeftShift, Value: K_DOWN},
				{Code: testKeyPause, Value: K_DOWN},
				{Code: testKeyPause, Value: K_UP},
				{Code: testKeyLeftShift, Value: K_UP},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := NewConverter()
			c.ConvKey = tt.convKey
			c.LSKeys = []uint16{125}
			c.Push(testKeyA, K_DOWN)
			c.Push(testKeyB, K_DOWN)

			for _, event := range tt.trigger {
				c.Push(event.Code, event.Value)
			}
			firstAction := c.Process()
			if firstAction != tt.want {
				t.Fatalf("first Process() = %v, want %v", firstAction, tt.want)
			}
			firstEvents := c.Convert(firstAction)
			if wasUndo := c.CompleteConversion(firstAction); wasUndo {
				t.Fatal("first conversion must not be classified as undo")
			}

			for _, event := range tt.trigger {
				c.Push(event.Code, event.Value)
			}
			secondAction := c.Process()
			if secondAction != tt.want {
				t.Fatalf("second Process() = %v, want %v", secondAction, tt.want)
			}
			if !c.CanUndo(secondAction) {
				t.Fatal("same trigger immediately after conversion must be eligible for undo")
			}
			secondEvents := c.Convert(secondAction)
			if !reflect.DeepEqual(secondEvents, firstEvents) {
				t.Fatalf("undo events differ from conversion events:\nfirst:  %#v\nsecond: %#v", firstEvents, secondEvents)
			}
			if wasUndo := c.CompleteConversion(secondAction); !wasUndo {
				t.Fatal("second conversion must be classified as undo")
			}
			if c.CanUndo(secondAction) {
				t.Fatal("completed undo must consume one-step undo state")
			}
		})
	}
}

func TestConverter_ContentInputInvalidatesUndo(t *testing.T) {
	c := NewConverter()
	c.LSKeys = []uint16{125}
	c.Push(testKeyA, K_DOWN)
	c.CompleteConversion(ActionConvertWord)

	if !c.CanUndo(ActionConvertWord) {
		t.Fatal("completed conversion must arm undo")
	}
	c.Push(testKeyB, K_DOWN)
	if c.CanUndo(ActionConvertWord) {
		t.Fatal("content input must invalidate undo")
	}
}

func TestConverter_Process_DoubleShiftHeld_NoText(t *testing.T) {
	c := NewConverter()

	for _, event := range []KeyEvent{
		{Code: testKeyLeftShift, Value: K_DOWN},
		{Code: testKeyRightShift, Value: K_DOWN},
		{Code: testKeyRightShift, Value: K_UP},
		{Code: testKeyRightShift, Value: K_DOWN},
		{Code: testKeyRightShift, Value: K_UP},
		{Code: testKeyLeftShift, Value: K_UP},
	} {
		c.Push(event.Code, event.Value)
	}

	if got := c.Process(); got != ActionDoubleShiftHeldNoText {
		t.Fatalf("Process() = %v, want ActionDoubleShiftHeldNoText", got)
	}
}
