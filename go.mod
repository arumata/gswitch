module github.com/arumata/gswitch

go 1.25.5

require (
	fyne.io/systray v1.12.2
	github.com/gotk3/gotk3 v0.6.5-0.20251124190141-e7a9e823ca35
	github.com/jezek/xgb v1.3.1
	golang.design/x/clipboard v0.7.1
	golang.org/x/sys v0.33.0
)

require (
	github.com/godbus/dbus/v5 v5.1.0 // indirect
	golang.org/x/exp/shiny v0.0.0-20250606033433-dcc06ee1d476 // indirect
	golang.org/x/image v0.41.0 // indirect
	golang.org/x/mobile v0.0.0-20250606033058-a2a15c67f36f // indirect
)

replace golang.design/x/clipboard => ./third_party/clipboard
