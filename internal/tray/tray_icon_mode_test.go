package tray

import (
	"bytes"
	"testing"
)

type recordingTrayBackend struct {
	icon    []byte
	title   string
	tooltip string
}

func (b *recordingTrayBackend) Run(func(), func()) error                { return nil }
func (b *recordingTrayBackend) Quit()                                   {}
func (b *recordingTrayBackend) SetIcon(icon []byte)                     { b.icon = icon }
func (b *recordingTrayBackend) SetTitle(title string)                   { b.title = title }
func (b *recordingTrayBackend) SetTooltip(tooltip string)               { b.tooltip = tooltip }
func (b *recordingTrayBackend) AddMenuItem(string, string) trayMenuItem { return nil }
func (b *recordingTrayBackend) AddSeparator()                           {}

func TestTrayIconModesAndStatusPriority(t *testing.T) {
	backend := &recordingTrayBackend{}
	tray := &Tray{backend: backend, iconMode: DefaultTrayIconMode}

	detectionInfo := DetectionInfo{Status: TrayStatusOK, KeyNames: "Alt+Shift", Source: "xkb"}
	tray.applyDetectionStatus(detectionInfo)
	if !bytes.Equal(backend.icon, appIcon) {
		t.Fatal("tray without a layout should show the application icon")
	}

	layout := LayoutInfo{ShortCode: "US", LongName: "English (US)"}
	tray.applyLayout(layout)
	if !bytes.Equal(backend.icon, appIcon) {
		t.Fatal("app mode should show the application icon")
	}
	tray.applyIconMode(TrayIconModeFlag)
	if !bytes.Equal(backend.icon, GetNormalIcon(TrayIconModeFlag, "US")) {
		t.Fatal("mode change did not immediately redraw the flag")
	}

	tray.applyDetectionStatus(DetectionInfo{Status: TrayStatusNeedsConfig})
	tray.applyIconMode(TrayIconModeAppWithFlag)
	if !bytes.Equal(backend.icon, GetWarningIcon()) {
		t.Fatal("mode change replaced warning icon")
	}
	tray.applyDetectionStatus(detectionInfo)
	if !bytes.Equal(backend.icon, GetNormalIcon(TrayIconModeAppWithFlag, "US")) {
		t.Fatal("normal status did not restore the latest mode and layout")
	}
	tray.applyLayout(layout)
	if got, want := backend.tooltip, "US - English (US)\nAlt+Shift (xkb)"; got != want {
		t.Fatalf("restored tooltip = %q, want %q", got, want)
	}

	tray.applyDetectionStatus(DetectionInfo{Status: TrayStatusServiceError})
	if !bytes.Equal(backend.icon, GetErrorIcon()) {
		t.Fatal("service error did not replace the normal icon")
	}
	tray.applyDetectionStatus(detectionInfo)
	tray.applyLayout(layout)
	if got, want := backend.tooltip, "US - English (US)\nAlt+Shift (xkb)"; got != want {
		t.Fatalf("tooltip after error recovery = %q, want %q", got, want)
	}
}
