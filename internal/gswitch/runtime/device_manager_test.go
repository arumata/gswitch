package runtime

import (
	"testing"
	"time"
)

func TestDeviceManagerSendEventUnblocksOnClose(t *testing.T) {
	dm := &DeviceManager{
		inotifyFD: -1,
		watchDesc: -1,
		events:    make(chan DeviceEvent, 1),
		done:      make(chan struct{}),
	}

	// Fill the channel so next send blocks.
	dm.events <- DeviceEvent{Path: "/dev/input/event0", Connected: true}

	result := make(chan bool, 1)
	go func() {
		result <- dm.sendEvent(DeviceEvent{Path: "/dev/input/event1", Connected: true})
	}()

	// Give sender a moment to block on full channel.
	time.Sleep(20 * time.Millisecond)

	if err := dm.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	select {
	case sent := <-result:
		if sent {
			t.Fatal("sendEvent() returned true after close, want false")
		}
	case <-time.After(250 * time.Millisecond):
		t.Fatal("sendEvent() did not unblock after Close()")
	}
}
