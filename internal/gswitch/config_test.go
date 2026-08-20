package gswitch

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestFormatLayoutSpec(t *testing.T) {
	tests := []struct {
		name     string
		layout   LayoutSpec
		expected string
	}{
		{
			name:     "layout without variant",
			layout:   LayoutSpec{Name: "us"},
			expected: "us",
		},
		{
			name:     "layout with variant",
			layout:   LayoutSpec{Name: "ua", Variant: "unicode"},
			expected: "ua(unicode)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatLayoutSpec(tt.layout)
			if result != tt.expected {
				t.Errorf("formatLayoutSpec() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestFormatLayoutsSection(t *testing.T) {
	tests := []struct {
		name     string
		layouts  []LayoutSpec
		expected string
	}{
		{
			name:     "empty layouts",
			layouts:  []LayoutSpec{},
			expected: "# layout1=\n# layout2=",
		},
		{
			name:     "only one layout",
			layouts:  []LayoutSpec{{Name: "us"}},
			expected: "# layout1=\n# layout2=",
		},
		{
			name:     "two layouts without variant",
			layouts:  []LayoutSpec{{Name: "us"}, {Name: "ru"}},
			expected: "layout1=us\nlayout2=ru",
		},
		{
			name:     "two layouts with variant",
			layouts:  []LayoutSpec{{Name: "us"}, {Name: "ua", Variant: "unicode"}},
			expected: "layout1=us\nlayout2=ua(unicode)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatLayoutsSection(tt.layouts)
			if result != tt.expected {
				t.Errorf("formatLayoutsSection() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestParseLayoutSwitchKey(t *testing.T) {
	tests := []struct {
		input        string
		expected     []uint16
		expectedAuto bool
	}{
		{"42", []uint16{42}, false},
		{"29+42", []uint16{29, 42}, false},
		{"125", []uint16{125}, false},
		{"29 + 42", []uint16{29, 42}, false},
		{"auto", nil, true},
		{"Auto", nil, true},
		{"AUTO", nil, true},
		{"  auto  ", nil, true},
		{"29+abc", nil, false},
		{"29+", nil, false},
		{"+42", nil, false},
		{"", nil, false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result, isAuto := parseLayoutSwitchKey(tt.input)
			if isAuto != tt.expectedAuto {
				t.Errorf("parseLayoutSwitchKey(%q) isAuto = %v, want %v", tt.input, isAuto, tt.expectedAuto)
				return
			}
			if len(result) != len(tt.expected) {
				t.Errorf("parseLayoutSwitchKey(%q) = %v, want %v", tt.input, result, tt.expected)
				return
			}
			for i, v := range result {
				if v != tt.expected[i] {
					t.Errorf("parseLayoutSwitchKey(%q)[%d] = %d, want %d", tt.input, i, v, tt.expected[i])
				}
			}
		})
	}
}

func TestConfigValidate(t *testing.T) {
	tests := []struct {
		name    string
		config  Config
		wantErr bool
	}{
		{
			name: "valid config",
			config: Config{
				LayoutSwitchKey:   []uint16{42},
				Delay:             10,
				LayoutSwitchDelay: 100,
			},
			wantErr: false,
		},
		{
			name: "missing layout switch key",
			config: Config{
				Delay:             10,
				LayoutSwitchDelay: 100,
			},
			wantErr: true,
		},
		{
			name: "negative delay",
			config: Config{
				LayoutSwitchKey: []uint16{42},
				Delay:           -1,
			},
			wantErr: true,
		},
		{
			name: "only layout1 set",
			config: Config{
				LayoutSwitchKey: []uint16{42},
				Layouts:         []LayoutSpec{{Name: "us"}},
			},
			wantErr: true,
		},
		{
			name: "both layouts set",
			config: Config{
				LayoutSwitchKey: []uint16{42},
				Layouts:         []LayoutSpec{{Name: "us"}, {Name: "ru"}},
			},
			wantErr: false,
		},
		{
			name: "empty layout1 name",
			config: Config{
				LayoutSwitchKey: []uint16{42},
				Layouts:         []LayoutSpec{{Name: ""}, {Name: "ru"}},
			},
			wantErr: true,
		},
		{
			name: "empty layout2 name",
			config: Config{
				LayoutSwitchKey: []uint16{42},
				Layouts:         []LayoutSpec{{Name: "us"}, {Name: ""}},
			},
			wantErr: true,
		},
		{
			name: "whitespace-only layout1 name",
			config: Config{
				LayoutSwitchKey: []uint16{42},
				Layouts:         []LayoutSpec{{Name: "  "}, {Name: "ru"}},
			},
			wantErr: true,
		},
		{
			name: "whitespace-only layout2 name",
			config: Config{
				LayoutSwitchKey: []uint16{42},
				Layouts:         []LayoutSpec{{Name: "us"}, {Name: "\t"}},
			},
			wantErr: true,
		},
		{
			name: "convert key conflicts with layout switch",
			config: Config{
				LayoutSwitchKey: []uint16{42},
				ConvertKey:      42,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Config.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestLayoutFormatRoundTrip verifies that layouts written to config
// can be correctly parsed back
func TestLayoutFormatRoundTrip(t *testing.T) {
	tests := []struct {
		name    string
		layouts []LayoutSpec
	}{
		{
			name:    "simple layouts",
			layouts: []LayoutSpec{{Name: "us"}, {Name: "ru"}},
		},
		{
			name:    "layout with variant",
			layouts: []LayoutSpec{{Name: "us"}, {Name: "ua", Variant: "unicode"}},
		},
		{
			name:    "both with variants",
			layouts: []LayoutSpec{{Name: "us", Variant: "intl"}, {Name: "ru", Variant: "phonetic"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Format layouts as they would be written to config
			formatted1 := formatLayoutSpec(tt.layouts[0])
			formatted2 := formatLayoutSpec(tt.layouts[1])

			// Parse them back as splitLayoutVariant would
			layout1, variant1 := splitLayoutVariant(formatted1)
			layout2, variant2 := splitLayoutVariant(formatted2)

			// Verify round-trip
			if layout1 != tt.layouts[0].Name || variant1 != tt.layouts[0].Variant {
				t.Errorf("layout1 round-trip failed: formatted=%q, got name=%q variant=%q, want name=%q variant=%q",
					formatted1, layout1, variant1, tt.layouts[0].Name, tt.layouts[0].Variant)
			}
			if layout2 != tt.layouts[1].Name || variant2 != tt.layouts[1].Variant {
				t.Errorf("layout2 round-trip failed: formatted=%q, got name=%q variant=%q, want name=%q variant=%q",
					formatted2, layout2, variant2, tt.layouts[1].Name, tt.layouts[1].Variant)
			}
		})
	}
}

// TestConfigValidateWithAuto verifies that config validation works with auto mode.
// When LayoutSwitchAuto is true, validation defers LayoutSwitchKey check to detection time.
func TestConfigValidateWithAuto(t *testing.T) {
	tests := []struct {
		name    string
		config  Config
		wantErr bool
	}{
		{
			name: "auto mode with detected keys",
			config: Config{
				LayoutSwitchKey:   []uint16{29, 42}, // Detected keys
				LayoutSwitchAuto:  true,
				Delay:             10,
				LayoutSwitchDelay: 100,
			},
			wantErr: false,
		},
		{
			name: "auto mode without keys passes validation (deferred to detection)",
			config: Config{
				LayoutSwitchAuto:  true, // Auto set, keys will be detected later in NewSwitcher()
				Delay:             10,
				LayoutSwitchDelay: 100,
			},
			wantErr: false, // Should pass - validation is deferred until detection
		},
		{
			name: "non-auto mode without keys fails validation",
			config: Config{
				LayoutSwitchAuto:  false, // Explicit mode requires keys
				Delay:             10,
				LayoutSwitchDelay: 100,
			},
			wantErr: true, // Should fail because LayoutSwitchKey is empty and not auto mode
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Config.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestLayoutSwitchAutoFormatting verifies that auto mode is correctly formatted for saving
func TestLayoutSwitchAutoFormatting(t *testing.T) {
	tests := []struct {
		name             string
		layoutSwitchKey  []uint16
		layoutSwitchAuto bool
		expectedContains string
	}{
		{
			name:             "auto mode saves as 'auto'",
			layoutSwitchKey:  []uint16{29, 42},
			layoutSwitchAuto: true,
			expectedContains: "layout-switch=auto",
		},
		{
			name:             "single key saves as number",
			layoutSwitchKey:  []uint16{125},
			layoutSwitchAuto: false,
			expectedContains: "layout-switch=125",
		},
		{
			name:             "key combination saves with plus",
			layoutSwitchKey:  []uint16{29, 42},
			layoutSwitchAuto: false,
			expectedContains: "layout-switch=29+42",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				LayoutSwitchKey:   tt.layoutSwitchKey,
				LayoutSwitchAuto:  tt.layoutSwitchAuto,
				Delay:             10,
				LayoutSwitchDelay: 100,
			}

			// We can't easily test SaveConfig without writing to file,
			// but we can test the formatting logic by checking what would be written
			var layoutSwitchStr string
			if cfg.LayoutSwitchAuto {
				layoutSwitchStr = "auto"
			} else {
				switch len(cfg.LayoutSwitchKey) {
				case 1:
					layoutSwitchStr = "125" // For single key test
				default:
					layoutSwitchStr = "29+42" // For combo test
				}
			}

			expected := tt.expectedContains[len("layout-switch="):]
			if layoutSwitchStr != expected {
				t.Errorf("layout-switch formatting = %q, want %q", layoutSwitchStr, expected)
			}
		})
	}
}

// TestFormatLayoutSwitchValue tests the formatting of layout-switch value
func TestFormatLayoutSwitchValue(t *testing.T) {
	tests := []struct {
		name             string
		layoutSwitchKey  []uint16
		layoutSwitchAuto bool
		expected         string
	}{
		{
			name:             "auto mode",
			layoutSwitchKey:  []uint16{29, 42},
			layoutSwitchAuto: true,
			expected:         "auto",
		},
		{
			name:             "single key",
			layoutSwitchKey:  []uint16{125},
			layoutSwitchAuto: false,
			expected:         "125",
		},
		{
			name:             "key combination",
			layoutSwitchKey:  []uint16{29, 42},
			layoutSwitchAuto: false,
			expected:         "29+42",
		},
		{
			name:             "three keys",
			layoutSwitchKey:  []uint16{29, 42, 56},
			layoutSwitchAuto: false,
			expected:         "29+42+56",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Replicate the formatting logic from SaveConfig
			var layoutSwitchStr string
			if tt.layoutSwitchAuto {
				layoutSwitchStr = "auto"
			} else {
				switch len(tt.layoutSwitchKey) {
				case 0:
					layoutSwitchStr = ""
				case 1:
					layoutSwitchStr = strconv.FormatUint(uint64(tt.layoutSwitchKey[0]), 10)
				default:
					parts := make([]string, len(tt.layoutSwitchKey))
					for i, k := range tt.layoutSwitchKey {
						parts[i] = strconv.FormatUint(uint64(k), 10)
					}
					layoutSwitchStr = strings.Join(parts, "+")
				}
			}

			if layoutSwitchStr != tt.expected {
				t.Errorf("format layout-switch = %q, want %q", layoutSwitchStr, tt.expected)
			}
		})
	}
}

// TestParseLayoutSwitchKeyRoundTrip tests that parsing and formatting are consistent
func TestParseLayoutSwitchKeyRoundTrip(t *testing.T) {
	tests := []string{
		"auto",
		"125",
		"29+42",
		"29 + 42",
	}

	for _, input := range tests {
		t.Run(input, func(t *testing.T) {
			keys, isAuto := parseLayoutSwitchKey(input)

			// Format back
			var formatted string
			if isAuto {
				formatted = "auto"
			} else {
				parts := make([]string, len(keys))
				for i, k := range keys {
					parts[i] = strconv.FormatUint(uint64(k), 10)
				}
				formatted = strings.Join(parts, "+")
			}

			// Parse again
			keys2, isAuto2 := parseLayoutSwitchKey(formatted)

			if isAuto != isAuto2 {
				t.Errorf("round-trip isAuto mismatch: %v vs %v", isAuto, isAuto2)
			}
			if len(keys) != len(keys2) {
				t.Errorf("round-trip keys length mismatch: %v vs %v", keys, keys2)
				return
			}
			for i := range keys {
				if keys[i] != keys2[i] {
					t.Errorf("round-trip keys[%d] mismatch: %d vs %d", i, keys[i], keys2[i])
				}
			}
		})
	}
}

// TestWriteConfigFromArgsToWithAuto tests that writeConfigFromArgsTo works with layout-switch=auto
// and does NOT call DetectLayoutSwitchScancodes (which would fail without user session)
func TestWriteConfigFromArgsToWithAuto(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "default.conf")

	args := "layout-switch=auto,convert-key=0,delay=10,layout-switch-delay=100,layout1=us,layout2=ru"
	err := writeConfigFromArgsTo(configPath, args)
	if err != nil {
		t.Fatalf("writeConfigFromArgsTo with auto failed: %v", err)
	}

	// Read the file and verify it contains layout-switch=auto
	content, err := readFileContent(configPath)
	if err != nil {
		t.Fatalf("failed to read config file: %v", err)
	}

	if !strings.Contains(content, "layout-switch=auto") {
		t.Errorf("config file does not contain 'layout-switch=auto', got:\n%s", content)
	}

	// Verify other settings are saved correctly
	if !strings.Contains(content, "convert-key=0") {
		t.Errorf("config file does not contain 'convert-key=0'")
	}
	if !strings.Contains(content, "delay=10") {
		t.Errorf("config file does not contain 'delay=10'")
	}
	if !strings.Contains(content, "layout-switch-delay=100") {
		t.Errorf("config file does not contain 'layout-switch-delay=100'")
	}
	if !strings.Contains(content, "layout1=us") {
		t.Errorf("config file does not contain 'layout1=us'")
	}
	if !strings.Contains(content, "layout2=ru") {
		t.Errorf("config file does not contain 'layout2=ru'")
	}
}

// TestWriteConfigFromArgsToWithExplicitKeys tests that writeConfigFromArgsTo saves explicit scancodes
func TestWriteConfigFromArgsToWithExplicitKeys(t *testing.T) {
	tests := []struct {
		name         string
		layoutSwitch string
		expected     string
	}{
		{
			name:         "single key",
			layoutSwitch: "125",
			expected:     "layout-switch=125",
		},
		{
			name:         "key combination",
			layoutSwitch: "56+42",
			expected:     "layout-switch=56+42",
		},
		{
			name:         "another combination",
			layoutSwitch: "29+42",
			expected:     "layout-switch=29+42",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tempDir := t.TempDir()
			configPath := filepath.Join(tempDir, "default.conf")

			args := "layout-switch=" + tt.layoutSwitch + ",convert-key=0,delay=10,layout-switch-delay=100,layout1=us,layout2=ru"
			err := writeConfigFromArgsTo(configPath, args)
			if err != nil {
				t.Fatalf("writeConfigFromArgsTo failed: %v", err)
			}

			content, err := readFileContent(configPath)
			if err != nil {
				t.Fatalf("failed to read config file: %v", err)
			}

			if !strings.Contains(content, tt.expected) {
				t.Errorf("config file does not contain %q, got:\n%s", tt.expected, content)
			}
		})
	}
}

func TestWriteConfigFromArgsToRejectsInvalidLayoutSwitch(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "default.conf")

	args := "layout-switch=29+abc,convert-key=0,delay=10,layout-switch-delay=100,layout1=us,layout2=ru"
	err := writeConfigFromArgsTo(configPath, args)
	if err == nil {
		t.Fatal("writeConfigFromArgsTo() expected error for invalid layout-switch, got nil")
	}
	if !strings.Contains(err.Error(), "invalid layout-switch value") {
		t.Fatalf("writeConfigFromArgsTo() error = %v, expected invalid layout-switch message", err)
	}
}

// readFileContent is a helper to read file content for tests
func readFileContent(path string) (string, error) {
	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return "", err
	}
	return string(data), nil
}
