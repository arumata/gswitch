package runtime

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"golang.design/x/clipboard"
)

// Clipboard handles reading and writing to the system clipboard
type Clipboard struct {
	useWayland   bool
	x11Selection *X11Selection
	sessionEnv   *SessionEnv
}

// NewClipboard creates a new Clipboard instance
func NewClipboard(sessionEnv *SessionEnv) (*Clipboard, error) {
	cb := &Clipboard{sessionEnv: sessionEnv}

	// In a Wayland session prefer wl-clipboard: it talks to the compositor
	// directly and sees selections of both native and XWayland clients.
	// The X11 path via XWayland only sees what the compositor bridges.
	isWayland := os.Getenv("WAYLAND_DISPLAY") != "" || os.Getenv("XDG_SESSION_TYPE") == "wayland"

	if isWayland {
		_, errCopy := exec.LookPath("wl-copy")
		_, errPaste := exec.LookPath("wl-paste")
		if errCopy == nil && errPaste == nil {
			cb.useWayland = true
			return cb, nil
		}
		// No wl-clipboard: fall back to X11 via XWayland if DISPLAY is
		// available, otherwise there is nothing to work with.
		if os.Getenv("DISPLAY") == "" {
			return nil, errors.New("wayland session but wl-copy/wl-paste not found (install wl-clipboard)")
		}
	}

	// X11 or XWayland
	if err := clipboard.Init(); err != nil {
		return nil, fmt.Errorf("failed to initialize clipboard: %w", err)
	}

	// Initialize X11 selection for PRIMARY support
	x11sel, err := NewX11Selection()
	if err != nil {
		// Not fatal - just won't have PRIMARY selection support
		cb.x11Selection = nil
	} else {
		cb.x11Selection = x11sel
	}

	return cb, nil
}

// BackendName returns the name of the clipboard backend
func (cb *Clipboard) BackendName() string {
	if cb.useWayland {
		return "wayland"
	}
	return "x11"
}

// Read reads text from the clipboard
func (cb *Clipboard) Read() (string, error) {
	if cb.useWayland {
		return cb.readWayland()
	}
	data := clipboard.Read(clipboard.FmtText)
	return string(data), nil
}

// Write writes text to the clipboard
func (cb *Clipboard) Write(text string) error {
	if cb.useWayland {
		return cb.writeWayland(text)
	}

	// The x/clipboard library doesn't return an error from Write,
	// so we verify by reading back immediately
	clipboard.Write(clipboard.FmtText, []byte(text))

	// Verify write succeeded by reading back
	readBack := clipboard.Read(clipboard.FmtText)
	if string(readBack) != text {
		return errors.New("clipboard write verification failed: content mismatch")
	}

	return nil
}

func (cb *Clipboard) readWayland() (string, error) {
	cmd, err := cb.waylandCommand("wl-paste", "--no-newline")
	if err != nil {
		return "", fmt.Errorf("prepare wl-paste: %w", err)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if strings.Contains(stderr.String(), "Nothing is copied") {
			return "", nil
		}
		return "", fmt.Errorf("wl-paste failed: %w: %s", err, stderr.String())
	}
	return stdout.String(), nil
}

func (cb *Clipboard) writeWayland(text string) error {
	cmd, err := cb.waylandCommand("wl-copy")
	if err != nil {
		return fmt.Errorf("prepare wl-copy: %w", err)
	}
	cmd.Stdin = strings.NewReader(text)
	// wl-copy forks a background owner for the Wayland clipboard. A captured
	// stderr pipe remains open in that child and makes exec.Cmd.Wait block until
	// clipboard ownership changes, preventing the subsequent Ctrl+V. Inherit a
	// real file descriptor so Run only waits for the short-lived parent.
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("wl-copy failed: %w", err)
	}
	return nil
}

// ReadPrimarySelection reads the PRIMARY selection (currently selected text)
// Returns empty string if nothing is selected
func (cb *Clipboard) ReadPrimarySelection() (string, error) {
	if cb.useWayland {
		// Wayland: use wl-paste with --primary
		cmd, err := cb.waylandCommand("wl-paste", "--primary", "--no-newline")
		if err != nil {
			return "", fmt.Errorf("prepare wl-paste --primary: %w", err)
		}
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr

		if err := cmd.Run(); err != nil {
			if strings.Contains(stderr.String(), "Nothing is copied") {
				return "", nil
			}
			return "", fmt.Errorf("wl-paste --primary failed: %w: %s", err, stderr.String())
		}
		return stdout.String(), nil
	}

	// X11: use our native implementation
	if cb.x11Selection != nil {
		return cb.x11Selection.ReadPrimary()
	}

	return "", errors.New("PRIMARY selection not available")
}

// ClearPrimarySelection drops the current PRIMARY owner after a successful
// paste. A subsequent selection of identical text must be treated as new.
func (cb *Clipboard) ClearPrimarySelection() error {
	if cb.useWayland {
		cmd, err := cb.waylandCommand("wl-copy", "--primary", "--clear")
		if err != nil {
			return fmt.Errorf("prepare wl-copy --primary --clear: %w", err)
		}
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("wl-copy --primary --clear failed: %w: %s", err, stderr.String())
		}
		return nil
	}

	if cb.x11Selection != nil {
		return cb.x11Selection.ClearPrimary()
	}

	return errors.New("PRIMARY selection not available")
}

func (cb *Clipboard) waylandCommand(name string, args ...string) (*exec.Cmd, error) {
	return CommandAsSessionUser(cb.sessionEnv, name, args...)
}

// HasPrimarySelection returns true if PRIMARY selection is available
func (cb *Clipboard) HasPrimarySelection() bool {
	return cb.useWayland || cb.x11Selection != nil
}

// WaitForSelectionModifiersReleased confirms that X11 has processed the
// physical Ctrl/Shift releases before the virtual keyboard emits Ctrl+V.
// Wayland compositors process the input stream directly and have no portable
// modifier-state query, so the local input-event barrier is used there.
func (cb *Clipboard) WaitForSelectionModifiersReleased() error {
	if cb.useWayland || cb.x11Selection == nil {
		return nil
	}
	return waitForSelectionModifiersReleased(
		cb.x11Selection,
		modifierReleasePollAttempts,
		time.Sleep,
	)
}

// Close releases resources
func (cb *Clipboard) Close() {
	if cb.x11Selection != nil {
		cb.x11Selection.Close()
	}
}
