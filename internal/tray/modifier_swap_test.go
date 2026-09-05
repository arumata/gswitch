package tray

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/gotk3/gotk3/gtk"

	gsconfig "github.com/arumata/gswitch/internal/gswitch/config"
)

func TestConversionModifierSettings(t *testing.T) {
	if os.Getenv("GSWITCH_GTK_TEST") != "1" {
		t.Skip("set GSWITCH_GTK_TEST=1 inside Xvfb")
	}
	runtime.LockOSThread()
	gtk.Init(nil)
	w := &SettingsWindow{ignoreComboChanged: true}
	for _, create := range []func() (*gtk.Frame, error){w.createKeysSection, w.createLayoutsSection, w.createDelaysSection} {
		frame, err := create()
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(frame.Destroy)
	}
	cfg := DefaultTrayConfig()
	cfg.ConvertKey = "119"
	w.setLayoutComboByValue(w.layout1Combo, cfg.Layout1)
	w.setLayoutComboByValue(w.layout2Combo, cfg.Layout2)
	w.delayBetweenSpin.SetValue(float64(cfg.Delay))
	w.delaySwitchSpin.SetValue(float64(cfg.LayoutSwitchDelay))
	w.loadKeyConfig(cfg)
	if !w.conversionModifiersCombo.GetSensitive() || w.conversionModifiersCombo.GetActive() != 0 {
		t.Fatal("custom key must enable standard modifier choice")
	}
	checkShortcutsAttention(t, w, false)
	w.conversionModifiersCombo.SetActive(1)
	checkShortcutsAttention(t, w, true)
	text, err := w.conversionShortcutKeys[1].GetText()
	if err != nil {
		t.Fatal(err)
	}
	if text != "Ctrl + Pause/Break" {
		t.Errorf("line shortcut = %q", text)
	}
	selection, err := w.conversionShortcutKeys[2].GetText()
	if err != nil {
		t.Fatal(err)
	}
	if selection != "Shift + Pause/Break" {
		t.Errorf("selection shortcut = %q", selection)
	}
	selected, err := w.collectConfigValues()
	if err != nil {
		t.Fatal(err)
	}
	if !selected.SwapConversionModifiers || sameTrayConfig(selected, cfg) {
		t.Fatal("Apply must detect modifier change")
	}
	path := filepath.Join(t.TempDir(), "default.conf")
	if err := gsconfig.WriteConfigFromArgsTo(path, trayConfigWriteArgs(selected)); err != nil {
		t.Fatal(err)
	}
	saved, err := gsconfig.LoadConfigFrom(path)
	if err != nil {
		t.Fatal(err)
	}
	if !saved.SwapConversionModifiers {
		t.Fatal("Apply lost modifier choice")
	}
	traySaved := loadTrayConfigFrom(path)
	if !traySaved.SwapConversionModifiers || traySaved.ConvertKey != "119" {
		t.Fatal("tray loader lost saved modifier choice")
	}
	w.recordConfigSave(selected, nil)
	w.loadKeyConfig(selected)
	checkShortcutsAttention(t, w, false)
	if w.conversionModifiersCombo.GetActive() != 1 {
		t.Fatal("reopen lost modifier choice")
	}
	// Rebuilding the custom-key list temporarily clears its active selection.
	// That must not be treated as choosing Double Shift.
	w.setConvertKeyFromValue("30")
	if w.conversionModifiersCombo.GetActive() != 1 {
		t.Fatal("changing between custom keys must preserve the selected preset")
	}
	w.setConvertKeyFromValue("0")
	if w.conversionModifiersCombo.GetSensitive() {
		t.Fatal("Double Shift must disable modifier choice")
	}
	if w.conversionModifiersCombo.GetActive() != 0 {
		t.Fatal("Double Shift must reset modifier choice to Standard")
	}
	reset, err := w.collectConfigValues()
	if err != nil {
		t.Fatal(err)
	}
	if reset.SwapConversionModifiers || sameTrayConfig(reset, selected) {
		t.Fatal("Apply must detect and persist the Double Shift reset")
	}
	if err := gsconfig.WriteConfigFromArgsTo(path, trayConfigWriteArgs(reset)); err != nil {
		t.Fatal(err)
	}
	if loadTrayConfigFrom(path).SwapConversionModifiers {
		t.Fatal("saved Double Shift configuration retained modifier swap")
	}
	w.setConvertKeyFromValue("70")
	if !w.conversionModifiersCombo.GetSensitive() || w.conversionModifiersCombo.GetActive() != 0 {
		t.Fatal("returning to custom key must re-enable Standard choice")
	}
	text, err = w.conversionShortcutKeys[0].GetText()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, "Scroll") || strings.Contains(text, "Pause") {
		t.Fatalf("hint did not track key: %q", text)
	}
	w.loadKeyConfig(cfg)
	if w.conversionModifiersCombo.GetActive() != 0 {
		t.Fatal("cancel/reopen failed to restore saved choice")
	}
	cfg.ConvertKey = "0"
	cfg.SwapConversionModifiers = true
	w.loadKeyConfig(cfg)
	if w.conversionModifiersCombo.GetActive() != 0 || w.conversionModifiersCombo.GetSensitive() {
		t.Fatal("loading Double Shift must normalize stored modifier swap to Standard")
	}
}

func checkShortcutsAttention(t *testing.T, w *SettingsWindow, want bool) {
	t.Helper()
	style, err := w.conversionShortcutsButton.GetStyleContext()
	if err != nil {
		t.Fatal(err)
	}
	if style.HasClass("shortcut-attention") != want {
		t.Fatalf("shortcut attention = %t, want %t", !want, want)
	}
}

func TestShortcutsFlashLifecycle(t *testing.T) {
	if os.Getenv("GSWITCH_GTK_TEST") != "1" {
		t.Skip("set GSWITCH_GTK_TEST=1 inside Xvfb")
	}
	runtime.LockOSThread()
	gtk.Init(nil)
	w := &SettingsWindow{ignoreComboChanged: true}
	frame, err := w.createKeysSection()
	if err != nil {
		t.Fatal(err)
	}
	defer frame.Destroy()
	cfg := DefaultTrayConfig()
	cfg.ConvertKey = "119"
	w.loadKeyConfig(cfg)
	if w.shortcutsFlashSource != 0 {
		t.Fatal("config load started a flash")
	}
	w.conversionModifiersCombo.SetActive(1)
	first := w.shortcutsFlashSource
	w.conversionModifiersCombo.SetActive(0)
	if first == 0 || w.shortcutsFlashSource == 0 || first == w.shortcutsFlashSource {
		t.Fatal("changing the preset again must restart the flash")
	}
	deadline := time.Now().Add(3 * time.Second)
	for w.shortcutsFlashSource != 0 && time.Now().Before(deadline) {
		for gtk.EventsPending() {
			gtk.MainIterationDo(false)
		}
		time.Sleep(time.Millisecond)
	}
	if w.shortcutsFlashSource != 0 {
		t.Fatal("flash did not finish")
	}
	css, err := w.shortcutsFlashStyle.ToString()
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(css) != "" || w.shortcutsAttentionDot.GetOpacity() != 1 {
		t.Fatal("only the unread dot should remain after fading")
	}
	w.conversionModifiersCombo.SetActive(1)
	w.setShortcutsAttention(false)
	if w.shortcutsFlashSource != 0 || w.shortcutsAttentionDot.GetOpacity() != 0 {
		t.Fatal("acknowledging help must cancel the flash and clear the dot")
	}
	w.conversionModifiersCombo.SetActive(0)
	w.conversionShortcutsButton.Destroy()
	if w.shortcutsFlashSource != 0 {
		t.Fatal("destroyed button retained an animation timer")
	}
}
