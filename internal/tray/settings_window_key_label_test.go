package tray

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/gotk3/gotk3/gtk"

	gsconfig "github.com/arumata/gswitch/internal/gswitch/config"
)

func TestPersistedCustomLayoutSwitchLabel(t *testing.T) {
	if os.Getenv("GSWITCH_GTK_TEST") != "1" {
		t.Skip("set GSWITCH_GTK_TEST=1 with a display to run the GTK settings check")
	}
	runtime.LockOSThread()
	gtk.Init(nil)

	combo, err := gtk.ComboBoxTextNew()
	if err != nil {
		t.Fatal(err)
	}
	w := &SettingsWindow{layoutSwitchCombo: combo}

	for _, tt := range []struct{ value, label string }{
		{"29", "LCtrl (29)"}, {"97", "RCtrl (97)"},
		{"56", "LAlt (56)"}, {"100", "RAlt (100)"},
		{"42", "LShift (42)"}, {"54", "RShift (54)"},
		{"125", "LSuper (125)"}, {"126", "RSuper (126)"},
	} {
		w.setLayoutSwitchFromValue(tt.value)
		if got := combo.GetActiveText(); got != tt.label {
			t.Fatalf("persisted layout switch %s label = %q, want %q", tt.value, got, tt.label)
		}
	}
}

func TestConversionKeyFeedbackTransitions(t *testing.T) {
	if os.Getenv("GSWITCH_GTK_TEST") != "1" {
		t.Skip("set GSWITCH_GTK_TEST=1 inside Xvfb")
	}
	runtime.LockOSThread()
	gtk.Init(nil)
	label, err := gtk.LabelNew("")
	if err != nil {
		t.Fatal(err)
	}
	defer label.Destroy()
	button, err := gtk.ButtonNew()
	if err != nil {
		t.Fatal(err)
	}
	defer button.Destroy()
	for _, tt := range []struct {
		context KeyPickerContext
		codes   []uint16
		message string
		valid   bool
	}{
		{KeyPickerForConvertKey, nil, "", false},
		{KeyPickerForConvertKey, []uint16{29}, "LCtrl", false},
		{KeyPickerForConvertKey, []uint16{97}, "RCtrl", false},
		{KeyPickerForConvertKey, []uint16{42}, "LShift", false},
		{KeyPickerForConvertKey, []uint16{54}, "RShift", false},
		{KeyPickerForConvertKey, []uint16{56}, "LAlt", false},
		{KeyPickerForConvertKey, []uint16{100}, "RAlt", false},
		{KeyPickerForConvertKey, []uint16{125}, "LSuper", false},
		{KeyPickerForConvertKey, []uint16{126}, "RSuper", false},
		{KeyPickerForConvertKey, []uint16{29, 30}, "Release the combination", false},
		{KeyPickerForConvertKey, []uint16{119}, "", true},
		{KeyPickerForLayoutSwitch, []uint16{29}, "", true},
	} {
		updateKeySelectionFeedback(label, button, tt.context, tt.codes)
		text, err := label.GetText()
		if err != nil {
			t.Fatal(err)
		}
		if (tt.message == "" && text != "") || !strings.Contains(text, tt.message) ||
			label.GetVisible() != (tt.message != "") || button.GetSensitive() != tt.valid {
			t.Fatalf("context %d codes %v: message=%q visible=%v enabled=%v", tt.context, tt.codes, text, label.GetVisible(), button.GetSensitive())
		}
	}
}

func TestLegacyConversionKeyRecovery(t *testing.T) {
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
	original := DefaultTrayConfig()
	w.setLayoutComboByValue(w.layout1Combo, original.Layout1)
	w.setLayoutComboByValue(w.layout2Combo, original.Layout2)
	w.delayBetweenSpin.SetValue(float64(original.Delay))
	w.delaySwitchSpin.SetValue(float64(original.LayoutSwitchDelay))
	for _, code := range []string{"29", "97", "42", "54", "56", "100", "125", "126"} {
		original.ConvertKey = code
		w.loadKeyConfig(original)
		text, err := w.conversionRecoveryWarning.GetText()
		if err != nil {
			t.Fatal(err)
		}
		if !w.conversionRecoveryWarning.GetVisible() || !strings.Contains(text, formatCustomKeyLabel(code)) {
			t.Fatalf("legacy %s: warning=%q", code, text)
		}
		candidate, err := w.collectConfigValues()
		if err != nil {
			t.Fatal(err)
		}
		if candidate.ConvertKey != "0" || w.loadedConfig.ConvertKey != code || original.ConvertKey != code || sameTrayConfig(candidate, w.loadedConfig) {
			t.Fatalf("Apply without edits: selected=%s stored=%s original=%s", candidate.ConvertKey, w.loadedConfig.ConvertKey, original.ConvertKey)
		}
		if err := w.validateConfig(candidate); err != nil {
			t.Fatal(err)
		}
		// Exercise the exact CLI argument serialization and writer used by Apply,
		// against a temporary file, without pkexec or the installed configuration.
		path := filepath.Join(t.TempDir(), "default.conf")
		checkStoredConversionKey(t, path, candidate, 0)
		for _, saveErr := range []error{errors.New("write failed"), ErrUserCanceled} {
			w.recordConfigSave(candidate, saveErr)
			if w.loadedConfig.ConvertKey != code || !w.conversionRecoveryWarning.GetVisible() {
				t.Fatal("failed save cleared recovery state")
			}
		}
		// Cancel/reopen retains the warning because the original file snapshot remains unchanged.
		w.loadKeyConfig(original)
		w.setConvertKeyFromValue("119")
		if !w.conversionRecoveryWarning.GetVisible() {
			t.Fatal("selection change cleared the saved-value warning")
		}
		candidate, err = w.collectConfigValues()
		if err != nil {
			t.Fatal(err)
		}
		if candidate.ConvertKey != "119" {
			t.Fatal("Pause selection lost")
		}
		checkStoredConversionKey(t, path, candidate, 119)
		w.recordConfigSave(candidate, nil)
		if w.loadedConfig.ConvertKey != "119" || w.conversionRecoveryWarning.GetVisible() {
			t.Fatal("successful save did not clear recovery state")
		}
		w.loadKeyConfig(candidate)
		if w.conversionRecoveryWarning.GetVisible() || w.getConvertKeyValueForWarning() != "119" {
			t.Fatal("supported key was changed on reload")
		}
	}
}

func checkStoredConversionKey(t *testing.T, path string, candidate *TrayConfig, want uint16) {
	t.Helper()
	if err := gsconfig.WriteConfigFromArgsTo(path, trayConfigWriteArgs(candidate)); err != nil {
		t.Fatal(err)
	}
	saved, err := gsconfig.LoadConfigFrom(path)
	if err != nil {
		t.Fatal(err)
	}
	if saved.ConvertKey != want || len(saved.Warnings) != 0 {
		t.Fatalf("saved key=%d warnings=%v; want %d without warnings", saved.ConvertKey, saved.Warnings, want)
	}
}
