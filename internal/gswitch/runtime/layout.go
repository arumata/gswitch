package runtime

import (
	"bufio"
	"errors"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

const xkbSymbolsPath = "/usr/share/X11/xkb/symbols"

// getRealUserHome returns the home directory of the real user,
// even when running as root via sudo or systemd service
func getRealUserHome() string {
	// Check SUDO_USER first (set when running via sudo)
	if sudoUser := os.Getenv("SUDO_USER"); sudoUser != "" {
		// Get home directory for the sudo user
		if home := os.Getenv("SUDO_USER_HOME"); home != "" {
			return home
		}
		// Try /home/username
		home := "/home/" + sudoUser
		if _, err := os.Stat(home); err == nil {
			return home
		}
	}

	// When running as systemd service, try to find user from graphical session
	if username := getGraphicalSessionUser(); username != "" {
		home := "/home/" + username
		if _, err := os.Stat(home); err == nil {
			return home
		}
	}

	// Fallback to current user's home
	if home, err := os.UserHomeDir(); err == nil {
		return home
	}

	return ""
}

// getGraphicalSessionUser returns the username of the graphical session owner
func getGraphicalSessionUser() string {
	// Use loginctl to find graphical sessions
	cmd := exec.Command("loginctl", "list-sessions", "--no-legend")
	output, err := cmd.Output()
	if err != nil {
		return ""
	}

	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 3 {
			continue
		}
		sessionID := fields[0]
		username := fields[2]

		// Check if this is a graphical session
		// #nosec G204 -- sessionID comes from loginctl list-sessions output
		cmd := exec.Command("loginctl", "show-session", sessionID, "-p", "Type")
		typeOutput, err := cmd.Output()
		if err != nil {
			continue
		}

		typeLine := strings.TrimSpace(string(typeOutput))
		if strings.HasPrefix(typeLine, "Type=x11") || strings.HasPrefix(typeLine, "Type=wayland") {
			return username
		}
	}

	return ""
}

// LayoutInfo holds information about a keyboard layout
type LayoutInfo struct {
	Name    string            // e.g., "us", "ru"
	Variant string            // e.g., "basic", "winkeys"
	KeyMap  map[string][]rune // keycode -> [normal, shift] runes
}

// LayoutConverter handles conversion between two keyboard layouts
type LayoutConverter struct {
	Layout1   *LayoutInfo
	Layout2   *LayoutInfo
	ToLayout2 map[rune]rune // char from layout1 -> char in layout2
	ToLayout1 map[rune]rune // char from layout2 -> char in layout1
}

func formatLayout(spec LayoutSpec) string {
	if spec.Variant == "" {
		return spec.Name
	}
	return fmt.Sprintf("%s(%s)", spec.Name, spec.Variant)
}

// FormatLayout returns layout name in config format.
func FormatLayout(spec LayoutSpec) string {
	return formatLayout(spec)
}

// GetCurrentLayouts returns the currently configured keyboard layouts
func GetCurrentLayouts() ([]LayoutSpec, error) {
	// Try fcitx5 first (common on KDE)
	if layouts, err := getLayoutsFromFcitx5(); err == nil && len(layouts) >= 2 {
		return layouts, nil
	}

	// Try ibus
	if layouts, err := getLayoutsFromIbus(); err == nil && len(layouts) >= 2 {
		return layouts, nil
	}

	// Fallback to setxkbmap
	return getLayoutsFromXkb()
}

func getLayoutsFromFcitx5() ([]LayoutSpec, error) {
	// When running as root via sudo, get the real user's home
	home := getRealUserHome()
	if home == "" {
		return nil, errors.New("cannot determine user home directory")
	}

	profilePath := home + "/.config/fcitx5/profile"
	file, err := os.Open(profilePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var layouts []LayoutSpec
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		// Look for lines like "Name=keyboard-us" or "Name=keyboard-ru"
		if after, ok := strings.CutPrefix(line, "Name=keyboard-"); ok {
			layout, variant := splitLayoutVariant(after)
			spec := LayoutSpec{Name: layout, Variant: variant}
			if !containsLayoutSpec(layouts, spec) {
				layouts = append(layouts, spec)
			}
		}
	}

	if len(layouts) == 0 {
		return nil, errors.New("no layouts found in fcitx5 profile")
	}

	return layouts, nil
}

func getLayoutsFromIbus() ([]LayoutSpec, error) {
	// Try to read ibus dconf settings via gsettings
	cmd := exec.Command("gsettings", "get", "org.freedesktop.ibus.general", "preload-engines")
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	// Parse output like: ['xkb:us::eng', 'xkb:ru::rus']
	outStr := strings.TrimSpace(string(output))
	if outStr == "[]" || outStr == "@as []" {
		return nil, errors.New("no ibus engines configured")
	}

	layouts := parseIbusEngines(outStr)

	if len(layouts) == 0 {
		return nil, errors.New("no layouts found in ibus")
	}

	return layouts, nil
}

func getLayoutsFromXkb() ([]LayoutSpec, error) {
	cmd := exec.Command("setxkbmap", "-query")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to run setxkbmap: %w", err)
	}

	var layouts []LayoutSpec
	var variants []string
	for line := range strings.SplitSeq(string(output), "\n") {
		line = strings.TrimSpace(line)
		if after, ok := strings.CutPrefix(line, "layout:"); ok {
			layoutStr := strings.TrimSpace(after)
			layouts = stringsToSpecs(strings.Split(layoutStr, ","))
		}
		if after, ok := strings.CutPrefix(line, "variant:"); ok {
			variantStr := strings.TrimSpace(after)
			rawVariants := strings.SplitSeq(variantStr, ",")
			for v := range rawVariants {
				variants = append(variants, strings.TrimSpace(v))
			}
		}
	}

	if len(layouts) == 0 {
		return nil, errors.New("no layouts found")
	}

	// Align variants with layouts positionally
	for i := range layouts {
		if i < len(variants) {
			layouts[i].Variant = variants[i]
		}
	}

	return layouts, nil
}

// Compiled regexps for XKB parsing (cached at package level for performance)
var (
	keyRegex     = regexp.MustCompile(`key\s+<(\w+)>\s*\{\s*\[\s*([^\]]+)\s*\]`)
	sectionRegex = regexp.MustCompile(`xkb_symbols\s+"(\w+)"`)
	includeRegex = regexp.MustCompile(`include\s+"(\w+)\((\w+)\)"`)
)

// LoadLayout loads a keyboard layout from XKB symbols file
func LoadLayout(name, variant string) (*LayoutInfo, error) {
	return loadLayoutWithVisited(name, variant, make(map[string]bool))
}

// loadLayoutWithVisited is the internal implementation that tracks visited layouts
// to prevent infinite recursion from circular includes
func loadLayoutWithVisited(name, variant string, visited map[string]bool) (*LayoutInfo, error) {
	// Validate layout name to prevent path traversal attacks
	if strings.Contains(name, "..") || filepath.IsAbs(name) || strings.ContainsAny(name, "/\\") {
		return nil, fmt.Errorf("invalid layout name: %s", name)
	}

	// Create a unique key for this name+variant combination
	visitKey := name + ":" + variant
	if visited[visitKey] {
		// Already visited this layout variant, skip to prevent infinite recursion
		return &LayoutInfo{
			Name:    name,
			Variant: variant,
			KeyMap:  make(map[string][]rune),
		}, nil
	}
	visited[visitKey] = true

	filePath := filepath.Join(xkbSymbolsPath, name)
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("cannot open layout file %s: %w", filePath, err)
	}
	defer file.Close()

	layout := &LayoutInfo{
		Name:    name,
		Variant: variant,
		KeyMap:  make(map[string][]rune),
	}

	// If no variant specified, use default
	if variant == "" {
		variant = "basic"
	}

	// Parse the file to find the right variant section
	scanner := bufio.NewScanner(file)
	inSection := false
	targetSection := variant
	braceCount := 0

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Check for section start
		if matches := sectionRegex.FindStringSubmatch(line); matches != nil {
			sectionName := matches[1]
			if sectionName == targetSection || (targetSection == "basic" && sectionName == "default") ||
				(name == "ru" && targetSection == "basic" && sectionName == "winkeys") {
				inSection = true
				braceCount = 0
			}
		}

		if !inSection {
			continue
		}

		// Track braces to know when section ends
		braceCount += strings.Count(line, "{") - strings.Count(line, "}")
		if braceCount <= 0 && inSection && strings.Contains(line, "}") {
			break
		}

		// Handle includes
		if matches := includeRegex.FindStringSubmatch(line); matches != nil {
			includeName := matches[1]
			includeVariant := matches[2]
			if includeName == name {
				// Include from same file - use visited map to prevent recursion
				includedLayout, err := loadLayoutWithVisited(includeName, includeVariant, visited)
				if err == nil {
					maps.Copy(layout.KeyMap, includedLayout.KeyMap)
				}
			}
		}

		// Parse key definitions
		if matches := keyRegex.FindStringSubmatch(line); matches != nil {
			keycode := matches[1]
			symsStr := matches[2]
			syms := strings.Split(symsStr, ",")

			runes := make([]rune, 0, len(syms))
			for _, sym := range syms {
				sym = strings.TrimSpace(sym)
				if r := keysymToRune(sym); r != 0 {
					runes = append(runes, r)
				}
			}

			if len(runes) > 0 {
				layout.KeyMap[keycode] = runes
			}
		}
	}

	return layout, nil
}

// NewLayoutConverter creates a converter between two layouts
func NewLayoutConverter(layout1, layout2 *LayoutInfo) *LayoutConverter {
	conv := &LayoutConverter{
		Layout1:   layout1,
		Layout2:   layout2,
		ToLayout2: make(map[rune]rune),
		ToLayout1: make(map[rune]rune),
	}

	// Build conversion maps based on physical key positions
	for keycode, runes1 := range layout1.KeyMap {
		if runes2, ok := layout2.KeyMap[keycode]; ok {
			// Map normal characters
			if len(runes1) > 0 && len(runes2) > 0 {
				conv.ToLayout2[runes1[0]] = runes2[0]
				conv.ToLayout1[runes2[0]] = runes1[0]
			}
			// Map shifted characters
			if len(runes1) > 1 && len(runes2) > 1 {
				conv.ToLayout2[runes1[1]] = runes2[1]
				conv.ToLayout1[runes2[1]] = runes1[1]
			}
		}
	}

	return conv
}

// Convert converts text from one layout to another
// direction: true = layout1->layout2, false = layout2->layout1
func (lc *LayoutConverter) Convert(text string, toLayout2 bool) string {
	var convMap map[rune]rune
	if toLayout2 {
		convMap = lc.ToLayout2
	} else {
		convMap = lc.ToLayout1
	}

	result := make([]rune, 0, len(text))
	for _, r := range text {
		if converted, ok := convMap[r]; ok {
			result = append(result, converted)
		} else {
			result = append(result, r)
		}
	}

	return string(result)
}

// DetectLayout attempts to detect which layout the text is in
// Returns true if text appears to be in layout1, false if in layout2
func (lc *LayoutConverter) DetectLayout(text string) bool {
	layout1Chars := 0
	layout2Chars := 0

	for _, r := range text {
		if _, ok := lc.ToLayout2[r]; ok {
			layout1Chars++
		}
		if _, ok := lc.ToLayout1[r]; ok {
			layout2Chars++
		}
	}

	return layout1Chars >= layout2Chars
}

// keysymToRune converts an X11 keysym name to a Unicode rune
func keysymToRune(keysym string) rune {
	// Single character keysyms (letters and digits)
	if len(keysym) == 1 {
		c := keysym[0]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') {
			return rune(c)
		}
	}

	// Unicode keysyms: U0041 -> 'A', U0436 -> Cyrillic zhe, etc.
	if len(keysym) >= 5 && keysym[0] == 'U' {
		if codepoint, err := strconv.ParseInt(keysym[1:], 16, 32); err == nil {
			if codepoint > 0 && codepoint <= 0x10FFFF {
				return rune(codepoint)
			}
		}
	}

	// Lookup in generated keysym map (see keysyms_generated.go)
	return keysymMap[keysym]
}

func splitLayoutVariant(s string) (layout, variant string) {
	if s == "" {
		return "", ""
	}
	s = strings.TrimSpace(s)

	// Handle parentheses format: "ua(unicode)" -> layout="ua", variant="unicode"
	if idx := strings.Index(s, "("); idx != -1 {
		layout = strings.TrimSpace(s[:idx])
		variant = strings.TrimSpace(strings.TrimSuffix(s[idx+1:], ")"))
		return layout, variant
	}

	// Handle dash format: "ru-phonetic" -> layout="ru", variant="phonetic"
	parts := strings.Split(s, "-")
	layout = strings.TrimSpace(parts[0])
	if len(parts) > 1 {
		variant = strings.TrimSpace(strings.Join(parts[1:], "-"))
	}
	return layout, variant
}

func stringsToSpecs(items []string) []LayoutSpec {
	specs := make([]LayoutSpec, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		specs = append(specs, LayoutSpec{Name: item})
	}
	return specs
}

func containsLayoutSpec(specs []LayoutSpec, target LayoutSpec) bool {
	for _, s := range specs {
		if s.Name == target.Name && s.Variant == target.Variant {
			return true
		}
	}
	return false
}

func parseIbusEngines(outStr string) []LayoutSpec {
	var layouts []LayoutSpec

	for p := range strings.SplitSeq(outStr, "'") {
		if !strings.HasPrefix(p, "xkb:") {
			continue
		}

		fields := strings.Split(p, ":")
		if len(fields) < 2 {
			continue
		}

		layout := fields[1]
		var variant string
		if len(fields) >= 3 {
			variant = fields[2]
		}

		if layout == "" {
			continue
		}

		spec := LayoutSpec{Name: layout, Variant: variant}
		if !containsLayoutSpec(layouts, spec) {
			layouts = append(layouts, spec)
		}
	}

	return layouts
}
