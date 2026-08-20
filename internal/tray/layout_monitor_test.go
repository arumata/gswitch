package tray

import (
	"testing"
	"time"
)

func TestParseSetxkbmapOutput(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []LayoutInfo
	}{
		{
			name: "single layout",
			input: `rules:      evdev
model:      pc104
layout:     us`,
			expected: []LayoutInfo{
				{ShortCode: "US", LongName: "English (US)"},
			},
		},
		{
			name: "two layouts",
			input: `rules:      evdev
model:      pc104
layout:     us,ru
variant:    ,
options:    grp:win_space_toggle`,
			expected: []LayoutInfo{
				{ShortCode: "US", LongName: "English (US)"},
				{ShortCode: "RU", LongName: "Russian"},
			},
		},
		{
			name: "three layouts with unknown",
			input: `rules:      evdev
model:      pc105
layout:     us,ru,de
variant:    ,,
options:    grp:alt_shift_toggle`,
			expected: []LayoutInfo{
				{ShortCode: "US", LongName: "English (US)"},
				{ShortCode: "RU", LongName: "Russian"},
				{ShortCode: "DE", LongName: "German"},
			},
		},
		{
			name: "unknown layout code",
			input: `rules:      evdev
model:      pc104
layout:     xyz`,
			expected: []LayoutInfo{
				{ShortCode: "XYZ", LongName: "XYZ"},
			},
		},
		{
			name:     "empty output",
			input:    "",
			expected: nil,
		},
		{
			name: "no layout line",
			input: `rules:      evdev
model:      pc104`,
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseSetxkbmapOutput(tt.input)

			if len(result) != len(tt.expected) {
				t.Errorf("expected %d layouts, got %d", len(tt.expected), len(result))
				return
			}

			for i, exp := range tt.expected {
				if result[i].ShortCode != exp.ShortCode {
					t.Errorf("layout %d: expected ShortCode %q, got %q", i, exp.ShortCode, result[i].ShortCode)
				}
				if result[i].LongName != exp.LongName {
					t.Errorf("layout %d: expected LongName %q, got %q", i, exp.LongName, result[i].LongName)
				}
			}
		})
	}
}

func TestParseKDELayoutsList(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []LayoutInfo
	}{
		{
			name:  "two layouts",
			input: "([('us', 'us', 'English (US)'), ('ru', 'ru', 'Russian')],)",
			expected: []LayoutInfo{
				{ShortCode: "US", LongName: "English (US)"},
				{ShortCode: "RU", LongName: "Russian"},
			},
		},
		{
			name:  "single layout",
			input: "([('us', 'us', 'English (US)')],)",
			expected: []LayoutInfo{
				{ShortCode: "US", LongName: "English (US)"},
			},
		},
		{
			name:  "three layouts",
			input: "([('us', 'us', 'English (US)'), ('ru', 'ru', 'Russian'), ('de', 'de', 'German')],)",
			expected: []LayoutInfo{
				{ShortCode: "US", LongName: "English (US)"},
				{ShortCode: "RU", LongName: "Russian"},
				{ShortCode: "DE", LongName: "German"},
			},
		},
		{
			name:     "empty output",
			input:    "",
			expected: nil,
		},
		{
			name:     "malformed output",
			input:    "invalid",
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseKDELayoutsList(tt.input)

			if len(result) != len(tt.expected) {
				t.Errorf("expected %d layouts, got %d", len(tt.expected), len(result))
				return
			}

			for i, exp := range tt.expected {
				if result[i].ShortCode != exp.ShortCode {
					t.Errorf("layout %d: expected ShortCode %q, got %q", i, exp.ShortCode, result[i].ShortCode)
				}
				if result[i].LongName != exp.LongName {
					t.Errorf("layout %d: expected LongName %q, got %q", i, exp.LongName, result[i].LongName)
				}
			}
		})
	}
}

func TestParseKDELayoutIndex(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected int
	}{
		{
			name:     "index 0",
			input:    "(uint32 0,)",
			expected: 0,
		},
		{
			name:     "index 1",
			input:    "(uint32 1,)",
			expected: 1,
		},
		{
			name:     "index 5",
			input:    "(uint32 5,)",
			expected: 5,
		},
		{
			name:     "with whitespace",
			input:    "  (uint32 2,)  \n",
			expected: 2,
		},
		{
			name:     "empty input",
			input:    "",
			expected: 0,
		},
		{
			name:     "malformed input",
			input:    "invalid",
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseKDELayoutIndex(tt.input)
			if result != tt.expected {
				t.Errorf("expected %d, got %d", tt.expected, result)
			}
		})
	}
}

func TestParseFcitx5CurrentMethod(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantCode string
		wantName string
		wantOK   bool
	}{
		{
			name:     "keyboard-us",
			input:    "('keyboard-us',)",
			wantCode: "US",
			wantName: "English (US)",
			wantOK:   true,
		},
		{
			name:     "keyboard-ru",
			input:    "('keyboard-ru',)",
			wantCode: "RU",
			wantName: "Russian",
			wantOK:   true,
		},
		{
			name:     "keyboard with variant",
			input:    "('keyboard-ru-phonetic',)",
			wantCode: "RU",
			wantName: "Russian",
			wantOK:   true,
		},
		{
			name:     "non-keyboard input method",
			input:    "('pinyin',)",
			wantCode: "IM",
			wantName: "pinyin",
			wantOK:   true,
		},
		{
			name:     "empty input",
			input:    "",
			wantCode: "",
			wantName: "",
			wantOK:   false,
		},
		{
			name:     "malformed input",
			input:    "invalid",
			wantCode: "",
			wantName: "",
			wantOK:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			layout, ok := parseFcitx5CurrentMethod(tt.input)
			if ok != tt.wantOK {
				t.Errorf("ok = %v, want %v", ok, tt.wantOK)
			}
			if layout.ShortCode != tt.wantCode {
				t.Errorf("ShortCode = %q, want %q", layout.ShortCode, tt.wantCode)
			}
			if layout.LongName != tt.wantName {
				t.Errorf("LongName = %q, want %q", layout.LongName, tt.wantName)
			}
		})
	}
}

func TestParseFcitx5GroupName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"default group", "('Default',)", "Default"},
		{"group with space", "('My Group',)", "My Group"},
		{"trailing newline", "('Default',)\n", "Default"},
		{"empty group name", "('',)", ""},
		{"empty input", "", ""},
		{"malformed input", "invalid", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := parseFcitx5GroupName(tt.input); got != tt.expected {
				t.Errorf("parseFcitx5GroupName(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestParseFcitx5GroupInputMethodCount(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected int
	}{
		{
			name:     "two layouts — fcitx5 switches",
			input:    "('us', [('keyboard-us', ''), ('keyboard-ru', '')])",
			expected: 2,
		},
		{
			name:     "single layout — fcitx5 does not switch",
			input:    "('us', [('keyboard-us', '')])",
			expected: 1,
		},
		{
			name:     "layout plus input method",
			input:    "('us', [('keyboard-us', ''), ('pinyin', ''), ('keyboard-ru', '')])",
			expected: 3,
		},
		{
			name:     "unknown group returns an empty list",
			input:    "('', @a(ss) [])",
			expected: 0,
		},
		{"empty input", "", 0},
		{"malformed input", "invalid", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := parseFcitx5GroupInputMethodCount(tt.input); got != tt.expected {
				t.Errorf("parseFcitx5GroupInputMethodCount(%q) = %d, want %d", tt.input, got, tt.expected)
			}
		})
	}
}

func TestFcitx5OwnsSwitchingUsesCache(t *testing.T) {
	// A fresh monitor must not treat the zero value of fcitx5CheckedAt as a
	// valid cache entry, or fcitx5 would never be consulted.
	m := NewLayoutMonitor(nil)
	if !m.fcitx5CheckedAt.IsZero() {
		t.Fatal("expected fcitx5CheckedAt to start zero")
	}

	// A cached answer inside the TTL is returned without touching D-Bus, which
	// is what keeps the twice-a-second poll from spawning gdbus processes.
	m.fcitx5Switches = true
	m.fcitx5CheckedAt = time.Now()
	if !m.fcitx5OwnsSwitching() {
		t.Error("expected the cached answer to be reused")
	}

	m.fcitx5Switches = false
	m.fcitx5CheckedAt = time.Now()
	if m.fcitx5OwnsSwitching() {
		t.Error("expected the cached negative answer to be reused")
	}
}

func TestLayoutCodeToName(t *testing.T) {
	tests := []struct {
		code     string
		expected string
	}{
		{"us", "English (US)"},
		{"US", "English (US)"},
		{"ru", "Russian"},
		{"RU", "Russian"},
		{"de", "German"},
		{"xyz", "XYZ"},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.code, func(t *testing.T) {
			result := layoutCodeToName(tt.code)
			if result != tt.expected {
				t.Errorf("layoutCodeToName(%q) = %q, want %q", tt.code, result, tt.expected)
			}
		})
	}
}
