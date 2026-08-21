package tray

import (
	"errors"
	"fmt"
	"hash/fnv"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// KeyboardDevice represents a keyboard input device.
type KeyboardDevice struct {
	UID  string // Unique identifier (bustype:vendor:product:version:hash)
	Name string // Human-readable name
	Path string // /dev/input/eventX
}

// DeviceManager manages input devices for the GUI.
// Devices are enumerated via sysfs, which is world-readable — no root or
// input group membership is required, unlike opening /dev/input nodes.
type DeviceManager struct {
	sysDir string // sysfs input class directory (overridable in tests)
}

// NewDeviceManager creates a new device manager.
func NewDeviceManager() *DeviceManager {
	return &DeviceManager{sysDir: sysClassInputDir}
}

const (
	evKEY            = 0x01                // EV_KEY event type bit
	keyA             = 30                  // KEY_A code (basic keyboard test)
	inputDevicesDir  = "/dev/input/"       // Input devices directory
	sysClassInputDir = "/sys/class/input/" // Sysfs input class directory

	// gswitch virtual keyboard identifiers (from uinput.go)
	gswitchVendorID  = 0x0777
	gswitchProductID = 0x0777
)

// ErrNoAccess indicates permission denied when accessing devices.
var ErrNoAccess = errors.New("no access to input devices")

// ErrVirtualKeyboard indicates this is the gswitch virtual keyboard.
var ErrVirtualKeyboard = errors.New("gswitch virtual keyboard")

// GetKeyboards returns a list of connected keyboard devices.
func (m *DeviceManager) GetKeyboards() ([]KeyboardDevice, error) {
	entries, err := os.ReadDir(m.sysDir)
	if err != nil {
		if os.IsPermission(err) {
			return nil, ErrNoAccess
		}
		return nil, fmt.Errorf("failed to read %s: %w", m.sysDir, err)
	}

	// Use map to deduplicate by UID (same device can have multiple event nodes)
	keyboardsByUID := make(map[string]KeyboardDevice)

	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), "event") {
			continue
		}

		device, err := m.probeSysfsDevice(entry.Name())
		if err != nil {
			continue
		}

		// Deduplicate by UID - keep first occurrence
		if _, exists := keyboardsByUID[device.UID]; !exists {
			keyboardsByUID[device.UID] = *device
		}
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

// probeSysfsDevice reads device information from sysfs and checks if it's a keyboard.
func (m *DeviceManager) probeSysfsDevice(event string) (*KeyboardDevice, error) {
	base := filepath.Join(m.sysDir, event, "device")

	// Check EV_KEY capability
	ev, err := readSysfsFile(filepath.Join(base, "capabilities", "ev"))
	if err != nil {
		return nil, err
	}
	if !hexBitmapHasBit(ev, evKEY) {
		return nil, errors.New("not a keyboard: no EV_KEY capability")
	}

	// Check KEY_A capability (basic keyboard test)
	key, err := readSysfsFile(filepath.Join(base, "capabilities", "key"))
	if err != nil {
		return nil, err
	}
	if !hexBitmapHasBit(key, keyA) {
		return nil, errors.New("not a keyboard: no KEY_A capability")
	}

	name, err := readSysfsFile(filepath.Join(base, "name"))
	if err != nil {
		return nil, err
	}

	var ids [4]uint16
	for i, f := range []string{"bustype", "vendor", "product", "version"} {
		raw, err := readSysfsFile(filepath.Join(base, "id", f))
		if err != nil {
			return nil, err
		}
		v, err := strconv.ParseUint(raw, 16, 16)
		if err != nil {
			return nil, fmt.Errorf("bad id/%s %q: %w", f, raw, err)
		}
		ids[i] = uint16(v)
	}

	// Filter out gswitch's own virtual keyboard
	if ids[1] == gswitchVendorID && ids[2] == gswitchProductID {
		return nil, ErrVirtualKeyboard
	}

	return &KeyboardDevice{
		UID:  makeDeviceUID(ids, name),
		Name: name,
		Path: inputDevicesDir + event,
	}, nil
}

// readSysfsFile reads a sysfs attribute and trims whitespace.
func readSysfsFile(path string) (string, error) {
	data, err := os.ReadFile(path) //nolint:gosec // paths are built from sysfs constants
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

// hexBitmapHasBit checks a bit in a sysfs capability bitmap.
// The bitmap is space-separated 64-bit hex words, most significant first:
// the LAST word holds bits 0..63.
func hexBitmapHasBit(bitmap string, bit int) bool {
	words := strings.Fields(bitmap)
	if len(words) == 0 {
		return false
	}
	wordIdx := bit / 64
	if wordIdx >= len(words) {
		return false
	}
	word, err := strconv.ParseUint(words[len(words)-1-wordIdx], 16, 64)
	if err != nil {
		return false
	}
	return word&(1<<(bit%64)) != 0
}

// makeDeviceUID creates a unique identifier for a device.
// This matches the algorithm in input_reader.go.
func makeDeviceUID(ids [4]uint16, name string) string {
	h := fnv.New64a()
	h.Write([]byte(name))
	hash := h.Sum64()

	return fmt.Sprintf("%04x:%04x:%04x:%04x:%016x",
		ids[0], ids[1], ids[2], ids[3], hash)
}
