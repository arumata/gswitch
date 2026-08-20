package detect

import (
	"errors"
	"os"
	"os/exec"
	"slices"
	"strings"
	"testing"
)

func TestMergeEnv(t *testing.T) {
	tests := []struct {
		name      string
		base      []string
		overrides map[string]string
		wantLen   int
		checkVars map[string]string
	}{
		{
			name:      "empty base and overrides",
			base:      []string{},
			overrides: map[string]string{},
			wantLen:   0,
			checkVars: nil,
		},
		{
			name: "overrides replace base",
			base: []string{"HOME=/root", "USER=root"},
			overrides: map[string]string{
				"HOME": "/home/testuser",
				"USER": "testuser",
			},
			wantLen: 2,
			checkVars: map[string]string{
				"HOME": "/home/testuser",
				"USER": "testuser",
			},
		},
		{
			name: "base variables preserved if not overridden",
			base: []string{"PATH=/usr/bin", "LANG=en_US.UTF-8"},
			overrides: map[string]string{
				"HOME": "/home/testuser",
			},
			wantLen: 3,
			checkVars: map[string]string{
				"HOME": "/home/testuser",
				"PATH": "/usr/bin",
				"LANG": "en_US.UTF-8",
			},
		},
		{
			name: "duplicates in base are deduplicated",
			base: []string{"HOME=/first", "USER=user1", "HOME=/second"},
			overrides: map[string]string{
				"NEW": "value",
			},
			wantLen: 3, // NEW, USER, HOME (first occurrence)
			checkVars: map[string]string{
				"NEW":  "value",
				"USER": "user1",
			},
		},
		{
			name: "overrides take priority over base duplicates",
			base: []string{"HOME=/first", "HOME=/second"},
			overrides: map[string]string{
				"HOME": "/override",
			},
			wantLen: 1,
			checkVars: map[string]string{
				"HOME": "/override",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := mergeEnv(tt.base, tt.overrides)

			if len(result) != tt.wantLen {
				t.Errorf("mergeEnv() returned %d vars, want %d. Got: %v", len(result), tt.wantLen, result)
			}

			// Check specific variables
			envMap := make(map[string]string)
			for _, e := range result {
				for i := range len(e) {
					if e[i] == '=' {
						envMap[e[:i]] = e[i+1:]
						break
					}
				}
			}

			for k, want := range tt.checkVars {
				if got := envMap[k]; got != want {
					t.Errorf("mergeEnv() %s = %q, want %q", k, got, want)
				}
			}
		})
	}
}

func TestMergeEnvDeterministicOrder(t *testing.T) {
	base := []string{"Z=1", "A=2"}
	overrides := map[string]string{
		"C": "3",
		"B": "4",
		"D": "5",
	}

	result1 := mergeEnv(base, overrides)
	result2 := mergeEnv(base, overrides)

	if len(result1) != len(result2) {
		t.Fatalf("mergeEnv() non-deterministic length: %d vs %d", len(result1), len(result2))
	}

	for i := range result1 {
		if result1[i] != result2[i] {
			t.Errorf("mergeEnv() non-deterministic at index %d: %q vs %q", i, result1[i], result2[i])
		}
	}

	// Check that overrides come first in sorted order
	if len(result1) >= 3 {
		if result1[0] != "B=4" || result1[1] != "C=3" || result1[2] != "D=5" {
			t.Errorf("mergeEnv() overrides not in sorted order: %v", result1[:3])
		}
	}
}

func TestApplySessionEnv(t *testing.T) {
	// Save original env
	origDisplay := os.Getenv("DISPLAY")
	origXAuth := os.Getenv("XAUTHORITY")
	origDBus := os.Getenv("DBUS_SESSION_BUS_ADDRESS")
	origRuntime := os.Getenv("XDG_RUNTIME_DIR")

	defer func() {
		// Restore original env
		os.Setenv("DISPLAY", origDisplay)
		os.Setenv("XAUTHORITY", origXAuth)
		os.Setenv("DBUS_SESSION_BUS_ADDRESS", origDBus)
		os.Setenv("XDG_RUNTIME_DIR", origRuntime)
	}()

	t.Run("nil env does nothing", func(t *testing.T) {
		err := ApplySessionEnv(nil)
		if err != nil {
			t.Errorf("ApplySessionEnv(nil) returned error: %v", err)
		}
	})

	t.Run("applies environment variables", func(t *testing.T) {
		env := &SessionEnv{
			Display:     ":99",
			XAuthority:  "/tmp/test-xauth",
			DBusAddress: "unix:path=/tmp/test-bus",
			RuntimeDir:  "/run/user/9999",
		}

		err := ApplySessionEnv(env)
		if err != nil {
			t.Errorf("ApplySessionEnv() returned error: %v", err)
		}

		if got := os.Getenv("DISPLAY"); got != ":99" {
			t.Errorf("DISPLAY = %q, want %q", got, ":99")
		}
		if got := os.Getenv("XAUTHORITY"); got != "/tmp/test-xauth" {
			t.Errorf("XAUTHORITY = %q, want %q", got, "/tmp/test-xauth")
		}
		if got := os.Getenv("DBUS_SESSION_BUS_ADDRESS"); got != "unix:path=/tmp/test-bus" {
			t.Errorf("DBUS_SESSION_BUS_ADDRESS = %q, want %q", got, "unix:path=/tmp/test-bus")
		}
		if got := os.Getenv("XDG_RUNTIME_DIR"); got != "/run/user/9999" {
			t.Errorf("XDG_RUNTIME_DIR = %q, want %q", got, "/run/user/9999")
		}
	})

	t.Run("empty fields are not set", func(t *testing.T) {
		// Reset env
		os.Setenv("DISPLAY", "original")

		env := &SessionEnv{
			Display: "", // empty
		}

		err := ApplySessionEnv(env)
		if err != nil {
			t.Errorf("ApplySessionEnv() returned error: %v", err)
		}

		// DISPLAY should remain unchanged
		if got := os.Getenv("DISPLAY"); got != "original" {
			t.Errorf("DISPLAY = %q, want %q (should be unchanged)", got, "original")
		}
	})
}

func TestSessionEnvStructure(t *testing.T) {
	// Test that SessionEnv has all required fields
	env := SessionEnv{
		UID:           1000,
		GID:           1000,
		User:          "testuser",
		Home:          "/home/testuser",
		Display:       ":0",
		XAuthority:    "/home/testuser/.Xauthority",
		DBusAddress:   "unix:path=/run/user/1000/bus",
		RuntimeDir:    "/run/user/1000",
		SessionType:   "x11",
		XDGConfigHome: "/home/testuser/.config",
		IBusAddress:   "unix:path=/run/user/1000/ibus/bus",
	}

	if env.UID != 1000 {
		t.Errorf("UID = %d, want 1000", env.UID)
	}
	if env.GID != 1000 {
		t.Errorf("GID = %d, want 1000", env.GID)
	}
	if env.User != "testuser" {
		t.Errorf("User = %q, want %q", env.User, "testuser")
	}
	if env.Home != "/home/testuser" {
		t.Errorf("Home = %q, want %q", env.Home, "/home/testuser")
	}
	if env.SessionType != "x11" {
		t.Errorf("SessionType = %q, want %q", env.SessionType, "x11")
	}
	if env.XDGConfigHome != "/home/testuser/.config" {
		t.Errorf("XDGConfigHome = %q, want %q", env.XDGConfigHome, "/home/testuser/.config")
	}
	if env.IBusAddress == "" {
		t.Error("IBusAddress should be set")
	}
}

func TestDisplayManagerUsers(t *testing.T) {
	// Test that common display managers are filtered
	dmUsers := []string{"gdm", "sddm", "lightdm", "root", "lxdm", "greetd"}

	for _, user := range dmUsers {
		if !displayManagerUsers[user] {
			t.Errorf("displayManagerUsers[%q] = false, want true", user)
		}
	}

	// Regular users should not be in the list
	regularUsers := []string{"sergey", "ubuntu", "user", "admin", "guest"}
	for _, user := range regularUsers {
		if displayManagerUsers[user] {
			t.Errorf("displayManagerUsers[%q] = true, want false", user)
		}
	}
}

func TestSelectBestSession(t *testing.T) {
	tests := []struct {
		name     string
		sessions []sessionInfo
		wantID   string
		wantNil  bool
	}{
		{
			name:     "empty sessions returns nil",
			sessions: []sessionInfo{},
			wantNil:  true,
		},
		{
			name: "single session is selected",
			sessions: []sessionInfo{
				{ID: "1", Active: true, Seat: "seat0"},
			},
			wantID: "1",
		},
		{
			name: "Active=yes preferred over inactive seat0",
			sessions: []sessionInfo{
				{ID: "1", Active: false, Seat: "seat0"},
				{ID: "2", Active: true, Seat: "seat1"},
			},
			wantID: "2",
		},
		{
			name: "seat0 preferred among Active=yes sessions",
			sessions: []sessionInfo{
				{ID: "1", Active: true, Seat: "seat1"},
				{ID: "2", Active: true, Seat: "seat0"},
			},
			wantID: "2",
		},
		{
			name: "Active=yes preferred over State=active seat0",
			sessions: []sessionInfo{
				{ID: "1", Active: false, State: "active", Seat: "seat0"},
				{ID: "2", Active: true, Seat: "seat1"},
			},
			wantID: "2",
		},
		{
			name: "State=active preferred over other seat0",
			sessions: []sessionInfo{
				{ID: "1", Active: false, State: "online", Seat: "seat0"},
				{ID: "2", Active: false, State: "active", Seat: "seat1"},
			},
			wantID: "2",
		},
		{
			name: "seat0 preferred among State=active sessions",
			sessions: []sessionInfo{
				{ID: "1", Active: false, State: "active", Seat: "seat1"},
				{ID: "2", Active: false, State: "active", Seat: "seat0"},
			},
			wantID: "2",
		},
		{
			name: "first found when no seat0",
			sessions: []sessionInfo{
				{ID: "1", Active: true, Seat: "seat1"},
				{ID: "2", Active: true, Seat: "seat2"},
			},
			wantID: "1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := selectBestSession(tt.sessions)

			if tt.wantNil {
				if result != nil {
					t.Errorf("selectBestSession() = %v, want nil", result)
				}
				return
			}

			if result == nil {
				t.Fatal("selectBestSession() returned nil, want non-nil")
			}

			if result.ID != tt.wantID {
				t.Errorf("selectBestSession().ID = %q, want %q", result.ID, tt.wantID)
			}
		})
	}
}

func TestFileExists(t *testing.T) {
	// Test with a file that should exist
	if !fileExists("/etc/passwd") {
		t.Error("fileExists(/etc/passwd) = false, want true")
	}

	// Test with a non-existent file
	if fileExists("/nonexistent/file/path") {
		t.Error("fileExists(/nonexistent/file/path) = true, want false")
	}

	// Test with a directory (should return false)
	if fileExists("/tmp") {
		t.Error("fileExists(/tmp) = true, want false (it's a directory)")
	}
}

func TestDirExists(t *testing.T) {
	// Test with a directory that should exist
	if !dirExists("/tmp") {
		t.Error("dirExists(/tmp) = false, want true")
	}

	// Test with a non-existent directory
	if dirExists("/nonexistent/directory/path") {
		t.Error("dirExists(/nonexistent/directory/path) = true, want false")
	}

	// Test with a file (should return false)
	if dirExists("/etc/passwd") {
		t.Error("dirExists(/etc/passwd) = true, want false (it's a file)")
	}
}

func TestErrors(t *testing.T) {
	// Test that errors are properly defined
	if ErrNoActiveSession.Error() != "no active graphical user session found" {
		t.Errorf("ErrNoActiveSession.Error() = %q, want %q",
			ErrNoActiveSession.Error(), "no active graphical user session found")
	}

	if ErrNoSystemd.Error() != "loginctl not found: root auto-detection requires systemd" {
		t.Errorf("ErrNoSystemd.Error() = %q, want %q",
			ErrNoSystemd.Error(), "loginctl not found: root auto-detection requires systemd")
	}
}

func TestRunCommandWithNilEnv(t *testing.T) {
	// Test that RunCommand works with nil env (uses current environment)
	output, err := RunCommand(nil, "echo", "test")
	if err != nil {
		t.Errorf("RunCommand(nil, echo) returned error: %v", err)
	}

	expected := "test\n"
	if string(output) != expected {
		t.Errorf("RunCommand(nil, echo) = %q, want %q", string(output), expected)
	}
}

func TestRunGsettingsWithNilEnv(t *testing.T) {
	// Test that RunGsettings works with nil env
	// This test will fail if gsettings is not installed, which is acceptable
	_, err := RunGsettings(nil, "--version")
	if err != nil {
		// It's OK if gsettings is not installed
		t.Logf("RunGsettings(nil) returned error (gsettings may not be installed): %v", err)
	}
}

func TestGetUserGroups(t *testing.T) {
	// Test with a user that should exist (root is always present)
	groups := getUserGroups("root", 0)
	if len(groups) == 0 {
		t.Error("getUserGroups(root) returned empty slice")
	}

	// Test with non-existent user falls back to primary GID
	groups = getUserGroups("nonexistent_user_12345", 9999)
	if len(groups) != 1 || groups[0] != 9999 {
		t.Errorf("getUserGroups(nonexistent) = %v, want [9999]", groups)
	}

	// Test that primary GID is included in groups for existing user
	groups = getUserGroups("root", 0)
	if !slices.Contains(groups, uint32(0)) {
		t.Errorf("getUserGroups(root) should contain primary GID 0, got %v", groups)
	}
}

// TestSessionInfoFiltering tests that session filtering works correctly with mock data.
func TestSessionInfoFiltering(t *testing.T) {
	// Simulate sessions that would come from loginctl
	mockSessions := []sessionInfo{
		// Should be filtered: Type is tty
		{ID: "1", Type: "tty", Class: "user", User: "sergey", Active: true},
		// Should be filtered: Class is greeter
		{ID: "2", Type: "x11", Class: "greeter", User: "sergey", Active: true},
		// Should be filtered: User is gdm
		{ID: "3", Type: "x11", Class: "user", User: "gdm", Active: true},
		// Should pass: valid x11 session
		{ID: "4", Type: "x11", Class: "user", User: "sergey", Active: true, Seat: "seat0"},
		// Should pass: valid wayland session
		{ID: "5", Type: "wayland", Class: "user", User: "ubuntu", Active: false, State: "online"},
	}

	// Filter sessions like listSessions() does
	filtered := make([]sessionInfo, 0, len(mockSessions))
	for _, s := range mockSessions {
		if s.Type != "x11" && s.Type != "wayland" {
			continue
		}
		if s.Class != "user" {
			continue
		}
		if displayManagerUsers[s.User] {
			continue
		}
		filtered = append(filtered, s)
	}

	if len(filtered) != 2 {
		t.Errorf("Filtered sessions count = %d, want 2. Got: %v", len(filtered), filtered)
	}

	// Check that session 4 and 5 are in the result
	foundIDs := make(map[string]bool)
	for _, s := range filtered {
		foundIDs[s.ID] = true
	}

	if !foundIDs["4"] {
		t.Error("Session 4 (valid x11) should be in filtered list")
	}
	if !foundIDs["5"] {
		t.Error("Session 5 (valid wayland) should be in filtered list")
	}
}

// TestBuildEnvOverrides tests that buildEnvOverrides only includes non-empty values.
func TestBuildEnvOverrides(t *testing.T) {
	tests := []struct {
		name     string
		env      *SessionEnv
		wantKeys []string
		notWant  []string
	}{
		{
			name: "all fields set",
			env: &SessionEnv{
				Home:        "/home/test",
				User:        "test",
				RuntimeDir:  "/run/user/1000",
				DBusAddress: "unix:path=/run/user/1000/bus",
				Display:     ":0",
				XAuthority:  "/home/test/.Xauthority",
			},
			wantKeys: []string{"HOME", "USER", "LOGNAME", "XDG_RUNTIME_DIR", "DBUS_SESSION_BUS_ADDRESS", "DISPLAY", "XAUTHORITY"},
		},
		{
			name: "empty display not included",
			env: &SessionEnv{
				Home: "/home/test",
				User: "test",
			},
			wantKeys: []string{"HOME", "USER", "LOGNAME"},
			notWant:  []string{"DISPLAY", "XAUTHORITY", "XDG_RUNTIME_DIR", "DBUS_SESSION_BUS_ADDRESS"},
		},
		{
			name: "XDG_CONFIG_HOME only if absolute",
			env: &SessionEnv{
				Home:          "/home/test",
				User:          "test",
				XDGConfigHome: "/home/test/.config",
			},
			wantKeys: []string{"HOME", "USER", "XDG_CONFIG_HOME"},
		},
		{
			name: "XDG_CONFIG_HOME skipped if relative",
			env: &SessionEnv{
				Home:          "/home/test",
				User:          "test",
				XDGConfigHome: ".config",
			},
			wantKeys: []string{"HOME", "USER"},
			notWant:  []string{"XDG_CONFIG_HOME"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildEnvOverrides(tt.env)

			for _, key := range tt.wantKeys {
				if _, ok := result[key]; !ok {
					t.Errorf("buildEnvOverrides() missing key %q", key)
				}
			}

			for _, key := range tt.notWant {
				if _, ok := result[key]; ok {
					t.Errorf("buildEnvOverrides() should not have key %q", key)
				}
			}
		})
	}
}

// TestFindX11DisplayForSessionFallback tests display detection fallback logic.
func TestFindX11DisplayForSessionFallback(t *testing.T) {
	// Test seat0 fallback when X0 socket exists
	if fileExists("/tmp/.X11-unix/X0") {
		display := findX11DisplayForSession("nonexistent-session", "seat0")
		// Should find :0 via socket scan or seat0 logic
		if display != ":0" {
			t.Logf("findX11DisplayForSession returned %q (expected :0 via seat0 fallback)", display)
		}
	}

	// Test with empty session and seat - should still find display via socket scan
	if fileExists("/tmp/.X11-unix/X0") {
		display := findX11DisplayForSession("", "")
		if display == "" {
			t.Log("findX11DisplayForSession returned empty (expected to find via socket scan)")
		}
	}
}

// TestFindXAuthorityForSessionFallback tests Xauthority detection fallback logic.
func TestFindXAuthorityForSessionFallback(t *testing.T) {
	// Test with non-existent paths - should return empty string
	env := &SessionEnv{
		RuntimeDir: "/nonexistent/path",
		Home:       "/nonexistent/home",
	}
	xauth := findXAuthorityForSession(env)
	if xauth != "" {
		t.Errorf("findXAuthorityForSession with invalid paths returned %q, want empty", xauth)
	}

	// Test with valid home directory (current user's home)
	homeDir, err := os.UserHomeDir()
	if err == nil {
		env := &SessionEnv{
			Home: homeDir,
		}
		xauth := findXAuthorityForSession(env)
		// May or may not find .Xauthority depending on the system
		t.Logf("findXAuthorityForSession(home=%s) returned %q", homeDir, xauth)
	}
}

// TestPriorityCorrectness verifies the exact priority order from spec.
func TestPriorityCorrectness(t *testing.T) {
	// This is the critical test case from the review:
	// Active=yes seat1 should be chosen over State=active seat0
	sessions := []sessionInfo{
		{ID: "state-active-seat0", Active: false, State: "active", Seat: "seat0"},
		{ID: "active-yes-seat1", Active: true, State: "", Seat: "seat1"},
	}

	result := selectBestSession(sessions)
	if result == nil {
		t.Fatal("selectBestSession returned nil")
	}

	if result.ID != "active-yes-seat1" {
		t.Errorf("Priority violation: got %q, want 'active-yes-seat1' (Active=yes > State=active)", result.ID)
	}
}

// TestGetActiveSessionEnvNoSystemd tests behavior when loginctl is not available.
func TestGetActiveSessionEnvNoSystemd(t *testing.T) {
	// Save original functions
	origLookPath := execLookPath
	defer func() { execLookPath = origLookPath }()

	// Mock execLookPath to return error (loginctl not found)
	execLookPath = func(file string) (string, error) {
		if file == "loginctl" {
			return "", errors.New("executable file not found in $PATH")
		}
		return exec.LookPath(file)
	}

	_, err := GetActiveSessionEnv()
	if !errors.Is(err, ErrNoSystemd) {
		t.Errorf("GetActiveSessionEnv() error = %v, want ErrNoSystemd", err)
	}
}

// TestRunAsSessionUserErrNotRoot tests that RunAsSessionUser returns ErrNotRoot for non-root.
func TestRunAsSessionUserErrNotRoot(t *testing.T) {
	// Skip if running as root
	if os.Geteuid() == 0 {
		t.Skip("Test requires non-root user")
	}

	env := &SessionEnv{
		UID:  1000,
		GID:  1000,
		User: "testuser",
		Home: "/home/testuser",
	}

	_, err := RunAsSessionUser(env, "echo", "test")
	if !errors.Is(err, ErrNotRoot) {
		t.Errorf("RunAsSessionUser() error = %v, want ErrNotRoot", err)
	}
}

// TestRunAsSessionUserNilEnv tests that RunAsSessionUser works with nil env.
func TestRunAsSessionUserNilEnv(t *testing.T) {
	output, err := RunAsSessionUser(nil, "echo", "test")
	if err != nil {
		t.Errorf("RunAsSessionUser(nil) error = %v", err)
	}
	if string(output) != "test\n" {
		t.Errorf("RunAsSessionUser(nil) output = %q, want %q", string(output), "test\n")
	}
}

// TestErrorMessages tests that error messages are correct.
func TestErrorMessages(t *testing.T) {
	tests := []struct {
		err  error
		want string
	}{
		{ErrNoActiveSession, "no active graphical user session found"},
		{ErrNoSystemd, "loginctl not found: root auto-detection requires systemd"},
		{ErrNotRoot, "RunAsSessionUser with env requires root privileges"},
	}

	for _, tt := range tests {
		if tt.err.Error() != tt.want {
			t.Errorf("%v.Error() = %q, want %q", tt.err, tt.err.Error(), tt.want)
		}
	}
}

// setupMockExec sets up mock functions and returns a cleanup function.
func setupMockExec() func() {
	origExecCommand := execCommand
	origExecLookPath := execLookPath
	execLookPath = func(file string) (string, error) {
		return "/usr/bin/" + file, nil
	}
	return func() {
		execCommand = origExecCommand
		execLookPath = origExecLookPath
	}
}

// TestMockedLoginctlX11Session tests GetActiveSessionEnv with a single x11 session.
func TestMockedLoginctlX11Session(t *testing.T) {
	cleanup := setupMockExec()
	defer cleanup()

	execCommand = func(name string, args ...string) *exec.Cmd {
		switch {
		case name == "loginctl" && len(args) >= 1 && args[0] == "list-sessions":
			return exec.Command("echo", "-n", "    42 1000 sergey seat0")
		case name == "loginctl" && len(args) >= 2 && args[0] == "show-session" && args[1] == "42":
			return exec.Command("echo", "-n", "Type=x11\nClass=user\nActive=yes\nState=active\nSeat=seat0\nDisplay=:0\nUser=1000\nName=sergey\nLeader=12345")
		case name == "getent":
			return exec.Command("echo", "-n", "sergey:x:1000:1000:Sergey:/home/sergey:/bin/bash")
		case name == "loginctl" && len(args) >= 2 && args[0] == "show-user":
			return exec.Command("echo", "-n", "/run/user/1000")
		default:
			return exec.Command("echo", "-n", "")
		}
	}

	env, err := GetActiveSessionEnv()
	if err != nil {
		t.Fatalf("GetActiveSessionEnv() error = %v", err)
	}

	if env.UID != 1000 {
		t.Errorf("UID = %d, want 1000", env.UID)
	}
	if env.User != "sergey" {
		t.Errorf("User = %q, want %q", env.User, "sergey")
	}
	if env.SessionType != "x11" {
		t.Errorf("SessionType = %q, want %q", env.SessionType, "x11")
	}
	if env.Display != ":0" {
		t.Errorf("Display = %q, want %q", env.Display, ":0")
	}
	if env.Home != "/home/sergey" {
		t.Errorf("Home = %q, want %q", env.Home, "/home/sergey")
	}
}

// TestMockedLoginctlWaylandSession tests GetActiveSessionEnv with a wayland session.
func TestMockedLoginctlWaylandSession(t *testing.T) {
	cleanup := setupMockExec()
	defer cleanup()

	execCommand = func(name string, args ...string) *exec.Cmd {
		switch {
		case name == "loginctl" && len(args) >= 1 && args[0] == "list-sessions":
			return exec.Command("echo", "-n", "    5 1000 ubuntu seat0")
		case name == "loginctl" && len(args) >= 2 && args[0] == "show-session" && args[1] == "5":
			return exec.Command("echo", "-n", "Type=wayland\nClass=user\nActive=yes\nState=active\nSeat=seat0\nDisplay=\nUser=1000\nName=ubuntu\nLeader=54321")
		case name == "getent":
			return exec.Command("echo", "-n", "ubuntu:x:1000:1000:Ubuntu:/home/ubuntu:/bin/bash")
		case name == "loginctl" && len(args) >= 2 && args[0] == "show-user":
			return exec.Command("echo", "-n", "/run/user/1000")
		default:
			return exec.Command("echo", "-n", "")
		}
	}

	env, err := GetActiveSessionEnv()
	if err != nil {
		t.Fatalf("GetActiveSessionEnv() error = %v", err)
	}

	if env.SessionType != "wayland" {
		t.Errorf("SessionType = %q, want %q", env.SessionType, "wayland")
	}
}

// TestMockedLoginctlNoSessions tests that ErrNoActiveSession is returned when no sessions.
func TestMockedLoginctlNoSessions(t *testing.T) {
	cleanup := setupMockExec()
	defer cleanup()

	execCommand = func(_ string, _ ...string) *exec.Cmd {
		return exec.Command("echo", "-n", "")
	}

	_, err := GetActiveSessionEnv()
	if !errors.Is(err, ErrNoActiveSession) {
		t.Errorf("GetActiveSessionEnv() error = %v, want ErrNoActiveSession", err)
	}
}

// TestMockedLoginctlFiltersTTY tests that tty sessions are filtered.
func TestMockedLoginctlFiltersTTY(t *testing.T) {
	cleanup := setupMockExec()
	defer cleanup()

	execCommand = func(name string, args ...string) *exec.Cmd {
		switch {
		case name == "loginctl" && len(args) >= 1 && args[0] == "list-sessions":
			return exec.Command("echo", "-n", "    1 1000 sergey seat0")
		case name == "loginctl" && len(args) >= 2 && args[0] == "show-session":
			return exec.Command("echo", "-n", "Type=tty\nClass=user\nActive=yes\nState=active\nSeat=seat0\nUser=1000\nName=sergey\nLeader=12345")
		default:
			return exec.Command("echo", "-n", "")
		}
	}

	_, err := GetActiveSessionEnv()
	if !errors.Is(err, ErrNoActiveSession) {
		t.Errorf("GetActiveSessionEnv() error = %v, want ErrNoActiveSession (tty should be filtered)", err)
	}
}

// TestMockedLoginctlFiltersDM tests that display manager users are filtered.
func TestMockedLoginctlFiltersDM(t *testing.T) {
	cleanup := setupMockExec()
	defer cleanup()

	execCommand = func(name string, args ...string) *exec.Cmd {
		switch {
		case name == "loginctl" && len(args) >= 1 && args[0] == "list-sessions":
			return exec.Command("echo", "-n", "    c1 120 gdm seat0")
		case name == "loginctl" && len(args) >= 2 && args[0] == "show-session":
			return exec.Command("echo", "-n", "Type=wayland\nClass=user\nActive=yes\nState=active\nSeat=seat0\nUser=120\nName=gdm\nLeader=999")
		default:
			return exec.Command("echo", "-n", "")
		}
	}

	_, err := GetActiveSessionEnv()
	if !errors.Is(err, ErrNoActiveSession) {
		t.Errorf("GetActiveSessionEnv() error = %v, want ErrNoActiveSession (gdm should be filtered)", err)
	}
}

// TestMockedLoginctlFiltersGreeter tests that greeter class sessions are filtered.
func TestMockedLoginctlFiltersGreeter(t *testing.T) {
	cleanup := setupMockExec()
	defer cleanup()

	execCommand = func(name string, args ...string) *exec.Cmd {
		switch {
		case name == "loginctl" && len(args) >= 1 && args[0] == "list-sessions":
			return exec.Command("echo", "-n", "    c2 1000 sergey seat0")
		case name == "loginctl" && len(args) >= 2 && args[0] == "show-session":
			return exec.Command("echo", "-n", "Type=x11\nClass=greeter\nActive=yes\nState=active\nSeat=seat0\nUser=1000\nName=sergey\nLeader=12345")
		default:
			return exec.Command("echo", "-n", "")
		}
	}

	_, err := GetActiveSessionEnv()
	if !errors.Is(err, ErrNoActiveSession) {
		t.Errorf("GetActiveSessionEnv() error = %v, want ErrNoActiveSession (greeter should be filtered)", err)
	}
}

// TestMockedLoginctlPriority tests that Active=yes takes priority over State=active.
func TestMockedLoginctlPriority(t *testing.T) {
	cleanup := setupMockExec()
	defer cleanup()

	execCommand = func(name string, args ...string) *exec.Cmd {
		switch {
		case name == "loginctl" && len(args) >= 1 && args[0] == "list-sessions":
			return exec.Command("echo", "-n", "    10 1000 user1 seat0\n    20 1001 user2 seat1")
		case name == "loginctl" && len(args) >= 2 && args[0] == "show-session" && args[1] == "10":
			return exec.Command("echo", "-n", "Type=x11\nClass=user\nActive=no\nState=active\nSeat=seat0\nDisplay=:0\nUser=1000\nName=user1\nLeader=1000")
		case name == "loginctl" && len(args) >= 2 && args[0] == "show-session" && args[1] == "20":
			return exec.Command("echo", "-n", "Type=x11\nClass=user\nActive=yes\nState=online\nSeat=seat1\nDisplay=:1\nUser=1001\nName=user2\nLeader=2000")
		case name == "getent" && len(args) >= 2 && args[1] == "user2":
			return exec.Command("echo", "-n", "user2:x:1001:1001:User2:/home/user2:/bin/bash")
		case name == "loginctl" && len(args) >= 2 && args[0] == "show-user":
			return exec.Command("echo", "-n", "/run/user/1001")
		default:
			return exec.Command("echo", "-n", "")
		}
	}

	env, err := GetActiveSessionEnv()
	if err != nil {
		t.Fatalf("GetActiveSessionEnv() error = %v", err)
	}

	if env.User != "user2" {
		t.Errorf("User = %q, want %q (Active=yes should win over State=active)", env.User, "user2")
	}
}

// TestListSessionsError tests error handling when loginctl list-sessions fails.
func TestListSessionsError(t *testing.T) {
	origExecCommand := execCommand
	origExecLookPath := execLookPath
	defer func() {
		execCommand = origExecCommand
		execLookPath = origExecLookPath
	}()

	execLookPath = func(file string) (string, error) {
		return "/usr/bin/" + file, nil
	}

	execCommand = func(name string, args ...string) *exec.Cmd {
		if name == "loginctl" && len(args) >= 1 && args[0] == "list-sessions" {
			return exec.Command("sh", "-c", "exit 1")
		}
		return exec.Command("echo", "-n", "")
	}

	_, err := GetActiveSessionEnv()
	if err == nil {
		t.Error("GetActiveSessionEnv() should return error when loginctl fails")
	}
}

// TestRunCmdWithStderrIncludesStderr tests that stderr is included in error messages.
func TestRunCmdWithStderrIncludesStderr(t *testing.T) {
	cmd := exec.Command("sh", "-c", "echo 'error message' >&2; exit 1")
	_, err := runCmdWithStderr(cmd)

	if err == nil {
		t.Fatal("runCmdWithStderr should return error")
	}

	errStr := err.Error()
	if !strings.Contains(errStr, "error message") {
		t.Errorf("runCmdWithStderr error should include stderr, got: %v", err)
	}
}
