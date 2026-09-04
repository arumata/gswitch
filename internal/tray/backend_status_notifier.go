package tray

import "fyne.io/systray"

type statusNotifierRuntime interface {
	Run(onReady, onExit func())
	SetTitle(string)
}

type systrayStatusNotifierRuntime struct{}

func (systrayStatusNotifierRuntime) Run(onReady, onExit func()) {
	systray.Run(onReady, onExit)
}

func (systrayStatusNotifierRuntime) SetTitle(title string) {
	systray.SetTitle(title)
}

type statusNotifierBackend struct {
	runtime statusNotifierRuntime
}

type statusNotifierMenuItem struct {
	item *systray.MenuItem
}

func newStatusNotifierBackend() statusNotifierBackend {
	return statusNotifierBackend{runtime: systrayStatusNotifierRuntime{}}
}

func (b statusNotifierBackend) Run(onReady, onExit func()) error {
	b.runtime.SetTitle(trayApplicationID)
	b.runtime.Run(onReady, onExit)
	return nil
}

func (statusNotifierBackend) Quit() {
	systray.Quit()
}

func (statusNotifierBackend) SetIcon(icon []byte) {
	systray.SetIcon(icon)
}

func (b statusNotifierBackend) SetTitle(title string) {
	b.runtime.SetTitle(title)
}

func (statusNotifierBackend) SetTooltip(tooltip string) {
	systray.SetTooltip(tooltip)
}

//nolint:ireturn // All tray backends expose menu items through this contract.
func (statusNotifierBackend) AddMenuItem(title, tooltip string) trayMenuItem {
	return &statusNotifierMenuItem{item: systray.AddMenuItem(title, tooltip)}
}

func (statusNotifierBackend) AddSeparator() {
	systray.AddSeparator()
}

func (m *statusNotifierMenuItem) Clicks() <-chan struct{} {
	return m.item.ClickedCh
}

func (m *statusNotifierMenuItem) SetTitle(title string) {
	m.item.SetTitle(title)
}

func (m *statusNotifierMenuItem) Enable() {
	m.item.Enable()
}

func (m *statusNotifierMenuItem) Disable() {
	m.item.Disable()
}
