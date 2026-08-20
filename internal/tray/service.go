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

// ErrUserCanceled is returned when user cancels pkexec authentication dialog.
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
	cmd := exec.Command("systemctl", "is-active", sm.serviceName)
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
	cmd := exec.Command("systemctl", "cat", sm.serviceName)
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
	cmd := exec.Command("systemctl", "is-enabled", sm.serviceName)
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

// Restart restarts the service using pkexec for privilege escalation.
func (sm *ServiceManager) Restart() error {
	return sm.runPrivileged("restart")
}

// Start starts the service using pkexec for privilege escalation.
func (sm *ServiceManager) Start() error {
	return sm.runPrivileged("start")
}

// Stop stops the service using pkexec for privilege escalation.
func (sm *ServiceManager) Stop() error {
	return sm.runPrivileged("stop")
}

// Enable enables the service to start at boot using pkexec.
func (sm *ServiceManager) Enable() error {
	return sm.runPrivileged("enable")
}

// Disable disables the service from starting at boot using pkexec.
func (sm *ServiceManager) Disable() error {
	return sm.runPrivileged("disable")
}

// runPrivileged runs a systemctl action through gswitch helper with pkexec for privilege escalation.
func (sm *ServiceManager) runPrivileged(action string) error {
	gswitchPath, err := findGswitchBinaryForPkexec()
	if err != nil {
		return fmt.Errorf("gswitch not found (required for privileged service actions): %w", err)
	}

	// #nosec G204 - gswitchPath is validated by findGswitchBinaryForPkexec and action is hardcoded by callers
	cmd := exec.Command("pkexec", gswitchPath, "--systemctl", action)
	output, err := cmd.CombinedOutput()

	if err == nil {
		return nil
	}

	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		// pkexec exit code 126 means user canceled authentication
		if exitErr.ExitCode() == 126 {
			return ErrUserCanceled
		}
		// pkexec exit code 127 means pkexec not found or authorization failed
		if exitErr.ExitCode() == 127 {
			return errors.New("authorization failed or pkexec not available")
		}
	}

	// Include output in error message for debugging
	outputStr := strings.TrimSpace(string(output))
	if outputStr != "" {
		return errors.New(outputStr)
	}
	return err
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
