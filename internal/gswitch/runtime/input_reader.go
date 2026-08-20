package runtime

import (
	"fmt"
	"hash/fnv"
	"os"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/unix"
)

// eventBufPool is a pool of byte buffers for reading input events
// This reduces allocations in the hot path (ReadEvent)
var eventBufPool = sync.Pool{
	New: func() any {
		buf := make([]byte, InputEventSize)
		return &buf
	},
}

// keyBitsPool is a pool for key capability bitmaps
// This reduces allocations during device detection
var keyBitsPool = sync.Pool{
	New: func() any {
		buf := make([]byte, KeyBitsSize)
		return &buf
	},
}

// ioctl constants for evdev
const (
	// EVIOCGID - get device ID
	EVIOCGID = 0x80084502
	// EVIOCGNAME - get device name (with size 256)
	EVIOCGNAME = 0x81004506
	// EVIOCGBIT - get event bits
	EVIOCGBIT_EV_KEY = 0x80604521 // EVIOCGBIT(EV_KEY, 96)
)

// input_id structure from linux/input.h
type inputID struct {
	Bustype uint16
	Vendor  uint16
	Product uint16
	Version uint16
}

// Device represents an input device
type Device struct {
	FD   int
	File *os.File
	Path string
	UID  string
	Name string
}

// InputReader manages multiple input devices
type InputReader struct {
	devices   map[int]*Device // fd -> Device
	pathToFD  map[string]int  // path -> fd
	blacklist map[string]bool // UID -> true
	epollFd   int             // epoll file descriptor
	mu        sync.RWMutex
}

// NewInputReader creates a new InputReader
func NewInputReader() *InputReader {
	epollFd, err := unix.EpollCreate1(0)
	if err != nil {
		// Fallback: epoll not available, will use polling
		epollFd = -1
	}

	return &InputReader{
		devices:   make(map[int]*Device),
		pathToFD:  make(map[string]int),
		blacklist: make(map[string]bool),
		epollFd:   epollFd,
	}
}

// AddToBlacklist adds a device UID to the blacklist
func (ir *InputReader) AddToBlacklist(uid string) {
	ir.mu.Lock()
	defer ir.mu.Unlock()
	ir.blacklist[uid] = true
}

// IsBlacklisted checks if a UID is blacklisted
func (ir *InputReader) IsBlacklisted(uid string) bool {
	ir.mu.RLock()
	defer ir.mu.RUnlock()
	return ir.blacklist[uid]
}

// makeDeviceUID creates a unique identifier for a device
func makeDeviceUID(id inputID, name string) string {
	h := fnv.New64a()
	h.Write([]byte(name))
	hash := h.Sum64()

	return fmt.Sprintf("%04x:%04x:%04x:%04x:%016x",
		id.Bustype, id.Vendor, id.Product, id.Version, hash)
}

// getDeviceInfo reads device information using ioctl
func getDeviceInfo(fd int) (inputID, string, error) {
	var id inputID

	// Get device ID
	_, _, errno := syscall.Syscall(
		syscall.SYS_IOCTL,
		uintptr(fd),
		EVIOCGID,
		uintptr(unsafe.Pointer(&id)),
	)
	if errno != 0 {
		return id, "", fmt.Errorf("EVIOCGID failed: %w", errno)
	}

	// Get device name
	nameBuf := make([]byte, MaxDeviceNameLength)
	_, _, errno = syscall.Syscall(
		syscall.SYS_IOCTL,
		uintptr(fd),
		EVIOCGNAME,
		uintptr(unsafe.Pointer(&nameBuf[0])),
	)
	if errno != 0 {
		return id, "", fmt.Errorf("EVIOCGNAME failed: %w", errno)
	}

	// Find null terminator
	name := ""
	for i, b := range nameBuf {
		if b == 0 {
			name = string(nameBuf[:i])
			break
		}
	}
	if name == "" {
		name = string(nameBuf)
	}

	return id, name, nil
}

// hasKeyboardCapability checks if device supports keyboard events
func hasKeyboardCapability(fd int) bool {
	var evBits uint32
	_, _, errno := syscall.Syscall(
		syscall.SYS_IOCTL,
		uintptr(fd),
		EVIOCGBIT_EV,
		uintptr(unsafe.Pointer(&evBits)),
	)
	if errno != 0 {
		return false
	}
	// Check if EV_KEY bit is set (bit 1)
	return evBits&(1<<EV_KEY) != 0
}

// HasKeyboardCapability reports whether device supports keyboard events.
func HasKeyboardCapability(fd int) bool {
	return hasKeyboardCapability(fd)
}

// hasKeyACapability checks if device has KEY_A (basic keyboard test)
func hasKeyACapability(fd int) bool {
	// Get buffer from pool to reduce allocations
	keyBitsPtr, ok := keyBitsPool.Get().(*[]byte)
	if !ok || keyBitsPtr == nil {
		buf := make([]byte, KeyBitsSize)
		keyBitsPtr = &buf
	}
	defer keyBitsPool.Put(keyBitsPtr)
	keyBits := *keyBitsPtr

	// Clear buffer before use (pool buffers may contain old data)
	clear(keyBits)

	_, _, errno := syscall.Syscall(
		syscall.SYS_IOCTL,
		uintptr(fd),
		EVIOCGBIT_EV_KEY,
		uintptr(unsafe.Pointer(&keyBits[0])),
	)
	if errno != 0 {
		return false
	}

	// Check for KEY_A (30) - byte 3, bit 6
	keyA := 30
	byteIndex := keyA / 8
	bitIndex := keyA % 8
	return keyBits[byteIndex]&(1<<bitIndex) != 0
}

// AddDevice adds a new input device
// Returns the device if successfully added, nil if skipped/failed
func (ir *InputReader) AddDevice(path string) (*Device, error) {
	ir.mu.Lock()
	defer ir.mu.Unlock()

	// Check if already added
	if _, exists := ir.pathToFD[path]; exists {
		return nil, fmt.Errorf("device already added: %s", path)
	}

	// Open device
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_NONBLOCK, 0)
	if err != nil {
		return nil, fmt.Errorf("failed to open %s: %w", path, err)
	}

	// Check if it's a keyboard
	if !hasKeyboardCapability(fd) || !hasKeyACapability(fd) {
		unix.Close(fd)
		return nil, fmt.Errorf("device is not a keyboard: %s", path)
	}

	// Get device info
	id, name, err := getDeviceInfo(fd)
	if err != nil {
		unix.Close(fd)
		return nil, fmt.Errorf("failed to get device info for %s: %w", path, err)
	}

	uid := makeDeviceUID(id, name)

	// Check blacklist
	if ir.blacklist[uid] {
		unix.Close(fd)
		return nil, fmt.Errorf("device is blacklisted: %s (UID=%s)", name, uid)
	}

	// Create os.File wrapper for easier reading
	file := os.NewFile(uintptr(fd), path)

	device := &Device{
		FD:   fd,
		File: file,
		Path: path,
		UID:  uid,
		Name: name,
	}

	// Add to epoll if available
	if ir.epollFd >= 0 {
		event := unix.EpollEvent{
			Events: unix.EPOLLIN,
			Fd:     int32(fd), //nolint:gosec // fd is always a valid small file descriptor
		}
		if err := unix.EpollCtl(ir.epollFd, unix.EPOLL_CTL_ADD, fd, &event); err != nil {
			// Epoll failed, disable it and fall back to polling
			unix.Close(ir.epollFd)
			ir.epollFd = -1
		}
	}

	ir.devices[fd] = device
	ir.pathToFD[path] = fd

	return device, nil
}

// RemoveDevice removes a device by path
func (ir *InputReader) RemoveDevice(path string) error {
	ir.mu.Lock()
	defer ir.mu.Unlock()

	fd, exists := ir.pathToFD[path]
	if !exists {
		return fmt.Errorf("device not found: %s", path)
	}

	// Remove from epoll before closing (ignore error - fd will be closed anyway)
	if ir.epollFd >= 0 {
		_ = unix.EpollCtl(ir.epollFd, unix.EPOLL_CTL_DEL, fd, nil)
	}

	device := ir.devices[fd]
	if device != nil && device.File != nil {
		device.File.Close()
	}

	delete(ir.devices, fd)
	delete(ir.pathToFD, path)

	return nil
}

// GetDevice returns a device by file descriptor
func (ir *InputReader) GetDevice(fd int) *Device {
	ir.mu.RLock()
	defer ir.mu.RUnlock()
	return ir.devices[fd]
}

// GetDeviceByPath returns a device by path
func (ir *InputReader) GetDeviceByPath(path string) *Device {
	ir.mu.RLock()
	defer ir.mu.RUnlock()
	fd, exists := ir.pathToFD[path]
	if !exists {
		return nil
	}
	return ir.devices[fd]
}

// GetAllDevices returns all devices
func (ir *InputReader) GetAllDevices() []*Device {
	ir.mu.RLock()
	defer ir.mu.RUnlock()

	devices := make([]*Device, 0, len(ir.devices))
	for _, d := range ir.devices {
		devices = append(devices, d)
	}
	return devices
}

// GetAllFDs returns all file descriptors
func (ir *InputReader) GetAllFDs() []int {
	ir.mu.RLock()
	defer ir.mu.RUnlock()

	fds := make([]int, 0, len(ir.devices))
	for fd := range ir.devices {
		fds = append(fds, fd)
	}
	return fds
}

// ReadEvent reads an input event from a device
// Returns the event and true if successful, false if no event available
func (ir *InputReader) ReadEvent(fd int) (*InputEvent, bool) {
	ir.mu.RLock()
	defer ir.mu.RUnlock()

	device := ir.devices[fd]
	if device == nil {
		return nil, false
	}

	// Get buffer from pool to reduce allocations
	eventBufPtr, ok := eventBufPool.Get().(*[]byte)
	if !ok || eventBufPtr == nil {
		buf := make([]byte, InputEventSize)
		eventBufPtr = &buf
	}
	eventBuf := *eventBufPtr

	// Safe to read while holding RLock since fd is O_NONBLOCK (won't block)
	n, err := unix.Read(fd, eventBuf)
	if err != nil || n != InputEventSize {
		eventBufPool.Put(eventBufPtr)
		return nil, false
	}

	// Copy event data before returning buffer to pool
	event := *(*InputEvent)(unsafe.Pointer(&eventBuf[0]))
	eventBufPool.Put(eventBufPtr)

	return &event, true
}

// Flush discards all pending events from all devices
func (ir *InputReader) Flush() {
	ir.mu.RLock()
	defer ir.mu.RUnlock()

	// Get buffer from pool to reduce allocations
	eventBufPtr, ok := eventBufPool.Get().(*[]byte)
	if !ok || eventBufPtr == nil {
		buf := make([]byte, InputEventSize)
		eventBufPtr = &buf
	}
	defer eventBufPool.Put(eventBufPtr)
	eventBuf := *eventBufPtr

	for _, device := range ir.devices {
		// Safe to read while holding RLock since fd is O_NONBLOCK (won't block)
		for {
			n, err := unix.Read(device.FD, eventBuf)
			if err != nil || n != InputEventSize {
				break
			}
		}
	}
}

// EVIOCGRAB ioctl constant
const EVIOCGRAB = 0x40044590

// GrabAll grabs all input devices exclusively
// This prevents events from reaching other applications
func (ir *InputReader) GrabAll() {
	ir.mu.RLock()
	defer ir.mu.RUnlock()

	for fd := range ir.devices {
		unix.IoctlSetInt(fd, EVIOCGRAB, 1)
	}
}

// UngrabAll releases all grabbed input devices
func (ir *InputReader) UngrabAll() {
	ir.mu.RLock()
	defer ir.mu.RUnlock()

	for fd := range ir.devices {
		unix.IoctlSetInt(fd, EVIOCGRAB, 0)
	}
}

// Count returns the number of active devices
func (ir *InputReader) Count() int {
	ir.mu.RLock()
	defer ir.mu.RUnlock()
	return len(ir.devices)
}

// Close closes all devices and epoll
func (ir *InputReader) Close() {
	ir.mu.Lock()
	defer ir.mu.Unlock()

	for _, device := range ir.devices {
		if device.File != nil {
			device.File.Close()
		}
	}

	if ir.epollFd >= 0 {
		unix.Close(ir.epollFd)
		ir.epollFd = -1
	}

	ir.devices = make(map[int]*Device)
	ir.pathToFD = make(map[string]int)
}

// HasEpoll returns true if epoll is available
func (ir *InputReader) HasEpoll() bool {
	return ir.epollFd >= 0
}

// WaitForEvents waits for input events using epoll
// Returns slice of file descriptors that have events ready
// timeoutMs: -1 for blocking, 0 for non-blocking, >0 for timeout in ms
func (ir *InputReader) WaitForEvents(timeoutMs int) []int {
	if ir.epollFd < 0 {
		return nil
	}

	events := make([]unix.EpollEvent, 16)
	n, err := unix.EpollWait(ir.epollFd, events, timeoutMs)
	if err != nil || n <= 0 {
		return nil
	}

	fds := make([]int, 0, n)
	for i := range n {
		fds = append(fds, int(events[i].Fd))
	}
	return fds
}
