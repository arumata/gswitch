package tray

import (
	"bufio"
	"errors"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"time"
)

const pollInterval = 500 * time.Millisecond

// fcitx5AuthorityTTL is how long the "does fcitx5 switch layouts here" answer is
// reused. The input method group changes only when the user reconfigures fcitx5,
// while the poll runs twice a second — without caching every tick would spawn two
// extra gdbus processes.
const fcitx5AuthorityTTL = 5 * time.Second

// LayoutInfo contains information about the current keyboard layout.
type LayoutInfo struct {
	ShortCode string // e.g., "US", "RU"
	LongName  string // e.g., "English (US)", "Russian"
}

// LayoutMonitor monitors the current keyboard layout.
type LayoutMonitor struct {
	callback func(layout LayoutInfo)
	current  LayoutInfo
	layouts  []LayoutInfo // configured layouts
	ticker   *time.Ticker
	done     chan struct{}
	stopOnce sync.Once
	mu       sync.RWMutex

	// Cached answer to "is fcitx5 the thing that switches layouts here", with
	// the time it was taken. Guarded by mu.
	fcitx5Switches  bool
	fcitx5CheckedAt time.Time
}

// NewLayoutMonitor creates a new layout monitor with a callback for layout changes.
func NewLayoutMonitor(callback func(layout LayoutInfo)) *LayoutMonitor {
	return &LayoutMonitor{
		callback: callback,
		done:     make(chan struct{}),
	}
}

// Start begins monitoring layout changes.
func (m *LayoutMonitor) Start() error {
	// Load initial layouts list
	layouts, err := m.loadLayouts()
	if err != nil {
		// Fallback to GNOME input-sources if KDE DBus fails
		layouts, err = m.loadLayoutsFromGnome()
	}
	if err != nil {
		// Fallback to setxkbmap
		layouts, err = m.loadLayoutsFromSetxkbmap()
	}
	if err != nil {
		// Last resort fallback
		layouts = []LayoutInfo{{ShortCode: "??", LongName: "Unknown"}}
	}

	m.mu.Lock()
	m.layouts = layouts
	m.mu.Unlock()

	// Get initial layout
	m.updateCurrentLayout()

	// Start polling
	m.ticker = time.NewTicker(pollInterval)
	go m.poll()

	return nil
}

// Stop stops monitoring layout changes.
func (m *LayoutMonitor) Stop() {
	m.stopOnce.Do(func() {
		if m.ticker != nil {
			m.ticker.Stop()
		}
		close(m.done)
	})
}

// GetCurrentLayout returns the current keyboard layout.
func (m *LayoutMonitor) GetCurrentLayout() LayoutInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.current
}

func (m *LayoutMonitor) poll() {
	for {
		select {
		case <-m.done:
			return
		case <-m.ticker.C:
			m.updateCurrentLayout()
		}
	}
}

func (m *LayoutMonitor) updateCurrentLayout() {
	var newLayout LayoutInfo

	// Try fcitx5 first (most common on modern systems), but only where it is
	// the thing doing the switching. A fcitx5 holding a single input method
	// answers that same method forever while XKB switches the real layout, and
	// since the call succeeds the fallbacks below would never run.
	if m.fcitx5OwnsSwitching() {
		if layout, ok := m.getLayoutFromFcitx5(); ok {
			newLayout = layout
		}
	}

	// Try KDE DBus if fcitx5 didn't work
	if newLayout.ShortCode == "" {
		index, err := m.getLayoutIndexFromKDE()
		if err == nil {
			m.mu.RLock()
			if index < len(m.layouts) {
				newLayout = m.layouts[index]
			}
			m.mu.RUnlock()
		}
	}

	// Try GNOME mru-sources if KDE didn't work
	if newLayout.ShortCode == "" {
		if layout, ok := m.getLayoutFromGnome(); ok {
			newLayout = layout
		}
	}

	// If nothing worked, use fallback
	if newLayout.ShortCode == "" {
		newLayout = m.getLayoutFromSetxkbmap()
	}

	// Check if layout changed
	m.mu.Lock()
	changed := newLayout.ShortCode != m.current.ShortCode
	m.current = newLayout
	m.mu.Unlock()

	if changed && m.callback != nil {
		m.callback(newLayout)
	}
}

// loadLayouts loads the list of configured layouts from KDE DBus.
func (m *LayoutMonitor) loadLayouts() ([]LayoutInfo, error) {
	cmd := exec.Command("gdbus", "call", "--session",
		"--dest", "org.kde.keyboard",
		"--object-path", "/Layouts",
		"--method", "org.kde.KeyboardLayouts.getLayoutsList")

	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	return parseKDELayoutsList(string(output)), nil
}

// loadLayoutsFromSetxkbmap loads layouts from setxkbmap -query.
func (m *LayoutMonitor) loadLayoutsFromSetxkbmap() ([]LayoutInfo, error) {
	cmd := exec.Command("setxkbmap", "-query")
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	return parseSetxkbmapOutput(string(output)), nil
}

// getLayoutIndexFromKDE gets the current layout index from KDE DBus.
func (m *LayoutMonitor) getLayoutIndexFromKDE() (int, error) {
	cmd := exec.Command("gdbus", "call", "--session",
		"--dest", "org.kde.keyboard",
		"--object-path", "/Layouts",
		"--method", "org.kde.KeyboardLayouts.getLayout")

	output, err := cmd.Output()
	if err != nil {
		return 0, err
	}

	return parseKDELayoutIndex(string(output)), nil
}

// getLayoutFromFcitx5 gets the current layout from fcitx5 DBus.
// Returns the layout and true if successful, empty layout and false otherwise.
func (m *LayoutMonitor) getLayoutFromFcitx5() (LayoutInfo, bool) {
	cmd := exec.Command("gdbus", "call", "--session",
		"--dest", "org.fcitx.Fcitx5",
		"--object-path", "/controller",
		"--method", "org.fcitx.Fcitx.Controller1.CurrentInputMethod")

	output, err := cmd.Output()
	if err != nil {
		return LayoutInfo{}, false
	}

	return parseFcitx5CurrentMethod(string(output))
}

// fcitx5OwnsSwitching reports whether fcitx5 is what switches layouts on this
// host. True only when its current input method group holds two or more input
// methods; with one it never changes its answer and cannot report the layout.
// The result is cached for fcitx5AuthorityTTL.
func (m *LayoutMonitor) fcitx5OwnsSwitching() bool {
	m.mu.RLock()
	cached, checkedAt := m.fcitx5Switches, m.fcitx5CheckedAt
	m.mu.RUnlock()

	if !checkedAt.IsZero() && time.Since(checkedAt) < fcitx5AuthorityTTL {
		return cached
	}

	owns := m.fcitx5GroupSize() >= 2

	m.mu.Lock()
	m.fcitx5Switches, m.fcitx5CheckedAt = owns, time.Now()
	m.mu.Unlock()

	return owns
}

// fcitx5GroupSize returns how many input methods the current fcitx5 group holds,
// or 0 when fcitx5 is absent or does not answer.
func (m *LayoutMonitor) fcitx5GroupSize() int {
	out, err := exec.Command("gdbus", "call", "--session",
		"--dest", "org.fcitx.Fcitx5",
		"--object-path", "/controller",
		"--method", "org.fcitx.Fcitx.Controller1.CurrentInputMethodGroup").Output()
	if err != nil {
		return 0
	}

	group := parseFcitx5GroupName(string(out))
	if group == "" {
		return 0
	}

	// An empty group name makes fcitx5 answer with an empty list rather than
	// the current group, hence the check above.
	// #nosec G204 -- group comes from fcitx5's own CurrentInputMethodGroup reply
	out, err = exec.Command("gdbus", "call", "--session",
		"--dest", "org.fcitx.Fcitx5",
		"--object-path", "/controller",
		"--method", "org.fcitx.Fcitx.Controller1.InputMethodGroupInfo", group).Output()
	if err != nil {
		return 0
	}

	return parseFcitx5GroupInputMethodCount(string(out))
}

// loadLayoutsFromGnome loads layouts from GNOME's input-sources gsettings key.
func (m *LayoutMonitor) loadLayoutsFromGnome() ([]LayoutInfo, error) {
	out, err := exec.Command("gsettings", "get",
		"org.gnome.desktop.input-sources", "sources").Output()
	if err != nil {
		return nil, err
	}

	layouts := parseGnomeSources(string(out))
	if len(layouts) == 0 {
		return nil, errors.New("no xkb sources in GNOME input-sources")
	}
	return layouts, nil
}

// getLayoutFromGnome returns the current layout from GNOME's mru-sources key
// (most-recently-used order; the first entry is the active source).
func (m *LayoutMonitor) getLayoutFromGnome() (LayoutInfo, bool) {
	out, err := exec.Command("gsettings", "get",
		"org.gnome.desktop.input-sources", "mru-sources").Output()
	if err != nil {
		return LayoutInfo{}, false
	}

	layouts := parseGnomeSources(string(out))
	if len(layouts) == 0 {
		// mru-sources is empty until the first switch; fall back to sources
		layouts, err = m.loadLayoutsFromGnome()
		if err != nil {
			return LayoutInfo{}, false
		}
	}
	return layouts[0], true
}

// gnomeSourceRe matches tuples like ('xkb', 'ru') in gsettings output.
var gnomeSourceRe = regexp.MustCompile(`\('([a-z]+)',\s*'([^']+)'\)`)

// parseGnomeSources parses gsettings output like: [('xkb', 'us'), ('xkb', 'ru')]
// The xkb id encodes a variant after '+': 'ua+unicode'.
func parseGnomeSources(s string) []LayoutInfo {
	matches := gnomeSourceRe.FindAllStringSubmatch(s, -1)
	layouts := make([]LayoutInfo, 0, len(matches))
	for _, match := range matches {
		if match[1] != "xkb" {
			continue
		}
		name, _, _ := strings.Cut(match[2], "+")
		if name == "" {
			continue
		}
		layouts = append(layouts, LayoutInfo{
			ShortCode: strings.ToUpper(name),
			LongName:  match[2],
		})
	}
	return layouts
}

// getLayoutFromSetxkbmap returns the first layout from setxkbmap (fallback).
func (m *LayoutMonitor) getLayoutFromSetxkbmap() LayoutInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if len(m.layouts) > 0 {
		return m.layouts[0]
	}
	return LayoutInfo{ShortCode: "??", LongName: "Unknown"}
}

// parseKDELayoutsList parses output like:
// ([('us', 'us', 'English (US)'), ('ru', 'ru', 'Russian')],)
func parseKDELayoutsList(output string) []LayoutInfo {
	var layouts []LayoutInfo

	// Find the array content between [( and )]
	start := strings.Index(output, "[(")
	end := strings.LastIndex(output, ")]")
	if start == -1 || end == -1 || start >= end {
		return layouts
	}

	content := output[start+2 : end]
	// Split by "), (" to get individual tuples
	tuples := strings.SplitSeq(content, "), (")

	for tuple := range tuples {
		tuple = strings.Trim(tuple, "() ")
		parts := strings.Split(tuple, ", ")
		if len(parts) >= 3 {
			shortCode := strings.Trim(parts[0], "'")
			longName := strings.Trim(parts[2], "'")
			layouts = append(layouts, LayoutInfo{
				ShortCode: strings.ToUpper(shortCode),
				LongName:  longName,
			})
		}
	}

	return layouts
}

// parseKDELayoutIndex parses output like: (uint32 0,)
func parseKDELayoutIndex(output string) int {
	output = strings.TrimSpace(output)
	// Format: (uint32 N,)
	start := strings.Index(output, "uint32 ")
	if start == -1 {
		return 0
	}
	numStr := output[start+7:]
	end := strings.Index(numStr, ",")
	if end == -1 {
		return 0
	}
	numStr = numStr[:end]

	var index int
	for _, c := range numStr {
		if c >= '0' && c <= '9' {
			index = index*10 + int(c-'0')
		}
	}
	return index
}

// parseSetxkbmapOutput parses output like:
// rules:      evdev
// model:      pc104
// layout:     us,ru
// variant:    ,
// options:    grp:win_space_toggle
func parseSetxkbmapOutput(output string) []LayoutInfo {
	var layouts []LayoutInfo

	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if after, ok := strings.CutPrefix(line, "layout:"); ok {
			layoutStr := strings.TrimSpace(after)
			for layout := range strings.SplitSeq(layoutStr, ",") {
				layout = strings.TrimSpace(layout)
				if layout != "" {
					layouts = append(layouts, LayoutInfo{
						ShortCode: strings.ToUpper(layout),
						LongName:  layoutCodeToName(layout),
					})
				}
			}
			break
		}
	}

	return layouts
}

// parseFcitx5CurrentMethod parses output like: ('keyboard-ru',)
// Returns the layout info and true if successful.
func parseFcitx5CurrentMethod(output string) (LayoutInfo, bool) {
	output = strings.TrimSpace(output)
	// Format: ('keyboard-XX',) or ('some-input-method',)
	start := strings.Index(output, "('")
	end := strings.Index(output, "',)")
	if start == -1 || end == -1 || start >= end {
		return LayoutInfo{}, false
	}

	method := output[start+2 : end]

	// Check if it's a keyboard layout (format: keyboard-XX)
	if after, ok := strings.CutPrefix(method, "keyboard-"); ok {
		code := after
		// Handle variants like "keyboard-ru-phonetic"
		if idx := strings.Index(code, "-"); idx != -1 {
			code = code[:idx]
		}
		return LayoutInfo{
			ShortCode: strings.ToUpper(code),
			LongName:  layoutCodeToName(code),
		}, true
	}

	// Non-keyboard input method (e.g., pinyin, mozc)
	// Just show the method name
	return LayoutInfo{
		ShortCode: "IM",
		LongName:  method,
	}, true
}

// parseFcitx5GroupName parses output like: ('Default',)
func parseFcitx5GroupName(output string) string {
	output = strings.TrimSpace(output)
	start := strings.Index(output, "('")
	end := strings.Index(output, "',)")
	if start == -1 || end == -1 || start >= end {
		return ""
	}
	return output[start+2 : end]
}

// parseFcitx5GroupInputMethodCount returns how many input methods a group info
// reply lists. The reply is the group's default layout followed by a list of
// (input method, layout) tuples; see TestParseFcitx5GroupInputMethodCount for
// the exact wire format.
func parseFcitx5GroupInputMethodCount(output string) int {
	start := strings.Index(output, "[")
	end := strings.LastIndex(output, "]")
	if start == -1 || end == -1 || start >= end {
		return 0
	}
	// Each entry is a ('name', 'layout') tuple; an empty list has no "('".
	return strings.Count(output[start:end], "('")
}

// layoutCodeToName converts layout code to a human-readable name.
func layoutCodeToName(code string) string {
	names := map[string]string{
		"us": "English (US)",
		"ru": "Russian",
		"de": "German",
		"fr": "French",
		"es": "Spanish",
		"it": "Italian",
		"pt": "Portuguese",
		"pl": "Polish",
		"ua": "Ukrainian",
		"by": "Belarusian",
		"kz": "Kazakh",
		"gb": "English (UK)",
	}
	if name, ok := names[strings.ToLower(code)]; ok {
		return name
	}
	return strings.ToUpper(code)
}
