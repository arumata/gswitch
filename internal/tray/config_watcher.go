package tray

import (
	"os"
	"sync"
	"time"
)

// ConfigWatcher watches the config file for changes and triggers callbacks.
type ConfigWatcher struct {
	onChange func()
	stopChan chan struct{}
	stopOnce sync.Once
	lastMod  time.Time
}

// NewConfigWatcher creates a new config watcher.
func NewConfigWatcher(onChange func()) *ConfigWatcher {
	return &ConfigWatcher{
		onChange: onChange,
		stopChan: make(chan struct{}),
	}
}

// Start begins watching the config file.
func (w *ConfigWatcher) Start() error {
	// Get initial mtime
	info, err := os.Stat(ConfigFile)
	if err == nil {
		w.lastMod = info.ModTime()
	}

	go w.watch()
	return nil
}

// Stop stops watching the config file.
func (w *ConfigWatcher) Stop() {
	w.stopOnce.Do(func() {
		close(w.stopChan)
	})
}

// watch polls the config file for changes.
// We use polling instead of inotify for simplicity and cross-platform compatibility.
func (w *ConfigWatcher) watch() {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			w.checkForChanges()
		case <-w.stopChan:
			return
		}
	}
}

// checkForChanges checks if the config file has been modified.
func (w *ConfigWatcher) checkForChanges() {
	info, err := os.Stat(ConfigFile)
	if err != nil {
		// File doesn't exist or can't be accessed
		return
	}

	modTime := info.ModTime()
	if modTime.After(w.lastMod) {
		w.lastMod = modTime
		if w.onChange != nil {
			w.onChange()
		}
	}
}
