package detect

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
)

func TestParseSetxkbmapOutput_WithOptions(t *testing.T) {
	output := `rules:      evdev
model:      pc105
layout:     us,ru
variant:    ,
options:    grp:ctrl_shift_toggle,caps:escape`

	got := parseSetxkbmapOutput(output)
	want := []string{"grp:ctrl_shift_toggle", "caps:escape"}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseSetxkbmapOutput() = %v, want %v", got, want)
	}
}

func TestParseSetxkbmapOutput_SingleOption(t *testing.T) {
	output := `rules:      evdev
model:      pc105
layout:     us,ru
variant:    ,
options:    grp:alt_shift_toggle`

	got := parseSetxkbmapOutput(output)
	want := []string{"grp:alt_shift_toggle"}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseSetxkbmapOutput() = %v, want %v", got, want)
	}
}

func TestParseSetxkbmapOutput_NoOptions(t *testing.T) {
	output := `rules:      evdev
model:      pc105
layout:     us,ru
variant:    ,`

	got := parseSetxkbmapOutput(output)

	if len(got) != 0 {
		t.Fatalf("parseSetxkbmapOutput() = %v, want empty slice", got)
	}
}

func TestParseSetxkbmapOutput_EmptyOptions(t *testing.T) {
	output := `rules:      evdev
model:      pc105
layout:     us
options:    `

	got := parseSetxkbmapOutput(output)

	if len(got) != 0 {
		t.Fatalf("parseSetxkbmapOutput() = %v, want empty slice", got)
	}
}

func TestParseSetxkbmapOutput_ManyOptions(t *testing.T) {
	output := `rules:      evdev
model:      pc105
layout:     us,ru
options:    grp:ctrl_shift_toggle,caps:escape,compose:ralt,terminate:ctrl_alt_bksp`

	got := parseSetxkbmapOutput(output)
	want := []string{"grp:ctrl_shift_toggle", "caps:escape", "compose:ralt", "terminate:ctrl_alt_bksp"}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseSetxkbmapOutput() = %v, want %v", got, want)
	}
}

func TestParseGsettingsOutput_WithOptions(t *testing.T) {
	output := "['grp:ctrl_shift_toggle', 'caps:escape']"

	got := parseGsettingsOutput(output)
	want := []string{"grp:ctrl_shift_toggle", "caps:escape"}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseGsettingsOutput() = %v, want %v", got, want)
	}
}

func TestParseGsettingsOutput_SingleOption(t *testing.T) {
	output := "['grp:alt_shift_toggle']"

	got := parseGsettingsOutput(output)
	want := []string{"grp:alt_shift_toggle"}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseGsettingsOutput() = %v, want %v", got, want)
	}
}

func TestParseGsettingsOutput_EmptyArray(t *testing.T) {
	tests := []string{"[]", "@as []"}
	for _, output := range tests {
		got := parseGsettingsOutput(output)
		if len(got) != 0 {
			t.Fatalf("parseGsettingsOutput(%q) = %v, want empty slice", output, got)
		}
	}
}

func TestParseXKBRulesNames_WithOptions(t *testing.T) {
	// Simulate _XKB_RULES_NAMES property: rules\0model\0layout\0variant\0options
	data := []byte("evdev\x00pc105\x00us,ru\x00,\x00grp:ctrl_shift_toggle,caps:escape\x00")

	got := parseXKBRulesNames(data)
	want := []string{"grp:ctrl_shift_toggle", "caps:escape"}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseXKBRulesNames() = %v, want %v", got, want)
	}
}

func TestParseXKBRulesNames_NoOptions(t *testing.T) {
	// No options (5th element is empty)
	data := []byte("evdev\x00pc105\x00us,ru\x00,\x00\x00")

	got := parseXKBRulesNames(data)

	if len(got) != 0 {
		t.Fatalf("parseXKBRulesNames() = %v, want empty slice", got)
	}
}

func TestParseXKBRulesNames_TooFewElements(t *testing.T) {
	// Less than 5 elements
	data := []byte("evdev\x00pc105\x00us,ru\x00")

	got := parseXKBRulesNames(data)

	if len(got) != 0 {
		t.Fatalf("parseXKBRulesNames() = %v, want empty slice", got)
	}
}

func TestParseDefaultKeyboardFile_WithOptions(t *testing.T) {
	content := `# KEYBOARD CONFIGURATION FILE
XKBMODEL="pc105"
XKBLAYOUT="us,ru"
XKBVARIANT=","
XKBOPTIONS="grp:ctrl_shift_toggle,caps:escape"
BACKSPACE="guess"
`
	tmpFile := createTempFile(t, content)

	got, err := parseDefaultKeyboardFile(tmpFile)
	if err != nil {
		t.Fatalf("parseDefaultKeyboardFile() error = %v", err)
	}

	want := []string{"grp:ctrl_shift_toggle", "caps:escape"}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseDefaultKeyboardFile() = %v, want %v", got, want)
	}
}

func TestParseDefaultKeyboardFile_SingleQuotes(t *testing.T) {
	content := `XKBMODEL='pc105'
XKBLAYOUT='us,ru'
XKBOPTIONS='grp:alt_shift_toggle'
`
	tmpFile := createTempFile(t, content)

	got, err := parseDefaultKeyboardFile(tmpFile)
	if err != nil {
		t.Fatalf("parseDefaultKeyboardFile() error = %v", err)
	}

	want := []string{"grp:alt_shift_toggle"}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseDefaultKeyboardFile() = %v, want %v", got, want)
	}
}

func TestParseDefaultKeyboardFile_NoOptions(t *testing.T) {
	content := `XKBMODEL="pc105"
XKBLAYOUT="us"
`
	tmpFile := createTempFile(t, content)

	got, err := parseDefaultKeyboardFile(tmpFile)
	if err != nil {
		t.Fatalf("parseDefaultKeyboardFile() error = %v", err)
	}

	if len(got) != 0 {
		t.Fatalf("parseDefaultKeyboardFile() = %v, want empty slice", got)
	}
}

func TestParseDefaultKeyboardFile_EmptyOptions(t *testing.T) {
	content := `XKBMODEL="pc105"
XKBLAYOUT="us"
XKBOPTIONS=""
`
	tmpFile := createTempFile(t, content)

	got, err := parseDefaultKeyboardFile(tmpFile)
	if err != nil {
		t.Fatalf("parseDefaultKeyboardFile() error = %v", err)
	}

	if len(got) != 0 {
		t.Fatalf("parseDefaultKeyboardFile() = %v, want empty slice", got)
	}
}

func TestSplitOptions_Basic(t *testing.T) {
	tests := []struct {
		input string
		want  []string
	}{
		{"grp:ctrl_shift_toggle", []string{"grp:ctrl_shift_toggle"}},
		{"grp:ctrl_shift_toggle,caps:escape", []string{"grp:ctrl_shift_toggle", "caps:escape"}},
		{"", nil},
		{"  ", nil},
	}

	for _, tt := range tests {
		got := splitOptions(tt.input)
		if !reflect.DeepEqual(got, tt.want) {
			t.Fatalf("splitOptions(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestSplitOptions_WithSpaces(t *testing.T) {
	got := splitOptions("grp:ctrl_shift_toggle , caps:escape")
	want := []string{"grp:ctrl_shift_toggle", "caps:escape"}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("splitOptions() = %v, want %v", got, want)
	}
}

// createTempFile creates a temporary file with the given content and returns its path.
func createTempFile(t *testing.T, content string) string {
	t.Helper()
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "keyboard")
	if err := os.WriteFile(tmpFile, []byte(content), 0o600); err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	return tmpFile
}

// ========== Task 02: XKB to Scancode Mapping Tests ==========

func TestDetectLayoutSwitchKeysFromOptions_CtrlShiftToggle(t *testing.T) {
	options := []string{"grp:ctrl_shift_toggle", "caps:escape"}

	got, err := detectLayoutSwitchKeysFromOptions(options)
	if err != nil {
		t.Fatalf("detectLayoutSwitchKeysFromOptions() error = %v", err)
	}

	want := []uint16{29, 42} // Left Ctrl + Left Shift
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("detectLayoutSwitchKeysFromOptions() = %v, want %v", got, want)
	}
}

func TestDetectLayoutSwitchKeysFromOptions_NoGrpOption(t *testing.T) {
	options := []string{"caps:escape", "terminate:ctrl_alt_bksp"}

	_, err := detectLayoutSwitchKeysFromOptions(options)
	if !errors.Is(err, ErrNoLayoutSwitchOption) {
		t.Fatalf("detectLayoutSwitchKeysFromOptions() error = %v, want %v", err, ErrNoLayoutSwitchOption)
	}
}

func TestDetectLayoutSwitchKeysFromOptions_EmptyOptions(t *testing.T) {
	options := []string{}

	_, err := detectLayoutSwitchKeysFromOptions(options)
	if !errors.Is(err, ErrNoLayoutSwitchOption) {
		t.Fatalf("detectLayoutSwitchKeysFromOptions() error = %v, want %v", err, ErrNoLayoutSwitchOption)
	}
}

func TestDetectLayoutSwitchKeysFromOptions_FirstGrpOptionUsed(t *testing.T) {
	// When multiple grp:* options exist, the first one should be used
	options := []string{"grp:alt_shift_toggle", "grp:ctrl_shift_toggle", "caps:escape"}

	got, err := detectLayoutSwitchKeysFromOptions(options)
	if err != nil {
		t.Fatalf("detectLayoutSwitchKeysFromOptions() error = %v", err)
	}

	want := []uint16{56, 42} // Left Alt + Left Shift (first grp option)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("detectLayoutSwitchKeysFromOptions() = %v, want %v", got, want)
	}
}

func TestDetectLayoutSwitchKeysFromOptions_UnsupportedOption(t *testing.T) {
	options := []string{"grp:unsupported_option_xyz"}

	_, err := detectLayoutSwitchKeysFromOptions(options)
	if err == nil {
		t.Fatal("detectLayoutSwitchKeysFromOptions() expected error for unsupported option")
	}
	if errors.Is(err, ErrNoLayoutSwitchOption) {
		t.Fatal("detectLayoutSwitchKeysFromOptions() should not return ErrNoLayoutSwitchOption for unsupported option")
	}
}

func TestDetectLayoutSwitchKeysFromOptions_GrpOptionNotFirst(t *testing.T) {
	// grp:* option is not the first in the list
	options := []string{"caps:escape", "compose:ralt", "grp:shifts_toggle"}

	got, err := detectLayoutSwitchKeysFromOptions(options)
	if err != nil {
		t.Fatalf("detectLayoutSwitchKeysFromOptions() error = %v", err)
	}

	want := []uint16{42, 54} // Left Shift + Right Shift
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("detectLayoutSwitchKeysFromOptions() = %v, want %v", got, want)
	}
}

// TestXkbToScancodes_AllMappings verifies all supported XKB options have correct scancodes
func TestXkbToScancodes_AllMappings(t *testing.T) {
	tests := []struct {
		option string
		want   []uint16
	}{
		{"grp:shift_caps_toggle", []uint16{42, 58}}, // Left Shift + CapsLock
		{"grp:caps_toggle", []uint16{58}},           // CapsLock
		{"grp:ctrl_shift_toggle", []uint16{29, 42}}, // Left Ctrl + Left Shift
		{"grp:alt_shift_toggle", []uint16{56, 42}},  // Left Alt + Left Shift
		{"grp:shifts_toggle", []uint16{42, 54}},     // Left Shift + Right Shift
		{"grp:win_space_toggle", []uint16{125, 57}}, // Left Meta + Space
		{"grp:alt_space_toggle", []uint16{56, 57}},  // Left Alt + Space
		{"grp:lwin_toggle", []uint16{125}},          // Left Meta
		{"grp:rwin_toggle", []uint16{126}},          // Right Meta
		{"grp:sclk_toggle", []uint16{70}},           // Scroll Lock
		{"grp:menu_toggle", []uint16{127}},          // Menu/Compose
		{"grp:lctrl_toggle", []uint16{29}},          // Left Ctrl
		{"grp:rctrl_toggle", []uint16{97}},          // Right Ctrl
		{"grp:lshift_toggle", []uint16{42}},         // Left Shift
		{"grp:rshift_toggle", []uint16{54}},         // Right Shift
	}

	for _, tt := range tests {
		t.Run(tt.option, func(t *testing.T) {
			got, err := detectLayoutSwitchKeysFromOptions([]string{tt.option})
			if err != nil {
				t.Fatalf("detectLayoutSwitchKeysFromOptions(%q) error = %v", tt.option, err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("detectLayoutSwitchKeysFromOptions(%q) = %v, want %v", tt.option, got, tt.want)
			}
		})
	}
}

// TestXkbToScancodes_MapSize verifies the map contains expected number of entries
func TestXkbToScancodes_MapSize(t *testing.T) {
	// Should have at least 14 entries as specified in the task
	if len(xkbToScancodes) < 14 {
		t.Fatalf("xkbToScancodes has %d entries, want at least 14", len(xkbToScancodes))
	}
}

// ========== Task 04a: KDE Plasma Support Tests ==========

func TestParseKDEGlobalShortcutsFile_WithShortcut(t *testing.T) {
	content := `[ActivityManager]
_k_friendly_name=Activity Manager
switch-to-activity-c031cdeb-1fab-48d8-b22e-d36182f79638=none,none,Switch to activity "Default"

[KDE Keyboard Layout Switcher]
Switch to Next Keyboard Layout=Meta+Alt+K,none,Switch to Next Keyboard Layout
_k_friendly_name=Keyboard Layout Switcher

[kaccess]
Toggle Screen Reader On and Off=Meta+Alt+S,Meta+Alt+S,Toggle Screen Reader On and Off
`
	tmpFile := createTempFile(t, content)

	got, err := parseKDEGlobalShortcutsFile(tmpFile)
	if err != nil {
		t.Fatalf("parseKDEGlobalShortcutsFile() error = %v", err)
	}

	want := "Meta+Alt+K"
	if got != want {
		t.Fatalf("parseKDEGlobalShortcutsFile() = %q, want %q", got, want)
	}
}

func TestParseKDEGlobalShortcutsFile_AltShift(t *testing.T) {
	content := `[KDE Keyboard Layout Switcher]
Switch to Next Keyboard Layout=Alt+Shift,Alt+Shift,Switch to Next Keyboard Layout
`
	tmpFile := createTempFile(t, content)

	got, err := parseKDEGlobalShortcutsFile(tmpFile)
	if err != nil {
		t.Fatalf("parseKDEGlobalShortcutsFile() error = %v", err)
	}

	want := "Alt+Shift"
	if got != want {
		t.Fatalf("parseKDEGlobalShortcutsFile() = %q, want %q", got, want)
	}
}

func TestParseKDEGlobalShortcutsFile_NoneShortcut(t *testing.T) {
	content := `[KDE Keyboard Layout Switcher]
Switch to Next Keyboard Layout=none,none,Switch to Next Keyboard Layout
`
	tmpFile := createTempFile(t, content)

	got, err := parseKDEGlobalShortcutsFile(tmpFile)
	if err != nil {
		t.Fatalf("parseKDEGlobalShortcutsFile() error = %v", err)
	}

	want := "none"
	if got != want {
		t.Fatalf("parseKDEGlobalShortcutsFile() = %q, want %q", got, want)
	}
}

func TestParseKDEGlobalShortcutsFile_NoSection(t *testing.T) {
	content := `[SomeOtherSection]
Switch to Next Keyboard Layout=Meta+Alt+K,none,Switch to Next Keyboard Layout
`
	tmpFile := createTempFile(t, content)

	got, err := parseKDEGlobalShortcutsFile(tmpFile)
	if err != nil {
		t.Fatalf("parseKDEGlobalShortcutsFile() error = %v", err)
	}

	// Should return empty string when section not found
	if got != "" {
		t.Fatalf("parseKDEGlobalShortcutsFile() = %q, want empty string", got)
	}
}

func TestParseKDEGlobalShortcutsFile_EmptyFile(t *testing.T) {
	content := ``
	tmpFile := createTempFile(t, content)

	got, err := parseKDEGlobalShortcutsFile(tmpFile)
	if err != nil {
		t.Fatalf("parseKDEGlobalShortcutsFile() error = %v", err)
	}

	if got != "" {
		t.Fatalf("parseKDEGlobalShortcutsFile() = %q, want empty string", got)
	}
}

func TestParseKDEGlobalShortcutsFile_FileNotFound(t *testing.T) {
	_, err := parseKDEGlobalShortcutsFile("/nonexistent/path/kglobalshortcutsrc")
	if err == nil {
		t.Fatal("parseKDEGlobalShortcutsFile() expected error for nonexistent file")
	}
}

func TestParseKDEShortcutToScancodes_MetaAltK(t *testing.T) {
	got, err := parseKDEShortcutToScancodes("Meta+Alt+K")
	if err != nil {
		t.Fatalf("parseKDEShortcutToScancodes() error = %v", err)
	}

	want := []uint16{125, 56, 37} // Meta(125) + Alt(56) + K(37)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseKDEShortcutToScancodes() = %v, want %v", got, want)
	}
}

func TestParseKDEShortcutToScancodes_AltShift(t *testing.T) {
	got, err := parseKDEShortcutToScancodes("Alt+Shift")
	if err != nil {
		t.Fatalf("parseKDEShortcutToScancodes() error = %v", err)
	}

	want := []uint16{56, 42} // Alt(56) + Shift(42)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseKDEShortcutToScancodes() = %v, want %v", got, want)
	}
}

func TestParseKDEShortcutToScancodes_CtrlShift(t *testing.T) {
	got, err := parseKDEShortcutToScancodes("Ctrl+Shift")
	if err != nil {
		t.Fatalf("parseKDEShortcutToScancodes() error = %v", err)
	}

	want := []uint16{29, 42} // Ctrl(29) + Shift(42)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseKDEShortcutToScancodes() = %v, want %v", got, want)
	}
}

func TestParseKDEShortcutToScancodes_SuperSpace(t *testing.T) {
	got, err := parseKDEShortcutToScancodes("Super+Space")
	if err != nil {
		t.Fatalf("parseKDEShortcutToScancodes() error = %v", err)
	}

	want := []uint16{125, 57} // Super(125) + Space(57)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseKDEShortcutToScancodes() = %v, want %v", got, want)
	}
}

func TestParseKDEShortcutToScancodes_SingleModifier(t *testing.T) {
	got, err := parseKDEShortcutToScancodes("Meta")
	if err != nil {
		t.Fatalf("parseKDEShortcutToScancodes() error = %v", err)
	}

	want := []uint16{125} // Meta(125)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseKDEShortcutToScancodes() = %v, want %v", got, want)
	}
}

func TestParseKDEShortcutToScancodes_EmptyShortcut(t *testing.T) {
	_, err := parseKDEShortcutToScancodes("")
	if err == nil {
		t.Fatal("parseKDEShortcutToScancodes() expected error for empty shortcut")
	}
}

func TestParseKDEShortcutToScancodes_NoneShortcut(t *testing.T) {
	_, err := parseKDEShortcutToScancodes("none")
	if err == nil {
		t.Fatal("parseKDEShortcutToScancodes() expected error for 'none' shortcut")
	}
}

func TestParseKDEShortcutToScancodes_UnknownKey(t *testing.T) {
	_, err := parseKDEShortcutToScancodes("Meta+UnknownKey123")
	if err == nil {
		t.Fatal("parseKDEShortcutToScancodes() expected error for unknown key")
	}
}

func TestParseKDEShortcutToScancodes_AllModifiers(t *testing.T) {
	tests := []struct {
		modifier string
		want     uint16
	}{
		{"Meta", 125},
		{"Super", 125},
		{"Alt", 56},
		{"Shift", 42},
		{"Ctrl", 29},
	}

	for _, tt := range tests {
		t.Run(tt.modifier, func(t *testing.T) {
			got, err := parseKDEShortcutToScancodes(tt.modifier)
			if err != nil {
				t.Fatalf("parseKDEShortcutToScancodes(%q) error = %v", tt.modifier, err)
			}
			if len(got) != 1 || got[0] != tt.want {
				t.Fatalf("parseKDEShortcutToScancodes(%q) = %v, want [%d]", tt.modifier, got, tt.want)
			}
		})
	}
}

func TestParseKDEShortcutToScancodes_Letters(t *testing.T) {
	// Test a few representative letters
	tests := []struct {
		letter string
		want   uint16
	}{
		{"A", 30},
		{"K", 37},
		{"Z", 44},
	}

	for _, tt := range tests {
		t.Run(tt.letter, func(t *testing.T) {
			got, err := parseKDEShortcutToScancodes(tt.letter)
			if err != nil {
				t.Fatalf("parseKDEShortcutToScancodes(%q) error = %v", tt.letter, err)
			}
			if len(got) != 1 || got[0] != tt.want {
				t.Fatalf("parseKDEShortcutToScancodes(%q) = %v, want [%d]", tt.letter, got, tt.want)
			}
		})
	}
}

// TestKDEModifierToScancode_MapContents verifies the KDE modifier map
func TestKDEModifierToScancode_MapContents(t *testing.T) {
	// Verify expected entries exist
	expectedMods := []string{"Meta", "Super", "Alt", "Shift", "Ctrl"}
	for _, mod := range expectedMods {
		if _, ok := kdeModifierToScancode[mod]; !ok {
			t.Fatalf("kdeModifierToScancode missing %q", mod)
		}
	}
}

// TestKDEKeyToScancode_MapContents verifies the KDE key map has essential entries
func TestKDEKeyToScancode_MapContents(t *testing.T) {
	// Verify all letters A-Z exist
	for c := 'A'; c <= 'Z'; c++ {
		key := string(c)
		if _, ok := kdeKeyToScancode[key]; !ok {
			t.Fatalf("kdeKeyToScancode missing letter %q", key)
		}
	}

	// Verify all numbers 0-9 exist
	for c := '0'; c <= '9'; c++ {
		key := string(c)
		if _, ok := kdeKeyToScancode[key]; !ok {
			t.Fatalf("kdeKeyToScancode missing number %q", key)
		}
	}

	// Verify Space key exists
	if _, ok := kdeKeyToScancode["Space"]; !ok {
		t.Fatal("kdeKeyToScancode missing 'Space'")
	}
}

// ========== Task 04: Provider Implementation Tests ==========

func TestXKBProvider_Name(t *testing.T) {
	p := NewXKBProvider()
	if p.Name() != SourceXKB {
		t.Fatalf("xkbProvider.Name() = %v, want %v", p.Name(), SourceXKB)
	}
}

func TestKDEProvider_Name(t *testing.T) {
	p := NewKDEProvider()
	if p.Name() != SourceKDE {
		t.Fatalf("kdeProvider.Name() = %v, want %v", p.Name(), SourceKDE)
	}
}

func TestKDEProvider_Detect_WithValidConfig(t *testing.T) {
	content := `[KDE Keyboard Layout Switcher]
Switch to Next Keyboard Layout=Alt+Shift,Alt+Shift,Switch to Next Keyboard Layout
`
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, ".config", "kglobalshortcutsrc")
	if err := os.MkdirAll(filepath.Dir(configPath), 0o750); err != nil {
		t.Fatalf("failed to create config dir: %v", err)
	}
	if err := os.WriteFile(configPath, []byte(content), 0o600); err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}

	// Create a session env pointing to our temp dir
	env := &SessionEnv{
		Home: tmpDir,
	}

	p := NewKDEProvider()
	attempt := p.Detect(env)

	if attempt.Status != StatusFound {
		t.Fatalf("kdeProvider.Detect() status = %v, want %v (error: %s)", attempt.Status, StatusFound, attempt.Error)
	}
	if attempt.RawValue != "Alt+Shift" {
		t.Fatalf("kdeProvider.Detect() rawValue = %q, want %q", attempt.RawValue, "Alt+Shift")
	}
	wantScancodes := []uint16{56, 42}
	if !reflect.DeepEqual(attempt.Scancodes, wantScancodes) {
		t.Fatalf("kdeProvider.Detect() scancodes = %v, want %v", attempt.Scancodes, wantScancodes)
	}
	if attempt.KeyNames != "Alt+Shift" {
		t.Fatalf("kdeProvider.Detect() keyNames = %q, want %q", attempt.KeyNames, "Alt+Shift")
	}
}

func TestKDEProvider_Detect_ConfigNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	env := &SessionEnv{
		Home: tmpDir,
	}

	p := NewKDEProvider()
	attempt := p.Detect(env)

	if attempt.Status != StatusInactive {
		t.Fatalf("kdeProvider.Detect() status = %v, want %v", attempt.Status, StatusInactive)
	}
}

func TestKDEProvider_Detect_NoneShortcut(t *testing.T) {
	content := `[KDE Keyboard Layout Switcher]
Switch to Next Keyboard Layout=none,none,Switch to Next Keyboard Layout
`
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, ".config", "kglobalshortcutsrc")
	if err := os.MkdirAll(filepath.Dir(configPath), 0o750); err != nil {
		t.Fatalf("failed to create config dir: %v", err)
	}
	if err := os.WriteFile(configPath, []byte(content), 0o600); err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}

	env := &SessionEnv{
		Home: tmpDir,
	}

	p := NewKDEProvider()
	attempt := p.Detect(env)

	if attempt.Status != StatusNotFound {
		t.Fatalf("kdeProvider.Detect() status = %v, want %v", attempt.Status, StatusNotFound)
	}
}

func TestKDEProvider_Detect_UnsupportedShortcut(t *testing.T) {
	content := `[KDE Keyboard Layout Switcher]
Switch to Next Keyboard Layout=UnknownKey+X,none,Switch to Next Keyboard Layout
`
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, ".config", "kglobalshortcutsrc")
	if err := os.MkdirAll(filepath.Dir(configPath), 0o750); err != nil {
		t.Fatalf("failed to create config dir: %v", err)
	}
	if err := os.WriteFile(configPath, []byte(content), 0o600); err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}

	env := &SessionEnv{
		Home: tmpDir,
	}

	p := NewKDEProvider()
	attempt := p.Detect(env)

	if attempt.Status != StatusUnsupported {
		t.Fatalf("kdeProvider.Detect() status = %v, want %v", attempt.Status, StatusUnsupported)
	}
	if attempt.RawValue != "UnknownKey+X" {
		t.Fatalf("kdeProvider.Detect() rawValue = %q, want %q", attempt.RawValue, "UnknownKey+X")
	}
}

func TestKDEProvider_Detect_NilEnv(t *testing.T) {
	// When env is nil, provider should use current environment
	// This test verifies it doesn't panic
	p := NewKDEProvider()
	attempt := p.Detect(nil)

	// Status depends on whether config exists in user's home
	// Just verify it returns a valid status without panic
	validStatuses := map[AttemptStatus]bool{
		StatusFound:       true,
		StatusNotFound:    true,
		StatusInactive:    true,
		StatusError:       true,
		StatusUnsupported: true,
	}
	if !validStatuses[attempt.Status] {
		t.Fatalf("kdeProvider.Detect(nil) returned invalid status: %v", attempt.Status)
	}
}

func TestKDEProvider_Detect_WithXDGConfigHome(t *testing.T) {
	content := `[KDE Keyboard Layout Switcher]
Switch to Next Keyboard Layout=Ctrl+Shift,none,Switch to Next Keyboard Layout
`
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "kglobalshortcutsrc")
	if err := os.WriteFile(configPath, []byte(content), 0o600); err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}

	env := &SessionEnv{
		XDGConfigHome: tmpDir,
		Home:          "/nonexistent", // Should be ignored when XDGConfigHome is set
	}

	p := NewKDEProvider()
	attempt := p.Detect(env)

	if attempt.Status != StatusFound {
		t.Fatalf("kdeProvider.Detect() status = %v, want %v (error: %s)", attempt.Status, StatusFound, attempt.Error)
	}
	if attempt.RawValue != "Ctrl+Shift" {
		t.Fatalf("kdeProvider.Detect() rawValue = %q, want %q", attempt.RawValue, "Ctrl+Shift")
	}
}

func TestGetKDEConfigPathWithEnv_WithXDGConfigHome(t *testing.T) {
	env := &SessionEnv{
		XDGConfigHome: "/custom/config",
		Home:          "/home/user",
	}

	got := getKDEConfigPathWithEnv(env)
	want := "/custom/config/kglobalshortcutsrc"
	if got != want {
		t.Fatalf("getKDEConfigPathWithEnv() = %q, want %q", got, want)
	}
}

func TestGetKDEConfigPathWithEnv_WithHomeOnly(t *testing.T) {
	env := &SessionEnv{
		Home: "/home/testuser",
	}

	got := getKDEConfigPathWithEnv(env)
	want := "/home/testuser/.config/kglobalshortcutsrc"
	if got != want {
		t.Fatalf("getKDEConfigPathWithEnv() = %q, want %q", got, want)
	}
}

func TestGetKDEConfigPathWithEnv_RelativeXDGConfigHome(t *testing.T) {
	env := &SessionEnv{
		XDGConfigHome: "relative/path", // Should be ignored (not absolute)
		Home:          "/home/testuser",
	}

	got := getKDEConfigPathWithEnv(env)
	want := "/home/testuser/.config/kglobalshortcutsrc"
	if got != want {
		t.Fatalf("getKDEConfigPathWithEnv() = %q, want %q", got, want)
	}
}

func TestGetKDEConfigPathWithEnv_EmptyEnv(t *testing.T) {
	env := &SessionEnv{}

	got := getKDEConfigPathWithEnv(env)
	if got != "" {
		t.Fatalf("getKDEConfigPathWithEnv() = %q, want empty string", got)
	}
}

func TestXKBProvider_Detect_NilEnv(t *testing.T) {
	// When env is nil, provider should use current environment
	// This test verifies it doesn't panic
	p := NewXKBProvider()
	attempt := p.Detect(nil)

	// Status depends on system configuration
	// Just verify it returns a valid status without panic
	validStatuses := map[AttemptStatus]bool{
		StatusFound:       true,
		StatusNotFound:    true,
		StatusInactive:    true,
		StatusError:       true,
		StatusUnsupported: true,
	}
	if !validStatuses[attempt.Status] {
		t.Fatalf("xkbProvider.Detect(nil) returned invalid status: %v", attempt.Status)
	}
}

// TestDetectKDELayoutSwitchKeys_BackwardsCompatibility ensures the original function still works
func TestDetectKDELayoutSwitchKeys_BackwardsCompatibility(t *testing.T) {
	// This test verifies DetectKDELayoutSwitchKeys() still works as before
	// The result depends on the actual system configuration
	// We just verify it doesn't panic and returns expected types
	scancodes, shortcut, err := DetectKDELayoutSwitchKeys()

	if err == nil {
		// If no error, scancodes should be non-empty
		if len(scancodes) == 0 {
			t.Fatal("DetectKDELayoutSwitchKeys() returned nil scancodes without error")
		}
		if shortcut == "" {
			t.Fatal("DetectKDELayoutSwitchKeys() returned empty shortcut without error")
		}
	} else if !errors.Is(err, ErrKDEConfigPathUnknown) &&
		!errors.Is(err, ErrKDENoShortcutConfigured) &&
		!os.IsNotExist(err) {
		// Error is expected if KDE is not configured
		// If it's an unexpected error type with shortcut info, that's still acceptable
		// (e.g., parse error with shortcut in error message)
		t.Logf("DetectKDELayoutSwitchKeys() unexpected error type: %v (shortcut: %q)", err, shortcut)
	}
}

// ========== Task 04: KDE opt-in Tests ==========

func TestGetDefaultProviders_DoesNotContainKDE(t *testing.T) {
	providers := getDefaultProviders()

	for _, p := range providers {
		if p.Name() == SourceKDE {
			t.Fatal("getDefaultProviders() should NOT contain KDE provider")
		}
	}
}

func TestGetDefaultProviders_ContainsXKB(t *testing.T) {
	providers := getDefaultProviders()

	hasXKB := false
	for _, p := range providers {
		if p.Name() == SourceXKB {
			hasXKB = true
			break
		}
	}

	if !hasXKB {
		t.Fatal("getDefaultProviders() should contain XKB provider")
	}
}

func TestGetProviderBySource_KDE(t *testing.T) {
	provider := getProviderBySource(SourceKDE)

	if provider == nil {
		t.Fatal("getProviderBySource(SourceKDE) returned nil")
	}
	if provider.Name() != SourceKDE {
		t.Fatalf("getProviderBySource(SourceKDE).Name() = %v, want %v", provider.Name(), SourceKDE)
	}
}

func TestGetProviderBySource_XKB(t *testing.T) {
	provider := getProviderBySource(SourceXKB)

	if provider == nil {
		t.Fatal("getProviderBySource(SourceXKB) returned nil")
	}
	if provider.Name() != SourceXKB {
		t.Fatalf("getProviderBySource(SourceXKB).Name() = %v, want %v", provider.Name(), SourceXKB)
	}
}

func TestGetProviderBySource_Unknown(t *testing.T) {
	provider := getProviderBySource("unknown")

	if provider != nil {
		t.Fatalf("getProviderBySource(\"unknown\") = %v, want nil", provider)
	}
}

func TestDetectLayoutSwitchKeys_DefaultDoesNotUseKDE(t *testing.T) {
	// Call with nil options (default behavior)
	result, _ := DetectLayoutSwitchKeys(nil)

	// Verify KDE is not in provider order
	for _, source := range result.Context.ProviderOrder {
		if source == SourceKDE {
			t.Fatal("DetectLayoutSwitchKeys(nil) should NOT include KDE in ProviderOrder")
		}
	}
}

func TestDetectLayoutSwitchKeys_SourceOverrideKDE(t *testing.T) {
	opts := &DetectionOptions{SourceOverride: SourceKDE}
	result, _ := DetectLayoutSwitchKeys(opts)

	// Verify only KDE is in provider order
	if len(result.Context.ProviderOrder) != 1 {
		t.Fatalf("DetectLayoutSwitchKeys with KDE override should have 1 provider, got %d", len(result.Context.ProviderOrder))
	}
	if result.Context.ProviderOrder[0] != SourceKDE {
		t.Fatalf("DetectLayoutSwitchKeys with KDE override should use KDE provider, got %v", result.Context.ProviderOrder[0])
	}
}

func TestDetectLayoutSwitchKeys_SourceOverrideXKB(t *testing.T) {
	opts := &DetectionOptions{SourceOverride: SourceXKB}
	result, _ := DetectLayoutSwitchKeys(opts)

	// Verify only XKB is in provider order
	if len(result.Context.ProviderOrder) != 1 {
		t.Fatalf("DetectLayoutSwitchKeys with XKB override should have 1 provider, got %d", len(result.Context.ProviderOrder))
	}
	if result.Context.ProviderOrder[0] != SourceXKB {
		t.Fatalf("DetectLayoutSwitchKeys with XKB override should use XKB provider, got %v", result.Context.ProviderOrder[0])
	}
}

func TestDetectLayoutSwitchKeys_SourceOverrideUnknown(t *testing.T) {
	opts := &DetectionOptions{SourceOverride: "unknown"}
	_, err := DetectLayoutSwitchKeys(opts)

	if err == nil {
		t.Fatal("DetectLayoutSwitchKeys with unknown source should return error")
	}
	if !strings.Contains(err.Error(), "unknown source") {
		t.Fatalf("Expected error to contain 'unknown source', got: %v", err)
	}
}

func TestDetectLayoutSwitchKeys_KDEWithValidConfig(t *testing.T) {
	// Create a temp KDE config
	content := `[KDE Keyboard Layout Switcher]
Switch to Next Keyboard Layout=Alt+Shift,Alt+Shift,Switch to Next Keyboard Layout
`
	tmpDir := t.TempDir()
	configDir := filepath.Join(tmpDir, ".config")
	configPath := filepath.Join(configDir, "kglobalshortcutsrc")
	if err := os.MkdirAll(configDir, 0o750); err != nil {
		t.Fatalf("failed to create config dir: %v", err)
	}
	if err := os.WriteFile(configPath, []byte(content), 0o600); err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}

	// Save and restore HOME and XDG_CONFIG_HOME
	origHome := os.Getenv("HOME")
	origXDGConfigHome := os.Getenv("XDG_CONFIG_HOME")
	os.Setenv("HOME", tmpDir)
	os.Setenv("XDG_CONFIG_HOME", configDir) // Point to our temp config dir
	defer func() {
		os.Setenv("HOME", origHome)
		if origXDGConfigHome == "" {
			os.Unsetenv("XDG_CONFIG_HOME")
		} else {
			os.Setenv("XDG_CONFIG_HOME", origXDGConfigHome)
		}
	}()

	// Test with KDE override
	opts := &DetectionOptions{SourceOverride: SourceKDE}
	result, err := DetectLayoutSwitchKeys(opts)

	if err != nil {
		t.Fatalf("DetectLayoutSwitchKeys with KDE override failed: %v", err)
	}
	if result.Source != SourceKDE {
		t.Fatalf("Expected source KDE, got %v", result.Source)
	}
	if result.RawValue != "Alt+Shift" {
		t.Fatalf("Expected RawValue 'Alt+Shift', got %q", result.RawValue)
	}
	wantScancodes := []uint16{56, 42}
	if !reflect.DeepEqual(result.Scancodes, wantScancodes) {
		t.Fatalf("Expected scancodes %v, got %v", wantScancodes, result.Scancodes)
	}
}

func TestDetectLayoutSwitchKeys_SourceOverrideAuto(t *testing.T) {
	// SourceAuto should be treated as default behavior (no override)
	opts := &DetectionOptions{SourceOverride: SourceAuto}
	result, _ := DetectLayoutSwitchKeys(opts)

	// Verify KDE is not in provider order (same as nil opts)
	for _, source := range result.Context.ProviderOrder {
		if source == SourceKDE {
			t.Fatal("DetectLayoutSwitchKeys with SourceOverride='auto' should NOT include KDE in ProviderOrder")
		}
	}

	// Should have XKB in provider order
	hasXKB := slices.Contains(result.Context.ProviderOrder, SourceXKB)
	if !hasXKB {
		t.Fatal("DetectLayoutSwitchKeys with SourceOverride='auto' should include XKB in ProviderOrder")
	}
}

// ========== Task 06: GNOME Provider Tests ==========

func TestGNOMEProvider_Name(t *testing.T) {
	p := NewGNOMEProvider()
	if p.Name() != SourceGNOME {
		t.Fatalf("gnomeProvider.Name() = %v, want %v", p.Name(), SourceGNOME)
	}
}

func TestGetProviderBySource_GNOME(t *testing.T) {
	provider := getProviderBySource(SourceGNOME)

	if provider == nil {
		t.Fatal("getProviderBySource(SourceGNOME) returned nil")
	}
	if provider.Name() != SourceGNOME {
		t.Fatalf("getProviderBySource(SourceGNOME).Name() = %v, want %v", provider.Name(), SourceGNOME)
	}
}

// NOTE: GNOME is now dynamically added based on DE detection (Task 07).
// getDefaultProviders() returns XKB only, but DetectLayoutSwitchKeys()
// uses isGNOME() to determine provider order.

// ========== parseGsettingsArray Tests ==========

func TestParseGsettingsArray_SuperSpace(t *testing.T) {
	got := parseGsettingsArray("['<Super>space']")
	want := "<Super>space"
	if got != want {
		t.Fatalf("parseGsettingsArray() = %q, want %q", got, want)
	}
}

func TestParseGsettingsArray_MultipleBindings(t *testing.T) {
	got := parseGsettingsArray("['<Super>space', '<Alt>Shift_L']")
	want := "<Super>space" // Should return first element
	if got != want {
		t.Fatalf("parseGsettingsArray() = %q, want %q", got, want)
	}
}

func TestParseGsettingsArray_Empty(t *testing.T) {
	tests := []string{"@as []", "[]"}
	for _, input := range tests {
		got := parseGsettingsArray(input)
		if got != "" {
			t.Fatalf("parseGsettingsArray(%q) = %q, want empty string", input, got)
		}
	}
}

func TestParseGsettingsArray_Disabled(t *testing.T) {
	got := parseGsettingsArray("['disabled']")
	want := "disabled"
	if got != want {
		t.Fatalf("parseGsettingsArray() = %q, want %q", got, want)
	}
}

func TestParseGsettingsArray_WithDoubleQuotes(t *testing.T) {
	got := parseGsettingsArray("[\"<Super>space\"]")
	want := "<Super>space"
	if got != want {
		t.Fatalf("parseGsettingsArray() = %q, want %q", got, want)
	}
}

// ========== parseGNOMEAccelerator Tests ==========

func TestParseGNOMEAccelerator_SuperSpace(t *testing.T) {
	scancodes, keyNames, err := parseGNOMEAccelerator("<Super>space")
	if err != nil {
		t.Fatalf("parseGNOMEAccelerator() error = %v", err)
	}

	wantScancodes := []uint16{125, 57} // Super_L + Space
	if !reflect.DeepEqual(scancodes, wantScancodes) {
		t.Fatalf("parseGNOMEAccelerator() scancodes = %v, want %v", scancodes, wantScancodes)
	}
	if keyNames != "Super+Space" {
		t.Fatalf("parseGNOMEAccelerator() keyNames = %q, want %q", keyNames, "Super+Space")
	}
}

func TestParseGNOMEAccelerator_ShiftAlt(t *testing.T) {
	scancodes, keyNames, err := parseGNOMEAccelerator("<Shift><Alt>")
	if err != nil {
		t.Fatalf("parseGNOMEAccelerator() error = %v", err)
	}

	wantScancodes := []uint16{42, 56} // Shift_L + Alt_L
	if !reflect.DeepEqual(scancodes, wantScancodes) {
		t.Fatalf("parseGNOMEAccelerator() scancodes = %v, want %v", scancodes, wantScancodes)
	}
	if keyNames != "Shift+Alt" {
		t.Fatalf("parseGNOMEAccelerator() keyNames = %q, want %q", keyNames, "Shift+Alt")
	}
}

func TestParseGNOMEAccelerator_PrimaryShift(t *testing.T) {
	scancodes, keyNames, err := parseGNOMEAccelerator("<Primary><Shift>")
	if err != nil {
		t.Fatalf("parseGNOMEAccelerator() error = %v", err)
	}

	wantScancodes := []uint16{29, 42} // Control_L + Shift_L
	if !reflect.DeepEqual(scancodes, wantScancodes) {
		t.Fatalf("parseGNOMEAccelerator() scancodes = %v, want %v", scancodes, wantScancodes)
	}
	if keyNames != "Ctrl+Shift" {
		t.Fatalf("parseGNOMEAccelerator() keyNames = %q, want %q", keyNames, "Ctrl+Shift")
	}
}

func TestParseGNOMEAccelerator_Disabled(t *testing.T) {
	scancodes, keyNames, err := parseGNOMEAccelerator("disabled")
	if err != nil {
		t.Fatalf("parseGNOMEAccelerator(\"disabled\") unexpected error: %v", err)
	}
	if scancodes != nil {
		t.Fatalf("parseGNOMEAccelerator(\"disabled\") scancodes = %v, want nil", scancodes)
	}
	if keyNames != "" {
		t.Fatalf("parseGNOMEAccelerator(\"disabled\") keyNames = %q, want empty", keyNames)
	}
}

func TestParseGNOMEAccelerator_Empty(t *testing.T) {
	scancodes, keyNames, err := parseGNOMEAccelerator("")
	if err != nil {
		t.Fatalf("parseGNOMEAccelerator(\"\") unexpected error: %v", err)
	}
	if scancodes != nil {
		t.Fatalf("parseGNOMEAccelerator(\"\") scancodes = %v, want nil", scancodes)
	}
	if keyNames != "" {
		t.Fatalf("parseGNOMEAccelerator(\"\") keyNames = %q, want empty", keyNames)
	}
}

func TestParseGNOMEAccelerator_UnsupportedKey(t *testing.T) {
	_, _, err := parseGNOMEAccelerator("<Super>F1")
	if err == nil {
		t.Fatal("parseGNOMEAccelerator(\"<Super>F1\") expected error for unsupported key")
	}
	if !strings.Contains(err.Error(), "unsupported key") {
		t.Fatalf("Expected error to contain 'unsupported key', got: %v", err)
	}
}

func TestParseGNOMEAccelerator_UnknownModifier(t *testing.T) {
	_, _, err := parseGNOMEAccelerator("<UnknownMod>space")
	if err == nil {
		t.Fatal("parseGNOMEAccelerator(\"<UnknownMod>space\") expected error")
	}
	if !strings.Contains(err.Error(), "unknown modifier") {
		t.Fatalf("Expected error to contain 'unknown modifier', got: %v", err)
	}
}

func TestParseGNOMEAccelerator_MalformedNoClose(t *testing.T) {
	_, _, err := parseGNOMEAccelerator("<Superspace")
	if err == nil {
		t.Fatal("parseGNOMEAccelerator(\"<Superspace\") expected error for malformed input")
	}
	if !strings.Contains(err.Error(), "malformed") {
		t.Fatalf("Expected error to contain 'malformed', got: %v", err)
	}
}

func TestParseGNOMEAccelerator_CapsLock(t *testing.T) {
	scancodes, keyNames, err := parseGNOMEAccelerator("Caps_Lock")
	if err != nil {
		t.Fatalf("parseGNOMEAccelerator() error = %v", err)
	}

	wantScancodes := []uint16{58} // CapsLock
	if !reflect.DeepEqual(scancodes, wantScancodes) {
		t.Fatalf("parseGNOMEAccelerator() scancodes = %v, want %v", scancodes, wantScancodes)
	}
	if keyNames != "CapsLock" {
		t.Fatalf("parseGNOMEAccelerator() keyNames = %q, want %q", keyNames, "CapsLock")
	}
}

func TestParseGNOMEAccelerator_AllModifiers(t *testing.T) {
	tests := []struct {
		modifier     string
		wantScancode uint16
		wantKeyName  string
	}{
		{"<Super>", 125, "Super"},
		{"<Shift>", 42, "Shift"},
		{"<Control>", 29, "Ctrl"},
		{"<Ctrl>", 29, "Ctrl"},
		{"<Alt>", 56, "Alt"},
		{"<Primary>", 29, "Ctrl"},
		{"<Meta>", 125, "Super"},
	}

	for _, tt := range tests {
		t.Run(tt.modifier, func(t *testing.T) {
			// Modifier without key needs a key after, so use space
			input := tt.modifier + "space"
			scancodes, keyNames, err := parseGNOMEAccelerator(input)
			if err != nil {
				t.Fatalf("parseGNOMEAccelerator(%q) error = %v", input, err)
			}

			// First scancode should be the modifier
			if len(scancodes) < 1 || scancodes[0] != tt.wantScancode {
				t.Fatalf("parseGNOMEAccelerator(%q) first scancode = %v, want %d", input, scancodes, tt.wantScancode)
			}

			// KeyNames should start with expected name
			if !strings.HasPrefix(keyNames, tt.wantKeyName) {
				t.Fatalf("parseGNOMEAccelerator(%q) keyNames = %q, want prefix %q", input, keyNames, tt.wantKeyName)
			}
		})
	}
}

// TestGnomeKeyvalToScancode_MapContents verifies the GNOME keyval map
func TestGnomeKeyvalToScancode_MapContents(t *testing.T) {
	// Verify expected entries exist
	expectedKeys := []string{
		"Shift_L", "Shift_R", "Control_L", "Control_R",
		"Alt_L", "Alt_R", "Super_L", "Super_R",
		"space", "Caps_Lock",
	}
	for _, key := range expectedKeys {
		if _, ok := gnomeKeyvalToScancode[key]; !ok {
			t.Fatalf("gnomeKeyvalToScancode missing %q", key)
		}
	}
}

// TestGnomeModifierAliases_MapContents verifies the GNOME modifier aliases map
func TestGnomeModifierAliases_MapContents(t *testing.T) {
	expectedAliases := []string{
		"Primary", "Super", "Shift", "Control", "Ctrl", "Alt", "Meta",
	}
	for _, alias := range expectedAliases {
		if _, ok := gnomeModifierAliases[alias]; !ok {
			t.Fatalf("gnomeModifierAliases missing %q", alias)
		}
	}
}

func TestDetectLayoutSwitchKeys_SourceOverrideGNOME(t *testing.T) {
	// Mock execLookPath to simulate gsettings not found
	// This ensures we don't call real gsettings in unit tests
	origLookPath := execLookPath
	execLookPath = func(file string) (string, error) {
		if file == "gsettings" {
			return "", errors.New("gsettings not found")
		}
		return origLookPath(file)
	}
	defer func() { execLookPath = origLookPath }()

	opts := &DetectionOptions{SourceOverride: SourceGNOME}
	result, _ := DetectLayoutSwitchKeys(opts)

	// Verify only GNOME is in provider order
	if len(result.Context.ProviderOrder) != 1 {
		t.Fatalf("DetectLayoutSwitchKeys with GNOME override should have 1 provider, got %d", len(result.Context.ProviderOrder))
	}
	if result.Context.ProviderOrder[0] != SourceGNOME {
		t.Fatalf("DetectLayoutSwitchKeys with GNOME override should use GNOME provider, got %v", result.Context.ProviderOrder[0])
	}
}

// TestGNOMEProvider_Detect_Integration tests GNOME provider on real GNOME system.
// This test is skipped by default; set GSWITCH_INTEGRATION_TEST=1 to run.
func TestGNOMEProvider_Detect_Integration(t *testing.T) {
	if os.Getenv("GSWITCH_INTEGRATION_TEST") != "1" {
		t.Skip("Skipping integration test; set GSWITCH_INTEGRATION_TEST=1 to run")
	}

	p := NewGNOMEProvider()
	attempt := p.Detect(nil)

	// Log the result for manual verification
	t.Logf("GNOME Provider Detect result:")
	t.Logf("  Status: %s", attempt.Status)
	t.Logf("  RawValue: %q", attempt.RawValue)
	t.Logf("  Scancodes: %v", attempt.Scancodes)
	t.Logf("  KeyNames: %q", attempt.KeyNames)
	if attempt.Error != "" {
		t.Logf("  Error: %s", attempt.Error)
	}

	// Just verify it returns a valid status without panic
	validStatuses := map[AttemptStatus]bool{
		StatusFound:       true,
		StatusNotFound:    true,
		StatusInactive:    true,
		StatusError:       true,
		StatusUnsupported: true,
	}
	if !validStatuses[attempt.Status] {
		t.Fatalf("gnomeProvider.Detect(nil) returned invalid status: %v", attempt.Status)
	}

	// If found, verify scancodes are non-empty
	if attempt.Status == StatusFound {
		if len(attempt.Scancodes) == 0 {
			t.Fatal("gnomeProvider.Detect() returned StatusFound but empty scancodes")
		}
		if attempt.KeyNames == "" {
			t.Fatal("gnomeProvider.Detect() returned StatusFound but empty keyNames")
		}
	}
}

// ========== Task 07: isGNOME() and Provider Order Tests ==========

func TestIsGNOME_WithXDGCurrentDesktopGNOME(t *testing.T) {
	env := &SessionEnv{
		XDGCurrentDesktop: "GNOME",
	}
	if !isGNOME(env) {
		t.Fatal("isGNOME() should return true for XDGCurrentDesktop='GNOME'")
	}
}

func TestIsGNOME_WithUbuntuGNOME(t *testing.T) {
	env := &SessionEnv{
		XDGCurrentDesktop: "ubuntu:GNOME",
	}
	if !isGNOME(env) {
		t.Fatal("isGNOME() should return true for XDGCurrentDesktop='ubuntu:GNOME'")
	}
}

func TestIsGNOME_WithGNOMEClassic(t *testing.T) {
	env := &SessionEnv{
		XDGCurrentDesktop: "GNOME-Classic:GNOME",
	}
	if !isGNOME(env) {
		t.Fatal("isGNOME() should return true for XDGCurrentDesktop='GNOME-Classic:GNOME'")
	}
}

func TestIsGNOME_WithKDE(t *testing.T) {
	env := &SessionEnv{
		XDGCurrentDesktop: "KDE",
	}
	if isGNOME(env) {
		t.Fatal("isGNOME() should return false for XDGCurrentDesktop='KDE'")
	}
}

func TestIsGNOME_WithXFCE(t *testing.T) {
	env := &SessionEnv{
		XDGCurrentDesktop: "XFCE",
	}
	if isGNOME(env) {
		t.Fatal("isGNOME() should return false for XDGCurrentDesktop='XFCE'")
	}
}

func TestIsGNOME_EmptyEnv(t *testing.T) {
	env := &SessionEnv{}

	// Clear OS environment variables to ensure clean test
	t.Setenv("XDG_CURRENT_DESKTOP", "")
	t.Setenv("XDG_SESSION_DESKTOP", "")
	t.Setenv("DESKTOP_SESSION", "")

	if isGNOME(env) {
		t.Fatal("isGNOME() should return false for empty env")
	}
}

func TestIsGNOME_NilEnv(t *testing.T) {
	// Clear OS environment variables to ensure clean test
	t.Setenv("XDG_CURRENT_DESKTOP", "")
	t.Setenv("XDG_SESSION_DESKTOP", "")
	t.Setenv("DESKTOP_SESSION", "")

	if isGNOME(nil) {
		t.Fatal("isGNOME(nil) should return false when OS env is empty")
	}
}

func TestIsGNOME_WithXDGSessionDesktop(t *testing.T) {
	env := &SessionEnv{
		XDGSessionDesktop: "gnome",
	}
	if !isGNOME(env) {
		t.Fatal("isGNOME() should return true for XDGSessionDesktop='gnome'")
	}
}

func TestIsGNOME_WithDesktopSession(t *testing.T) {
	env := &SessionEnv{
		DesktopSession: "gnome",
	}
	if !isGNOME(env) {
		t.Fatal("isGNOME() should return true for DesktopSession='gnome'")
	}
}

func TestIsGNOME_CaseInsensitive(t *testing.T) {
	tests := []string{"GNOME", "gnome", "Gnome", "GnOmE"}
	for _, val := range tests {
		env := &SessionEnv{XDGCurrentDesktop: val}
		if !isGNOME(env) {
			t.Fatalf("isGNOME() should return true for XDGCurrentDesktop=%q (case insensitive)", val)
		}
	}
}

func TestIsGNOME_FallbackToOSEnv(t *testing.T) {
	// Set OS environment variable
	t.Setenv("XDG_CURRENT_DESKTOP", "GNOME")

	// Pass nil env - should use OS env
	if !isGNOME(nil) {
		t.Fatal("isGNOME(nil) should return true when OS env XDG_CURRENT_DESKTOP='GNOME'")
	}
}

func TestIsGNOME_NoFallbackWhenEnvHasData(t *testing.T) {
	// Set OS environment to GNOME
	t.Setenv("XDG_CURRENT_DESKTOP", "GNOME")

	// But SessionEnv says KDE - should NOT fallback to os.Getenv
	env := &SessionEnv{
		XDGCurrentDesktop: "KDE",
	}

	if isGNOME(env) {
		t.Fatal("isGNOME() should return false when env has KDE, even if OS env has GNOME")
	}
}

func TestDetectLayoutSwitchKeys_ProviderOrderOnGNOME(t *testing.T) {
	// Set OS environment to simulate GNOME
	t.Setenv("XDG_CURRENT_DESKTOP", "GNOME")

	// Mock execLookPath to prevent actual gsettings calls
	origLookPath := execLookPath
	execLookPath = func(file string) (string, error) {
		if file == "gsettings" || file == "loginctl" {
			return "", errors.New("not found")
		}
		return origLookPath(file)
	}
	defer func() { execLookPath = origLookPath }()

	result, _ := DetectLayoutSwitchKeys(nil)

	// On GNOME: GNOME should be first in provider order
	if len(result.Context.ProviderOrder) < 2 {
		t.Fatalf("Expected at least 2 providers in order, got %d", len(result.Context.ProviderOrder))
	}
	if result.Context.ProviderOrder[0] != SourceGNOME {
		t.Fatalf("On GNOME: first provider should be GNOME, got %v", result.Context.ProviderOrder[0])
	}
	if result.Context.ProviderOrder[1] != SourceXKB {
		t.Fatalf("On GNOME: second provider should be XKB, got %v", result.Context.ProviderOrder[1])
	}
	// Verify DE is set in context
	if result.Context.DE != "gnome" {
		t.Fatalf("On GNOME: Context.DE should be 'gnome', got %q", result.Context.DE)
	}
}

func TestDetectLayoutSwitchKeys_ProviderOrderOnNonGNOME(t *testing.T) {
	// Set OS environment to simulate KDE
	t.Setenv("XDG_CURRENT_DESKTOP", "KDE")
	t.Setenv("XDG_SESSION_DESKTOP", "")
	t.Setenv("DESKTOP_SESSION", "")

	// Mock execLookPath to prevent actual gsettings calls
	origLookPath := execLookPath
	execLookPath = func(file string) (string, error) {
		if file == "gsettings" || file == "loginctl" {
			return "", errors.New("not found")
		}
		return origLookPath(file)
	}
	defer func() { execLookPath = origLookPath }()

	result, _ := DetectLayoutSwitchKeys(nil)

	// On non-GNOME: XKB should be first in provider order
	if len(result.Context.ProviderOrder) < 2 {
		t.Fatalf("Expected at least 2 providers in order, got %d", len(result.Context.ProviderOrder))
	}
	if result.Context.ProviderOrder[0] != SourceXKB {
		t.Fatalf("On non-GNOME: first provider should be XKB, got %v", result.Context.ProviderOrder[0])
	}
	if result.Context.ProviderOrder[1] != SourceGNOME {
		t.Fatalf("On non-GNOME: second provider should be GNOME, got %v", result.Context.ProviderOrder[1])
	}
	// Verify DE is set in context
	if result.Context.DE != "other" {
		t.Fatalf("On non-GNOME: Context.DE should be 'other', got %q", result.Context.DE)
	}
}

func TestDetectLayoutSwitchKeys_KDENotInDefaultProviders(t *testing.T) {
	// Clear GNOME env vars
	t.Setenv("XDG_CURRENT_DESKTOP", "")

	// Mock execLookPath to prevent actual gsettings/loginctl calls
	origLookPath := execLookPath
	execLookPath = func(file string) (string, error) {
		if file == "gsettings" || file == "loginctl" {
			return "", errors.New("not found")
		}
		return origLookPath(file)
	}
	defer func() { execLookPath = origLookPath }()

	result, _ := DetectLayoutSwitchKeys(nil)

	// KDE should NOT be in provider order
	for _, source := range result.Context.ProviderOrder {
		if source == SourceKDE {
			t.Fatal("KDE provider should NOT be in default provider order")
		}
	}
}
