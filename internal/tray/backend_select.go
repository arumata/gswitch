package tray

import (
	"context"
	"errors"
	"fmt"
	"time"
)

type trayBackendKind string

const (
	trayBackendStatusNotifier trayBackendKind = "status-notifier"
	trayBackendXEmbed         trayBackendKind = "xembed"
)

type trayAvailability struct {
	statusNotifier    bool
	statusNotifierBus bool
	xembed            bool
}

var errNoTrayHost = errors.New("no supported system tray host is available")

func selectTrayBackend(availability trayAvailability) (trayBackendKind, error) {
	if availability.statusNotifier {
		return trayBackendStatusNotifier, nil
	}
	if availability.xembed {
		return trayBackendXEmbed, nil
	}
	if availability.statusNotifierBus {
		return trayBackendStatusNotifier, nil
	}
	return "", errNoTrayHost
}

func waitForTrayBackend(
	ctx context.Context,
	ticks <-chan time.Time,
	probe func() (trayAvailability, error),
) (trayBackendKind, error) {
	var (
		xembedAvailable            bool
		statusNotifierBusAvailable bool
		lastErr                    error
	)

	for {
		availability, err := probe()
		if err != nil {
			lastErr = err
		}
		if availability.statusNotifier {
			return trayBackendStatusNotifier, nil
		}
		xembedAvailable = xembedAvailable || availability.xembed
		// An SNI client must stay available when the watcher starts after it.
		// Prefer a real XEmbed host over this D-Bus-only fallback, but do not
		// require the watcher to own its name before exporting the SNI item.
		statusNotifierBusAvailable = statusNotifierBusAvailable || availability.statusNotifierBus

		select {
		case <-ctx.Done():
			if errors.Is(ctx.Err(), context.Canceled) {
				return "", context.Canceled
			}
			if xembedAvailable {
				return trayBackendXEmbed, nil
			}
			if statusNotifierBusAvailable {
				return trayBackendStatusNotifier, nil
			}
			if lastErr != nil {
				return "", fmt.Errorf("%w: %w", errNoTrayHost, lastErr)
			}
			return "", errNoTrayHost
		case <-ticks:
		}
	}
}
