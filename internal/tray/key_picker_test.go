package tray

import (
	"testing"
)

func TestIsModifier(t *testing.T) {
	tests := []struct {
		name     string
		code     uint16
		expected bool
	}{
		{"LShift", 42, true},
		{"RShift", 54, true},
		{"LCtrl", 29, true},
		{"RCtrl", 97, true},
		{"LAlt", 56, true},
		{"RAlt", 100, true},
		{"LSuper", 125, true},
		{"RSuper", 126, true},
		{"Space", 57, false},
		{"Caps_Lock", 58, false},
		{"A", 30, false},
		{"Enter", 28, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isModifier(tt.code)
			if result != tt.expected {
				t.Errorf("isModifier(%d) = %v, want %v", tt.code, result, tt.expected)
			}
		})
	}
}

func TestSortScancodes(t *testing.T) {
	tests := []struct {
		name     string
		input    []uint16
		expected []uint16
	}{
		{
			name:     "Super+Space becomes Super first",
			input:    []uint16{57, 125}, // Space, LSuper
			expected: []uint16{125, 57}, // LSuper, Space
		},
		{
			name:     "Alt+Shift stays sorted by scancode",
			input:    []uint16{42, 56}, // LShift, LAlt
			expected: []uint16{42, 56}, // Both are modifiers, sorted numerically
		},
		{
			name:     "Alt+Shift reversed input",
			input:    []uint16{56, 42}, // LAlt, LShift
			expected: []uint16{42, 56}, // Both are modifiers, sorted numerically
		},
		{
			name:     "Ctrl+Alt+Delete",
			input:    []uint16{111, 56, 29}, // Delete, LAlt, LCtrl
			expected: []uint16{29, 56, 111}, // LCtrl, LAlt, Delete
		},
		{
			name:     "Triple: Shift+Super+Space",
			input:    []uint16{57, 42, 125}, // Space, LShift, LSuper
			expected: []uint16{42, 125, 57}, // LShift, LSuper, Space
		},
		{
			name:     "Single key passthrough",
			input:    []uint16{58}, // Caps_Lock
			expected: []uint16{58}, // Unchanged
		},
		{
			name:     "Empty input",
			input:    []uint16{},
			expected: []uint16{},
		},
		{
			name:     "Single modifier passthrough",
			input:    []uint16{125}, // LSuper
			expected: []uint16{125}, // Unchanged
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sortScancodes(tt.input)
			if len(result) != len(tt.expected) {
				t.Errorf("sortScancodes(%v) = %v, want %v", tt.input, result, tt.expected)
				return
			}
			for i := range result {
				if result[i] != tt.expected[i] {
					t.Errorf("sortScancodes(%v) = %v, want %v", tt.input, result, tt.expected)
					return
				}
			}
		})
	}
}

func TestReorderKeyNames(t *testing.T) {
	tests := []struct {
		name        string
		sortedCodes []uint16
		codeToName  map[uint16]string
		expected    []string
	}{
		{
			name:        "LSuper+Space",
			sortedCodes: []uint16{125, 57},
			codeToName:  map[uint16]string{57: "Space", 125: "LSuper"},
			expected:    []string{"LSuper", "Space"},
		},
		{
			name:        "LCtrl+LShift",
			sortedCodes: []uint16{29, 42},
			codeToName:  map[uint16]string{29: "LCtrl", 42: "LShift"},
			expected:    []string{"LCtrl", "LShift"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := reorderKeyNames(tt.sortedCodes, tt.codeToName)
			if len(result) != len(tt.expected) {
				t.Errorf("reorderKeyNames(%v, %v) = %v, want %v",
					tt.sortedCodes, tt.codeToName, result, tt.expected)
				return
			}
			for i := range result {
				if result[i] != tt.expected[i] {
					t.Errorf("reorderKeyNames(%v, %v) = %v, want %v",
						tt.sortedCodes, tt.codeToName, result, tt.expected)
					return
				}
			}
		})
	}
}

func TestFormatKeyValue(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"0", "Double Shift"},
		{"58", "Caps Lock"},
		{"29+42", "LCtrl+LShift"},
		{"56+42", "LAlt+LShift"},
		{"125+57", "LSuper+Space"},
		{"119", "Pause/Break"},
		{"70", "Scroll Lock"},
		{"123", "123"}, // Unknown value returned as-is
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := formatKeyValue(tt.input)
			if result != tt.expected {
				t.Errorf("formatKeyValue(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestFormatCustomKeyLabel(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  string
	}{
		{name: "left control", value: "29", want: "LCtrl (29)"},
		{name: "right control", value: "97", want: "RCtrl (97)"},
		{name: "custom combination", value: "29+30", want: "LCtrl+A (29+30)"},
		{name: "unknown code", value: "999", want: "Key999 (999)"},
		{name: "invalid value", value: "not-a-key", want: "Custom (not-a-key)"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := formatCustomKeyLabel(tt.value); got != tt.want {
				t.Fatalf("formatCustomKeyLabel(%q) = %q, want %q", tt.value, got, tt.want)
			}
		})
	}
}

func TestGetKeyNameFromCode(t *testing.T) {
	tests := []struct {
		code     uint16
		expected string
	}{
		{29, "LCtrl"},
		{42, "LShift"},
		{54, "RShift"},
		{56, "LAlt"},
		{57, "Space"},
		{58, "Caps_Lock"},
		{97, "RCtrl"},
		{100, "RAlt"},
		{119, "Pause"},
		{125, "LSuper"},
		{126, "RSuper"},
		{999, "Key_999"}, // Unknown key
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := GetKeyNameFromCode(tt.code)
			if result != tt.expected {
				t.Errorf("GetKeyNameFromCode(%d) = %q, want %q", tt.code, result, tt.expected)
			}
		})
	}
}

func TestKeySelectionValidForContext(t *testing.T) {
	tests := []struct {
		name    string
		context KeyPickerContext
		codes   []uint16
		want    bool
	}{
		{name: "layout single key", context: KeyPickerForLayoutSwitch, codes: []uint16{57}, want: true},
		{name: "layout combination", context: KeyPickerForLayoutSwitch, codes: []uint16{29, 42}, want: true},
		{name: "convert single key", context: KeyPickerForConvertKey, codes: []uint16{119}, want: true},
		{name: "convert combination", context: KeyPickerForConvertKey, codes: []uint16{29, 42}, want: false},
		{name: "convert modifier", context: KeyPickerForConvertKey, codes: []uint16{29}, want: false},
		{name: "empty selection", context: KeyPickerForConvertKey, codes: nil, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := keySelectionValid(tt.context, tt.codes); got != tt.want {
				t.Fatalf("keySelectionValid(%v, %v) = %v, want %v", tt.context, tt.codes, got, tt.want)
			}
		})
	}
}

func TestGDKHardwareKeycodeToEvdev(t *testing.T) {
	tests := []struct {
		name     string
		hardware uint16
		want     uint16
		wantOK   bool
	}{
		{name: "escape", hardware: 9, want: 1, wantOK: true},
		{name: "left ctrl", hardware: 37, want: 29, wantOK: true},
		{name: "right alt", hardware: 108, want: 100, wantOK: true},
		{name: "down arrow", hardware: 116, want: 108, wantOK: true},
		{name: "below XKB minimum", hardware: 8, wantOK: false},
		{name: "zero", hardware: 0, wantOK: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := gdkHardwareKeycodeToEvdev(tt.hardware)
			if ok != tt.wantOK || got != tt.want {
				t.Fatalf("gdkHardwareKeycodeToEvdev(%d) = (%d, %v), want (%d, %v)",
					tt.hardware, got, ok, tt.want, tt.wantOK)
			}
		})
	}
}

func TestValidateConfigRejectsConvertKeyCombination(t *testing.T) {
	w := &SettingsWindow{}
	cfg := &TrayConfig{
		LayoutSwitch:      "auto",
		ConvertKey:        "29+42",
		Delay:             10,
		LayoutSwitchDelay: 100,
	}

	if err := w.validateConfig(cfg); err == nil {
		t.Fatal("validateConfig() expected error for conversion key combination, got nil")
	}
}

func TestValidateConfigRejectsModifierConvertKey(t *testing.T) {
	tests := []struct {
		name string
		code string
	}{
		{name: "LShift", code: "42"},
		{name: "RShift", code: "54"},
		{name: "LCtrl", code: "29"},
		{name: "RCtrl", code: "97"},
		{name: "LAlt", code: "56"},
		{name: "RAlt", code: "100"},
		{name: "LSuper", code: "125"},
		{name: "RSuper", code: "126"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := &SettingsWindow{}
			cfg := &TrayConfig{
				LayoutSwitch:      "auto",
				ConvertKey:        tt.code,
				Delay:             10,
				LayoutSwitchDelay: 100,
			}

			err := w.validateConfig(cfg)
			if err == nil {
				t.Fatalf("validateConfig() expected error for modifier convert-key=%s, got nil", tt.code)
			}
		})
	}
}

func TestKeyPickerCaptureControlWithXcape(t *testing.T) {
	for _, tt := range []struct {
		name     string
		hardware uint16
		want     uint16
		label    string
	}{
		{"left", 37, 29, "LCtrl"},
		{"right", 105, 97, "RCtrl"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			c := &keyPickerCapture{}
			c.press(tt.hardware, "USB Keyboard")
			c.release(tt.hardware, "USB Keyboard")
			c.press(93, "Virtual core XTEST keyboard")
			c.release(93, "Virtual core XTEST keyboard")
			if len(c.saved) != 1 || c.saved[tt.want] != tt.label {
				t.Fatalf("captured after xcape = %v, want {%d: %s}", c.saved, tt.want, tt.label)
			}
		})
	}
}

func TestKeyPickerCapturePhysicalGroupKeyAndReselection(t *testing.T) {
	c := &keyPickerCapture{}
	// A physical key mapped to an ISO action must remain selectable.
	c.press(105, "USB Keyboard")
	c.release(105, "USB Keyboard")
	if c.saved[97] != "RCtrl" {
		t.Fatalf("physical group key = %v", c.saved)
	}
	c.press(37, "USB Keyboard")
	// An injected release must not drop the held physical modifier.
	c.release(37, "Virtual core XTEST keyboard")
	c.press(38, "USB Keyboard")
	c.release(38, "USB Keyboard")
	c.release(37, "USB Keyboard")
	if len(c.saved) != 2 || c.saved[29] != "LCtrl" || c.saved[30] != "A" {
		t.Fatalf("replacement chord = %v", c.saved)
	}
	// Missing source metadata is accepted (e.g. a backend without a source).
	c.press(105, "")
	if len(c.saved) != 1 || c.saved[97] != "RCtrl" {
		t.Fatalf("replacement key = %v", c.saved)
	}
}
