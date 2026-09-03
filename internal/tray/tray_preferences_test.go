package tray

import (
	"os"
	"path/filepath"
	"testing"
)

func TestTrayIconModePreferences(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "gswitch", "tray.conf")
	previous := trayPreferencesPath
	trayPreferencesPath = func() (string, error) { return path, nil }
	t.Cleanup(func() { trayPreferencesPath = previous })

	if got := LoadTrayIconMode(); got != DefaultTrayIconMode {
		t.Fatalf("missing preference = %q, want %q", got, DefaultTrayIconMode)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("# comment\ntray-icon-mode=unknown\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := LoadTrayIconMode(); got != DefaultTrayIconMode {
		t.Fatalf("invalid preference = %q, want %q", got, DefaultTrayIconMode)
	}
	if err := os.WriteFile(path, []byte("tray-icon-mode=flag\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := LoadTrayIconMode(); got != TrayIconModeFlag {
		t.Fatalf("valid preference = %q, want %q", got, TrayIconModeFlag)
	}
	if err := SaveTrayIconMode(TrayIconModeAppWithFlag); err != nil {
		t.Fatal(err)
	}
	if got := LoadTrayIconMode(); got != TrayIconModeAppWithFlag {
		t.Fatalf("saved preference = %q, want %q", got, TrayIconModeAppWithFlag)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("preference permissions = %o, want 600", info.Mode().Perm())
	}
}
