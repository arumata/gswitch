package tray

import (
	"testing"
	"time"
)

func TestScheduleSettingsWindowCreationUsesGTKMainThread(t *testing.T) {
	originalSchedule := scheduleGTK
	originalFactory := newSettingsWindow
	defer func() {
		scheduleGTK = originalSchedule
		newSettingsWindow = originalFactory
	}()

	var scheduled func()
	scheduleGTK = func(fn func()) {
		scheduled = fn
	}
	created := false
	newSettingsWindow = func(_ *App) (*SettingsWindow, error) {
		created = true
		return &SettingsWindow{}, nil
	}

	app := New()
	app.scheduleSettingsWindowCreation()
	if created {
		t.Fatal("settings window was created before the GTK main-loop callback ran")
	}
	if scheduled == nil {
		t.Fatal("settings window creation was not scheduled on the GTK main loop")
	}

	scheduled()
	if !created {
		t.Fatal("scheduled GTK callback did not create the settings window")
	}
	if app.settingsWindow == nil {
		t.Fatal("created settings window was not stored on the app")
	}
}

func TestOnSettingsClickedUsesGTKMainThread(t *testing.T) {
	originalSchedule := scheduleGTK
	defer func() { scheduleGTK = originalSchedule }()

	called := false
	scheduleGTK = func(_ func()) {
		called = true
	}

	New().OnSettingsClicked()
	if !called {
		t.Fatal("OnSettingsClicked did not schedule GTK work on the main loop")
	}
}

func TestConfigWatcherStopIdempotent(_ *testing.T) {
	w := NewConfigWatcher(nil)
	w.Stop()
	w.Stop()
}

func TestLayoutMonitorStopIdempotent(_ *testing.T) {
	m := NewLayoutMonitor(nil)
	m.ticker = time.NewTicker(time.Second)
	m.Stop()
	m.Stop()
}
