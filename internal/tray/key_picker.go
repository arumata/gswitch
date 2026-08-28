package tray

import (
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"

	"github.com/gotk3/gotk3/gdk"
	"github.com/gotk3/gotk3/gtk"
)

const xkbKeycodeOffset uint16 = 8

// gdkHardwareKeycodeToEvdev converts the XKB keycode reported by GDK on
// X11 and Wayland to the Linux evdev scancode used by gswitch.
func gdkHardwareKeycodeToEvdev(hardwareKeycode uint16) (uint16, bool) {
	if hardwareKeycode <= xkbKeycodeOffset {
		return 0, false
	}
	return hardwareKeycode - xkbKeycodeOffset, true
}

// isModifier checks if scancode is a modifier key.
// Modifiers: Shift (42, 54), Ctrl (29, 97), Alt (56, 100), Super (125, 126)
func isModifier(code uint16) bool {
	modifiers := map[uint16]bool{
		42:  true, // Shift_L
		54:  true, // Shift_R
		29:  true, // Ctrl_L
		97:  true, // Ctrl_R
		56:  true, // Alt_L
		100: true, // Alt_R
		125: true, // Super_L
		126: true, // Super_R
	}
	return modifiers[code]
}

// sortScancodes sorts keycodes: modifiers first, main key last.
// Example: [57, 125] (Space, Super) -> [125, 57] (Super, Space)
func sortScancodes(codes []uint16) []uint16 {
	if len(codes) <= 1 {
		return codes
	}

	modifiers := make([]uint16, 0, len(codes))
	others := make([]uint16, 0, len(codes))

	for _, code := range codes {
		if isModifier(code) {
			modifiers = append(modifiers, code)
		} else {
			others = append(others, code)
		}
	}

	// Sort modifiers and others for stable output
	slices.Sort(modifiers)
	slices.Sort(others)

	return append(modifiers, others...)
}

// reorderKeyNames reorders key names to match sorted scancodes.
func reorderKeyNames(sortedCodes []uint16, codeToName map[uint16]string) []string {
	names := make([]string, len(sortedCodes))
	for i, code := range sortedCodes {
		names[i] = codeToName[code]
	}
	return names
}

// KeyPickerResult represents the result of the key picker dialog.
type KeyPickerResult struct {
	KeyCodes []uint16 // List of hardware keycodes (scancodes)
	KeyNames []string // List of key names for display
	Value    string   // Config value (e.g., "29+42" for Ctrl+Shift)
}

// KeyPickerContext indicates what key is being selected.
type KeyPickerContext int

const (
	// KeyPickerForLayoutSwitch - selecting the layout switch key.
	KeyPickerForLayoutSwitch KeyPickerContext = iota
	// KeyPickerForConvertKey - selecting the convert key.
	KeyPickerForConvertKey
)

// keyPickerTitle returns the dialog title based on context.
func keyPickerTitle(context KeyPickerContext) string {
	switch context {
	case KeyPickerForLayoutSwitch:
		return strKeyPickerTitleLayoutSwitch
	case KeyPickerForConvertKey:
		return strKeyPickerTitleConvertKey
	default:
		return strKeyPickerTitleConvertKey
	}
}

// keyPickerWarningLabel returns the warning label text based on context.
func keyPickerWarningLabel(context KeyPickerContext) string {
	switch context {
	case KeyPickerForLayoutSwitch:
		return strKeyPickerCurrentConvertKey
	case KeyPickerForConvertKey:
		return strKeyPickerCurrentLayoutSwitch
	default:
		return strKeyPickerCurrentLayoutSwitch
	}
}

func keyPickerHint(context KeyPickerContext) string {
	if context == KeyPickerForConvertKey {
		return strKeyPickerHintConvert
	}
	return strKeyPickerHint
}

func keySelectionValid(context KeyPickerContext, keyCount int) bool {
	if keyCount == 0 {
		return false
	}
	return context != KeyPickerForConvertKey || keyCount == 1
}

// ShowKeyPickerDialog shows a dialog for capturing a key or key combination.
// context specifies what key is being selected (layout switch or convert key).
// otherKeyValue is the other key's current value to display as warning (to avoid conflicts).
// Returns the result and true if a key was selected, or empty result and false if canceled.
func ShowKeyPickerDialog(parent *gtk.Window, context KeyPickerContext, otherKeyValue string) (KeyPickerResult, bool) {
	dialog, err := gtk.DialogNew()
	if err != nil {
		return KeyPickerResult{}, false
	}
	defer dialog.Destroy()

	dialog.SetTitle(keyPickerTitle(context))
	dialog.SetTransientFor(parent)
	dialog.SetModal(true)
	dialog.SetDefaultSize(350, 200)
	dialog.SetPosition(gtk.WIN_POS_CENTER_ON_PARENT)
	dialog.SetBorderWidth(10)

	contentArea, err := dialog.GetContentArea()
	if err != nil {
		return KeyPickerResult{}, false
	}

	box, err := gtk.BoxNew(gtk.ORIENTATION_VERTICAL, 10)
	if err != nil {
		return KeyPickerResult{}, false
	}
	box.SetMarginTop(10)
	box.SetMarginBottom(10)
	box.SetMarginStart(10)
	box.SetMarginEnd(10)

	instructionLabel, err := gtk.LabelNew(strKeyPickerInstruction)
	if err != nil {
		return KeyPickerResult{}, false
	}
	box.PackStart(instructionLabel, false, false, 0)

	keyLabel, err := gtk.LabelNew("")
	if err != nil {
		return KeyPickerResult{}, false
	}
	keyLabel.SetMarkup("<span size='x-large' weight='bold'>-</span>")
	box.PackStart(keyLabel, true, true, 10)

	scancodeLabel, err := gtk.LabelNew("")
	if err != nil {
		return KeyPickerResult{}, false
	}
	scancodeLabel.SetMarkup("<span color='gray'>" + strKeyPickerScancode + " -</span>")
	box.PackStart(scancodeLabel, false, false, 0)

	hintLabel, err := gtk.LabelNew(keyPickerHint(context))
	if err != nil {
		return KeyPickerResult{}, false
	}
	hintLabel.SetLineWrap(true)
	if ctx, _ := hintLabel.GetStyleContext(); ctx != nil {
		ctx.AddClass("dim-label")
	}
	box.PackStart(hintLabel, false, false, 10)

	// Show other key warning if set (to help avoid conflicts)
	if otherKeyValue != "" && otherKeyValue != "custom" {
		if currentKeyLabel, err := gtk.LabelNew(""); err == nil {
			currentKeyLabel.SetMarkup(fmt.Sprintf("<span color='#cc6600'>%s <b>%s</b></span>",
				keyPickerWarningLabel(context), formatKeyValue(otherKeyValue)))
			currentKeyLabel.SetLineWrap(true)
			box.PackStart(currentKeyLabel, false, false, 5)
		}
	}

	contentArea.Add(box)

	// Add buttons
	if _, err := dialog.AddButton(strButtonCancel, gtk.RESPONSE_CANCEL); err != nil {
		return KeyPickerResult{}, false
	}
	okButton, err := dialog.AddButton(strButtonOK, gtk.RESPONSE_OK)
	if err != nil {
		return KeyPickerResult{}, false
	}
	if okButton != nil {
		okButton.SetSensitive(false) // Disable until a key is pressed
	}

	// State for capturing keys
	var result KeyPickerResult
	currentlyPressed := make(map[uint16]string) // Keys currently held down
	savedCombination := make(map[uint16]string) // Saved combination to return

	// Connect key-press-event
	dialog.Connect("key-press-event", func(_ *gtk.Dialog, event *gdk.Event) bool {
		keyEvent := gdk.EventKeyNewFromEvent(event)
		keycode, ok := gdkHardwareKeycodeToEvdev(keyEvent.HardwareKeyCode())
		if !ok {
			return true
		}
		keyval := keyEvent.KeyVal()

		// Convert keyval to base key name (without modifier influence)
		// Use ToUpper to get consistent naming (e.g., always "R" not "r")
		keyName := gdk.KeyValName(gdk.KeyvalToUpper(keyval))
		if keyName == "" {
			keyName = gdk.KeyValName(keyval)
		}
		if keyName == "" {
			keyName = "Key_" + strconv.FormatUint(uint64(keycode), 10)
		}

		// Add to currently pressed keys
		currentlyPressed[keycode] = keyName

		// Save current combination (all keys currently pressed)
		savedCombination = make(map[uint16]string)
		maps.Copy(savedCombination, currentlyPressed)

		// Update display with saved combination
		updateKeyDisplay(keyLabel, scancodeLabel, savedCombination)

		// Conversion accepts one key only; layout switching also supports combinations.
		if okButton != nil {
			okButton.SetSensitive(keySelectionValid(context, len(savedCombination)))
		}

		return true // Consume the event
	})

	// Connect key-release-event - remove from currently pressed but keep saved combination
	dialog.Connect("key-release-event", func(_ *gtk.Dialog, event *gdk.Event) bool {
		keyEvent := gdk.EventKeyNewFromEvent(event)
		keycode, ok := gdkHardwareKeycodeToEvdev(keyEvent.HardwareKeyCode())
		if !ok {
			return true
		}

		// Remove from currently pressed (but savedCombination stays intact)
		delete(currentlyPressed, keycode)

		return true
	})

	dialog.ShowAll()
	response := dialog.Run()

	if response == gtk.RESPONSE_OK && len(savedCombination) > 0 {
		// Collect keycodes from saved combination
		codes := make([]uint16, 0, len(savedCombination))
		for code := range savedCombination {
			codes = append(codes, code)
		}

		// Sort: modifiers first, main key last (fixes bug with Super+Space showing as "57+125")
		result.KeyCodes = sortScancodes(codes)
		result.KeyNames = reorderKeyNames(result.KeyCodes, savedCombination)

		// Build config value string
		codeStrings := make([]string, len(result.KeyCodes))
		for i, code := range result.KeyCodes {
			codeStrings[i] = strconv.FormatUint(uint64(code), 10)
		}
		result.Value = strings.Join(codeStrings, "+")

		return result, true
	}

	return KeyPickerResult{}, false
}

// updateKeyDisplay updates the key and scancode labels.
func updateKeyDisplay(keyLabel, scancodeLabel *gtk.Label, pressedKeys map[uint16]string) {
	if len(pressedKeys) == 0 {
		keyLabel.SetMarkup("<span size='x-large' weight='bold'>-</span>")
		scancodeLabel.SetMarkup("<span color='gray'>" + strKeyPickerScancode + " -</span>")
		return
	}

	// Collect and sort keys: modifiers first, main key last
	codes := make([]uint16, 0, len(pressedKeys))
	for code := range pressedKeys {
		codes = append(codes, code)
	}
	codes = sortScancodes(codes)

	// Build display strings
	names := make([]string, len(codes))
	codeStrings := make([]string, len(codes))
	for i, code := range codes {
		names[i] = pressedKeys[code]
		codeStrings[i] = strconv.FormatUint(uint64(code), 10)
	}

	keyLabel.SetMarkup(fmt.Sprintf("<span size='x-large' weight='bold'>%s</span>",
		strings.Join(names, " + ")))
	scancodeLabel.SetMarkup(fmt.Sprintf("<span color='gray'>%s %s</span>",
		strKeyPickerScancode, strings.Join(codeStrings, "+")))
}

// formatKeyValue converts a key value to human-readable format.
func formatKeyValue(value string) string {
	// Known presets
	knownValues := map[string]string{
		"0":      "Double Shift",
		"58":     "Caps Lock",
		"29+42":  "Ctrl+Shift",
		"56+42":  "Alt+Shift",
		"125+57": "Super+Space",
		"119":    "Pause/Break",
		"70":     "Scroll Lock",
	}

	if name, ok := knownValues[value]; ok {
		return name
	}

	// Return as-is (scancode)
	return value
}

// GetKeyNameFromCode returns a human-readable name for a keycode.
func GetKeyNameFromCode(code uint16) string {
	knownKeys := map[uint16]string{
		29:  "Ctrl_L",
		42:  "Shift_L",
		54:  "Shift_R",
		56:  "Alt_L",
		57:  "Space",
		58:  "Caps_Lock",
		70:  "Scroll_Lock",
		97:  "Ctrl_R",
		100: "Alt_R",
		119: "Pause",
		125: "Super_L",
		126: "Super_R",
	}

	if name, ok := knownKeys[code]; ok {
		return name
	}
	return "Key_" + strconv.FormatUint(uint64(code), 10)
}
