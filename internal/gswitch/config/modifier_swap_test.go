package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSwapConversionModifiersRoundTrip(t *testing.T) {
	for _, value := range []string{"false", "true"} {
		t.Run(value, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "default.conf")
			if err := WriteConfigFromArgsTo(path, "layout-switch=auto,convert-key=119,swap-conversion-modifiers="+value); err != nil {
				t.Fatal(err)
			}
			cfg, err := LoadConfigFrom(path)
			if err != nil {
				t.Fatal(err)
			}
			if cfg.SwapConversionModifiers != (value == "true") {
				t.Fatalf("swap = %t, want %s", cfg.SwapConversionModifiers, value)
			}
			if err := SaveConfigTo(path, cfg); err != nil {
				t.Fatal(err)
			}
			loaded, err := LoadConfigFrom(path)
			if err != nil {
				t.Fatal(err)
			}
			if loaded.SwapConversionModifiers != cfg.SwapConversionModifiers {
				t.Fatal("save lost modifier setting")
			}
		})
	}
}

func TestSwapConversionModifiersLegacyDefault(t *testing.T) {
	path := filepath.Join(t.TempDir(), "default.conf")
	if err := os.WriteFile(path, []byte("layout-switch=auto\nconvert-key=119\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfigFrom(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.SwapConversionModifiers {
		t.Fatal("legacy config must preserve standard shortcuts")
	}
}
