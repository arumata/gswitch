package runtime

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

// setupX11Environment tries to detect and set X11 environment variables
// when running as a systemd service (without DISPLAY set)
func setupX11Environment() {
	// Already have DISPLAY - nothing to do
	if os.Getenv("DISPLAY") != "" {
		return
	}

	// Try to find X11 display from running X server
	display := findX11Display()
	if display != "" {
		os.Setenv("DISPLAY", display)
	}

	// Try to find XAUTHORITY
	if os.Getenv("XAUTHORITY") == "" {
		xauth := findXauthority()
		if xauth != "" {
			os.Setenv("XAUTHORITY", xauth)
		}
	}
}

// x11SocketDir is the directory where X11 sockets are located
const x11SocketDir = "/tmp/.X11-unix"

// maxX11Display is the maximum display number to check
const maxX11Display = 20

// findX11Display tries to detect the X11 display
func findX11Display() string {
	// Check displays :0 through :maxX11Display (covers multi-seat systems)
	for i := range maxX11Display {
		socketPath := fmt.Sprintf("%s/X%d", x11SocketDir, i)
		if _, err := os.Stat(socketPath); err == nil {
			return fmt.Sprintf(":%d", i)
		}
	}
	return ""
}

// findXauthority tries to find a valid XAUTHORITY file
func findXauthority() string {
	// Try to find from running X processes
	if xauth := findXauthorityFromProcess(); xauth != "" {
		return xauth
	}

	// Try to find xauth files in /tmp
	if xauth := findXauthorityInTmp(); xauth != "" {
		return xauth
	}

	// Try ~/.Xauthority in user home directories
	if xauth := findXauthorityInHome(); xauth != "" {
		return xauth
	}

	// Try common paths with dynamic UID detection
	if xauth := findXauthorityByUID(); xauth != "" {
		return xauth
	}

	// Try to find from loginctl sessions
	if xauth := findXauthorityFromSessions(); xauth != "" {
		return xauth
	}

	return ""
}

func findXauthorityFromProcess() string {
	cmd := exec.Command("bash", "-c", "ps aux | grep -E 'Xorg|X11' | grep -v grep | head -1")
	output, err := cmd.Output()
	if err != nil {
		return ""
	}

	line := string(output)
	// Look for -auth flag
	if idx := strings.Index(line, "-auth "); idx != -1 {
		rest := line[idx+6:]
		// Extract path (ends with space or newline)
		fields := strings.Fields(rest)
		if len(fields) > 0 {
			authPath := fields[0]
			if _, err := os.Stat(authPath); err == nil {
				return authPath
			}
		}
	}
	return ""
}

func findXauthorityInTmp() string {
	matches, err := filepath.Glob("/tmp/xauth_*")
	if err != nil || len(matches) == 0 {
		return ""
	}

	// Return the most recently modified one
	var newest string
	var newestTime int64
	for _, m := range matches {
		info, err := os.Stat(m)
		if err == nil && info.ModTime().Unix() > newestTime {
			newestTime = info.ModTime().Unix()
			newest = m
		}
	}
	return newest
}

func findXauthorityInHome() string {
	// Try ~/.Xauthority in user home directories
	homeMatches, err := filepath.Glob("/home/*/.Xauthority")
	if err == nil {
		for _, m := range homeMatches {
			if _, err := os.Stat(m); err == nil {
				return m
			}
		}
	}
	// Also check root's home
	if _, err := os.Stat("/root/.Xauthority"); err == nil {
		return "/root/.Xauthority"
	}
	return ""
}

func findXauthorityByUID() string {
	// Get list of logged-in user UIDs
	uids := getLoggedInUserUIDs()

	// Pre-allocate: 2 paths per UID + 2 static paths
	commonPaths := make([]string, 0, len(uids)*2+2)
	for _, uid := range uids {
		commonPaths = append(commonPaths,
			fmt.Sprintf("/run/user/%s/gdm/Xauthority", uid), // GDM
			fmt.Sprintf("/run/user/%s/.Xauthority", uid),    // Generic
		)
	}
	commonPaths = append(commonPaths,
		"/run/sddm/xauth_*",             // SDDM
		"/var/run/lightdm/*/xauthority", // LightDM
	)

	for _, p := range commonPaths {
		if strings.Contains(p, "*") {
			matches, err := filepath.Glob(p)
			if err == nil && len(matches) > 0 {
				return matches[0]
			}
		} else if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

func findXauthorityFromSessions() string {
	sessions := getGraphicalSessions()
	for _, session := range sessions {
		xauth := getSessionXauthority(session)
		if xauth != "" {
			return xauth
		}
	}
	return ""
}

// getLoggedInUserUIDs returns UIDs of users with active sessions
func getLoggedInUserUIDs() []string {
	cmd := exec.Command("loginctl", "list-users", "--no-legend")
	output, err := cmd.Output()
	if err != nil {
		return []string{"1000"} // fallback
	}

	var uids []string
	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) >= 1 {
			uids = append(uids, fields[0])
		}
	}

	if len(uids) == 0 {
		return []string{"1000"} // fallback
	}
	return uids
}

// getGraphicalSessions returns list of graphical session IDs
func getGraphicalSessions() []string {
	cmd := exec.Command("loginctl", "list-sessions", "--no-legend")
	output, err := cmd.Output()
	if err != nil {
		return nil
	}

	var sessions []string
	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) >= 1 {
			sessions = append(sessions, fields[0])
		}
	}
	return sessions
}

// getSessionXauthority tries to get XAUTHORITY for a session
func getSessionXauthority(sessionID string) string {
	cmd := exec.Command("loginctl", "show-session", sessionID, "-p", "Type", "-p", "Display")
	output, err := cmd.Output()
	if err != nil {
		return ""
	}

	lines := strings.Split(string(output), "\n")
	isGraphical := false
	for _, line := range lines {
		if strings.HasPrefix(line, "Type=x11") || strings.HasPrefix(line, "Type=wayland") {
			isGraphical = true
			break
		}
	}

	if !isGraphical {
		return ""
	}

	// Try to get user's environment
	cmd = exec.Command("loginctl", "show-session", sessionID, "-p", "User")
	output, err = cmd.Output()
	if err != nil {
		return ""
	}

	userLine := strings.TrimSpace(string(output))
	if !strings.HasPrefix(userLine, "User=") {
		return ""
	}
	uid := strings.TrimPrefix(userLine, "User=")

	// Check common xauth locations for this user
	paths := []string{
		fmt.Sprintf("/run/user/%s/gdm/Xauthority", uid),
	}

	// Also check /tmp/xauth_* files owned by this user
	matches, err := filepath.Glob("/tmp/xauth_*")
	if err == nil {
		for _, m := range matches {
			if xauthPath := matchXauthorityByUID(m, uid); xauthPath != "" {
				paths = append([]string{xauthPath}, paths...)
			}
		}
	}

	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}

	return ""
}

// matchXauthorityByUID returns the path if the file is owned by the given UID
func matchXauthorityByUID(path, uid string) string {
	info, err := os.Stat(path)
	if err != nil {
		return ""
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return ""
	}
	if strconv.FormatUint(uint64(stat.Uid), 10) == uid {
		return path
	}
	return ""
}
