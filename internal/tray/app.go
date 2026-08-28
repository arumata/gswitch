// Package tray provides system tray GUI application for gswitch.
package tray

import (
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"

	"fyne.io/systray"
	"github.com/gotk3/gotk3/glib"
	"github.com/gotk3/gotk3/gtk"
)

// App represents the main tray application.
type App struct {
	mu sync.Mutex

	tray           *Tray
	layoutMonitor  *LayoutMonitor
	settingsWindow *SettingsWindow
	configWatcher  *ConfigWatcher
	detectionInfo  DetectionInfo

	quitOnce sync.Once
}

var (
	scheduleGTK = func(fn func()) {
		glib.IdleAdd(fn)
	}
	newSettingsWindow = NewSettingsWindow
)

// New creates a new App instance.
func New() *App {
	return &App{}
}

// Run starts the application main loop.
// It sets up signal handlers and runs the systray event loop.
func (a *App) Run() error {
	// Setup signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigChan)

	signalStop := make(chan struct{})

	go func() {
		select {
		case sig := <-sigChan:
			fmt.Printf("\nReceived signal %v, shutting down...\n", sig)
			a.Quit()
		case <-signalStop:
		}
	}()

	// Create tray instance
	a.mu.Lock()
	a.tray = NewTray(a)
	tray := a.tray
	a.mu.Unlock()

	// Run systray (blocking)
	systray.Run(tray.onReady, tray.onExit)
	close(signalStop)

	return nil
}

// onTrayReady is called when the tray is initialized and ready.
func (a *App) onTrayReady() {
	a.scheduleSettingsWindowCreation()

	a.mu.Lock()
	tray := a.tray
	a.mu.Unlock()
	if tray == nil {
		return
	}

	// Check detection status at startup
	info := CheckDetectionStatus(tray.GetServiceManager())
	a.mu.Lock()
	a.detectionInfo = info
	a.mu.Unlock()
	tray.UpdateDetectionStatus(info)

	// Show first-run dialog on GNOME if needed
	a.showFirstRunDialogIfNeeded(info)

	// Start config watcher
	configWatcher := NewConfigWatcher(a.onConfigChanged)
	if err := configWatcher.Start(); err != nil {
		fmt.Printf("Warning: failed to start config watcher: %v\n", err)
	}
	a.mu.Lock()
	a.configWatcher = configWatcher
	a.mu.Unlock()

	// Create and start layout monitor (only if detection OK)
	if info.Status == TrayStatusOK {
		layoutMonitor := NewLayoutMonitor(a.onLayoutChanged)
		if err := layoutMonitor.Start(); err != nil {
			fmt.Printf("Warning: failed to start layout monitor: %v\n", err)
		} else {
			a.mu.Lock()
			a.layoutMonitor = layoutMonitor
			a.mu.Unlock()

			// Update tray with initial layout
			layout := layoutMonitor.GetCurrentLayout()
			tray.UpdateLayout(layout)
		}
	}
}

func (a *App) scheduleSettingsWindowCreation() {
	scheduleGTK(func() {
		settingsWindow, err := newSettingsWindow(a)
		if err != nil {
			fmt.Printf("Warning: failed to create settings window: %v\n", err)
		}
		a.mu.Lock()
		a.settingsWindow = settingsWindow
		a.mu.Unlock()
	})
}

// onLayoutChanged is called when the keyboard layout changes.
func (a *App) onLayoutChanged(layout LayoutInfo) {
	a.mu.Lock()
	tray := a.tray
	a.mu.Unlock()
	if tray != nil {
		tray.UpdateLayout(layout)
	}
}

// Quit gracefully shuts down the application.
func (a *App) Quit() {
	a.quitOnce.Do(func() {
		a.mu.Lock()
		layoutMonitor := a.layoutMonitor
		configWatcher := a.configWatcher
		a.layoutMonitor = nil
		a.configWatcher = nil
		a.mu.Unlock()

		// Stop layout monitor
		if layoutMonitor != nil {
			layoutMonitor.Stop()
		}
		// Stop config watcher
		if configWatcher != nil {
			configWatcher.Stop()
		}
		systray.Quit()
	})
}

// OnSettingsClicked is called when Settings menu item is clicked.
func (a *App) OnSettingsClicked() {
	scheduleGTK(func() {
		a.mu.Lock()
		settingsWindow := a.settingsWindow
		a.mu.Unlock()
		if settingsWindow != nil {
			// Show() will focus the window if already visible, or show it if hidden
			settingsWindow.Show()
		} else {
			fmt.Println("Settings window not available")
		}
	})
}

// UpdateServiceStatus updates the service status in the tray menu.
func (a *App) UpdateServiceStatus() {
	a.mu.Lock()
	tray := a.tray
	a.mu.Unlock()
	if tray != nil {
		tray.UpdateServiceStatus()
	}
}

// RefreshDetectionStatus re-checks detection status and updates the tray.
func (a *App) RefreshDetectionStatus() {
	a.mu.Lock()
	tray := a.tray
	a.mu.Unlock()
	if tray == nil {
		return
	}

	info := CheckDetectionStatus(tray.GetServiceManager())
	tray.UpdateDetectionStatus(info)

	a.mu.Lock()
	a.detectionInfo = info
	layoutMonitor := a.layoutMonitor
	a.mu.Unlock()

	// Handle layout monitor based on detection status
	if info.Status != TrayStatusOK {
		return
	}
	if layoutMonitor == nil {
		layoutMonitor = a.startLayoutMonitor()
	}
	// Update tray with current layout to restore proper icon
	if layoutMonitor != nil {
		tray.UpdateLayout(layoutMonitor.GetCurrentLayout())
	}
}

// startLayoutMonitor starts a layout monitor, keeping the existing one if
// another goroutine initialized it first. Returns the active monitor, or nil
// if starting failed.
func (a *App) startLayoutMonitor() *LayoutMonitor {
	newMonitor := NewLayoutMonitor(a.onLayoutChanged)
	if err := newMonitor.Start(); err != nil {
		fmt.Printf("Warning: failed to start layout monitor: %v\n", err)
		return nil
	}
	a.mu.Lock()
	if a.layoutMonitor == nil {
		a.layoutMonitor = newMonitor
	}
	active := a.layoutMonitor
	a.mu.Unlock()
	if active != newMonitor {
		newMonitor.Stop()
	}
	return active
}

// onConfigChanged is called when the config file changes.
func (a *App) onConfigChanged() {
	a.RefreshDetectionStatus()
}

// showFirstRunDialogIfNeeded shows a setup dialog on GNOME if detection failed.
func (a *App) showFirstRunDialogIfNeeded(info DetectionInfo) {
	// Only show if detection failed
	if info.Status != TrayStatusNeedsConfig {
		return
	}

	// Check if already shown (marker file)
	markerPath := filepath.Join(os.Getenv("HOME"), ".config", "gswitch", "wizard-shown")
	if fileExists(markerPath) {
		return
	}

	// Only show on GNOME (no tray icon support in some GNOME versions)
	if !desktopIsGNOME() {
		return
	}

	// Show dialog on GTK main thread
	scheduleGTK(func() {
		dialog := gtk.MessageDialogNew(
			nil,
			gtk.DIALOG_MODAL,
			gtk.MESSAGE_INFO,
			gtk.BUTTONS_NONE,
			"%s",
			strFirstRunTitle,
		)
		dialog.FormatSecondaryText("%s", strFirstRunMessage)
		_, _ = dialog.AddButton(strFirstRunConfig, gtk.RESPONSE_OK)
		_, _ = dialog.AddButton(strFirstRunDismiss, gtk.RESPONSE_CANCEL)

		response := dialog.Run()
		dialog.Destroy()

		if response == gtk.RESPONSE_OK {
			a.OnSettingsClicked()
		}

		// Create marker file
		markerDir := filepath.Dir(markerPath)
		if err := os.MkdirAll(markerDir, 0o750); err != nil {
			fmt.Printf("Warning: failed to create marker directory: %v\n", err)
			return
		}
		if err := os.WriteFile(markerPath, []byte{}, 0o600); err != nil {
			fmt.Printf("Warning: failed to create marker file: %v\n", err)
		}
	})
}

// fileExists checks if a file exists.
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
