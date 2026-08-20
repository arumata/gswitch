package detect

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
)

// ErrNoActiveSession is returned when no active graphical user session is found.
// This typically happens when the systemd service starts before user login.
var ErrNoActiveSession = errors.New("no active graphical user session found")

// ErrNoSystemd is returned when loginctl is not available.
// This indicates the system doesn't use systemd and root auto-detection is not supported.
var ErrNoSystemd = errors.New("loginctl not found: root auto-detection requires systemd")

// ErrNotRoot is returned when RunAsSessionUser is called with env != nil from non-root.
var ErrNotRoot = errors.New("RunAsSessionUser with env requires root privileges")

// execCommand is a variable for exec.Command to allow mocking in tests.
var execCommand = exec.Command

// execLookPath is a variable for exec.LookPath to allow mocking in tests.
var execLookPath = exec.LookPath

// displayManagerUsers contains usernames that should be ignored when searching for sessions.
var displayManagerUsers = map[string]bool{
	"gdm":      true,
	"sddm":     true,
	"lightdm":  true,
	"root":     true,
	"lxdm":     true,
	"slim":     true,
	"nodm":     true,
	"greetd":   true,
	"emptty":   true,
	"ly":       true,
	"tbsm":     true,
	"entrance": true,
}

// SessionEnv contains environment variables and credentials for an active user session.
type SessionEnv struct {
	UID           uint32 // User ID
	GID           uint32 // Group ID
	User          string // Username
	Home          string // Home directory
	Display       string // X11 DISPLAY (may be empty for pure Wayland)
	XAuthority    string // X11 XAUTHORITY file path
	DBusAddress   string // D-Bus session bus address
	RuntimeDir    string // XDG_RUNTIME_DIR
	SessionType   string // "x11" or "wayland"
	XDGConfigHome string // XDG_CONFIG_HOME for non-standard config paths
	IBusAddress   string // IBUS_ADDRESS for checking ibus activity without pgrep

	// Desktop environment detection (Task 07)
	XDGCurrentDesktop string // "ubuntu:GNOME", "GNOME", "KDE", etc.
	XDGSessionDesktop string // "gnome", "plasma", etc.
	DesktopSession    string // legacy fallback
}

// sessionInfo holds parsed session properties from loginctl.
type sessionInfo struct {
	ID        string
	UID       uint32
	User      string
	Type      string // "x11", "wayland", "tty", etc.
	Class     string // "user", "greeter", "lock-screen"
	Active    bool
	State     string // "active", "online", "closing"
	Seat      string
	Display   string
	LeaderPID string
}

// GetActiveSessionEnv returns the environment of the active graphical session.
// Returns ErrNoActiveSession if no suitable session is found.
// Returns ErrNoSystemd if loginctl is not available.
func GetActiveSessionEnv() (*SessionEnv, error) {
	// Check if loginctl is available
	if _, err := execLookPath("loginctl"); err != nil {
		return nil, ErrNoSystemd
	}

	// List all sessions
	sessions, err := listSessions()
	if err != nil {
		return nil, fmt.Errorf("failed to list sessions: %w", err)
	}

	if len(sessions) == 0 {
		return nil, ErrNoActiveSession
	}

	// Find the best session
	session := selectBestSession(sessions)
	if session == nil {
		return nil, ErrNoActiveSession
	}

	// Build SessionEnv
	env, err := buildSessionEnv(session)
	if err != nil {
		return nil, fmt.Errorf("failed to build session env: %w", err)
	}

	return env, nil
}

// listSessions returns all sessions that match our criteria.
func listSessions() ([]sessionInfo, error) {
	cmd := execCommand("loginctl", "list-sessions", "--no-legend")
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	var sessions []sessionInfo
	scanner := bufio.NewScanner(bytes.NewReader(output))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 1 {
			continue
		}
		sessionID := fields[0]

		info, err := getSessionInfo(sessionID)
		if err != nil {
			continue
		}

		// Filter: Type must be x11 or wayland
		if info.Type != "x11" && info.Type != "wayland" {
			continue
		}

		// Filter: Class must be user
		if info.Class != "user" {
			continue
		}

		// Filter: User must not be a display manager
		if displayManagerUsers[info.User] {
			continue
		}

		sessions = append(sessions, info)
	}

	return sessions, nil
}

// getSessionInfo retrieves detailed information about a session.
func getSessionInfo(sessionID string) (sessionInfo, error) {
	info := sessionInfo{ID: sessionID}

	// Get multiple properties in one call
	cmd := execCommand("loginctl", "show-session", sessionID,
		"-p", "Type",
		"-p", "Class",
		"-p", "Active",
		"-p", "State",
		"-p", "Seat",
		"-p", "Display",
		"-p", "User",
		"-p", "Name",
		"-p", "Leader",
	)
	output, err := cmd.Output()
	if err != nil {
		return info, err
	}

	for line := range strings.SplitSeq(string(output), "\n") {
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key, value := parts[0], parts[1]

		switch key {
		case "Type":
			info.Type = value
		case "Class":
			info.Class = value
		case "Active":
			info.Active = value == "yes"
		case "State":
			info.State = value
		case "Seat":
			info.Seat = value
		case "Display":
			info.Display = value
		case "User":
			if uid, err := strconv.ParseUint(value, 10, 32); err == nil {
				info.UID = uint32(uid)
			}
		case "Name":
			info.User = value
		case "Leader":
			info.LeaderPID = value
		}
	}

	return info, nil
}

// selectBestSession chooses the best session from candidates.
// Priority: Active=yes > State=active > Seat=seat0 > first found.
func selectBestSession(sessions []sessionInfo) *sessionInfo {
	if len(sessions) == 0 {
		return nil
	}

	// Priority 1: Active=yes
	var activeYes []*sessionInfo
	// Priority 2: State=active (but not Active=yes)
	var stateActive []*sessionInfo
	// Priority 3: other sessions
	var other []*sessionInfo

	for i := range sessions {
		switch {
		case sessions[i].Active:
			activeYes = append(activeYes, &sessions[i])
		case sessions[i].State == "active":
			stateActive = append(stateActive, &sessions[i])
		default:
			other = append(other, &sessions[i])
		}
	}

	// Select candidates by priority
	var candidates []*sessionInfo
	switch {
	case len(activeYes) > 0:
		candidates = activeYes
	case len(stateActive) > 0:
		candidates = stateActive
	case len(other) > 0:
		candidates = other
	default:
		return nil
	}

	// Among equal priority, prefer seat0 (local session)
	for _, s := range candidates {
		if s.Seat == "seat0" {
			return s
		}
	}

	return candidates[0]
}

// buildSessionEnv constructs SessionEnv from session info.
func buildSessionEnv(session *sessionInfo) (*SessionEnv, error) {
	env := &SessionEnv{
		UID:         session.UID,
		User:        session.User,
		SessionType: session.Type,
		Display:     session.Display,
	}

	// Get GID and Home from passwd
	if err := fillUserInfo(env); err != nil {
		return nil, err
	}

	// Get RuntimeDir from loginctl
	env.RuntimeDir = getRuntimeDir(session.UID)

	// Fill environment from /proc/<Leader>/environ (best-effort)
	fillSessionEnvFromProcess(env, session.LeaderPID)

	// Apply fallbacks for missing values
	applyFallbacks(env, session)

	return env, nil
}

// fillUserInfo fills GID and Home from /etc/passwd or getent.
func fillUserInfo(env *SessionEnv) error {
	// Try getent first
	cmd := execCommand("getent", "passwd", env.User)
	output, err := cmd.Output()
	if err == nil {
		parts := strings.Split(strings.TrimSpace(string(output)), ":")
		if len(parts) >= 6 {
			if gid, err := strconv.ParseUint(parts[3], 10, 32); err == nil {
				env.GID = uint32(gid)
			}
			env.Home = parts[5]
			return nil
		}
	}

	// Fallback: parse /etc/passwd directly
	file, err := os.Open("/etc/passwd")
	if err != nil {
		return fmt.Errorf("failed to read passwd: %w", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		parts := strings.Split(scanner.Text(), ":")
		if len(parts) >= 6 && parts[0] == env.User {
			if gid, err := strconv.ParseUint(parts[3], 10, 32); err == nil {
				env.GID = uint32(gid)
			}
			env.Home = parts[5]
			return nil
		}
	}

	// Last resort fallback
	env.GID = env.UID
	env.Home = "/home/" + env.User
	return nil
}

// getRuntimeDir gets XDG_RUNTIME_DIR from loginctl or constructs default.
func getRuntimeDir(uid uint32) string {
	// Try loginctl show-user
	cmd := execCommand("loginctl", "show-user", strconv.FormatUint(uint64(uid), 10),
		"-p", "RuntimePath", "--value")
	output, err := cmd.Output()
	if err == nil {
		runtimeDir := strings.TrimSpace(string(output))
		if runtimeDir != "" && dirExists(runtimeDir) {
			return runtimeDir
		}
	}

	// Fallback: standard path
	runtimeDir := fmt.Sprintf("/run/user/%d", uid)
	if dirExists(runtimeDir) {
		return runtimeDir
	}

	return ""
}

// fillSessionEnvFromProcess fills SessionEnv from /proc/<Leader>/environ.
// This is best-effort: errors are logged but don't fail the function.
func fillSessionEnvFromProcess(env *SessionEnv, leaderPID string) {
	if leaderPID == "" || leaderPID == "0" {
		return
	}

	envPath := fmt.Sprintf("/proc/%s/environ", leaderPID)
	data, err := os.ReadFile(envPath) //nolint:gosec // reading /proc/<pid>/environ is safe
	if err != nil {
		log.Printf("Warning: failed to read %s: %v, using fallbacks", envPath, err)
		return
	}

	// Parse null-separated environment
	procEnv := make(map[string]string)
	for entry := range bytes.SplitSeq(data, []byte{0}) {
		if len(entry) == 0 {
			continue
		}
		parts := bytes.SplitN(entry, []byte{'='}, 2)
		if len(parts) == 2 {
			procEnv[string(parts[0])] = string(parts[1])
		}
	}

	// Fill fields from procEnv
	if v := procEnv["XDG_RUNTIME_DIR"]; v != "" && env.RuntimeDir == "" {
		env.RuntimeDir = v
	}
	if v := procEnv["DBUS_SESSION_BUS_ADDRESS"]; v != "" {
		env.DBusAddress = v
	}
	if v := procEnv["DISPLAY"]; v != "" && env.Display == "" {
		env.Display = v
	}
	if v := procEnv["XAUTHORITY"]; v != "" {
		if fileExists(v) {
			env.XAuthority = v
		}
	}
	if v := procEnv["WAYLAND_DISPLAY"]; v != "" && env.SessionType == "" {
		env.SessionType = "wayland"
	}
	if v := procEnv["XDG_CONFIG_HOME"]; v != "" {
		env.XDGConfigHome = v
	}
	if v := procEnv["IBUS_ADDRESS"]; v != "" {
		env.IBusAddress = v
	}
	// Desktop environment detection (Task 07)
	if v := procEnv["XDG_CURRENT_DESKTOP"]; v != "" {
		env.XDGCurrentDesktop = v
	}
	if v := procEnv["XDG_SESSION_DESKTOP"]; v != "" {
		env.XDGSessionDesktop = v
	}
	if v := procEnv["DESKTOP_SESSION"]; v != "" {
		env.DesktopSession = v
	}
}

// applyFallbacks fills missing values using fallback methods.
func applyFallbacks(env *SessionEnv, session *sessionInfo) {
	// DISPLAY fallback
	if env.Display == "" && env.SessionType == "x11" {
		env.Display = findX11DisplayForSession(session.ID, session.Seat)
	}

	// DBUS_SESSION_BUS_ADDRESS fallback
	if env.DBusAddress == "" && env.RuntimeDir != "" {
		busPath := filepath.Join(env.RuntimeDir, "bus")
		if isSocket(busPath) {
			env.DBusAddress = "unix:path=" + busPath
		}
	}

	// XAUTHORITY fallback
	if env.XAuthority == "" {
		env.XAuthority = findXAuthorityForSession(env)
	}
}

// findX11DisplayForSession finds DISPLAY for a session.
func findX11DisplayForSession(sessionID, seat string) string {
	// 1. Try loginctl show-session -p Display
	cmd := execCommand("loginctl", "show-session", sessionID, "-p", "Display", "--value")
	output, err := cmd.Output()
	if err == nil {
		display := strings.TrimSpace(string(output))
		if display != "" {
			return display
		}
	}

	// 2. Scan /tmp/.X11-unix/ sockets
	sockets, _ := filepath.Glob("/tmp/.X11-unix/X*")
	if len(sockets) == 1 {
		return ":" + strings.TrimPrefix(filepath.Base(sockets[0]), "X")
	}

	// 3. Multiple X servers - try to determine by seat
	if seat == "seat0" && fileExists("/tmp/.X11-unix/X0") {
		return ":0"
	}

	// Fallback: first found
	if len(sockets) > 0 {
		return ":" + strings.TrimPrefix(filepath.Base(sockets[0]), "X")
	}

	return ""
}

// findXAuthorityForSession finds XAUTHORITY for a session.
func findXAuthorityForSession(env *SessionEnv) string {
	// 1. RuntimeDir/gdm/Xauthority (GDM on Wayland+XWayland)
	if env.RuntimeDir != "" {
		gdmXauth := filepath.Join(env.RuntimeDir, "gdm", "Xauthority")
		if fileExists(gdmXauth) {
			return gdmXauth
		}
	}

	// 2. Home/.Xauthority (standard)
	if env.Home != "" {
		homeXauth := filepath.Join(env.Home, ".Xauthority")
		if fileExists(homeXauth) {
			return homeXauth
		}
	}

	// 3. Try SDDM location
	if env.RuntimeDir != "" {
		sddmXauth := filepath.Join(env.RuntimeDir, ".Xauthority")
		if fileExists(sddmXauth) {
			return sddmXauth
		}
	}

	return ""
}

// mergeEnv creates a slice of environment variables without duplicates.
// Variables from overrides REPLACE existing values in base.
// Order of overrides is deterministic (sorted by key) for reproducible tests/logs.
func mergeEnv(base []string, overrides map[string]string) []string {
	result := make([]string, 0, len(base)+len(overrides))
	seen := make(map[string]bool, len(base)+len(overrides))

	// First add overrides in deterministic order
	keys := make([]string, 0, len(overrides))
	for k := range overrides {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		result = append(result, k+"="+overrides[k])
		seen[k] = true
	}

	// Then add base variables (skip if key already seen)
	for _, env := range base {
		idx := strings.Index(env, "=")
		var key string
		if idx > 0 {
			key = env[:idx]
		} else {
			key = env
		}
		if !seen[key] {
			result = append(result, env)
			seen[key] = true
		}
	}

	return result
}

// buildEnvOverrides creates environment overrides from SessionEnv.
// Only non-empty values are added to avoid overwriting valid environment.
func buildEnvOverrides(env *SessionEnv) map[string]string {
	overrides := make(map[string]string)
	if env.Home != "" {
		overrides["HOME"] = env.Home
	}
	if env.User != "" {
		overrides["USER"] = env.User
		overrides["LOGNAME"] = env.User
	}
	if env.RuntimeDir != "" {
		overrides["XDG_RUNTIME_DIR"] = env.RuntimeDir
	}
	if env.DBusAddress != "" {
		overrides["DBUS_SESSION_BUS_ADDRESS"] = env.DBusAddress
	}
	if env.Display != "" {
		overrides["DISPLAY"] = env.Display
	}
	if env.XAuthority != "" {
		overrides["XAUTHORITY"] = env.XAuthority
	}
	// XDG_CONFIG_HOME - only if non-empty and absolute path
	if env.XDGConfigHome != "" && filepath.IsAbs(env.XDGConfigHome) {
		overrides["XDG_CONFIG_HOME"] = env.XDGConfigHome
	}
	return overrides
}

// RunAsSessionUser executes a command as the session user.
// Uses exec.Cmd.SysProcAttr.Credential to switch user.
// If env is nil, runs command with current environment.
// Returns ErrNotRoot if env is not nil and current process is not running as root.
// On error, stderr is included in the error message for diagnostics.
func RunAsSessionUser(env *SessionEnv, name string, args ...string) ([]byte, error) {
	cmd := execCommand(name, args...)

	if env != nil {
		// Switching user requires root privileges
		if os.Geteuid() != 0 {
			return nil, ErrNotRoot
		}

		cmd.Env = mergeEnv(os.Environ(), buildEnvOverrides(env))

		// Get supplementary groups for the user
		groups := getUserGroups(env.User, env.GID)

		cmd.SysProcAttr = &syscall.SysProcAttr{
			Credential: &syscall.Credential{
				Uid:    env.UID,
				Gid:    env.GID,
				Groups: groups,
			},
		}
	}

	return runCmdWithStderr(cmd)
}

// runCmdWithStderr runs command and includes stderr in error message for diagnostics.
// On success, returns stdout only. On error, error message includes stderr.
func runCmdWithStderr(cmd *exec.Cmd) ([]byte, error) {
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	output, err := cmd.Output()
	if err != nil {
		stderrStr := strings.TrimSpace(stderr.String())
		if stderrStr != "" {
			return output, fmt.Errorf("%w: %s", err, stderrStr)
		}
	}
	return output, err
}

// RunCommand executes a command, optionally as session user.
// If env is nil, runs with current environment.
// On error, stderr is included in the error message for diagnostics.
func RunCommand(env *SessionEnv, name string, args ...string) ([]byte, error) {
	if env == nil {
		return runCmdWithStderr(execCommand(name, args...))
	}
	return RunAsSessionUser(env, name, args...)
}

// RunGsettings executes gsettings command, optionally as session user.
// Works both from root (using RunAsSessionUser) and from regular user.
func RunGsettings(env *SessionEnv, args ...string) ([]byte, error) {
	return RunCommand(env, "gsettings", args...)
}

// ApplySessionEnv applies session environment to current process.
// Used for X11-specific operations (setxkbmap, xgb).
func ApplySessionEnv(env *SessionEnv) error {
	if env == nil {
		return nil
	}

	if env.Display != "" {
		if err := os.Setenv("DISPLAY", env.Display); err != nil {
			return err
		}
	}
	if env.XAuthority != "" {
		if err := os.Setenv("XAUTHORITY", env.XAuthority); err != nil {
			return err
		}
	}
	if env.DBusAddress != "" {
		if err := os.Setenv("DBUS_SESSION_BUS_ADDRESS", env.DBusAddress); err != nil {
			return err
		}
	}
	if env.RuntimeDir != "" {
		if err := os.Setenv("XDG_RUNTIME_DIR", env.RuntimeDir); err != nil {
			return err
		}
	}

	return nil
}

// fileExists checks if a file exists and is not a directory.
func fileExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return !info.IsDir()
}

// dirExists checks if a directory exists.
func dirExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.IsDir()
}

// isSocket checks if a path is a Unix socket.
func isSocket(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.Mode().Type()&os.ModeSocket != 0
}

// getUserGroups returns supplementary groups for a user.
// Falls back to primary GID only if lookup fails.
func getUserGroups(username string, primaryGID uint32) []uint32 {
	u, err := user.Lookup(username)
	if err != nil {
		return []uint32{primaryGID}
	}

	gids, err := u.GroupIds()
	if err != nil {
		return []uint32{primaryGID}
	}

	groups := make([]uint32, 0, len(gids))
	for _, gidStr := range gids {
		gid, err := strconv.ParseUint(gidStr, 10, 32)
		if err == nil {
			groups = append(groups, uint32(gid))
		}
	}

	if len(groups) == 0 {
		return []uint32{primaryGID}
	}

	return groups
}
