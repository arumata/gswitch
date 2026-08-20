package config

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

const (
	// DefaultConfigPath is the default system config location.
	DefaultConfigPath = "/etc/gswitch/default.conf"
	// MaxDelayMs is the maximum allowed delay value in config.
	MaxDelayMs = 1000
	// MaxLayoutSwitchDelayMs is the maximum allowed layout switch delay.
	MaxLayoutSwitchDelayMs = 2000
)

// uidHexRegex validates the hex format of device UID
var uidHexRegex = regexp.MustCompile(`^[0-9a-fA-F]{4}:[0-9a-fA-F]{4}:[0-9a-fA-F]{4}:[0-9a-fA-F]{4}:[0-9a-fA-F]{16}$`)

// LayoutSpec represents a layout with optional variant.
type LayoutSpec struct {
	Name    string
	Variant string
}

// Config holds the application configuration
type Config struct {
	Blacklist []string // List of device UIDs to ignore

	LayoutSwitchKey   []uint16
	LayoutSwitchAuto  bool   // If true, layout switch keys were auto-detected (for saving as "auto")
	ConvertKey        uint16 // Key to trigger conversion (0 = double-shift mode)
	ReverseMode       bool
	Delay             int
	LayoutSwitchDelay int          // Delay after layout switch (ms), default 100
	Layouts           []LayoutSpec // Explicit layouts for conversion (optional, must be exactly 2 if set)
}

// LoadConfig reads configuration from the default config path.
func LoadConfig() (*Config, error) {
	return LoadConfigFrom(DefaultConfigPath)
}

// LoadConfigFrom reads configuration from file.
func LoadConfigFrom(path string) (*Config, error) {
	cfg := &Config{
		ReverseMode:       false,
		Delay:             10, // default delay
		LayoutSwitchDelay: 100,
	}

	cleanPath := filepath.Clean(path)
	// #nosec G304 -- path is controlled by trusted callers (default path or test temp files).
	file, err := os.Open(cleanPath)
	if err != nil {
		return nil, fmt.Errorf("cannot open config file: %w", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := strings.TrimSpace(scanner.Text())

		// Skip comments and empty lines
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "[") {
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.Trim(strings.TrimSpace(parts[1]), "\"")

		switch key {
		case "layout-switch-key", "layout-switch":
			layoutKeys, isAuto, parseErr := parseLayoutSwitchKeyStrict(value)
			if parseErr != nil {
				return nil, fmt.Errorf("invalid %s at line %d: %w", key, lineNo, parseErr)
			}
			cfg.LayoutSwitchKey = layoutKeys
			cfg.LayoutSwitchAuto = isAuto
		case "convert-key", "replace-key":
			if v, err := strconv.ParseUint(value, 10, 16); err == nil {
				cfg.ConvertKey = uint16(v)
			}
		case "reverse-mode":
			cfg.ReverseMode = strings.EqualFold(value, "true")
		case "delay":
			if v, err := strconv.Atoi(value); err == nil {
				cfg.Delay = v
			}
		case "layout-switch-delay":
			if v, err := strconv.Atoi(value); err == nil {
				cfg.LayoutSwitchDelay = v
			}
		case "blacklist":
			if value != "" {
				cfg.Blacklist = ParseBlacklist(value)
			}
		case "layout1":
			if value != "" {
				layout, variant := SplitLayoutVariant(value)
				if len(cfg.Layouts) == 0 {
					cfg.Layouts = make([]LayoutSpec, 0, 2)
				}
				// Insert at position 0
				cfg.Layouts = append([]LayoutSpec{{Name: layout, Variant: variant}}, cfg.Layouts...)
			}
		case "layout2":
			if value != "" {
				layout, variant := SplitLayoutVariant(value)
				if len(cfg.Layouts) == 0 {
					cfg.Layouts = make([]LayoutSpec, 0, 2)
				}
				cfg.Layouts = append(cfg.Layouts, LayoutSpec{Name: layout, Variant: variant})
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading config file: %w", err)
	}

	// NOTE: Auto-detection of layout switch keys is NOT performed here.
	// When LayoutSwitchAuto is true, detection will be performed later:
	// - For daemon: in NewSwitcher() after session environment is prepared
	// - For CLI: in the appropriate command (e.g., --detect-layout-switch)
	// This allows proper handling of root/systemd scenarios where session
	// environment must be obtained first.

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

// Validate checks configuration for errors.
// When LayoutSwitchAuto is true, LayoutSwitchKey validation is deferred until detection.
// Auto-detection will be performed later in NewSwitcher() or the appropriate CLI command.
func (cfg *Config) Validate() error {
	// For layout-switch=auto, LayoutSwitchKey validation is deferred
	// until detection runs in NewSwitcher()
	if !cfg.LayoutSwitchAuto {
		// Validate required fields for explicit configuration
		if len(cfg.LayoutSwitchKey) == 0 {
			return errors.New("layout switch key not configured")
		}
	}

	// ConvertKey=0 is valid (double-shift mode), so no validation needed

	// Validate delay bounds
	if cfg.Delay < 0 {
		return fmt.Errorf("delay must be non-negative, got %d", cfg.Delay)
	}
	if cfg.Delay > MaxDelayMs {
		return fmt.Errorf("delay too large: %dms (max %dms)", cfg.Delay, MaxDelayMs)
	}

	// Validate layout switch delay bounds
	if cfg.LayoutSwitchDelay < 0 {
		return fmt.Errorf("layout-switch-delay must be non-negative, got %d", cfg.LayoutSwitchDelay)
	}
	if cfg.LayoutSwitchDelay > MaxLayoutSwitchDelayMs {
		return fmt.Errorf("layout-switch-delay too large: %dms (max %dms)", cfg.LayoutSwitchDelay, MaxLayoutSwitchDelayMs)
	}

	// Check that convert key doesn't conflict with layout switch keys (unless it's 0 or auto)
	if cfg.ConvertKey != 0 && !cfg.LayoutSwitchAuto && slices.Contains(cfg.LayoutSwitchKey, cfg.ConvertKey) {
		return fmt.Errorf("convert key (%d) conflicts with layout switch key", cfg.ConvertKey)
	}

	// If layouts are specified, exactly two must be provided
	if len(cfg.Layouts) != 0 && len(cfg.Layouts) != 2 {
		return fmt.Errorf("both layout1 and layout2 must be set (got %d)", len(cfg.Layouts))
	}

	// Validate that layout names are not empty (including whitespace-only)
	if len(cfg.Layouts) == 2 {
		if strings.TrimSpace(cfg.Layouts[0].Name) == "" {
			return errors.New("layout1 name cannot be empty")
		}
		if strings.TrimSpace(cfg.Layouts[1].Name) == "" {
			return errors.New("layout2 name cannot be empty")
		}
	}

	return nil
}

// SplitLayoutVariant parses "layout", "layout(variant)" and "layout-variant" forms.
func SplitLayoutVariant(s string) (layout, variant string) {
	if s == "" {
		return "", ""
	}
	s = strings.TrimSpace(s)

	if idx := strings.Index(s, "("); idx != -1 {
		layout = strings.TrimSpace(s[:idx])
		variant = strings.TrimSpace(strings.TrimSuffix(s[idx+1:], ")"))
		return layout, variant
	}

	parts := strings.Split(s, "-")
	layout = strings.TrimSpace(parts[0])
	if len(parts) > 1 {
		variant = strings.TrimSpace(strings.Join(parts[1:], "-"))
	}
	return layout, variant
}

// ParseLayoutSwitchKey parses layout switch key configuration.
// Format: "auto", "42", or "29+42" for key combinations
// Returns (scancodes, isAuto)
func ParseLayoutSwitchKey(value string) ([]uint16, bool) {
	keys, isAuto, err := parseLayoutSwitchKeyStrict(value)
	if err != nil {
		return nil, false
	}
	return keys, isAuto
}

// parseLayoutSwitchKeyStrict parses layout-switch value and validates all tokens.
// Unlike ParseLayoutSwitchKey, this returns an explicit error for invalid input.
func parseLayoutSwitchKeyStrict(value string) ([]uint16, bool, error) {
	trimmed := strings.TrimSpace(value)
	if strings.EqualFold(trimmed, "auto") {
		return nil, true, nil
	}
	if trimmed == "" {
		return nil, false, errors.New("layout switch key is empty")
	}

	parts := strings.Split(trimmed, "+")
	keys := make([]uint16, 0, len(parts))
	for _, p := range parts {
		token := strings.TrimSpace(p)
		if token == "" {
			return nil, false, errors.New("layout switch key contains empty token")
		}
		v, err := strconv.ParseUint(token, 10, 16)
		if err != nil {
			return nil, false, fmt.Errorf("invalid key code %q", token)
		}
		keys = append(keys, uint16(v))
	}

	return keys, false, nil
}

// ParseBlacklist parses comma or semicolon-separated list of device UIDs.
func ParseBlacklist(value string) []string {
	// Replace semicolons with commas for uniform parsing
	// Semicolons are used by tray --write-config to avoid conflicts with arg separator
	value = strings.ReplaceAll(value, ";", ",")
	parts := strings.Split(value, ",")
	uids := make([]string, 0, len(parts))
	for _, p := range parts {
		uid := strings.TrimSpace(p)
		// Validate UID format with hex chars: xxxx:xxxx:xxxx:xxxx:xxxxxxxxxxxxxxxx
		if uidHexRegex.MatchString(uid) {
			uids = append(uids, uid)
		}
	}
	return uids
}

// FormatLayoutsSection formats layout1/layout2 for config output.
// Returns commented lines if layouts are not set, actual values otherwise
func FormatLayoutsSection(layouts []LayoutSpec) string {
	if len(layouts) < 2 {
		return "# layout1=\n# layout2="
	}
	return fmt.Sprintf("layout1=%s\nlayout2=%s",
		FormatLayoutSpec(layouts[0]), FormatLayoutSpec(layouts[1]))
}

// FormatLayoutSpec formats a single LayoutSpec as "name" or "name(variant)".
func FormatLayoutSpec(l LayoutSpec) string {
	if l.Variant != "" {
		return fmt.Sprintf("%s(%s)", l.Name, l.Variant)
	}
	return l.Name
}

// WriteConfigFromArgs parses key=value pairs and saves configuration to the default config file.
// Format: "layout-switch=125,convert-key=0,delay=10,layout-switch-delay=100,layout1=us,layout2=ru"
func WriteConfigFromArgs(args string) error {
	return WriteConfigFromArgsTo(DefaultConfigPath, args)
}

// WriteConfigFromArgsTo parses key=value pairs and saves configuration to specified path.
// This function is used by WriteConfigFromArgs and can be used in tests with temporary paths.
func WriteConfigFromArgsTo(path, args string) error {
	cfg := &Config{
		Delay:             10,
		LayoutSwitchDelay: 100,
	}

	pairs := strings.SplitSeq(args, ",")
	for pair := range pairs {
		kv := strings.SplitN(pair, "=", 2)
		if len(kv) != 2 {
			continue
		}
		key := strings.TrimSpace(kv[0])
		value := strings.TrimSpace(kv[1])

		switch key {
		case "layout-switch":
			layoutKeys, isAuto, parseErr := parseLayoutSwitchKeyStrict(value)
			if parseErr != nil {
				return fmt.Errorf("invalid layout-switch value %q: %w", value, parseErr)
			}
			cfg.LayoutSwitchKey = layoutKeys
			cfg.LayoutSwitchAuto = isAuto
		case "convert-key":
			if v, err := strconv.ParseUint(value, 10, 16); err == nil {
				cfg.ConvertKey = uint16(v)
			}
		case "delay":
			if v, err := strconv.Atoi(value); err == nil {
				cfg.Delay = v
			}
		case "layout-switch-delay":
			if v, err := strconv.Atoi(value); err == nil {
				cfg.LayoutSwitchDelay = v
			}
		case "layout1":
			if value != "" {
				layout, variant := SplitLayoutVariant(value)
				if len(cfg.Layouts) == 0 {
					cfg.Layouts = make([]LayoutSpec, 0, 2)
				}
				// Insert at position 0 or replace
				if len(cfg.Layouts) == 0 {
					cfg.Layouts = append(cfg.Layouts, LayoutSpec{Name: layout, Variant: variant})
				} else {
					cfg.Layouts[0] = LayoutSpec{Name: layout, Variant: variant}
				}
			}
		case "layout2":
			if value != "" {
				layout, variant := SplitLayoutVariant(value)
				if len(cfg.Layouts) == 0 {
					cfg.Layouts = make([]LayoutSpec, 1, 2)
				}
				if len(cfg.Layouts) == 1 {
					cfg.Layouts = append(cfg.Layouts, LayoutSpec{Name: layout, Variant: variant})
				} else {
					cfg.Layouts[1] = LayoutSpec{Name: layout, Variant: variant}
				}
			}
		case "blacklist":
			if value != "" {
				cfg.Blacklist = ParseBlacklist(value)
			}
		}
	}

	// NOTE: Auto-detection is NOT performed here.
	// layout-switch=auto is saved as-is, detection happens:
	// - In daemon at startup (NewSwitcher)
	// - Via CLI: gswitch --detect-layout-switch --json
	// This allows --write-config to work under pkexec without user session.

	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("validation error: %w", err)
	}

	return SaveConfigTo(path, cfg)
}

// SaveConfig saves configuration to the default config file
func SaveConfig(cfg *Config) error {
	return SaveConfigTo(DefaultConfigPath, cfg)
}

// SaveConfigTo saves configuration to the specified file path.
// This function is used by SaveConfig and can be used in tests with temporary paths.
// Uses atomic write (temp file + rename) to prevent partial writes on failure.
func SaveConfigTo(path string, cfg *Config) error {
	cleanPath := filepath.Clean(path)
	dir := filepath.Dir(cleanPath)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return fmt.Errorf("cannot create config directory: %w", err)
	}

	// Create temporary file in the same directory for atomic rename
	tempFile, err := os.CreateTemp(dir, ".gswitch-config-*.tmp")
	if err != nil {
		return fmt.Errorf("cannot create temp config file: %w", err)
	}
	tempPath := tempFile.Name()

	// Helper to cleanup temp file on error
	cleanup := func() {
		tempFile.Close()
		os.Remove(tempPath)
	}

	// Format layout switch key
	var layoutSwitchStr string
	if cfg.LayoutSwitchAuto {
		layoutSwitchStr = "auto"
	} else {
		switch len(cfg.LayoutSwitchKey) {
		case 0:
			cleanup()
			return errors.New("cannot save config: layout switch key is not configured")
		case 1:
			layoutSwitchStr = strconv.Itoa(int(cfg.LayoutSwitchKey[0]))
		default:
			// Support for 2+ keys in combination
			parts := make([]string, len(cfg.LayoutSwitchKey))
			for i, k := range cfg.LayoutSwitchKey {
				parts[i] = strconv.Itoa(int(k))
			}
			layoutSwitchStr = strings.Join(parts, "+")
		}
	}

	// Format blacklist
	blacklistStr := strings.Join(cfg.Blacklist, ",")

	content := fmt.Sprintf(`[gswitch]
# gswitch configuration file.


# Scancode of the key or key combination used to switch
# the keyboard layout in your system.
# Set to 'auto' for automatic detection (recommended).
# Key combinations are supported; use '+' as a delimiter.
# Run 'sudo showkey' to find your key scancodes.
# Examples:
# layout-switch=auto
# layout-switch=125
# layout-switch=29+42

layout-switch=%s


# Scancode of the key used to correct the entered text.
# Key combinations are not supported.
# Set to 0 to use double SHIFT as the convert trigger (default).
# Run 'sudo showkey' to find your key scancodes.
# Examples:
# convert-key=0
# convert-key=119

convert-key=%d


# gswitch waits a small delay before sending keys.
# This helps your system handle all events correctly.
# Smaller delay makes switching faster, but may cause errors.
# If you see wrong or mixed symbols, try to increase the delay.
# Default delay value is 10 ms.
# Example:
# delay=10

delay=%d


# Delay after layout switch in milliseconds.
# Some desktop environments need extra time to process the layout change.
# Increase this value if converted text appears in wrong layout.
# Default layout-switch-delay value is 100
# layout-switch-delay=100

layout-switch-delay=%d


# If you get unwanted input from a specific device,
# add its UID to the blacklist below.
# gswitch will ignore all blacklisted devices.
# Use commas (,) to separate multiple UIDs.
# Run 'sudo gswitch --debug' to list your devices' UIDs.
# Examples:
# blacklist=0000:0000:0000:0000:0000000000000000
# blacklist=0000:0000:0000:0000:0000000000000000,0000:0000:0000:0000:0000000000000000

blacklist=%s


# Explicit layouts for conversion (optional).
# Use these if you have more than 2 keyboard layouts configured
# and want gswitch to convert between specific two.
# Format: layout or layout(variant)
# Examples:
# layout1=us
# layout2=ru
# layout1=us
# layout2=ua(unicode)

%s
`, layoutSwitchStr, cfg.ConvertKey, cfg.Delay, cfg.LayoutSwitchDelay, blacklistStr,
		FormatLayoutsSection(cfg.Layouts))

	// Write content to temp file
	if _, err = tempFile.WriteString(content); err != nil {
		cleanup()
		return fmt.Errorf("cannot write config file: %w", err)
	}

	// Sync to ensure data is flushed to disk before rename (durability)
	if err = tempFile.Sync(); err != nil {
		cleanup()
		return fmt.Errorf("cannot sync config file: %w", err)
	}

	// Close temp file and check for errors
	if err = tempFile.Close(); err != nil {
		os.Remove(tempPath)
		return fmt.Errorf("cannot close config file: %w", err)
	}

	// Atomic rename to target path
	if err = os.Rename(tempPath, cleanPath); err != nil {
		os.Remove(tempPath)
		return fmt.Errorf("cannot rename config file: %w", err)
	}

	// Restore file permissions to 0644 (CreateTemp creates with 0600)
	// System config in /etc/ should be world-readable for diagnostics
	if err = os.Chmod(cleanPath, 0o644); err != nil { //nolint:gosec // G302: system config needs to be readable
		return fmt.Errorf("cannot set config file permissions: %w", err)
	}

	return nil
}
