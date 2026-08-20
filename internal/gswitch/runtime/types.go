//go:generate go run ./gen

package runtime

import (
	"syscall"
	"unsafe"
)

// Linux input event structure
type InputEvent struct {
	Time  syscall.Timeval
	Type  uint16
	Code  uint16
	Value int32
}

const InputEventSize = int(unsafe.Sizeof(InputEvent{}))

// uinput_setup structure
type UinputSetup struct {
	ID struct {
		Bustype uint16
		Vendor  uint16
		Product uint16
		Version uint16
	}
	Name         [80]byte
	FFEffectsMax uint32
}

// Buffer action type
type BufferAction int

const (
	KeepBuffer BufferAction = iota
	ReplaceAll
	ReplaceWord
)

// Paths
const (
	SystemdUnitFile = "/lib/systemd/system/gswitch.service"
	ConfigFile      = "/etc/gswitch/default.conf"
	InputDevicesDir = "/dev/input/"
	UinputFile      = "/dev/uinput"
)

// Event types
const (
	EV_SYN = 0x00
	EV_KEY = 0x01
)

// Synchronization event codes
const (
	SYN_REPORT = 0x00
)

// Bus types
const (
	BUS_USB = 0x03
)

// ioctl commands for uinput
const (
	UI_SET_EVBIT   = 0x40045564
	UI_SET_KEYBIT  = 0x40045565
	UI_DEV_SETUP   = 0x405C5503
	UI_DEV_CREATE  = 0x00005501
	UI_DEV_DESTROY = 0x00005502
)

// ioctl commands for evdev
const (
	// EVIOCGBIT(0, 4) - get event type bits (EV_KEY, EV_REL, etc.)
	EVIOCGBIT_EV = 0x80044520
)

// Key codes
const (
	KEY_BACKSPACE = 14
	KEY_SPACE     = 57
	KEY_ENTER     = 28
	KEY_KPENTER   = 96
	KEY_LEFTCTRL  = 29
	KEY_RIGHTCTRL = 97
	KEY_C         = 46
	KEY_V         = 47
)

// Timeouts and limits for keyboard detection (in iterations)
const (
	// DetectKeyboardIterations is ~57 seconds at 10ms per iteration
	DetectKeyboardIterations = 5700
	// DetectKeyIterations is 60 seconds at 10ms per iteration
	DetectKeyIterations = 6000
	// WaitForKeyboardIterations is 60 seconds at 100ms per iteration
	WaitForKeyboardIterations = 600
	// DetectSleepMs is the sleep duration between detection attempts
	DetectSleepMs = 10
)

// Buffer limits
const (
	// MaxKeyBufSize limits the key buffer to prevent unbounded memory growth
	// This is approximately 4096 keypresses which is more than enough for any phrase
	MaxKeyBufSize = 4096
)

// Device info buffer sizes
const (
	// MaxDeviceNameLength is the maximum length of a device name
	MaxDeviceNameLength = 256
	// KeyBitsSize is the size of the key capability bitmap (covers all keys up to 768)
	KeyBitsSize = 96
)

// Timing constants for event loop
const (
	// EventLoopTimeoutMs is the timeout for main event loop select
	EventLoopTimeoutMs = 100
	// ShutdownTimeoutMs is the timeout waiting for goroutines to finish
	ShutdownTimeoutMs = 500
	// PollingIntervalMs is the interval for polling-based event reading
	PollingIntervalMs = 5
	// DeviceSettleMs is the delay after device hotplug before reading
	DeviceSettleMs = 100
	// SelectionConversionDelayMs is the initial delay before reading selection
	SelectionConversionInitDelayMs = 20
	// ClipboardWriteDelayMs is the delay after writing to clipboard
	ClipboardWriteDelayMs = 50
)

// Configuration limits
const (
	// MaxDelayMs is the maximum allowed delay value in config
	MaxDelayMs = 1000
	// MaxLayoutSwitchDelayMs is the maximum allowed layout switch delay
	MaxLayoutSwitchDelayMs = 2000
)

// FNV-1a hash constants
const (
	// FNV1aOffset is the FNV-1a offset basis
	FNV1aOffset = 14695981039346656037
	// FNV1aPrime is the FNV-1a prime
	FNV1aPrime = 1099511628211
)

// Keys to watch and replace (Letters)
var Letters = map[uint16]bool{
	2: true, 3: true, 4: true, 5: true, 6: true, 7: true, 8: true, 9: true, 10: true, 11: true,
	12: true, 13: true, 14: true, 16: true, 17: true, 18: true, 19: true, 20: true, 21: true,
	22: true, 23: true, 24: true, 25: true, 26: true, 27: true, 28: true, 30: true, 31: true,
	32: true, 33: true, 34: true, 35: true, 36: true, 37: true, 38: true, 39: true, 40: true,
	41: true, 43: true, 44: true, 45: true, 46: true, 47: true, 48: true, 49: true, 50: true,
	51: true, 52: true, 53: true, 55: true, 57: true, 71: true, 72: true, 73: true, 74: true,
	75: true, 76: true, 77: true, 78: true, 79: true, 80: true, 81: true, 82: true, 83: true,
	96: true, 98: true,
}

// Shift keys
var Shifts = map[uint16]bool{
	42: true,
	54: true,
}

// Buffer killers (keys that clear the buffer)
var BufKillers = map[uint16]bool{
	15: true, 29: true, 56: true, 97: true, 100: true,
	102: true, 103: true, 104: true, 105: true, 106: true,
	107: true, 108: true, 109: true, 110: true,
}

// Key names for debugging
var KeyName = []string{
	"RESERVED", "ESC", "1", "2", "3", "4", "5", "6", "7", "8", "9", "0", "MINUS", "EQUAL",
	"BACKSPACE", "TAB", "Q", "W", "E", "R", "T", "Y", "U", "I", "O", "P", "LEFTBRACE",
	"RIGHTBRACE", "ENTER", "LEFTCTRL", "A", "S", "D", "F", "G", "H", "J", "K", "L", "SEMICOLON",
	"APOSTROPHE", "GRAVE", "LEFTSHIFT", "BACKSLASH", "Z", "X", "C", "V", "B", "N", "M", "COMMA",
	"DOT", "SLASH", "RIGHTSHIFT", "KPASTERISK", "LEFTALT", "SPACE", "CAPSLOCK", "F1", "F2",
	"F3", "F4", "F5", "F6", "F7", "F8", "F9", "F10", "NUMLOCK", "SCROLLLOCK", "KP7", "KP8",
	"KP9", "KPMINUS", "KP4", "KP5", "KP6", "KPPLUS", "KP1", "KP2", "KP3", "KP0", "KPDOT",
	"(84)", "ZENKAKUHANKAKU", "102ND", "F11", "F12", "RO", "KATAKANA", "HIRAGANA", "HENKAN",
	"KATAKANAHIRAGANA", "MUHENKAN", "KPJPCOMMA", "KPENTER", "RIGHTCTRL", "KPSLASH", "SYSRQ",
	"RIGHTALT", "LINEFEED", "HOME", "UP", "PAGEUP", "LEFT", "RIGHT", "END", "DOWN", "PAGEDOWN",
	"INSERT", "DELETE", "MACRO", "MUTE", "VOLUMEDOWN", "VOLUMEUP", "POWER", "KPEQUAL",
	"KPPLUSMINUS", "PAUSE", "SCALE", "KPCOMMA", "HANGEUL", "HANJA", "YEN", "LEFTMETA",
	"RIGHTMETA", "COMPOSE", "STOP", "AGAIN", "PROPS", "UNDO", "FRONT", "COPY", "OPEN", "PASTE",
	"FIND", "CUT", "HELP", "MENU", "CALC", "SETUP", "SLEEP", "WAKEUP", "FILE", "SENDFILE",
	"DELETEFILE", "XFER", "PROG1", "PROG2", "WWW", "MSDOS", "COFFEE", "ROTATE_DISPLAY",
	"CYCLEWINDOWS", "MAIL", "BOOKMARKS", "COMPUTER", "BACK", "FORWARD", "CLOSECD", "EJECTCD",
	"EJECTCLOSECD", "NEXTSONG", "PLAYPAUSE", "PREVIOUSSONG", "STOPCD", "RECORD", "REWIND",
	"PHONE", "ISO", "CONFIG", "HOMEPAGE", "REFRESH", "EXIT", "MOVE", "EDIT", "SCROLLUP",
	"SCROLLDOWN", "KPLEFTPAREN", "KPRIGHTPAREN", "NEW", "REDO", "F13", "F14", "F15", "F16",
	"F17", "F18", "F19", "F20", "F21", "F22", "F23", "F24", "(195)", "(196)", "(197)", "(198)",
	"(199)", "PLAYCD", "PAUSECD", "PROG3", "PROG4", "ALL_APPLICATIONS", "SUSPEND", "CLOSE",
	"PLAY", "FASTFORWARD", "BASSBOOST", "PRINT", "HP", "CAMERA", "SOUND", "QUESTION", "EMAIL",
	"CHAT", "SEARCH", "CONNECT", "FINANCE", "SPORT", "SHOP", "ALTERASE", "CANCEL",
	"BRIGHTNESSDOWN", "BRIGHTNESSUP", "MEDIA", "SWITCHVIDEOMODE", "KBDILLUMTOGGLE",
	"KBDILLUMDOWN", "KBDILLUMUP", "SEND", "REPLY", "FORWARDMAIL", "SAVE", "DOCUMENTS", "BATTERY",
	"BLUETOOTH", "WLAN", "UWB", "UNKNOWN", "VIDEO_NEXT", "VIDEO_PREV", "BRIGHTNESS_CYCLE",
	"BRIGHTNESS_AUTO", "DISPLAY_OFF", "WWAN", "RFKILL", "MICMUTE",
}

func getKeyName(code uint16) string {
	if int(code) < len(KeyName) {
		return KeyName[code]
	}
	return "UNKNOWN"
}

// GetKeyName returns human-readable key name by scan code.
func GetKeyName(code uint16) string {
	return getKeyName(code)
}

func getKeyAction(value int32) string {
	switch value {
	case 0:
		return "up"
	case 1:
		return "down"
	case 2:
		return "autorepeat"
	default:
		return "unknown"
	}
}
