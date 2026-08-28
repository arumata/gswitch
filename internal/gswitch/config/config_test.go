package config

import (
	"os"
	"path/filepath"
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
