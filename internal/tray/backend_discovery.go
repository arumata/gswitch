package tray

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/godbus/dbus/v5"
	"github.com/jezek/xgb"
	"github.com/jezek/xgb/xproto"
)

const (
	trayHostGracePeriod  = 3 * time.Second
	trayHostPollInterval = 100 * time.Millisecond
)

//nolint:ireturn // Backend selection intentionally returns the shared contract.
func discoverTrayBackend(parent context.Context) (trayBackend, error) {
	ctx, cancel := context.WithTimeout(parent, trayHostGracePeriod)
	defer cancel()

	ticker := time.NewTicker(trayHostPollInterval)
	defer ticker.Stop()

	kind, err := waitForTrayBackend(ctx, ticker.C, probeTrayAvailability)
	if err != nil {
		return nil, fmt.Errorf(
			"system tray unavailable: %w (need session D-Bus for StatusNotifierItem or an XEmbed _NET_SYSTEM_TRAY host)",
			err,
		)
	}

	switch kind {
	case trayBackendStatusNotifier:
		fmt.Println("Using StatusNotifier system tray backend")
		return newStatusNotifierBackend(), nil
	case trayBackendXEmbed:
		fmt.Println("Using XEmbed system tray backend")
		return newXEmbedBackend(), nil
	default:
		return nil, fmt.Errorf("unsupported tray backend %q", kind)
	}
}

func probeTrayAvailability() (trayAvailability, error) {
	statusNotifier, statusNotifierBus, statusErr := statusNotifierAvailability()
	xembed, xembedErr := xembedHostAvailable(os.Getenv("DISPLAY"))

	return trayAvailability{
		statusNotifier:    statusNotifier,
		statusNotifierBus: statusNotifierBus,
		xembed:            xembed,
	}, errors.Join(statusErr, xembedErr)
}

func statusNotifierAvailability() (watcher, sessionBus bool, err error) {
	conn, err := dbus.SessionBus()
	if err != nil {
		return false, false, fmt.Errorf("connect to session D-Bus: %w", err)
	}

	var available bool
	call := conn.BusObject().Call(
		"org.freedesktop.DBus.NameHasOwner",
		0,
		"org.kde.StatusNotifierWatcher",
	)
	if err := call.Store(&available); err != nil {
		return false, false, fmt.Errorf("check StatusNotifierWatcher: %w", err)
	}
	return available, true, nil
}

func xembedHostAvailable(display string) (bool, error) {
	if display == "" {
		return false, nil
	}

	conn, err := xgb.NewConnDisplay(display)
	if err != nil {
		return false, fmt.Errorf("connect to X display %q: %w", display, err)
	}
	defer conn.Close()

	selectionName := fmt.Sprintf("_NET_SYSTEM_TRAY_S%d", conn.DefaultScreen)
	// #nosec G115 -- the fixed prefix plus a decimal X screen number is far
	// below the protocol's uint16 string-length limit.
	atom, err := xproto.InternAtom(conn, true, uint16(len(selectionName)), selectionName).Reply()
	if err != nil {
		return false, fmt.Errorf("find XEmbed tray selection %s: %w", selectionName, err)
	}
	if atom.Atom == xproto.AtomNone {
		return false, nil
	}

	owner, err := xproto.GetSelectionOwner(conn, atom.Atom).Reply()
	if err != nil {
		return false, fmt.Errorf("read XEmbed tray selection %s: %w", selectionName, err)
	}
	return owner.Owner != xproto.WindowNone, nil
}
