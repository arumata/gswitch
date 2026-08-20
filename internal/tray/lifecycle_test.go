package tray

import (
	"testing"
	"time"
)

func TestConfigWatcherStopIdempotent(_ *testing.T) {
	w := NewConfigWatcher(nil)
	w.Stop()
	w.Stop()
}

func TestLayoutMonitorStopIdempotent(_ *testing.T) {
	m := NewLayoutMonitor(nil)
	m.ticker = time.NewTicker(time.Second)
	m.Stop()
	m.Stop()
}
