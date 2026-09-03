package detect

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/jezek/xgb"
	"github.com/jezek/xgb/xproto"
)

// ========== Detection Types and Provider Interface (AD2-01) ==========

// DetectionSource identifies the source/provider of layout switch detection.
type DetectionSource string

const (
	// SourceAuto means automatic detection using default providers.
	SourceAuto DetectionSource = "auto"
	// SourceXKB detects layout switch from XKB options (setxkbmap, X11 property, gsettings xkb-options).
	SourceXKB DetectionSource = "xkb"
	// SourceGNOME detects layout switch from GNOME WM keybindings (switch-input-source).
	SourceGNOME DetectionSource = "gnome"
	// SourceKDE detects layout switch from KDE Plasma kglobalshortcutsrc.
	SourceKDE DetectionSource = "kde"
	// SourceIBus detects layout switch from IBus hotkey configuration.
	SourceIBus DetectionSource = "ibus"
	// SourceFcitx5 detects layout switch from Fcitx5 configuration.
	SourceFcitx5 DetectionSource = "fcitx5"
	// SourceSession is an internal source for session-level detection (not a provider).
	SourceSession DetectionSource = "session"
)

// AttemptStatus represents the outcome of a detection attempt.
type AttemptStatus string

const (
	// StatusFound means the configuration was found and successfully parsed.
	StatusFound AttemptStatus = "found"
	// StatusNotFound means the provider ran successfully but no configuration was set.
	StatusNotFound AttemptStatus = "not_found"
	// StatusInactive means the provider is not applicable (e.g., daemon not running).
	StatusInactive AttemptStatus = "inactive"
	// StatusError means an error occurred during detection (e.g., access denied).
	StatusError AttemptStatus = "error"
	// StatusUnsupported means the configuration was found but format is unknown.
	StatusUnsupported AttemptStatus = "unsupported"
)

// DetectionAttempt represents a single detection attempt by a provider.
type DetectionAttempt struct {
	// Provider is the source that made this attempt.
	Provider DetectionSource
	// Status is the outcome of the attempt.
	Status AttemptStatus
	// RawValue is the raw configuration value found (e.g., "grp:alt_shift_toggle").
	// May be set even on error/unsupported status for diagnostics.
	RawValue string
	// KeyNames is the human-readable key combination (e.g., "Alt+Shift").
	KeyNames string
	// Scancodes are the detected scancodes (only set if Status == StatusFound).
	Scancodes []uint16
	// Error contains the error/diagnostic message for StatusError or StatusUnsupported.
	Error string
}

// DetectionContext provides context about the detection environment.
type DetectionContext struct {
	// DE is the detected desktop environment (e.g., "gnome", "kde", "xfce").
	DE string
	// SessionType is the session type ("x11" or "wayland").
	SessionType string
	// ProviderOrder is the order in which providers were tried.
	ProviderOrder []DetectionSource
}

// DetectionResult contains the full result of layout switch detection.
type DetectionResult struct {
	// Scancodes are the detected layout switch scancodes.
	Scancodes []uint16
	// Source is the provider that successfully detected the configuration.
	Source DetectionSource
	// RawValue is the raw configuration value (e.g., "grp:alt_shift_toggle").
	RawValue string
	// KeyNames is the human-readable key combination (e.g., "Alt+Shift").
	KeyNames string
	// Warning is an optional warning message (e.g., "Win+Space intercepted by GNOME Shell").
	Warning string
	// Attempts contains all detection attempts made (for diagnostics).
	Attempts []DetectionAttempt
	// Context provides environment context for diagnostics.
	Context DetectionContext
}

// Provider is the interface for layout switch detection providers.
type Provider interface {
	// Name returns the identifier of this provider.
	Name() DetectionSource
	// Detect attempts to detect layout switch configuration.
	// env may be nil when running as regular user (uses current environment).
	Detect(env *SessionEnv) DetectionAttempt
}

// DetectionOptions contains options for layout switch detection.
type DetectionOptions struct {
	// SourceOverride forces using only the specified provider.
	// Empty string or "auto" means automatic detection.
	SourceOverride DetectionSource
}

// scancodeToKeyName maps scancodes to human-readable key names.
// Includes modifiers, special keys, letters A-Z, and numbers 0-9.
var scancodeToKeyName = map[uint16]string{
	// Modifiers
	scancodeLeftCtrl:   "Ctrl",
	scancodeLeftShift:  "Shift",
	scancodeRightShift: "RShift",
	scancodeLeftAlt:    "Alt",
	scancodeRightAlt:   "RAlt",
	scancodeRightCtrl:  "RCtrl",
	scancodeLeftMeta:   "Super",
	scancodeRightMeta:  "RSuper",
	// Special keys
	scancodeSpace:      "Space",
	scancodeCapsLock:   "CapsLock",
	scancodeScrollLock: "ScrollLock",
	scancodeCompose:    "Menu",
	scancodeF16:        "Launch7",
	scancodeKeyboard:   "Keyboard",
	15:                 "Tab",
	28:                 "Enter",
	14:                 "Backspace",
	1:                  "Escape",
	// Letters A-Z (scancodes from kdeKeyToScancode)
	30: "A", 48: "B", 46: "C", 32: "D", 18: "E", 33: "F", 34: "G", 35: "H",
	23: "I", 36: "J", 37: "K", 38: "L", 50: "M", 49: "N", 24: "O", 25: "P",
	16: "Q", 19: "R", 31: "S", 20: "T", 22: "U", 47: "V", 17: "W", 45: "X",
	21: "Y", 44: "Z",
	// Numbers 0-9
	11: "0", 2: "1", 3: "2", 4: "3", 5: "4", 6: "5", 7: "6", 8: "7", 9: "8", 10: "9",
}

// ScancodesToKeyNames converts scancodes to a human-readable representation.
// Example: [56, 42] -> "Alt+Shift"
func ScancodesToKeyNames(scancodes []uint16) string {
	if len(scancodes) == 0 {
		return ""
	}

	names := make([]string, 0, len(scancodes))
	for _, sc := range scancodes {
		if name, ok := scancodeToKeyName[sc]; ok {
			names = append(names, name)
		} else {
			names = append(names, fmt.Sprintf("Key%d", sc))
		}
	}
	return strings.Join(names, "+")
}

// appendWarning combines two warning strings, separating with "; " if both non-empty.
func appendWarning(base, addition string) string {
	if base == "" {
		return addition
	}
	if addition == "" {
		return base
	}
	return base + "; " + addition
}

// Scancode constants for layout switch keys.
// These correspond to Linux input event codes (evdev scancodes).
const (
	scancodeLeftCtrl   uint16 = 29
	scancodeLeftShift  uint16 = 42
	scancodeRightShift uint16 = 54
	scancodeLeftAlt    uint16 = 56
	scancodeSpace      uint16 = 57
	scancodeCapsLock   uint16 = 58
	scancodeScrollLock uint16 = 70
	scancodeRightCtrl  uint16 = 97
	scancodeRightAlt   uint16 = 100
	scancodeLeftMeta   uint16 = 125
	scancodeRightMeta  uint16 = 126
	scancodeCompose    uint16 = 127 // Menu key
	scancodeF16        uint16 = 186
	scancodeKeyboard   uint16 = 374 // KEY_KEYBOARD / XF86Keyboard
)

// xkbToScancodes maps XKB grp:* options to their corresponding scancodes.
// Only layout group switching options (grp:*) are supported.
var xkbToScancodes = map[string][]uint16{
	"grp:shift_caps_toggle": {scancodeLeftShift, scancodeCapsLock},
	"grp:caps_toggle":       {scancodeCapsLock},
	"grp:ctrl_shift_toggle": {scancodeLeftCtrl, scancodeLeftShift},
	"grp:alt_shift_toggle":  {scancodeLeftAlt, scancodeLeftShift},
	"grp:shifts_toggle":     {scancodeLeftShift, scancodeRightShift},
	"grp:win_space_toggle":  {scancodeLeftMeta, scancodeSpace},
	"grp:alt_space_toggle":  {scancodeLeftAlt, scancodeSpace},
	"grp:lwin_toggle":       {scancodeLeftMeta},
	"grp:rwin_toggle":       {scancodeRightMeta},
	"grp:sclk_toggle":       {scancodeScrollLock},
	"grp:menu_toggle":       {scancodeCompose},
	"grp:lctrl_toggle":      {scancodeLeftCtrl},
	"grp:rctrl_toggle":      {scancodeRightCtrl},
	"grp:lshift_toggle":     {scancodeLeftShift},
	"grp:rshift_toggle":     {scancodeRightShift},
}

// ErrNoLayoutSwitchOption is returned when no grp:* option is found in XKB options.
var ErrNoLayoutSwitchOption = errors.New("no layout switch option (grp:*) found in XKB options")

// ErrKDEConfigPathUnknown is returned when KDE config path cannot be determined.
var ErrKDEConfigPathUnknown = errors.New("cannot determine KDE config path")

// ErrKDENoShortcutConfigured is returned when KDE keyboard layout shortcut is not configured.
var ErrKDENoShortcutConfigured = errors.New("no KDE keyboard layout shortcut configured")

// kdeModifierToScancode maps KDE modifier names to scancodes.
var kdeModifierToScancode = map[string]uint16{
	"Meta":  scancodeLeftMeta,
	"Super": scancodeLeftMeta,
	"Alt":   scancodeLeftAlt,
	"Shift": scancodeLeftShift,
	"Ctrl":  scancodeLeftCtrl,
}

// kdeKeyToScancode maps KDE key names to scancodes.
// This includes letters, numbers, and special keys.
var kdeKeyToScancode = map[string]uint16{
	// Letters (A-Z)
	"A": 30, "B": 48, "C": 46, "D": 32, "E": 18, "F": 33, "G": 34, "H": 35,
	"I": 23, "J": 36, "K": 37, "L": 38, "M": 50, "N": 49, "O": 24, "P": 25,
	"Q": 16, "R": 19, "S": 31, "T": 20, "U": 22, "V": 47, "W": 17, "X": 45,
	"Y": 21, "Z": 44,
	// Numbers (0-9)
	"0": 11, "1": 2, "2": 3, "3": 4, "4": 5, "5": 6, "6": 7, "7": 8, "8": 9, "9": 10,
	// Special keys
	"Space":      scancodeSpace,
	"Tab":        15,
	"Return":     28,
	"Enter":      28,
	"Backspace":  14,
	"Escape":     1,
	"CapsLock":   scancodeCapsLock,
	"ScrollLock": scancodeScrollLock,
	"Menu":       scancodeCompose,
}

// getDefaultProviders returns the base list of providers for layout-switch=auto.
// Note: This function returns only XKB. The actual provider order is determined
// dynamically in DetectLayoutSwitchKeys() based on DE detection (isGNOME).
// KDE provider is NOT included by default because KDE shortcut (Meta+Alt+K)
// shows OSD but doesn't switch layout. KDE is available via opt-in with SourceOverride="kde".
func getDefaultProviders() []Provider {
	return []Provider{
		&xkbProvider{},
		// KDE provider is opt-in only via SourceOverride
		// GNOME provider is added dynamically in DetectLayoutSwitchKeys() based on isGNOME()
	}
}

// getProviderBySource returns a provider for explicit source selection.
// Returns nil for unknown sources.
//
//nolint:ireturn // Runtime selection requires the common provider interface.
func getProviderBySource(source DetectionSource) Provider {
	switch source {
	case SourceXKB:
		return &xkbProvider{}
	case SourceKDE:
		return &kdeProvider{}
	case SourceGNOME:
		return &gnomeProvider{}
	default:
		return nil
	}
}

// DetectLayoutSwitchKeys detects the keyboard layout switch keys from system configuration.
//
// If opts.SourceOverride is set (e.g., "kde"), only that provider is used.
// Otherwise, providers are tried in DE-dependent order:
// - On GNOME: GNOME provider first, then XKB
// - On other DEs: XKB first, then GNOME
// KDE is NOT included by default (opt-in via SourceOverride).
//
// opts may be nil for default behavior.
//
// Returns:
//   - *DetectionResult: full detection result with scancodes, source, and diagnostics
//   - error: ErrNoLayoutSwitchOption if no layout switch option is found, or other error
//
// Note: If grp:win_space_toggle is detected, a warning is included in the result
// because Win+Space is intercepted by GNOME Shell and may not work correctly.
func DetectLayoutSwitchKeys(opts *DetectionOptions) (*DetectionResult, error) {
	// Get session environment when running as root
	var env *SessionEnv
	var sessionWarning string
	if os.Getuid() == 0 {
		var sessionErr error
		env, sessionErr = GetActiveSessionEnv()
		if sessionErr != nil {
			sessionWarning = fmt.Sprintf("session env unavailable: %v (DE detection may be inaccurate)", sessionErr)
		}
	}

	result := &DetectionResult{
		Attempts: make([]DetectionAttempt, 0, 3),
	}

	// Detect DE and fill context
	// Use isGNOME for provider ordering even when session env failed (falls back to os.Getenv)
	gnomeDetected := isGNOME(env)
	switch {
	case os.Getuid() == 0 && sessionWarning != "":
		// Under root with failed session detection - mark as unknown for diagnostics
		result.Context.DE = "unknown"
	case gnomeDetected:
		result.Context.DE = "gnome"
	default:
		result.Context.DE = "other"
	}
	if env != nil {
		result.Context.SessionType = env.SessionType
	}

	// If explicit source override is specified (not empty and not "auto"), use only that provider
	if opts != nil && opts.SourceOverride != "" && opts.SourceOverride != SourceAuto {
		provider := getProviderBySource(opts.SourceOverride)
		if provider == nil {
			return nil, fmt.Errorf("unknown source: %s", opts.SourceOverride)
		}

		result.Context.ProviderOrder = []DetectionSource{provider.Name()}
		attempt := provider.Detect(env)
		result.Attempts = append(result.Attempts, attempt)

		if attempt.Status == StatusFound {
			result.Scancodes = attempt.Scancodes
			result.Source = attempt.Provider
			result.RawValue = attempt.RawValue
			result.KeyNames = attempt.KeyNames

			// Warn about Win+Space being intercepted by GNOME Shell
			if attempt.RawValue == "grp:win_space_toggle" {
				result.Warning = "Win+Space (grp:win_space_toggle) is intercepted by GNOME Shell and may not work for layout conversion"
			}
			result.Warning = appendWarning(result.Warning, sessionWarning)

			return result, nil
		}

		// Return more specific error for override mode
		result.Warning = appendWarning(result.Warning, sessionWarning)
		switch attempt.Status {
		case StatusError:
			return result, fmt.Errorf("%s provider error: %s", provider.Name(), attempt.Error)
		case StatusUnsupported:
			return result, fmt.Errorf("%s provider: unsupported configuration: %s", provider.Name(), attempt.Error)
		default:
			// StatusNotFound, StatusInactive
			return result, ErrNoLayoutSwitchOption
		}
	}

	// Provider order depends on DE (Task 07)
	var providers []Provider
	if gnomeDetected {
		// On GNOME: WM keybinding is the source of truth
		providers = []Provider{&gnomeProvider{}, &xkbProvider{}}
		result.Context.ProviderOrder = []DetectionSource{SourceGNOME, SourceXKB}
	} else {
		// On other DEs: XKB is usually reliable
		providers = []Provider{&xkbProvider{}, &gnomeProvider{}}
		result.Context.ProviderOrder = []DetectionSource{SourceXKB, SourceGNOME}
	}
	// KDE is NOT included by default (opt-in via SourceOverride)

	// Try each provider in order
	for _, provider := range providers {
		attempt := provider.Detect(env)
		result.Attempts = append(result.Attempts, attempt)

		if attempt.Status == StatusFound {
			result.Scancodes = attempt.Scancodes
			result.Source = attempt.Provider
			result.RawValue = attempt.RawValue
			result.KeyNames = attempt.KeyNames

			// Warn about Win+Space being intercepted by GNOME Shell
			if attempt.RawValue == "grp:win_space_toggle" {
				result.Warning = "Win+Space (grp:win_space_toggle) is intercepted by GNOME Shell and may not work for layout conversion"
			}
			result.Warning = appendWarning(result.Warning, sessionWarning)

			return result, nil
		}
	}

	result.Warning = appendWarning(result.Warning, sessionWarning)
	return result, ErrNoLayoutSwitchOption
}

// DetectLayoutSwitchScancodes is a compatibility wrapper for DetectLayoutSwitchKeys.
// It returns only the scancodes for use in existing code.
func DetectLayoutSwitchScancodes() ([]uint16, error) {
	result, err := DetectLayoutSwitchKeys(nil)
	if err != nil {
		return nil, err
	}
	return result.Scancodes, nil
}

// detectLayoutSwitchKeysFromOptions is the internal implementation that takes options as parameter.
// This allows for easier testing without mocking GetXKBOptions.
func detectLayoutSwitchKeysFromOptions(options []string) ([]uint16, error) {
	// Find the first grp:* option
	for _, opt := range options {
		if !strings.HasPrefix(opt, "grp:") {
			continue
		}

		scancodes, ok := xkbToScancodes[opt]
		if !ok {
			return nil, fmt.Errorf("unsupported XKB option: %s", opt)
		}

		// Note: Win+Space warning is handled in DetectLayoutSwitchKeys() via result.Warning
		return scancodes, nil
	}

	return nil, ErrNoLayoutSwitchOption
}

// GetXKBOptions returns the currently configured XKB options from multiple sources.
// It tries sources in order of priority:
// 1. setxkbmap -query (main source)
// 2. X11 property _XKB_RULES_NAMES (fallback)
// 3. gsettings for GNOME (fallback)
// 4. /etc/default/keyboard (fallback for system defaults)
//
// Returns an empty slice if no options are configured.
func GetXKBOptions() ([]string, error) {
	// Try setxkbmap first (most reliable when DISPLAY is available)
	if opts, err := getXKBOptionsFromSetxkbmap(); err == nil && len(opts) > 0 {
		return opts, nil
	}

	// Fallback to X11 property
	if opts, err := getXKBOptionsFromX11Property(); err == nil && len(opts) > 0 {
		return opts, nil
	}

	// Fallback to gsettings (GNOME)
	if opts, err := getXKBOptionsFromGsettings(); err == nil && len(opts) > 0 {
		return opts, nil
	}

	// Final fallback to /etc/default/keyboard
	if opts, err := getXKBOptionsFromDefaultKeyboard(); err == nil && len(opts) > 0 {
		return opts, nil
	}

	// No options found - return empty slice (not an error, just no options configured)
	return []string{}, nil
}

// getXKBOptionsFromSetxkbmap parses XKB options from setxkbmap -query output.
// Expected output format:
//
//	rules:      evdev
//	model:      pc105
//	layout:     us,ru
//	variant:    ,
//	options:    grp:ctrl_shift_toggle,caps:escape
func getXKBOptionsFromSetxkbmap() ([]string, error) {
	cmd := exec.Command("setxkbmap", "-query")
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	return parseSetxkbmapOutput(string(output)), nil
}

// parseSetxkbmapOutput parses the output of setxkbmap -query and returns XKB options.
func parseSetxkbmapOutput(output string) []string {
	for line := range strings.SplitSeq(output, "\n") {
		line = strings.TrimSpace(line)
		if after, ok := strings.CutPrefix(line, "options:"); ok {
			optionsStr := strings.TrimSpace(after)
			if optionsStr == "" {
				return []string{}
			}
			return splitOptions(optionsStr)
		}
	}
	return []string{}
}

// getXKBOptionsFromX11Property reads XKB options from X11 property _XKB_RULES_NAMES.
// The property contains 5 null-separated strings: rules, model, layout, variant, options.
func getXKBOptionsFromX11Property() ([]string, error) {
	conn, err := xgb.NewConn()
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	setup := xproto.Setup(conn)
	root := setup.DefaultScreen(conn).Root

	atomName := "_XKB_RULES_NAMES"
	// #nosec G115 -- atomName is a constant string, length fits in uint16
	atomCookie := xproto.InternAtom(conn, false, uint16(len(atomName)), atomName)
	atomReply, err := atomCookie.Reply()
	if err != nil {
		return nil, err
	}

	propCookie := xproto.GetProperty(conn, false, root, atomReply.Atom, xproto.AtomString, 0, 1024)
	propReply, err := propCookie.Reply()
	if err != nil {
		return nil, err
	}

	if propReply.ValueLen == 0 {
		return []string{}, nil
	}

	return parseXKBRulesNames(propReply.Value), nil
}

// parseXKBRulesNames parses the _XKB_RULES_NAMES property value.
// The value contains 5 null-separated strings: rules, model, layout, variant, options.
func parseXKBRulesNames(data []byte) []string {
	// Split by null bytes
	parts := strings.Split(string(data), "\x00")

	// Options is the 5th element (index 4)
	if len(parts) < 5 {
		return []string{}
	}

	optionsStr := strings.TrimSpace(parts[4])
	if optionsStr == "" {
		return []string{}
	}

	return splitOptions(optionsStr)
}

// getXKBOptionsFromGsettings reads XKB options from GNOME gsettings.
// Expected output format: ['grp:ctrl_shift_toggle', 'caps:escape'] or @as []
func getXKBOptionsFromGsettings() ([]string, error) {
	cmd := exec.Command("gsettings", "get", "org.gnome.desktop.input-sources", "xkb-options")
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	return parseGsettingsOutput(string(output)), nil
}

// parseGsettingsOutput parses gsettings output for xkb-options.
// Format: ['grp:ctrl_shift_toggle', 'caps:escape'] or @as []
func parseGsettingsOutput(output string) []string {
	output = strings.TrimSpace(output)

	// Empty array cases
	if output == "[]" || output == "@as []" {
		return []string{}
	}

	var options []string
	// Parse array elements between single quotes
	for p := range strings.SplitSeq(output, "'") {
		p = strings.TrimSpace(p)
		// Skip brackets, commas, and empty strings
		if p == "" || p == "[" || p == "]" || p == "," || p == ", " {
			continue
		}
		// Skip items that start with bracket/comma (array separators)
		if strings.HasPrefix(p, "[") || strings.HasPrefix(p, ",") || strings.HasPrefix(p, "]") {
			continue
		}
		// Valid option
		if strings.Contains(p, ":") {
			options = append(options, p)
		}
	}

	return options
}

// getXKBOptionsFromDefaultKeyboard reads XKB options from /etc/default/keyboard.
// Format: XKBOPTIONS="grp:ctrl_shift_toggle,caps:escape"
func getXKBOptionsFromDefaultKeyboard() ([]string, error) {
	return parseDefaultKeyboardFile("/etc/default/keyboard")
}

// parseDefaultKeyboardFile parses /etc/default/keyboard file and returns XKBOPTIONS.
func parseDefaultKeyboardFile(filePath string) ([]string, error) {
	// #nosec G304 -- filePath is either a constant or test file path
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if after, ok := strings.CutPrefix(line, "XKBOPTIONS="); ok {
			// Remove quotes
			optionsStr := strings.Trim(after, "\"'")
			if optionsStr == "" {
				return []string{}, nil
			}
			return splitOptions(optionsStr), nil
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return []string{}, nil
}

// splitOptions splits a comma-separated options string into a slice.
func splitOptions(optionsStr string) []string {
	var options []string
	for opt := range strings.SplitSeq(optionsStr, ",") {
		opt = strings.TrimSpace(opt)
		if opt != "" {
			options = append(options, opt)
		}
	}
	return options
}

// ========== KDE Plasma Support ==========

// getKDEConfigPath returns the path to KDE's kglobalshortcutsrc file.
// When running as root via sudo/pkexec, it uses $SUDO_USER to find the original user's config.
func getKDEConfigPath() string {
	// Check for SUDO_USER first (running via sudo/pkexec)
	if sudoUser := os.Getenv("SUDO_USER"); sudoUser != "" {
		home := "/home"
		return filepath.Join(home, sudoUser, ".config", "kglobalshortcutsrc")
	}
	// Fallback to current user's HOME
	if home := os.Getenv("HOME"); home != "" {
		return filepath.Join(home, ".config", "kglobalshortcutsrc")
	}
	return ""
}

// DetectKDELayoutSwitchKeys detects layout switch keys from KDE Plasma configuration.
// It reads ~/.config/kglobalshortcutsrc and parses the keyboard layout switcher shortcut.
func DetectKDELayoutSwitchKeys() ([]uint16, string, error) {
	configPath := getKDEConfigPath()
	if configPath == "" {
		return nil, "", ErrKDEConfigPathUnknown
	}

	shortcut, err := parseKDEGlobalShortcuts(configPath)
	if err != nil {
		return nil, "", err
	}

	if shortcut == "" || shortcut == "none" {
		return nil, "", ErrKDENoShortcutConfigured
	}

	scancodes, err := parseKDEShortcutToScancodes(shortcut)
	if err != nil {
		return nil, shortcut, fmt.Errorf("failed to parse KDE shortcut %q: %w", shortcut, err)
	}

	return scancodes, shortcut, nil
}

// parseKDEGlobalShortcuts parses ~/.config/kglobalshortcutsrc and returns
// the keyboard layout switch shortcut.
// Returns empty string if not found.
func parseKDEGlobalShortcuts(filePath string) (string, error) {
	return parseKDEGlobalShortcutsFile(filePath)
}

// parseKDEGlobalShortcutsFile parses the KDE kglobalshortcutsrc file.
// Format is INI-like:
//
//	[KDE Keyboard Layout Switcher]
//	Switch to Next Keyboard Layout=Meta+Alt+K,none,Switch to Next Keyboard Layout
//
// The value format is: current_shortcut,default_shortcut,description
func parseKDEGlobalShortcutsFile(filePath string) (string, error) {
	// #nosec G304 -- filePath is either from getKDEConfigPath or test file path
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	inSection := false

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Check for section header
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section := line[1 : len(line)-1]
			inSection = section == "KDE Keyboard Layout Switcher"
			continue
		}

		// Parse key in the right section
		if inSection {
			if after, ok := strings.CutPrefix(line, "Switch to Next Keyboard Layout="); ok {
				// Value format: current_shortcut,default_shortcut,description
				// We only need the first part (current shortcut)
				parts := strings.SplitN(after, ",", 2)
				if len(parts) > 0 {
					return strings.TrimSpace(parts[0]), nil
				}
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return "", err
	}

	return "", nil
}

// parseKDEShortcutToScancodes converts a KDE shortcut string to scancodes.
// Examples:
//   - "Meta+Alt+K" -> [125, 56, 37]
//   - "Alt+Shift" -> [56, 42]
//   - "Ctrl+Shift" -> [29, 42]
func parseKDEShortcutToScancodes(shortcut string) ([]uint16, error) {
	if shortcut == "" || shortcut == "none" {
		return nil, errors.New("empty shortcut")
	}

	parts := strings.Split(shortcut, "+")
	if len(parts) == 0 {
		return nil, errors.New("invalid shortcut format")
	}

	var scancodes []uint16
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		// Check if it's a modifier
		if scancode, ok := kdeModifierToScancode[part]; ok {
			scancodes = append(scancodes, scancode)
			continue
		}

		// Check if it's a regular key
		if scancode, ok := kdeKeyToScancode[part]; ok {
			scancodes = append(scancodes, scancode)
			continue
		}

		// Unknown key
		return nil, fmt.Errorf("unknown key: %s", part)
	}

	if len(scancodes) == 0 {
		return nil, errors.New("no valid keys in shortcut")
	}

	return scancodes, nil
}

// ========== Provider Implementations (AD2-00) ==========

// xkbProvider implements Provider interface for XKB-based layout switch detection.
// It tries multiple sub-sources in order:
// 1. setxkbmap -query (X11)
// 2. X11 property _XKB_RULES_NAMES (via xgb)
// 3. gsettings org.gnome.desktop.input-sources xkb-options (GNOME)
// 4. /etc/default/keyboard (file)
type xkbProvider struct{}

// Name returns the identifier of this provider.
func (p *xkbProvider) Name() DetectionSource {
	return SourceXKB
}

// Detect attempts to detect layout switch configuration from XKB options.
// env may be nil when running as regular user (uses current environment).
func (p *xkbProvider) Detect(env *SessionEnv) DetectionAttempt {
	attempt := DetectionAttempt{Provider: SourceXKB}

	// Get XKB options using environment-aware method
	options, err := p.getXKBOptionsWithEnv(env)
	if err != nil {
		// Real error accessing XKB sources
		attempt.Status = StatusError
		attempt.Error = err.Error()
		return attempt
	}

	if len(options) == 0 {
		attempt.Status = StatusNotFound
		return attempt
	}

	// Find the grp:* option for RawValue
	var rawValue string
	for _, opt := range options {
		if strings.HasPrefix(opt, "grp:") {
			rawValue = opt
			break
		}
	}

	if rawValue == "" {
		attempt.Status = StatusNotFound
		return attempt
	}

	// Try to convert to scancodes
	scancodes, detectErr := detectLayoutSwitchKeysFromOptions(options)
	if detectErr != nil {
		if errors.Is(detectErr, ErrNoLayoutSwitchOption) {
			attempt.Status = StatusNotFound
		} else {
			// Unknown grp:* option
			attempt.Status = StatusUnsupported
			attempt.RawValue = rawValue
			attempt.Error = detectErr.Error()
		}
		return attempt
	}

	attempt.Status = StatusFound
	attempt.RawValue = rawValue
	attempt.Scancodes = scancodes
	attempt.KeyNames = ScancodesToKeyNames(scancodes)
	return attempt
}

// getXKBOptionsWithEnv returns XKB options using environment-aware methods.
// It tries sources in order, skipping unavailable ones without error.
func (p *xkbProvider) getXKBOptionsWithEnv(env *SessionEnv) ([]string, error) {
	// Apply session environment if running as root with env
	if env != nil && os.Geteuid() == 0 {
		if err := ApplySessionEnv(env); err != nil {
			return nil, fmt.Errorf("failed to apply session env: %w", err)
		}
	}

	// 1. Try setxkbmap first (most reliable when DISPLAY is available)
	if _, err := exec.LookPath("setxkbmap"); err == nil {
		if opts, err := getXKBOptionsFromSetxkbmap(); err == nil && len(opts) > 0 {
			return opts, nil
		}
	}

	// 2. Fallback to X11 property (uses xgb, needs DISPLAY)
	if opts, err := getXKBOptionsFromX11Property(); err == nil && len(opts) > 0 {
		return opts, nil
	}

	// 3. KDE stores XKB options in kxkbrc (works on both X11 and Wayland,
	// where setxkbmap and X11 properties are unavailable)
	if opts, err := getXKBOptionsFromKxkbrc(env); err == nil && len(opts) > 0 {
		return opts, nil
	}

	// 4. Fallback to gsettings (GNOME) - use RunGsettings for proper user context
	if _, err := exec.LookPath("gsettings"); err == nil {
		if opts, err := p.getXKBOptionsFromGsettingsWithEnv(env); err == nil && len(opts) > 0 {
			return opts, nil
		}
	}

	// 4. Final fallback to /etc/default/keyboard
	if opts, err := getXKBOptionsFromDefaultKeyboard(); err == nil && len(opts) > 0 {
		return opts, nil
	}

	// No options found - return empty slice (not an error)
	return []string{}, nil
}

// getXKBOptionsFromKxkbrc reads XKB options from KDE's kxkbrc config.
func getXKBOptionsFromKxkbrc(env *SessionEnv) ([]string, error) {
	configDir := resolveConfigDir(env)
	if configDir == "" {
		return nil, errors.New("cannot determine config directory")
	}

	data, err := os.ReadFile(filepath.Join(configDir, "kxkbrc")) //nolint:gosec // path built from session config dir
	if err != nil {
		return nil, err
	}
	return parseKxkbrcXKBOptions(string(data)), nil
}

// parseKxkbrcXKBOptions parses the Options key of the [Layout] section:
//
//	[Layout]
//	Options=grp:caps_toggle,terminate:ctrl_alt_bksp
func parseKxkbrcXKBOptions(content string) []string {
	inLayout := false
	for line := range strings.SplitSeq(content, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "[") {
			inLayout = line == "[Layout]"
			continue
		}
		if !inLayout {
			continue
		}
		if after, ok := strings.CutPrefix(line, "Options="); ok {
			return splitOptions(after)
		}
	}
	return nil
}

// getXKBOptionsFromGsettingsWithEnv reads XKB options from GNOME gsettings using session env.
func (p *xkbProvider) getXKBOptionsFromGsettingsWithEnv(env *SessionEnv) ([]string, error) {
	output, err := RunGsettings(env, "get", "org.gnome.desktop.input-sources", "xkb-options")
	if err != nil {
		return nil, err
	}
	return parseGsettingsOutput(string(output)), nil
}

// kdeProvider implements Provider interface for KDE Plasma layout switch detection.
// It reads ~/.config/kglobalshortcutsrc for the keyboard layout switcher shortcut.
type kdeProvider struct{}

// Name returns the identifier of this provider.
func (p *kdeProvider) Name() DetectionSource {
	return SourceKDE
}

// Detect attempts to detect layout switch configuration from KDE Plasma.
// env may be nil when running as regular user (uses current environment).
func (p *kdeProvider) Detect(env *SessionEnv) DetectionAttempt {
	attempt := DetectionAttempt{Provider: SourceKDE}

	configPath := getKDEConfigPathWithEnv(env)
	if configPath == "" {
		attempt.Status = StatusInactive
		attempt.Error = ErrKDEConfigPathUnknown.Error()
		return attempt
	}

	shortcut, err := parseKDEGlobalShortcuts(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			attempt.Status = StatusInactive
		} else {
			attempt.Status = StatusError
			attempt.Error = err.Error()
		}
		return attempt
	}

	if shortcut == "" || shortcut == "none" {
		attempt.Status = StatusNotFound
		return attempt
	}

	scancodes, err := parseKDEShortcutToScancodes(shortcut)
	if err != nil {
		attempt.Status = StatusUnsupported
		attempt.RawValue = shortcut
		attempt.Error = err.Error()
		return attempt
	}

	attempt.Status = StatusFound
	attempt.RawValue = shortcut
	attempt.Scancodes = scancodes
	attempt.KeyNames = ScancodesToKeyNames(scancodes)
	return attempt
}

// getKDEConfigPathWithEnv returns the path to KDE's kglobalshortcutsrc file.
// Logic:
// 1. If env != nil → use env.XDGConfigHome (if absolute) or env.Home
// 2. If env == nil → use os.Getenv("XDG_CONFIG_HOME") or os.UserHomeDir()
// 3. Fallback to ~/.config if XDG_CONFIG_HOME is empty or relative
func getKDEConfigPathWithEnv(env *SessionEnv) string {
	configDir := resolveConfigDir(env)
	if configDir == "" {
		return ""
	}
	return filepath.Join(configDir, "kglobalshortcutsrc")
}

// resolveConfigDir determines the config directory based on SessionEnv or environment.
func resolveConfigDir(env *SessionEnv) string {
	if env != nil {
		return resolveConfigDirFromSessionEnv(env)
	}
	return resolveConfigDirFromOSEnv()
}

// resolveConfigDirFromSessionEnv gets config dir from SessionEnv.
func resolveConfigDirFromSessionEnv(env *SessionEnv) string {
	if env.XDGConfigHome != "" && filepath.IsAbs(env.XDGConfigHome) {
		return env.XDGConfigHome
	}
	if env.Home != "" {
		return filepath.Join(env.Home, ".config")
	}
	return ""
}

// resolveConfigDirFromOSEnv gets config dir from OS environment variables.
func resolveConfigDirFromOSEnv() string {
	if xdgConfig := os.Getenv("XDG_CONFIG_HOME"); xdgConfig != "" && filepath.IsAbs(xdgConfig) {
		return xdgConfig
	}
	// Try SUDO_USER for root context
	if sudoUser := os.Getenv("SUDO_USER"); sudoUser != "" {
		return "/home/" + sudoUser + "/.config"
	}
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, ".config")
	}
	return ""
}

// NewXKBProvider creates a new XKB provider instance.
//
//nolint:ireturn // Public factory exposes provider behavior, not implementation.
func NewXKBProvider() Provider {
	return &xkbProvider{}
}

// NewKDEProvider creates a new KDE provider instance.
//
//nolint:ireturn // Public factory exposes provider behavior, not implementation.
func NewKDEProvider() Provider {
	return &kdeProvider{}
}

// ========== GNOME Provider (Task 06) ==========

// gnomeKeyvalToScancode maps GNOME keyval names to evdev scancodes.
// Only includes keys commonly used for layout switching.
var gnomeKeyvalToScancode = map[string]uint16{
	// Modifiers
	"Shift_L":   scancodeLeftShift,
	"Shift_R":   scancodeRightShift,
	"Control_L": scancodeLeftCtrl,
	"Control_R": scancodeRightCtrl,
	"Alt_L":     scancodeLeftAlt,
	"Alt_R":     scancodeRightAlt,
	"Super_L":   scancodeLeftMeta,
	"Super_R":   scancodeRightMeta,
	// Special keys
	"space":        scancodeSpace,
	"Caps_Lock":    scancodeCapsLock,
	"XF86Keyboard": scancodeKeyboard,
	"XF86Launch7":  scancodeF16,
}

// gnomeModifierAliases maps GNOME accelerator modifier names to keyval names.
var gnomeModifierAliases = map[string]string{
	"Primary": "Control_L", // <Primary> = Ctrl in GNOME
	"Super":   "Super_L",
	"Shift":   "Shift_L",
	"Control": "Control_L",
	"Ctrl":    "Control_L",
	"Alt":     "Alt_L",
	"Meta":    "Super_L", // Meta usually = Super
}

// gnomeProvider implements Provider interface for GNOME WM keybindings.
// It reads switch-input-source from org.gnome.desktop.wm.keybindings via gsettings.
type gnomeProvider struct{}

// Name returns the identifier of this provider.
func (p *gnomeProvider) Name() DetectionSource {
	return SourceGNOME
}

// Detect attempts to detect layout switch configuration from GNOME WM keybindings.
// env may be nil when running as regular user (uses current environment).
func (p *gnomeProvider) Detect(env *SessionEnv) DetectionAttempt {
	attempt := DetectionAttempt{Provider: SourceGNOME}

	// gsettings returns schema defaults (e.g. <Super>space) even on systems
	// where GNOME is not running; trust it only in an actual GNOME session.
	if !isGNOME(env) {
		attempt.Status = StatusInactive
		attempt.Error = "GNOME session not detected"
		return attempt
	}

	// Check if gsettings is available
	if _, err := execLookPath("gsettings"); err != nil {
		attempt.Status = StatusInactive
		attempt.Error = "gsettings not found"
		return attempt
	}

	// Read switch-input-source keybinding
	output, err := RunGsettings(env, "get",
		"org.gnome.desktop.wm.keybindings", "switch-input-source")
	if err != nil {
		// Schema not found = not GNOME
		if strings.Contains(err.Error(), "No such schema") ||
			strings.Contains(err.Error(), "No such key") {
			attempt.Status = StatusInactive
			return attempt
		}
		attempt.Status = StatusError
		attempt.Error = err.Error()
		return attempt
	}

	// GNOME X11 gets a verified persistent single-key accelerator. Other
	// backends preserve the user's first configured binding.
	preferInternalX11 := isGNOMEX11Session(env)
	accel := selectGNOMEAccelerator(
		parseGsettingsArray(strings.TrimSpace(string(output))),
		preferInternalX11,
	)
	if accel == "" || accel == "disabled" {
		// Try switch-input-source-backward as fallback
		output, err = RunGsettings(env, "get",
			"org.gnome.desktop.wm.keybindings", "switch-input-source-backward")
		if err == nil {
			accel = selectGNOMEAccelerator(
				parseGsettingsArray(strings.TrimSpace(string(output))),
				preferInternalX11,
			)
		}
	}

	if accel == "" || accel == "disabled" {
		attempt.Status = StatusNotFound
		return attempt
	}

	scancodes, keyNames, err := parseGNOMEAccelerator(accel)
	if err != nil {
		attempt.Status = StatusUnsupported
		attempt.RawValue = accel
		attempt.Error = err.Error()
		return attempt
	}

	attempt.Status = StatusFound
	attempt.RawValue = accel
	attempt.Scancodes = scancodes
	attempt.KeyNames = keyNames
	return attempt
}

// parseGsettingsArray parses every element from gsettings array output.
// Input formats:
//   - ['<Super>space'] → ["<Super>space"]
//   - ['<Super>space', 'XF86Keyboard'] → ["<Super>space", "XF86Keyboard"]
//   - @as [] → nil
//   - ['disabled'] → ["disabled"]
func parseGsettingsArray(output string) []string {
	output = strings.TrimSpace(output)

	// Empty array
	if output == "@as []" || output == "[]" {
		return nil
	}

	// Remove brackets and parse
	if strings.HasPrefix(output, "[") && strings.HasSuffix(output, "]") {
		output = strings.TrimPrefix(output, "[")
		output = strings.TrimSuffix(output, "]")
	}

	// Accelerator values do not contain commas, so gsettings' simple array
	// representation can be parsed without interpreting arbitrary GVariant.
	parts := strings.Split(output, ",")
	accelerators := make([]string, 0, len(parts))
	for _, part := range parts {
		accel := strings.Trim(strings.TrimSpace(part), "'\"")
		if accel != "" {
			accelerators = append(accelerators, accel)
		}
	}
	return accelerators
}

// selectGNOMEAccelerator prefers the internal X11 accelerator only in the
// backend that installs and verifies it. Other backends preserve GNOME's first
// configured accelerator.
func selectGNOMEAccelerator(accelerators []string, preferInternalX11 bool) string {
	if preferInternalX11 {
		for _, accel := range accelerators {
			if accel == gnomeX11Accelerator {
				return accel
			}
		}
	}
	if len(accelerators) == 0 {
		return ""
	}
	return accelerators[0]
}

// parseGNOMEAccelerator parses GNOME accelerator format and returns scancodes.
// Input formats:
//   - "<Super>space" → [125, 57] (Super_L + Space)
//   - "<Shift><Alt>" → [42, 56] (Shift_L + Alt_L)
//   - "<Primary><Shift>" → [29, 42] (Control_L + Shift_L)
//   - "disabled" or "" → empty result (nil, "", nil)
//
// Returns scancodes slice, human-readable key names, and error if unsupported.
func parseGNOMEAccelerator(accel string) ([]uint16, string, error) {
	// Empty or disabled returns empty result (not an error per spec)
	if accel == "" || accel == "disabled" {
		return nil, "", nil
	}

	var scancodes []uint16
	remaining := accel

	// Parse modifiers in angle brackets
	for strings.HasPrefix(remaining, "<") {
		endIdx := strings.Index(remaining, ">")
		if endIdx == -1 {
			return nil, "", fmt.Errorf("malformed accelerator: missing '>' in %s", accel)
		}

		modifier := remaining[1:endIdx]
		remaining = remaining[endIdx+1:]

		// Resolve modifier alias to keyval name
		keyval, ok := gnomeModifierAliases[modifier]
		if !ok {
			return nil, "", fmt.Errorf("unknown modifier: %s", modifier)
		}

		scancode, ok := gnomeKeyvalToScancode[keyval]
		if !ok {
			return nil, "", fmt.Errorf("unsupported key: %s", keyval)
		}

		scancodes = append(scancodes, scancode)
	}

	// Parse remaining key (if any)
	if remaining != "" {
		scancode, ok := gnomeKeyvalToScancode[remaining]
		if !ok {
			return nil, "", fmt.Errorf("unsupported key: %s", remaining)
		}

		scancodes = append(scancodes, scancode)
	}

	if len(scancodes) == 0 {
		return nil, "", errors.New("no keys found in accelerator")
	}

	// Use ScancodesToKeyNames for consistent key name formatting
	return scancodes, ScancodesToKeyNames(scancodes), nil
}

// NewGNOMEProvider creates a new GNOME provider instance.
//
//nolint:ireturn // Public factory exposes provider behavior, not implementation.
func NewGNOMEProvider() Provider {
	return &gnomeProvider{}
}

// ========== Desktop Environment Detection (Task 07) ==========

// ========== CLI JSON Output Structures (Task 08) ==========

// DetectJSONOutput is the JSON output structure for --detect-layout-switch.
type DetectJSONOutput struct {
	Schema   int                 `json:"schema"`
	Status   string              `json:"status"`
	Result   *DetectJSONResult   `json:"result"`
	Error    string              `json:"error,omitempty"`
	Warning  string              `json:"warning,omitempty"`
	Attempts []DetectJSONAttempt `json:"attempts"`
}

// DetectJSONResult contains the successful detection result.
type DetectJSONResult struct {
	Scancodes []uint16 `json:"scancodes"`
	Source    string   `json:"source"`
	Raw       string   `json:"raw"`
	Keys      string   `json:"keys"`
}

// DetectJSONAttempt represents a single provider attempt in JSON output.
type DetectJSONAttempt struct {
	Provider string `json:"provider"`
	Status   string `json:"status"`
	Raw      string `json:"raw,omitempty"`
	Keys     string `json:"keys,omitempty"`
	Error    string `json:"error,omitempty"`
}

// BuildDetectJSONOutput converts DetectionResult and error to JSON output structure.
// Status values:
//   - "found": layout switch detected successfully
//   - "not_found": no layout switch configuration found
//   - "error": error occurred during detection
func BuildDetectJSONOutput(result *DetectionResult, err error) *DetectJSONOutput {
	output := &DetectJSONOutput{
		Schema:   1,
		Attempts: make([]DetectJSONAttempt, 0),
	}

	// Convert attempts
	if result != nil {
		for _, attempt := range result.Attempts {
			jsonAttempt := DetectJSONAttempt{
				Provider: string(attempt.Provider),
				Status:   string(attempt.Status),
			}
			if attempt.RawValue != "" {
				jsonAttempt.Raw = attempt.RawValue
			}
			if attempt.KeyNames != "" {
				jsonAttempt.Keys = attempt.KeyNames
			}
			if attempt.Error != "" {
				jsonAttempt.Error = attempt.Error
			}
			output.Attempts = append(output.Attempts, jsonAttempt)
		}

		// Set warning if present
		if result.Warning != "" {
			output.Warning = result.Warning
		}
	}

	// Determine status and fill result
	if err != nil {
		if errors.Is(err, ErrNoLayoutSwitchOption) {
			output.Status = "not_found"
			output.Error = "no layout switch option found"
		} else {
			output.Status = "error"
			output.Error = err.Error()
		}
		return output
	}

	// Success
	output.Status = "found"
	if result != nil && len(result.Scancodes) > 0 {
		output.Result = &DetectJSONResult{
			Scancodes: result.Scancodes,
			Source:    string(result.Source),
			Raw:       result.RawValue,
			Keys:      result.KeyNames,
		}
	}

	return output
}

func isDesktopSession(env *SessionEnv, desktop string) bool {
	// Check SessionEnv first if provided
	if env != nil {
		candidates := []string{
			env.XDGCurrentDesktop,
			env.XDGSessionDesktop,
			env.DesktopSession,
		}

		// Check if any field is set
		hasEnvData := false
		for _, val := range candidates {
			if val != "" {
				hasEnvData = true
				// XDG_CURRENT_DESKTOP can be "ubuntu:GNOME" or "GNOME-Classic"
				lower := strings.ToLower(val)
				if strings.Contains(lower, desktop) {
					return true
				}
			}
		}

		// If SessionEnv has DE data, don't fall back to os.Getenv
		// (avoid mixing env from different sources)
		if hasEnvData {
			return false
		}
	}

	// Fallback to local environment only if env is nil or has no DE data
	osCandidates := []string{
		os.Getenv("XDG_CURRENT_DESKTOP"),
		os.Getenv("XDG_SESSION_DESKTOP"),
		os.Getenv("DESKTOP_SESSION"),
	}

	for _, val := range osCandidates {
		if val == "" {
			continue
		}
		lower := strings.ToLower(val)
		if strings.Contains(lower, desktop) {
			return true
		}
	}
	return false
}

// isGNOME checks if the current desktop environment is GNOME.
// When env is provided (root context), it takes precedence over process env.
func isGNOME(env *SessionEnv) bool {
	return isDesktopSession(env, "gnome")
}

// IsGNOMESession reports whether env (or the current process environment when
// env is nil) belongs to a GNOME graphical session.
func IsGNOMESession(env *SessionEnv) bool {
	return isGNOME(env)
}

// IsAwesomeSession reports whether the active graphical session is Awesome.
func IsAwesomeSession(env *SessionEnv) bool {
	return isDesktopSession(env, "awesome")
}
