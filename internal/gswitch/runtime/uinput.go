package runtime

import (
	"fmt"
	"os"
	"syscall"
	"time"
	"unsafe"
)

// VirtualKeyboard represents a virtual keyboard device via uinput
type VirtualKeyboard struct {
	fd *os.File
}

// NewVirtualKeyboard creates a new virtual keyboard device
func NewVirtualKeyboard() (*VirtualKeyboard, error) {
	fd, err := os.OpenFile(UinputFile, os.O_WRONLY|syscall.O_SYNC, 0)
	if err != nil {
		return nil, fmt.Errorf("cannot open %s: %w", UinputFile, err)
	}

	vk := &VirtualKeyboard{fd: fd}

	// Set up uinput device
	if err := vk.setupDevice(); err != nil {
		fd.Close()
		return nil, err
	}

	return vk, nil
}

func (vk *VirtualKeyboard) setupDevice() error {
	fd := int(vk.fd.Fd())

	// Set EV_SYN and EV_KEY event bits
	if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), UI_SET_EVBIT, EV_SYN); errno != 0 {
		return fmt.Errorf("ioctl UI_SET_EVBIT EV_SYN failed: %w", errno)
	}
	if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), UI_SET_EVBIT, EV_KEY); errno != 0 {
		return fmt.Errorf("ioctl UI_SET_EVBIT EV_KEY failed: %w", errno)
	}

	// Set all Linux key bits, including KEY_KEYBOARD/XF86Keyboard.
	for _, code := range virtualKeyboardKeyCodes() {
		if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), UI_SET_KEYBIT, uintptr(code)); errno != 0 {
			return fmt.Errorf("ioctl UI_SET_KEYBIT %d failed: %w", code, errno)
		}
	}

	// Set up device info
	setup := UinputSetup{}
	setup.ID.Bustype = BUS_USB
	setup.ID.Vendor = 0x0777
	setup.ID.Product = 0x0777
	copy(setup.Name[:], "gswitch virtual input device")

	if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), UI_DEV_SETUP, uintptr(unsafe.Pointer(&setup))); errno != 0 {
		return fmt.Errorf("ioctl UI_DEV_SETUP failed: %w", errno)
	}

	// Create the device
	if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), UI_DEV_CREATE, 0); errno != 0 {
		return fmt.Errorf("ioctl UI_DEV_CREATE failed: %w", errno)
	}

	return nil
}

func virtualKeyboardKeyCodes() []uint16 {
	codes := make([]uint16, int(KEY_MAX)+1)
	for code := uint16(0); code <= KEY_MAX; code++ {
		codes[code] = code
	}
	return codes
}

// Close destroys the virtual keyboard device
func (vk *VirtualKeyboard) Close() error {
	fd := int(vk.fd.Fd())
	if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), UI_DEV_DESTROY, 0); errno != 0 {
		// Log but don't fail - we still need to close the file descriptor
		_ = errno // Best effort cleanup
	}
	return vk.fd.Close()
}

// EmitKey sends a key event through the virtual keyboard
func (vk *VirtualKeyboard) EmitKey(code uint16, value int32) error {
	var tv syscall.Timeval
	syscall.Gettimeofday(&tv)

	// Key event
	keyEvent := InputEvent{
		Time:  tv,
		Type:  EV_KEY,
		Code:  code,
		Value: value,
	}

	// Sync event with properly handled microseconds overflow
	tv.Usec += 100
	if tv.Usec >= 1000000 {
		tv.Sec++
		tv.Usec -= 1000000
	}
	synEvent := InputEvent{
		Time:  tv,
		Type:  EV_SYN,
		Code:  SYN_REPORT,
		Value: 0,
	}

	// Write key event
	keyBytes := (*[InputEventSize]byte)(unsafe.Pointer(&keyEvent))[:]
	if _, err := vk.fd.Write(keyBytes); err != nil {
		return fmt.Errorf("failed to write key event: %w", err)
	}

	// Write sync event
	synBytes := (*[InputEventSize]byte)(unsafe.Pointer(&synEvent))[:]
	if _, err := vk.fd.Write(synBytes); err != nil {
		return fmt.Errorf("failed to write sync event: %w", err)
	}

	return nil
}

// PressKey simulates a key press (down + up) with a small delay between
func (vk *VirtualKeyboard) PressKey(code uint16, delayMs int) error {
	if err := vk.EmitKey(code, 1); err != nil {
		return err
	}
	time.Sleep(time.Duration(delayMs) * time.Millisecond)
	return vk.EmitKey(code, 0)
}

// KeyDown simulates a key press (down only)
func (vk *VirtualKeyboard) KeyDown(code uint16) error {
	return vk.EmitKey(code, 1)
}

// KeyUp simulates a key release (up only)
func (vk *VirtualKeyboard) KeyUp(code uint16) error {
	return vk.EmitKey(code, 0)
}
