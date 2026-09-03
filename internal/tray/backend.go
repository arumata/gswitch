package tray

type trayMenuItem interface {
	Clicks() <-chan struct{}
	SetTitle(string)
	Enable()
	Disable()
}

type trayBackend interface {
	Run(onReady, onExit func()) error
	Quit()
	SetIcon([]byte)
	SetTitle(string)
	SetTooltip(string)
	AddMenuItem(title, tooltip string) trayMenuItem
	AddSeparator()
}
