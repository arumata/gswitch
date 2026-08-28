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
		{"Shift_L", 42, true},
		{"Shift_R", 54, true},
		{"Ctrl_L", 29, true},
		{"Ctrl_R", 97, true},
		{"Alt_L", 56, true},
		{"Alt_R", 100, true},
		{"Super_L", 125, true},
		{"Super_R", 126, true},
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
			input:    []uint16{57, 125}, // Space, Super_L
			expected: []uint16{125, 57}, // Super_L, Space
		},
		{
			name:     "Alt+Shift stays sorted by scancode",
			input:    []uint16{42, 56}, // Shift_L, Alt_L
			expected: []uint16{42, 56}, // Both are modifiers, sorted numerically
		},
		{
			name:     "Alt+Shift reversed input",
			input:    []uint16{56, 42}, // Alt_L, Shift_L
			expected: []uint16{42, 56}, // Both are modifiers, sorted numerically
		},
		{
			name:     "Ctrl+Alt+Delete",
			input:    []uint16{111, 56, 29}, // Delete, Alt_L, Ctrl_L
			expected: []uint16{29, 56, 111}, // Ctrl_L, Alt_L, Delete
		},
		{
			name:     "Triple: Shift+Super+Space",
			input:    []uint16{57, 42, 125}, // Space, Shift_L, Super_L
			expected: []uint16{42, 125, 57}, // Shift_L, Super_L, Space
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
			input:    []uint16{125}, // Super_L
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
			name:        "Super+Space",
			sortedCodes: []uint16{125, 57},
			codeToName:  map[uint16]string{57: "Space", 125: "Super_L"},
			expected:    []string{"Super_L", "Space"},
		},
		{
			name:        "Ctrl+Shift",
			sortedCodes: []uint16{29, 42},
			codeToName:  map[uint16]string{29: "Ctrl_L", 42: "Shift_L"},
			expected:    []string{"Ctrl_L", "Shift_L"},
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
		{"29+42", "Ctrl+Shift"},
		{"56+42", "Alt+Shift"},
		{"125+57", "Super+Space"},
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

func TestGetKeyNameFromCode(t *testing.T) {
	tests := []struct {
		code     uint16
		expected string
	}{
		{29, "Ctrl_L"},
		{42, "Shift_L"},
		{54, "Shift_R"},
		{56, "Alt_L"},
		{57, "Space"},
		{58, "Caps_Lock"},
		{97, "Ctrl_R"},
		{100, "Alt_R"},
		{119, "Pause"},
		{125, "Super_L"},
		{126, "Super_R"},
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
		name     string
		context  KeyPickerContext
		keyCount int
		want     bool
	}{
		{name: "layout single key", context: KeyPickerForLayoutSwitch, keyCount: 1, want: true},
		{name: "layout combination", context: KeyPickerForLayoutSwitch, keyCount: 2, want: true},
		{name: "convert single key", context: KeyPickerForConvertKey, keyCount: 1, want: true},
		{name: "convert combination", context: KeyPickerForConvertKey, keyCount: 2, want: false},
		{name: "empty selection", context: KeyPickerForConvertKey, keyCount: 0, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := keySelectionValid(tt.context, tt.keyCount); got != tt.want {
				t.Fatalf("keySelectionValid(%v, %d) = %v, want %v", tt.context, tt.keyCount, got, tt.want)
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
