//nolint:gocritic // The cgo pseudo-package is misidentified as a duplicate package import.
package tray

/*
#cgo pkg-config: gtk+-3.0
#include <gdk/gdk.h>

static const char *gswitch_key_source_name(GdkEvent *event) {
	GdkDevice *source = gdk_event_get_source_device(event);
	return source == NULL ? "" : gdk_device_get_name(source);
}
*/
import "C" //nolint:gocritic // C is the cgo pseudo-package, not a duplicate Go import.

import (
	"unsafe"

	"github.com/gotk3/gotk3/gdk"
)

// gotk3 does not expose gdk_event_get_source_device. Use the source device,
// since the master keyboard is shared by physical and XTest events.
func keyEventSourceName(event *gdk.Event) string {
	return C.GoString(C.gswitch_key_source_name((*C.GdkEvent)(unsafe.Pointer(event.GdkEvent))))
}
