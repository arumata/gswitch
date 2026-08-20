package tray

import (
	"errors"
	"fmt"
	"hash/fnv"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"unsafe"
)

// KeyboardDevice represents a keyboard input device.
type KeyboardDevice struct {
	UID  string // Unique identifier (bustype:vendor:product:version:hash)
	Name string // Human-readable name
	Path string // /dev/input/eventX
}

// DeviceManager manages input devices for the GUI.
type DeviceManager struct{}

// NewDeviceManager creates a new device manager.
func NewDeviceManager() *DeviceManager {
	return &DeviceManager{}
}

// ioctl constants for evdev
const (
	ioctlEVIOCGID         = 0x80084502          // EVIOCGID - get device ID
	ioctlEVIOCGNAME       = 0x81004506          // EVIOCGNAME - get device name (size 256)
	ioctlEVIOCGBIT_EV     = 0x80044520          // EVIOCGBIT(0, 4) - get event type bits
	ioctlEVIOCGBIT_EV_KEY = 0x80604521          // EVIOCGBIT(EV_KEY, 96)
	evKEY                 = 0x01                // EV_KEY event type
	maxDeviceNameLength   = 256                 // Max device name length
	keyBitsSize           = 96                  // Key capability bitmap size
	inputDevicesDir       = "/dev/input/"       // Input devices directory
	sysClassInputDir      = "/sys/class/input/" // Sysfs input class directory

	// gswitch virtual keyboard identifiers (from uinput.go)
	gswitchVendorID  = 0x0777
	gswitchProductID = 0x0777
)

// inputID structure from linux/input.h
type inputID struct {
	Bustype uint16
	Vendor  uint16
	Product uint16
	Version uint16
}

// ErrNoAccess indicates permission denied when accessing devices.
var ErrNoAccess = errors.New("no access to input devices")

// GetKeyboards returns a list of connected keyboard devices.
func (m *DeviceManager) GetKeyboards() ([]KeyboardDevice, error) {
	entries, err := os.ReadDir(inputDevicesDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read %s: %w", inputDevicesDir, err)
	}

	// Use map to deduplicate by UID (same device can have multiple event nodes)
	keyboardsByUID := make(map[string]KeyboardDevice)
	permissionDenied := 0
	eventDevices := 0

	for _, entry := range entries {
		// Skip directories and non-event files
		if entry.IsDir() || !strings.HasPrefix(entry.Name(), "event") {
			continue
		}

		eventDevices++
		path := filepath.Join(inputDevicesDir, entry.Name())
		device, err := m.probeDevice(path)
		if err != nil {
			// Track permission errors separately
			if os.IsPermission(err) {
				permissionDenied++
			}
			continue
		}

		// Deduplicate by UID - keep first occurrence
		if _, exists := keyboardsByUID[device.UID]; !exists {
			keyboardsByUID[device.UID] = *device
		}
	}

	// If we found event devices but couldn't open any due to permissions
	if len(keyboardsByUID) == 0 && permissionDenied > 0 && permissionDenied == eventDevices {
		return nil, ErrNoAccess
	}

	// Convert map to slice
	keyboards := make([]KeyboardDevice, 0, len(keyboardsByUID))
	for _, kb := range keyboardsByUID {
		keyboards = append(keyboards, kb)
	}

	// Sort by name for consistent display
	sort.Slice(keyboards, func(i, j int) bool {
		return keyboards[i].Name < keyboards[j].Name
	})

	return keyboards, nil
}

// ErrVirtualKeyboard indicates this is the gswitch virtual keyboard.
var ErrVirtualKeyboard = errors.New("gswitch virtual keyboard")

// probeDevice reads device information and checks if it's a keyboard.
func (m *DeviceManager) probeDevice(path string) (*KeyboardDevice, error) {
	fd, err := syscall.Open(path, syscall.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		// Wrap syscall.Errno to work with os.IsPermission
		return nil, &os.PathError{Op: "open", Path: path, Err: err}
	}
	defer syscall.Close(fd)

	// Check if it has EV_KEY capability
	if !hasKeyboardCapability(fd) {
		return nil, errors.New("not a keyboard: no EV_KEY capability")
	}

	// Check if it has KEY_A (basic keyboard test)
	if !hasKeyACapability(fd) {
		return nil, errors.New("not a keyboard: no KEY_A capability")
	}

	// Get device info
	id, name, err := getDeviceInfo(fd)
	if err != nil {
		return nil, fmt.Errorf("failed to get device info: %w", err)
	}

	// Filter out gswitch's own virtual keyboard
	if id.Vendor == gswitchVendorID && id.Product == gswitchProductID {
		return nil, ErrVirtualKeyboard
	}

	// Generate UID using the same algorithm as in input_reader.go
	uid := makeDeviceUID(id, name)

	return &KeyboardDevice{
		UID:  uid,
		Name: name,
		Path: path,
	}, nil
}

// hasKeyboardCapability checks if device supports keyboard events.
func hasKeyboardCapability(fd int) bool {
	var evBits uint32
	_, _, errno := syscall.Syscall(
		syscall.SYS_IOCTL,
		uintptr(fd),
		ioctlEVIOCGBIT_EV,
		uintptr(unsafe.Pointer(&evBits)),
	)
	if errno != 0 {
		return false
	}
	// Check if EV_KEY bit is set (bit 1)
	return evBits&(1<<evKEY) != 0
}

// hasKeyACapability checks if device has KEY_A (basic keyboard test).
func hasKeyACapability(fd int) bool {
	keyBits := make([]byte, keyBitsSize)

	_, _, errno := syscall.Syscall(
		syscall.SYS_IOCTL,
		uintptr(fd),
		ioctlEVIOCGBIT_EV_KEY,
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

// getDeviceInfo reads device information using ioctl.
func getDeviceInfo(fd int) (inputID, string, error) {
	var id inputID

	// Get device ID
	_, _, errno := syscall.Syscall(
		syscall.SYS_IOCTL,
		uintptr(fd),
		ioctlEVIOCGID,
		uintptr(unsafe.Pointer(&id)),
	)
	if errno != 0 {
		return id, "", fmt.Errorf("EVIOCGID failed: %w", errno)
	}

	// Get device name
	nameBuf := make([]byte, maxDeviceNameLength)
	_, _, errno = syscall.Syscall(
		syscall.SYS_IOCTL,
		uintptr(fd),
		ioctlEVIOCGNAME,
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

// makeDeviceUID creates a unique identifier for a device.
// This matches the algorithm in input_reader.go.
func makeDeviceUID(id inputID, name string) string {
	h := fnv.New64a()
	h.Write([]byte(name))
	hash := h.Sum64()

	return fmt.Sprintf("%04x:%04x:%04x:%04x:%016x",
		id.Bustype, id.Vendor, id.Product, id.Version, hash)
}
