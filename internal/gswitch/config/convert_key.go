package config

import "fmt"

// IsModifierKey reports whether code is an evdev modifier scancode.
func IsModifierKey(code uint16) bool {
	switch code {
	case 29, 42, 54, 56, 97, 100, 125, 126:
		return true
	default:
		return false
	}
}

// ValidateConvertKey rejects modifiers because Ctrl and Shift are reserved for
// selection, case-swap, and whole-line conversion gestures.
func ValidateConvertKey(code uint16) error {
	if IsModifierKey(code) {
		return fmt.Errorf("convert key %d is a modifier; choose a non-modifier key or 0 for Double Shift", code)
	}
	return nil
}

// EffectiveConvertKey preserves legacy configurations by falling back to
// Double Shift for modifiers. It does not authorize saving the original key.
func EffectiveConvertKey(code uint16) uint16 {
	if IsModifierKey(code) {
		return 0
	}
	return code
}
