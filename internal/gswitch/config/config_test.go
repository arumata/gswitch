package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestLoadConfigFromRejectsInvalidConvertKey(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "default.conf")
	content := "layout-switch=auto\nconvert-key=29+42\ndelay=10\nlayout-switch-delay=100\nlayout1=us\nlayout2=ru\n"
	if err := os.WriteFile(configPath, []byte(content), 0o600); err != nil {
		t.Fatalf("write test config: %v", err)
	}

	_, err := LoadConfigFrom(configPath)
	if err == nil {
		t.Fatal("LoadConfigFrom() expected error for convert-key combination, got nil")
	}
	if !strings.Contains(err.Error(), "invalid convert-key") {
		t.Fatalf("LoadConfigFrom() error = %v, expected invalid convert-key message", err)
	}
}

func TestConfigValidateRejectsModifierConvertKey(t *testing.T) {
	tests := []struct {
		name string
		code uint16
	}{
		{name: "Shift_L", code: 42},
		{name: "Shift_R", code: 54},
		{name: "Ctrl_L", code: 29},
		{name: "Ctrl_R", code: 97},
		{name: "Alt_L", code: 56},
		{name: "Alt_R", code: 100},
		{name: "Super_L", code: 125},
		{name: "Super_R", code: 126},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				LayoutSwitchAuto:  true,
				ConvertKey:        tt.code,
				Delay:             10,
				LayoutSwitchDelay: 100,
			}

			if err := cfg.Validate(); err == nil {
				t.Fatalf("Validate() expected error for modifier convert key %d, got nil", tt.code)
			}
		})
	}
}

func TestWriteConfigFromArgsToRejectsModifierConvertKey(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "default.conf")
	args := "layout-switch=auto,convert-key=29,delay=10,layout-switch-delay=100,layout1=us,layout2=ru"

	err := WriteConfigFromArgsTo(configPath, args)
	if err == nil {
		t.Fatal("WriteConfigFromArgsTo() expected error for modifier convert-key=29, got nil")
	}
	if !strings.Contains(err.Error(), "modifier") {
		t.Fatalf("WriteConfigFromArgsTo() error = %v, expected modifier message", err)
	}
}

func TestSaveConfigToRejectsModifierConvertKey(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "default.conf")
	cfg := &Config{
		LayoutSwitchAuto:  true,
		ConvertKey:        29,
		Delay:             10,
		LayoutSwitchDelay: 100,
	}

	err := SaveConfigTo(configPath, cfg)
	if err == nil {
		t.Fatal("SaveConfigTo() expected error for modifier convert-key=29, got nil")
	}
	if !strings.Contains(err.Error(), "modifier") {
		t.Fatalf("SaveConfigTo() error = %v, expected modifier message", err)
	}
}

func TestLoadConfigFromFallsBackFromLegacyModifierConvertKey(t *testing.T) {
	for _, code := range []uint16{29, 97, 42, 54, 56, 100, 125, 126} {
		t.Run(strconv.FormatUint(uint64(code), 10), func(t *testing.T) {
			configPath := filepath.Join(t.TempDir(), "default.conf")
			content := fmt.Sprintf("layout-switch=auto\nconvert-key=%d\ndelay=10\nlayout-switch-delay=100\nlayout1=us\nlayout2=ru\n", code)
			if err := os.WriteFile(configPath, []byte(content), 0o600); err != nil {
				t.Fatal(err)
			}
			cfg, err := LoadConfigFrom(configPath)
			if err != nil {
				t.Fatal(err)
			}
			if cfg.ConvertKey != 0 || len(cfg.Warnings) != 1 || !strings.Contains(cfg.Warnings[0], strconv.FormatUint(uint64(code), 10)) {
				t.Fatalf("legacy %d: key=%d warnings=%v", code, cfg.ConvertKey, cfg.Warnings)
			}
			stored, err := os.ReadFile(configPath) //nolint:gosec // Path is created inside t.TempDir above.
			if err != nil {
				t.Fatal(err)
			}
			if string(stored) != content {
				t.Fatal("loading rewrote the configuration")
			}
		})
	}
}
