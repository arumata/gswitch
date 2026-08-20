package runtime

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"unsafe"

	"golang.org/x/sys/unix"
)

// DeviceEvent represents a device connection/disconnection event
type DeviceEvent struct {
	Path      string
	Connected bool
}

// DeviceManager watches /dev/input/ for device changes using inotify
type DeviceManager struct {
	inotifyFD int
	watchDesc int
	events    chan DeviceEvent
	done      chan struct{}
	mu        sync.Mutex
	closed    bool
}

// sendEvent safely sends an event to the channel, checking if closed
// Returns true if event was sent, false if manager is closed
func (dm *DeviceManager) sendEvent(event DeviceEvent) bool {
	dm.mu.Lock()
	if dm.closed {
		dm.mu.Unlock()
		return false
	}
	events := dm.events
	done := dm.done
	dm.mu.Unlock()

	select {
	case events <- event:
		return true
	case <-done:
		return false
	}
}

// NewDeviceManager creates a new DeviceManager instance
func NewDeviceManager() (*DeviceManager, error) {
	fd, err := unix.InotifyInit1(unix.IN_NONBLOCK)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize inotify: %w", err)
	}

	wd, err := unix.InotifyAddWatch(fd, InputDevicesDir, unix.IN_CREATE|unix.IN_DELETE)
	if err != nil {
		unix.Close(fd)
		return nil, fmt.Errorf("failed to add inotify watch on %s: %w", InputDevicesDir, err)
	}

	dm := &DeviceManager{
		inotifyFD: fd,
		watchDesc: wd,
		events:    make(chan DeviceEvent, 64),
		done:      make(chan struct{}),
	}

	return dm, nil
}

// ScanExisting scans existing event devices and sends them as connected events
func (dm *DeviceManager) ScanExisting() error {
	entries, err := os.ReadDir(InputDevicesDir)
	if err != nil {
		return fmt.Errorf("failed to read input devices directory: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if !strings.HasPrefix(entry.Name(), "event") {
			continue
		}

		path := filepath.Join(InputDevicesDir, entry.Name())
		if !dm.sendEvent(DeviceEvent{Path: path, Connected: true}) {
			return nil // Manager closed
		}
	}

	return nil
}

// Watch starts watching for device changes in a goroutine
// Returns a channel that receives device events
func (dm *DeviceManager) Watch() <-chan DeviceEvent {
	return dm.events
}

// ProcessEvents reads inotify events and sends them to the events channel
// This should be called in a loop or goroutine
func (dm *DeviceManager) ProcessEvents() error {
	dm.mu.Lock()
	if dm.closed {
		dm.mu.Unlock()
		return nil
	}
	dm.mu.Unlock()

	buf := make([]byte, 4096)
	n, err := unix.Read(dm.inotifyFD, buf)
	if err != nil {
		if err == unix.EAGAIN || err == unix.EWOULDBLOCK {
			return nil // No events available
		}
		return fmt.Errorf("failed to read inotify events: %w", err)
	}

	if n <= 0 {
		return nil
	}

	offset := 0
	for offset < n {
		if offset+unix.SizeofInotifyEvent > n {
			break
		}

		// Parse inotify event header
		event := (*unix.InotifyEvent)(unsafe.Pointer(&buf[offset]))
		nameLen := int(event.Len)

		if offset+unix.SizeofInotifyEvent+nameLen > n {
			break
		}

		// Get the filename if present
		var name string
		if nameLen > 0 {
			nameBytes := buf[offset+unix.SizeofInotifyEvent : offset+unix.SizeofInotifyEvent+nameLen]
			// Find null terminator
			for i, b := range nameBytes {
				if b == 0 {
					name = string(nameBytes[:i])
					break
				}
			}
			if name == "" {
				name = string(nameBytes)
			}
		}

		offset += unix.SizeofInotifyEvent + nameLen

		// Skip non-event devices
		if !strings.HasPrefix(name, "event") {
			continue
		}

		// Skip directories
		if event.Mask&unix.IN_ISDIR != 0 {
			continue
		}

		path := filepath.Join(InputDevicesDir, name)

		if event.Mask&unix.IN_CREATE != 0 {
			if !dm.sendEvent(DeviceEvent{Path: path, Connected: true}) {
				return nil // Manager closed
			}
		}

		if event.Mask&unix.IN_DELETE != 0 {
			if !dm.sendEvent(DeviceEvent{Path: path, Connected: false}) {
				return nil // Manager closed
			}
		}
	}

	return nil
}

// FD returns the inotify file descriptor for use with epoll/select
func (dm *DeviceManager) FD() int {
	return dm.inotifyFD
}

// Close closes the DeviceManager and releases resources
func (dm *DeviceManager) Close() error {
	dm.mu.Lock()
	if dm.closed {
		dm.mu.Unlock()
		return nil
	}
	dm.closed = true
	close(dm.done)
	inotifyFD := dm.inotifyFD
	watchDesc := dm.watchDesc
	dm.inotifyFD = -1
	dm.watchDesc = -1
	dm.mu.Unlock()

	if watchDesc != -1 {
		//nolint:gosec // watchDesc is always a valid small watch descriptor
		unix.InotifyRmWatch(inotifyFD, uint32(watchDesc))
	}

	if inotifyFD != -1 {
		return unix.Close(inotifyFD)
	}

	return nil
}
