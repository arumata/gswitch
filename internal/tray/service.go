package tray

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// ServiceStatus represents the status of a systemd service.
type ServiceStatus int

const (
	StatusUnknown ServiceStatus = iota
	StatusRunning
	StatusStopped
	StatusFailed
	StatusNotInstalled
)

// ErrUserCanceled is retained for UI compatibility with older service
// managers. User-service actions do not require authentication.
var ErrUserCanceled = errors.New("authentication canceled by user")

// ServiceManager manages a systemd service.
type ServiceManager struct {
	serviceName string
}

// NewServiceManager creates a new service manager.
func NewServiceManager(serviceName string) *ServiceManager {
	return &ServiceManager{
		serviceName: serviceName,
	}
}

// GetStatus returns the current status of the service.
func (sm *ServiceManager) GetStatus() (ServiceStatus, error) {
	installed, err := sm.isInstalled()
	if err != nil {
		return StatusUnknown, err
	}
	if !installed {
		return StatusNotInstalled, nil
	}

	// #nosec G204 - serviceName is not user-controlled input
	cmd := sm.command("is-active")
	output, err := cmd.CombinedOutput()

	// systemctl is-active returns non-zero for inactive/failed, so we ignore exit error
	// and just parse the output
	result := strings.TrimSpace(string(output))

	switch result {
	case "active":
		return StatusRunning, nil
	case "inactive":
		return StatusStopped, nil
	case "failed":
		return StatusFailed, nil
	default:
		if err != nil {
			return StatusUnknown, fmt.Errorf("failed to read service status: %w", err)
		}
		return StatusUnknown, nil
	}
}

func (sm *ServiceManager) isInstalled() (bool, error) {
	// #nosec G204 - serviceName is not user-controlled input
	cmd := sm.command("cat")
	output, err := cmd.CombinedOutput()
	if err == nil {
		return true, nil
	}

	out := strings.ToLower(strings.TrimSpace(string(output)))
	if isUnitNotFoundOutput(out) {
		return false, nil
	}

	return false, fmt.Errorf("failed to query service definition: %w", err)
}

func isUnitNotFoundOutput(out string) bool {
	return strings.Contains(out, "no files found for") ||
		strings.Contains(out, "could not be found") ||
		strings.Contains(out, "not-found")
}

// IsEnabled returns true if the service is enabled to start at boot.
func (sm *ServiceManager) IsEnabled() (bool, error) {
	// #nosec G204 - serviceName is not user-controlled input
	cmd := sm.command("is-enabled")
	output, _ := cmd.Output()

	result := strings.TrimSpace(string(output))

	// "enabled" means the service is enabled
	// "disabled" means not enabled
	// error exit code is returned for disabled and not-found, so we check output
	switch result {
	case "enabled":
		return true, nil
	case "disabled", "masked", "static":
		return false, nil
	default:
		return false, nil
	}
}

// Restart restarts the current user's service.
func (sm *ServiceManager) Restart() error {
	return sm.run("restart")
}

// Start starts the current user's service.
func (sm *ServiceManager) Start() error {
	return sm.run("start")
}

// Stop stops the current user's service.
func (sm *ServiceManager) Stop() error {
	return sm.run("stop")
}

// Enable enables the service for this user's graphical sessions.
func (sm *ServiceManager) Enable() error {
	return sm.run("enable")
}

// Disable disables the service for this user.
func (sm *ServiceManager) Disable() error {
	return sm.run("disable")
}

func (sm *ServiceManager) run(action string) error {
	output, err := sm.command(action).CombinedOutput()

	if err == nil {
		return nil
	}

	outputStr := strings.TrimSpace(string(output))
	if outputStr != "" {
		return fmt.Errorf("systemctl --user %s failed: %s", action, outputStr)
	}
	return fmt.Errorf("systemctl --user %s failed: %w", action, err)
}

func (sm *ServiceManager) command(action string) *exec.Cmd {
	// #nosec G204 - serviceName is fixed by the application and action is
	// selected by internal callers.
	return exec.Command("systemctl", "--user", action, sm.serviceName)
}

// String returns a localized string representation of the service status.
func (s ServiceStatus) String() string {
	switch s {
	case StatusRunning:
		return strStatusRunning
	case StatusStopped:
		return strStatusStopped
	case StatusFailed:
		return strStatusFailed
	case StatusNotInstalled:
		return strStatusNotInstalled
	default:
		return strStatusUnknown
	}
}

// StatusColor returns the color name for the status indicator.
func (s ServiceStatus) StatusColor() string {
	switch s {
	case StatusRunning:
		return "green"
	case StatusStopped:
		return "gray"
	case StatusFailed:
		return "red"
	case StatusNotInstalled:
		return "orange"
	default:
		return "gray"
	}
}
