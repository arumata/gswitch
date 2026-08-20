package tray

import (
	"bufio"
	"os"
	"slices"
	"strconv"
	"strings"
)

// ConfigFile is the path to the gswitch configuration file.
const ConfigFile = "/etc/gswitch/default.conf"

// TrayConfig holds the configuration values relevant to the tray application.
type TrayConfig struct {
	LayoutSwitch      string   // "125" or "29+42"
	ConvertKey        string   // "0" for double-shift, or key code
	Layout1           string   // "us"
	Layout2           string   // "ru"
	Delay             int      // 10
	LayoutSwitchDelay int      // 100
	Blacklist         []string // Device UIDs to ignore
}

// DefaultTrayConfig returns a config with default values.
func DefaultTrayConfig() *TrayConfig {
	return &TrayConfig{
		LayoutSwitch:      "auto",
		ConvertKey:        "0",
		Layout1:           "us",
		Layout2:           "ru",
		Delay:             10,
		LayoutSwitchDelay: 100,
		Blacklist:         nil,
	}
}

// LoadTrayConfig reads configuration from the config file.
// Returns default config if file doesn't exist or can't be read.
func LoadTrayConfig() *TrayConfig {
	cfg := DefaultTrayConfig()

	file, err := os.Open(ConfigFile)
	if err != nil {
		return cfg
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
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
			cfg.LayoutSwitch = value
		case "convert-key", "replace-key":
			cfg.ConvertKey = value
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
				cfg.Layout1 = value
			}
		case "layout2":
			if value != "" {
				cfg.Layout2 = value
			}
		case "blacklist":
			if value != "" {
				// Parse comma or semicolon-separated UIDs
				// Replace semicolons with commas for uniform parsing
				value = strings.ReplaceAll(value, ";", ",")
				uids := strings.SplitSeq(value, ",")
				for uid := range uids {
					uid = strings.TrimSpace(uid)
					if uid != "" {
						cfg.Blacklist = append(cfg.Blacklist, uid)
					}
				}
			}
		}
	}

	return cfg
}

// IsBlacklisted checks if a device UID is in the blacklist.
func (c *TrayConfig) IsBlacklisted(uid string) bool {
	return slices.Contains(c.Blacklist, uid)
}
