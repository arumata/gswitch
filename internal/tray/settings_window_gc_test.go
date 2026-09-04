package tray

import (
	"os"
	"runtime"
	"runtime/debug"
	"testing"

	"github.com/gotk3/gotk3/gtk"
)

// TestDelaysSectionSurvivesGC reproduces the XEmbed gate crash of 2026-09-04:
// gtk_spin_button_new hit "g_object_ref_sink: assertion 'G_IS_OBJECT'" when
// the Go GC finalized a freshly created GtkAdjustment during the cgo call.
// It needs a display and GTK, so it runs only under GSWITCH_GTK_TEST=1, e.g.
//
//	xvfb-run -a env G_DEBUG=fatal-criticals GSWITCH_GTK_TEST=1 \
//	  go test -run TestDelaysSectionSurvivesGC ./internal/tray
func TestDelaysSectionSurvivesGC(t *testing.T) {
	if os.Getenv("GSWITCH_GTK_TEST") != "1" {
		t.Skip("set GSWITCH_GTK_TEST=1 with a display to run the GTK lifetime check")
	}
	runtime.LockOSThread()
	gtk.Init(nil)
	defer debug.SetGCPercent(debug.SetGCPercent(1))

	stop := make(chan struct{})
	defer close(stop)
	go func() {
		for {
			select {
			case <-stop:
				return
			default:
				_ = make([]byte, 64<<10)
				runtime.GC()
			}
		}
	}()

	// Keep every section reachable: the check targets the adjustment handed
	// to gtk_spin_button_new, not widgets being finalized between iterations.
	windows := make([]*SettingsWindow, 0, 400)
	frames := make([]*gtk.Frame, 0, 400)
	for i := range 400 {
		w := &SettingsWindow{}
		frame, err := w.createDelaysSection()
		if err != nil {
			t.Fatalf("iteration %d: %v", i, err)
		}
		if w.delayBetweenSpin == nil || w.delaySwitchSpin == nil {
			t.Fatalf("iteration %d: spin buttons were not created", i)
		}
		windows = append(windows, w)
		frames = append(frames, frame)
	}
	runtime.KeepAlive(windows)
	runtime.KeepAlive(frames)
}
