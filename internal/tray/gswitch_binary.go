package tray

import (
	"errors"
	"os/exec"
)

// gswitchSearchPaths contains paths to search for gswitch binary.
// Order matters: more common installation paths first.
var gswitchSearchPaths = []string{
	"/usr/bin/gswitch",
	"/usr/local/bin/gswitch",
}

const gswitchPolkitPath = "/usr/bin/gswitch"

// findGswitchBinary finds the gswitch binary path.
// It first checks common installation paths, then falls back to PATH lookup.
func findGswitchBinary() (string, error) {
	// Check common locations first (more reliable in autostart environments
	// where PATH may be limited)
	for _, p := range gswitchSearchPaths {
		if _, err := exec.LookPath(p); err == nil {
			return p, nil
		}
	}

	// Try to find in PATH
	path, err := exec.LookPath("gswitch")
	if err == nil {
		return path, nil
	}

	return "", errors.New("gswitch not found in system paths")
}

// findGswitchBinaryForPkexec finds gswitch binary path for privileged actions.
// It must match polkit's org.freedesktop.policykit.exec.path exactly.
func findGswitchBinaryForPkexec() (string, error) {
	if _, err := exec.LookPath(gswitchPolkitPath); err == nil {
		return gswitchPolkitPath, nil
	}

	return "", errors.New("/usr/bin/gswitch not found (required for pkexec operations)")
}
