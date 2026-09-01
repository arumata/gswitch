package runtime

import "testing"

func TestVirtualKeyboardKeyCodesIncludeKeyboardLayoutKey(t *testing.T) {
	codes := virtualKeyboardKeyCodes()
	if len(codes) != int(KEY_MAX)+1 {
		t.Fatalf("virtualKeyboardKeyCodes() returned %d codes, want %d", len(codes), int(KEY_MAX)+1)
	}
	if codes[0] != 0 || codes[len(codes)-1] != KEY_MAX {
		t.Fatalf("virtualKeyboardKeyCodes() bounds = %d..%d, want 0..%d", codes[0], codes[len(codes)-1], KEY_MAX)
	}
	if codes[KEY_KEYBOARD] != KEY_KEYBOARD {
		t.Fatalf("virtual keyboard does not advertise KEY_KEYBOARD=%d", KEY_KEYBOARD)
	}
}
