package gswitch

import cfg "github.com/arumata/gswitch/internal/gswitch/config"

type Config = cfg.Config
type LayoutSpec = cfg.LayoutSpec

func LoadConfig() (*Config, error) {
	return cfg.LoadConfigFrom(cfg.DefaultConfigPath)
}

func SaveConfig(config *Config) error {
	return cfg.SaveConfigTo(cfg.DefaultConfigPath, config)
}

func SaveConfigTo(path string, config *Config) error {
	return cfg.SaveConfigTo(path, config)
}

func writeConfigFromArgs(args string) error {
	return cfg.WriteConfigFromArgsTo(cfg.DefaultConfigPath, args)
}

func writeConfigFromArgsTo(path, args string) error {
	return cfg.WriteConfigFromArgsTo(path, args)
}

func parseLayoutSwitchKey(value string) ([]uint16, bool) {
	return cfg.ParseLayoutSwitchKey(value)
}

func formatLayoutsSection(layouts []LayoutSpec) string {
	return cfg.FormatLayoutsSection(layouts)
}

func formatLayoutSpec(layout LayoutSpec) string {
	return cfg.FormatLayoutSpec(layout)
}

func splitLayoutVariant(s string) (layout, variant string) {
	return cfg.SplitLayoutVariant(s)
}
