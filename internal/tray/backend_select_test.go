package tray

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestSelectTrayBackend(t *testing.T) {
	tests := []struct {
		name         string
		availability trayAvailability
		want         trayBackendKind
		wantErr      bool
	}{
		{
			name: "status notifier",
			availability: trayAvailability{
				statusNotifier: true,
			},
			want: trayBackendStatusNotifier,
		},
		{
			name: "xembed",
			availability: trayAvailability{
				xembed: true,
			},
			want: trayBackendXEmbed,
		},
		{
			name: "status notifier preferred when both exist",
			availability: trayAvailability{
				statusNotifier: true,
				xembed:         true,
			},
			want: trayBackendStatusNotifier,
		},
		{
			name: "xembed preferred over session bus fallback",
			availability: trayAvailability{
				statusNotifierBus: true,
				xembed:            true,
			},
			want: trayBackendXEmbed,
		},
		{
			name: "session bus supports late status notifier watcher",
			availability: trayAvailability{
				statusNotifierBus: true,
			},
			want: trayBackendStatusNotifier,
		},
		{
			name:    "no host",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := selectTrayBackend(tt.availability)
			if tt.wantErr {
				if err == nil {
					t.Fatal("selectTrayBackend() expected an error")
				}
				return
			}
			if err != nil {
				t.Fatalf("selectTrayBackend() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("selectTrayBackend() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestWaitForTrayBackend(t *testing.T) {
	t.Run("delayed status notifier wins over xembed", func(t *testing.T) {
		responses := []trayAvailability{
			{xembed: true},
			{statusNotifier: true, xembed: true},
		}
		probe := func() (trayAvailability, error) {
			got := responses[0]
			responses = responses[1:]
			return got, nil
		}
		ticks := make(chan time.Time, 1)
		ticks <- time.Time{}

		got, err := waitForTrayBackend(context.Background(), ticks, probe)
		if err != nil {
			t.Fatalf("waitForTrayBackend() error = %v", err)
		}
		if got != trayBackendStatusNotifier {
			t.Fatalf("waitForTrayBackend() = %q, want %q", got, trayBackendStatusNotifier)
		}
	})

	t.Run("xembed selected after grace period", func(t *testing.T) {
		ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
		defer cancel()

		got, err := waitForTrayBackend(ctx, nil, func() (trayAvailability, error) {
			return trayAvailability{xembed: true, statusNotifierBus: true}, nil
		})
		if err != nil {
			t.Fatalf("waitForTrayBackend() error = %v", err)
		}
		if got != trayBackendXEmbed {
			t.Fatalf("waitForTrayBackend() = %q, want %q", got, trayBackendXEmbed)
		}
	})

	t.Run("session bus selects status notifier after grace period", func(t *testing.T) {
		ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
		defer cancel()

		got, err := waitForTrayBackend(ctx, nil, func() (trayAvailability, error) {
			return trayAvailability{statusNotifierBus: true}, nil
		})
		if err != nil {
			t.Fatalf("waitForTrayBackend() error = %v", err)
		}
		if got != trayBackendStatusNotifier {
			t.Fatalf("waitForTrayBackend() = %q, want %q", got, trayBackendStatusNotifier)
		}
	})

	t.Run("no host reports the probe error", func(t *testing.T) {
		ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
		defer cancel()
		probeErr := errors.New("probe failed")

		_, err := waitForTrayBackend(ctx, nil, func() (trayAvailability, error) {
			return trayAvailability{}, probeErr
		})
		if !errors.Is(err, probeErr) {
			t.Fatalf("waitForTrayBackend() error = %v, want %v", err, probeErr)
		}
	})

	t.Run("explicit cancellation stops discovery", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, err := waitForTrayBackend(ctx, nil, func() (trayAvailability, error) {
			return trayAvailability{}, nil
		})
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("waitForTrayBackend() error = %v, want %v", err, context.Canceled)
		}
	})
}
