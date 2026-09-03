//nolint:gocritic // The cgo pseudo-package is misidentified as a duplicate package import.
package tray

/*
#cgo CFLAGS: -Wno-deprecated-declarations
#cgo pkg-config: gtk+-3.0
#include <stdlib.h>
#include "xembed_linux.h"
*/
import "C" //nolint:gocritic // C is the cgo pseudo-package, not a duplicate Go import.

import (
	"errors"
	"fmt"
	"runtime/cgo"
	"sync"
)

type xembedBackend struct {
	mu         sync.RWMutex
	native     *C.GSwitchXEmbedTray
	items      map[int]*xembedMenuItem
	nextID     int
	done       chan struct{}
	quitOnce   sync.Once
	doneOnce   sync.Once
	handleOnce sync.Once
	handle     cgo.Handle
	closed     bool
}

type xembedMenuItem struct {
	backend *xembedBackend
	native  *C.GtkWidget
	clicks  chan struct{}
}

func newXEmbedBackend() *xembedBackend {
	backend := &xembedBackend{
		items: make(map[int]*xembedMenuItem),
		done:  make(chan struct{}),
	}
	backend.handle = cgo.NewHandle(backend)
	return backend
}

func (b *xembedBackend) Run(onReady, onExit func()) error {
	started := make(chan error, 1)
	scheduleGTK(func() {
		b.mu.RLock()
		closed := b.closed
		b.mu.RUnlock()
		if closed {
			started <- nil
			return
		}

		native := C.gswitch_xembed_tray_new(C.uintptr_t(b.handle))
		if native == nil {
			started <- errors.New("create XEmbed tray icon")
			return
		}
		b.mu.Lock()
		b.native = native
		b.mu.Unlock()
		started <- nil
	})

	if err := <-started; err != nil {
		b.deleteHandle()
		return err
	}
	b.mu.RLock()
	closed := b.closed
	b.mu.RUnlock()
	if !closed {
		onReady()
	}
	<-b.done
	onExit()
	return nil
}

func (b *xembedBackend) Quit() {
	b.quitOnce.Do(func() {
		b.mu.Lock()
		b.closed = true
		b.mu.Unlock()
		scheduleGTK(func() {
			b.mu.Lock()
			b.native = nil
			b.mu.Unlock()
			// GtkStatusIcon teardown can crash inside GTK while its GtkPlug is
			// embedded. The process exits immediately after this callback, so
			// leave the native objects and callback handle to process cleanup.
			b.closeDone()
		})
	})
}

func (b *xembedBackend) deleteHandle() {
	b.handleOnce.Do(func() {
		b.handle.Delete()
	})
}

func (b *xembedBackend) closeDone() {
	b.doneOnce.Do(func() {
		close(b.done)
	})
}

func (b *xembedBackend) SetIcon(icon []byte) {
	if len(icon) == 0 {
		return
	}
	scheduleGTK(func() {
		b.mu.RLock()
		native := b.native
		closed := b.closed
		b.mu.RUnlock()
		if native == nil || closed {
			return
		}
		data := C.CBytes(icon)
		defer C.gswitch_xembed_free(data)
		if ok := C.gswitch_xembed_tray_set_icon(
			native,
			(*C.uchar)(data),
			C.size_t(len(icon)),
		); ok == C.FALSE {
			fmt.Println("Warning: failed to decode XEmbed tray icon")
		}
	})
}

func (b *xembedBackend) SetTitle(title string) {
	b.updateText(title, func(native *C.GSwitchXEmbedTray, value *C.char) {
		C.gswitch_xembed_tray_set_title(native, value)
	})
}

func (b *xembedBackend) SetTooltip(tooltip string) {
	b.updateText(tooltip, func(native *C.GSwitchXEmbedTray, value *C.char) {
		C.gswitch_xembed_tray_set_tooltip(native, value)
	})
}

func (b *xembedBackend) updateText(
	text string,
	update func(*C.GSwitchXEmbedTray, *C.char),
) {
	scheduleGTK(func() {
		b.mu.RLock()
		native := b.native
		closed := b.closed
		b.mu.RUnlock()
		if native == nil || closed {
			return
		}
		value := C.CString(text)
		defer C.gswitch_xembed_free_string(value)
		update(native, value)
	})
}

//nolint:ireturn // All tray backends expose menu items through this contract.
func (b *xembedBackend) AddMenuItem(title, tooltip string) trayMenuItem {
	b.mu.Lock()
	b.nextID++
	itemID := b.nextID
	item := &xembedMenuItem{
		backend: b,
		clicks:  make(chan struct{}, 1),
	}
	b.items[itemID] = item
	closed := b.closed
	b.mu.Unlock()
	if closed {
		return item
	}

	b.runGTKSync(func() {
		b.mu.RLock()
		native := b.native
		closed := b.closed
		b.mu.RUnlock()
		if native == nil || closed {
			return
		}

		titleValue := C.CString(title)
		defer C.gswitch_xembed_free_string(titleValue)
		tooltipValue := C.CString(tooltip)
		defer C.gswitch_xembed_free_string(tooltipValue)
		item.native = C.gswitch_xembed_tray_add_item(
			native,
			titleValue,
			tooltipValue,
			C.int(itemID),
		)
	})
	return item
}

func (b *xembedBackend) AddSeparator() {
	b.runGTKSync(func() {
		b.mu.RLock()
		native := b.native
		closed := b.closed
		b.mu.RUnlock()
		if native != nil && !closed {
			C.gswitch_xembed_tray_add_separator(native)
		}
	})
}

func (b *xembedBackend) runGTKSync(fn func()) {
	done := make(chan struct{})
	scheduleGTK(func() {
		fn()
		close(done)
	})
	<-done
}

func (m *xembedMenuItem) Clicks() <-chan struct{} {
	return m.clicks
}

func (m *xembedMenuItem) SetTitle(title string) {
	m.backend.scheduleMenuItemUpdate(m.native, func(item *C.GtkWidget) {
		value := C.CString(title)
		defer C.gswitch_xembed_free_string(value)
		C.gswitch_xembed_menu_item_set_title(item, value)
	})
}

func (m *xembedMenuItem) Enable() {
	m.backend.scheduleMenuItemUpdate(m.native, func(item *C.GtkWidget) {
		C.gswitch_xembed_menu_item_set_enabled(item, C.TRUE)
	})
}

func (m *xembedMenuItem) Disable() {
	m.backend.scheduleMenuItemUpdate(m.native, func(item *C.GtkWidget) {
		C.gswitch_xembed_menu_item_set_enabled(item, C.FALSE)
	})
}

func (b *xembedBackend) scheduleMenuItemUpdate(
	item *C.GtkWidget,
	update func(*C.GtkWidget),
) {
	if item == nil {
		return
	}
	scheduleGTK(func() {
		b.mu.RLock()
		closed := b.closed
		b.mu.RUnlock()
		if !closed {
			update(item)
		}
	})
}

//export goXEmbedMenuActivated
func goXEmbedMenuActivated(backendHandle C.uintptr_t, itemID C.int) {
	backend, ok := cgo.Handle(backendHandle).Value().(*xembedBackend)
	if !ok {
		return
	}
	backend.mu.RLock()
	item := backend.items[int(itemID)]
	backend.mu.RUnlock()
	if item == nil {
		return
	}
	select {
	case item.clicks <- struct{}{}:
	default:
	}
}
