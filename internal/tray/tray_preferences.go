package tray

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type TrayIconMode string

const (
	TrayIconModeApp         TrayIconMode = "app"
	TrayIconModeFlag        TrayIconMode = "flag"
	TrayIconModeAppWithFlag TrayIconMode = "app-with-flag"
	DefaultTrayIconMode     TrayIconMode = TrayIconModeFlag
)

func normalizeTrayIconMode(mode TrayIconMode) TrayIconMode {
	switch mode {
	case TrayIconModeApp, TrayIconModeFlag, TrayIconModeAppWithFlag:
		return mode
	default:
		return DefaultTrayIconMode
	}
}

var trayPreferencesPath = defaultTrayPreferencesPath

func defaultTrayPreferencesPath() (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(configDir, "gswitch", "tray.conf"), nil
}

func LoadTrayIconMode() TrayIconMode {
	path, err := trayPreferencesPath()
	if err != nil {
		return DefaultTrayIconMode
	}

	// #nosec G304 -- the path comes from os.UserConfigDir or a test-controlled override.
	data, err := os.ReadFile(path)
	if err != nil {
		return DefaultTrayIconMode
	}
	for line := range strings.SplitSeq(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, found := strings.Cut(line, "=")
		if found && strings.TrimSpace(key) == "tray-icon-mode" {
			return normalizeTrayIconMode(TrayIconMode(strings.TrimSpace(value)))
		}
	}
	return DefaultTrayIconMode
}

func SaveTrayIconMode(mode TrayIconMode) error {
	path, err := trayPreferencesPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create tray preferences directory: %w", err)
	}

	tempFile, err := os.CreateTemp(filepath.Dir(path), ".tray.conf-*")
	if err != nil {
		return fmt.Errorf("create tray preferences: %w", err)
	}
	tempPath := tempFile.Name()
	defer os.Remove(tempPath)
	if err := tempFile.Chmod(0o600); err != nil {
		tempFile.Close()
		return fmt.Errorf("set tray preferences permissions: %w", err)
	}
	if _, err := fmt.Fprintf(tempFile, "tray-icon-mode=%s\n", normalizeTrayIconMode(mode)); err != nil {
		tempFile.Close()
		return fmt.Errorf("write tray preferences: %w", err)
	}
	if err := tempFile.Close(); err != nil {
		return fmt.Errorf("close tray preferences: %w", err)
	}
	if err := os.Rename(tempPath, path); err != nil {
		return fmt.Errorf("save tray preferences: %w", err)
	}
	return nil
}
