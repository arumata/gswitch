package tray

/*
#cgo pkg-config: x11
#include <X11/XKBlib.h>

static int gswitch_xkb_current_group(void) {
	Display *display = XOpenDisplay(NULL);
	if (display == NULL) {
		return -1;
	}
	XkbStateRec state;
	int status = XkbGetState(display, XkbUseCoreKbd, &state);
	XCloseDisplay(display);
	if (status != Success) {
		return -1;
	}
	return state.group;
}
*/
import "C"

import "errors"

var currentXKBGroup = readCurrentXKBGroup

func readCurrentXKBGroup() (int, error) {
	group := int(C.gswitch_xkb_current_group())
	if group < 0 {
		return 0, errors.New("cannot read current XKB group")
	}
	return group, nil
}
