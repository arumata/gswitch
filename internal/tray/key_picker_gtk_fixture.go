//go:build gtk_test

//nolint:gocritic // The cgo pseudo-package is misidentified as a duplicate package import.
package tray

/*
#cgo pkg-config: gtk+-3.0
#include <gtk/gtk.h>

// Feed a physical chord at the GTK boundary without system input. Xvfb has
// no physical keyboard; this does not claim evdev integration coverage.
static void gswitch_test_control_chord(GtkWindow *window) {
	GdkWindow *target = gtk_widget_get_window(GTK_WIDGET(window));
	GdkDevice *keyboard = gdk_seat_get_keyboard(gdk_display_get_default_seat(gdk_window_get_display(target)));
	for (int i = 0; i < 2; i++) {
		GdkEvent *event = gdk_event_new(i == 0 ? GDK_KEY_PRESS : GDK_KEY_RELEASE);
		event->key.window = g_object_ref(target);
		event->key.hardware_keycode = 105;
		event->key.keyval = GDK_KEY_Control_R;
		gdk_event_set_device(event, keyboard);
		gdk_event_set_source_device(event, keyboard);
		gtk_widget_event(GTK_WIDGET(window), event);
		gdk_event_free(event);
	}
}
*/
import "C" //nolint:gocritic // C is the cgo pseudo-package.

import (
	"unsafe"

	"github.com/gotk3/gotk3/gtk"
)

func sendPickerControlChord(window *gtk.Window) {
	C.gswitch_test_control_chord((*C.GtkWindow)(unsafe.Pointer(window.GObject)))
}
