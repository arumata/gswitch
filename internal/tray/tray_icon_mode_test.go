package tray

import (
	"bytes"
	"testing"
)

type iconModeRecordingTrayBackend struct {
	icon    []byte
	title   string
	tooltip string
}

func (b *iconModeRecordingTrayBackend) Run(func(), func()) error  { return nil }
func (b *iconModeRecordingTrayBackend) Quit()                     {}
func (b *iconModeRecordingTrayBackend) SetIcon(icon []byte)       { b.icon = icon }
func (b *iconModeRecordingTrayBackend) SetTitle(title string)     { b.title = title }
func (b *iconModeRecordingTrayBackend) SetTooltip(tooltip string) { b.tooltip = tooltip }

//nolint:ireturn // The fake implements the production trayBackend contract.
func (b *iconModeRecordingTrayBackend) AddMenuItem(string, string) trayMenuItem { return nil }
func (b *iconModeRecordingTrayBackend) AddSeparator()                           {}

func TestTrayIconModesAndStatusPriority(t *testing.T) {
	backend := &iconModeRecordingTrayBackend{}
	tray := &Tray{backend: backend, iconMode: DefaultTrayIconMode}

	detectionInfo := DetectionInfo{Status: TrayStatusOK, KeyNames: "Alt+Shift", Source: "xkb"}
	tray.applyDetectionStatus(detectionInfo)
	if !bytes.Equal(backend.icon, appIcon) {
		t.Fatal("tray without a layout should show the application icon")
	}

	layout := LayoutInfo{ShortCode: "US", LongName: "English (US)"}
	tray.applyLayout(layout)
	if !bytes.Equal(backend.icon, GetNormalIcon(TrayIconModeFlag, "US")) {
		t.Fatal("default mode should show the layout flag")
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
