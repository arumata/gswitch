//go:build gtk_test

package tray

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"testing"

	"github.com/gotk3/gotk3/glib"
	"github.com/gotk3/gotk3/gtk"
)

// This UI-only check must run in Xvfb. It never opens evdev or uinput.
func TestKeyPickerDialogIgnoresXTest(t *testing.T) {
	if os.Getenv("GSWITCH_GTK_TEST") != "1" {
		t.Skip("set GSWITCH_GTK_TEST=1 inside Xvfb")
	}
	runtime.LockOSThread()
	gtk.Init(nil)
	parent, err := gtk.WindowNew(gtk.WINDOW_TOPLEVEL)
	if err != nil {
		t.Fatal(err)
	}
	defer parent.Destroy()

	if output, err := exec.Command("xmodmap", "-e", "keycode 93 = ISO_Last_Group").CombinedOutput(); err != nil {
		t.Fatalf("isolated keymap: %v: %s", err, output)
	}
	var callbackErr error
	glib.IdleAdd(func() bool {
		windows := gtk.WindowListToplevels()
		for i := range windows.Length() {
			window := windows.NthData(i).(*gtk.Window)
			title, _ := window.GetTitle()
			if title != keyPickerTitle(KeyPickerForLayoutSwitch) {
				continue
			}
			gwindow, err := window.GetWindow()
			if err != nil {
				callbackErr = err
				return false
			}
			id := strconv.FormatUint(uint64(gwindow.GetXID()), 10)
			sendPickerControlChord(window)
			// XTest provides xcape's subsequent action.
			// The only variable argument is this isolated window ID.
			if output, err := exec.Command("xdotool", "windowfocus", id, "key", "ISO_Last_Group").CombinedOutput(); err != nil { //nolint:gosec // Only the isolated GTK window ID varies.
				callbackErr = fmt.Errorf("input fixture: %w: %s", err, output)
			}

			// Input events have higher priority than idle callbacks.
			glib.IdleAdd(func() bool {
				dialog := &gtk.Dialog{Window: *window}
				dialog.Response(gtk.RESPONSE_OK)
				return false
			})
			return false
		}
		callbackErr = errors.New("picker window not found")
		return false
	})
	result, ok := ShowKeyPickerDialog(parent, KeyPickerForLayoutSwitch, "")
	if callbackErr != nil {
		t.Fatal(callbackErr)
	}
	if !ok || result.Value != "97" || len(result.KeyNames) != 1 || result.KeyNames[0] != "RCtrl" {
		t.Fatalf("picker after XTest = %+v, %v; want RCtrl (97)", result, ok)
	}
	combo, err := gtk.ComboBoxTextNew()
	if err != nil {
		t.Fatal(err)
	}
	defer combo.Destroy()
	w := &SettingsWindow{layoutSwitchCombo: combo}
	w.setLayoutSwitchFromValue(result.Value)
	if got := combo.GetActiveText(); got != "RCtrl (97)" {
		t.Fatalf("combo after picker = %q", got)
	}
}
