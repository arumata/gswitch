package tray

import (
	"bytes"
	"testing"
)

type recordingTrayBackend struct {
	icon    []byte
	titles  []string
	tooltip string
}

func (*recordingTrayBackend) Run(onReady, onExit func()) error {
	onReady()
	onExit()
	return nil
}

func (*recordingTrayBackend) Quit() {}

func (b *recordingTrayBackend) SetIcon(icon []byte) {
	b.icon = append(b.icon[:0], icon...)
}

func (b *recordingTrayBackend) SetTitle(title string) {
	b.titles = append(b.titles, title)
}

func (b *recordingTrayBackend) SetTooltip(tooltip string) {
	b.tooltip = tooltip
}

//nolint:ireturn // The fake implements the production trayBackend contract.
func (*recordingTrayBackend) AddMenuItem(string, string) trayMenuItem {
	return &recordingTrayMenuItem{clicks: make(chan struct{})}
}

func (*recordingTrayBackend) AddSeparator() {}

type recordingTrayMenuItem struct {
	clicks chan struct{}
}

func (m *recordingTrayMenuItem) Clicks() <-chan struct{} {
	return m.clicks
}

func (*recordingTrayMenuItem) SetTitle(string) {}

func (*recordingTrayMenuItem) Enable() {}

func (*recordingTrayMenuItem) Disable() {}

func TestTrayInitialAppearanceUsesStableTitle(t *testing.T) {
	t.Parallel()

	backend := &recordingTrayBackend{}
	tray := &Tray{backend: backend}

	tray.setInitialAppearance()

	if !bytes.Equal(backend.icon, GetNormalIcon(TrayIconModeApp, "")) {
		t.Fatal("initial icon is not the application icon")
	}
	if len(backend.titles) != 1 || backend.titles[0] != trayApplicationID {
		t.Fatalf("initial titles = %v, want [%q]", backend.titles, trayApplicationID)
	}
	if backend.tooltip == "" {
		t.Fatal("initial tooltip is empty")
	}
}
