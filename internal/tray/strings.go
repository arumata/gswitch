package tray

// UI strings - prepared for future localization.
// All user-visible strings should be defined here.

// Window titles
const (
	strWindowTitle = "gswitch - Settings"
)

// Section titles
const (
	strSectionService = "Service"
	strSectionKeys    = "Keys"
	strSectionLayouts = "Layouts"
	strSectionTray    = "Tray"
	strSectionDelays  = "Delays"
	strSectionDevices = "Devices"
)

// Service section
const (
	strLabelStatus        = "Status:"
	strStatusUnknown      = "Unknown"
	strStatusRunning      = "Running"
	strStatusStopped      = "Stopped"
	strStatusFailed       = "Error"
	strStatusNotInstalled = "Not installed"
	strButtonRestart      = "Restart"
	strButtonStart        = "Start"
	strButtonStop         = "Stop"
	strCheckAutostart     = "Autostart on system startup"
)

// Tray menu - service status
const (
	strTrayServiceRunning      = "● Service is running"
	strTrayServiceStopped      = "○ Service is stopped"
	strTrayServiceFailed       = "✖ Service: error"
	strTrayServiceNotInstalled = "○ Service is not installed"
	strTrayServiceUnknown      = "? Service: unknown"
)

// Tray tooltips and detection status
const (
	strTooltipOK             = "gswitch: %s (%s)" // e.g. "gswitch: Alt+Shift (xkb)"
	strTooltipNeedsConfig    = "gswitch: configuration required"
	strTooltipServiceError   = "gswitch: service is not running"
	strTooltipServiceStopped = "service is not running" // For tooltip composition (without "gswitch:" prefix)
	strTooltipServiceFailed  = "service: error"         // For tooltip composition (without "gswitch:" prefix)
	strTooltipDetectError    = "gswitch: auto-detection error"
	strTooltipClickConfig    = "Click to configure"
	strTrayRefresh           = "Refresh status"
)

// First-run dialog (GNOME)
const (
	strFirstRunTitle   = "gswitch: configuration required"
	strFirstRunMessage = "Auto-detect could not determine the layout switch key.\nClick 'Configure' to select it."
	strFirstRunConfig  = "Configure"
	strFirstRunDismiss = "Don't show again"
)

// Error messages
const (
	strErrorTitle          = "Error"
	strErrorRestartFailed  = "Failed to restart service"
	strErrorStartFailed    = "Failed to start service"
	strErrorStopFailed     = "Failed to stop service"
	strErrorEnableFailed   = "Failed to enable autostart"
	strErrorDisableFailed  = "Failed to disable autostart"
	strErrorValidation     = "Validation error"
	strErrorSaveFailed     = "Failed to save settings"
	strErrorTraySaveFailed = "Failed to save tray icon setting"
)

// Warning messages
const (
	strWarnRestartFailed = "Settings saved, but failed to restart service"
)

// Keys section
const (
	strLabelLayoutSwitch           = "Layout switch key:"
	strLabelConvertKey             = "Text conversion:"
	strLayoutSwitchHint            = "Specify the key used to switch keyboard layout in the system"
	strLabelConversionModifiers    = "Modifier preset:"
	strConversionShortcutsTitle    = "Shortcuts"
	strShortcutsChanged            = "Shortcuts changed. Open to see the new combinations."
	strShortcutsTyped              = "TYPED TEXT"
	strShortcutsSelected           = "SELECTED TEXT"
	strShortcutsWord               = "Last word"
	strShortcutsLine               = "Whole line"
	strShortcutsLayout             = "Convert layout"
	strShortcutsCase               = "Swap case"
	strShortcutsUndoTitle          = "Undo correction"
	strShortcutsUndo               = "Repeat the same trigger immediately."
	strShortcutsDoubleTap          = "×2 means double-tap. Hold the other keys."
	strShortcutsOtherShift         = "other Shift"
	strShortcutsInfoIcon           = "ⓘ"
	strConversionModifiersStandard = "Standard"
	strConversionModifiersPunto    = "Like Punto Switcher"
	strAutoDetect                  = "Detect automatically"
)

// Layout switch options
// Order: Auto, Alt+Shift, Ctrl+Shift, Super+Space, Caps Lock, Custom
// Note: "Super (Win)" removed - single key is ambiguous for layout switching
var layoutSwitchOptions = []struct {
	Label string
	Value string
}{
	{strAutoDetect, "auto"},
	{"LAlt+LShift (56+42)", "56+42"},
	{"LCtrl+LShift (29+42)", "29+42"},
	{"LSuper+Space (125+57)", "125+57"},
	{"Caps Lock (58)", "58"},
	{"Other...", "custom"},
}

// Convert key options
var convertKeyOptions = []struct {
	Label string
	Value string
}{
	{"Double Shift", "0"},
	{"Pause/Break", "119"},
	{"Scroll Lock", "70"},
	{"Other...", "custom"},
}

// Layouts section
const (
	strCheckAutoDetect = "Detect automatically"
	strLabelLayout1    = "Layout 1:"
	strLabelLayout2    = "Layout 2:"
)

// Layout options
var layoutOptions = []string{
	"us", "ru", "ua", "de", "fr", "es", "it", "pt", "pl", "cz", "sk", "hu",
}

// Tray section
const strLabelTrayIcon = "Tray icon:"

var trayIconModeOptions = []struct {
	Label string
	Value TrayIconMode
}{
	{"Application icon", TrayIconModeApp},
	{"Application icon with flag", TrayIconModeAppWithFlag},
	{"Flag", TrayIconModeFlag},
}

// Delays section
const (
	strLabelDelayBetween = "Between keystrokes:"
	strLabelDelaySwitch  = "After switching:"
	strLabelMs           = "ms"
)

// Devices section
const (
	strDevicesLoading    = "Loading device list..."
	strDevicesNoAccess   = "No access to input device metadata."
	strDevicesEmpty      = "No keyboards found"
	strDevicesAllBlocked = "All keyboards are blocked!"
	strDeviceBlocked     = "(blocked)"
)

// Buttons
const (
	strButtonCancel = "Cancel"
	strButtonApply  = "Apply"
	strButtonOK     = "OK"
)

// Key picker dialog
const (
	strKeyPickerTitleLayoutSwitch   = "Choose layout switch key"
	strKeyPickerTitleConvertKey     = "Choose conversion key"
	strKeyPickerInstruction         = "Press the desired key or combination..."
	strKeyPickerScancode            = "Scancode:"
	strKeyPickerHint                = "For combinations, hold modifiers (Ctrl, Shift, Alt)"
	strKeyPickerHintConvert         = "Press one key. Key combinations are not supported for text conversion."
	strConversionKeyRecovery        = "The saved conversion key %s is no longer supported. Choose a supported key and click Apply to update the configuration."
	strKeyPickerModifierRejected    = "%s is a modifier key and cannot be used for conversion. Release it and press another key, such as Pause."
	strKeyPickerCombinationRejected = "Conversion requires a single key without Ctrl, Shift, Alt or Super. Release the combination and press another key, such as Pause."

	strKeyPickerCurrentLayoutSwitch = "Current layout switch key:"
	strKeyPickerCurrentConvertKey   = "Current conversion key:"
)

// Detection status (settings window)
const (
	strDetecting          = "Detecting..."
	strDetectedOK         = "✓ Detected: %s (%s)"
	strDetectedWarning    = "⚠ %s (%s)\n%s"
	strDetectNeedsConfig  = "⚠ Detection failed. Select a key manually."
	strDetectError        = "✖ Error: %s"
	strDetectErrorUnknown = "unknown error"
)

// Auto-detect validation dialog (shown when saving with failed detection)
const (
	strAutoDetectFailTitle   = "Auto-detect could not determine the layout switch key."
	strAutoDetectFailReason  = "Reason: %s"
	strAutoDetectFailDefault = "No matching rule found"
	strAutoDetectFailSave    = "Save as is"
	strAutoDetectFailManual  = "Select manually"
	strAutoDetectFailSources = "Checked sources:"
)
