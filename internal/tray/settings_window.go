package tray

import (
	"errors"
	"fmt"
	"os/exec"
	"runtime"
	"strconv"
	"strings"

	"github.com/gotk3/gotk3/glib"
	"github.com/gotk3/gotk3/gtk"

	gsconfig "github.com/arumata/gswitch/internal/gswitch/config"
)

// SettingsWindow represents the settings dialog.
type SettingsWindow struct {
	window *gtk.Window
	app    *App

	// Service manager
	serviceManager *ServiceManager

	// Service section widgets
	statusLabel       *gtk.Label
	restartButton     *gtk.Button
	autostartCheck    *gtk.CheckButton
	autostartUpdating bool // flag to prevent recursive toggle handling

	// Keys section widgets
	layoutSwitchCombo     *gtk.ComboBoxText
	layoutSwitchStatus    *gtk.Label // Status label for auto-detection result
	convertKeyCombo       *gtk.ComboBoxText
	customLayoutSwitch    string         // Custom value if "Other..." is selected
	customConvertKey      string         // Custom value if "Other..." is selected
	prevLayoutSwitchIdx   int            // Previous index before "Other..." was selected
	prevConvertKeyIdx     int            // Previous index before "Other..." was selected
	ignoreComboChanged    bool           // Flag to prevent recursive change handling
	lastDetectionResult   *DetectionInfo // Cached detection result for validation
	detectionGeneration   uint64         // Generation counter for stale update protection
	detectionInFlight     bool           // Guard against concurrent detection runs
	detectionPendingRerun bool           // Schedule rerun after in-flight detection completes

	// Layouts section widgets
	autoDetectCheck   *gtk.CheckButton
	layout1Combo      *gtk.ComboBoxText
	layout2Combo      *gtk.ComboBoxText
	trayIconModeCombo *gtk.ComboBoxText

	// Delays section widgets. The adjustments are kept alongside their spin
	// buttons on purpose: gotk3 hands gtk_spin_button_new a raw pointer and
	// the Go wrapper is otherwise dead by then, so the GC could finalize
	// (unref) the adjustment inside the cgo call. That surfaced as
	// "g_object_ref_sink: assertion 'G_IS_OBJECT (object)' failed" and, under
	// G_DEBUG=fatal-criticals, as a SIGTRAP in the XEmbed release gate.
	delayBetweenAdj  *gtk.Adjustment
	delayBetweenSpin *gtk.SpinButton
	delaySwitchAdj   *gtk.Adjustment
	delaySwitchSpin  *gtk.SpinButton

	// Devices section widgets
	devicesBox                *gtk.Box
	deviceCheckboxes          map[string]*gtk.CheckButton // UID -> checkbox
	deviceManager             *DeviceManager
	loadedConfig              *TrayConfig
	conversionRecoveryWarning *gtk.Label
}

// NewSettingsWindow creates a new settings window.
func NewSettingsWindow(app *App) (*SettingsWindow, error) {
	w := &SettingsWindow{
		app:              app,
		serviceManager:   NewServiceManager("gswitch.service"),
		deviceManager:    NewDeviceManager(),
		deviceCheckboxes: make(map[string]*gtk.CheckButton),
	}

	if err := w.build(); err != nil {
		return nil, fmt.Errorf("failed to build settings window: %w", err)
	}

	return w, nil
}

// build constructs all GTK widgets for the settings window.
func (w *SettingsWindow) build() error {
	var err error

	// Create main window
	w.window, err = gtk.WindowNew(gtk.WINDOW_TOPLEVEL)
	if err != nil {
		return fmt.Errorf("failed to create window: %w", err)
	}

	w.window.SetTitle(strWindowTitle)
	w.window.SetDefaultSize(500, 700)
	w.window.SetPosition(gtk.WIN_POS_CENTER)
	w.window.SetResizable(true)

	// Handle window close button
	w.window.Connect("delete-event", func() bool {
		w.Hide()
		return true // Prevent destruction, just hide
	})

	// Handle window destruction (when app exits)
	w.window.Connect("destroy", func() {
		w.detectionGeneration++ // Cancel any pending detection updates
		w.window = nil
	})

	// Create main vertical box with scrolling
	scrolledWindow, err := gtk.ScrolledWindowNew(nil, nil)
	if err != nil {
		return fmt.Errorf("failed to create scrolled window: %w", err)
	}
	scrolledWindow.SetPolicy(gtk.POLICY_NEVER, gtk.POLICY_AUTOMATIC)

	mainBox, err := gtk.BoxNew(gtk.ORIENTATION_VERTICAL, 10)
	if err != nil {
		return fmt.Errorf("failed to create main box: %w", err)
	}
	mainBox.SetMarginTop(10)
	mainBox.SetMarginBottom(10)
	mainBox.SetMarginStart(10)
	mainBox.SetMarginEnd(10)

	// Create sections
	serviceFrame, err := w.createServiceSection()
	if err != nil {
		return fmt.Errorf("failed to create service section: %w", err)
	}
	mainBox.PackStart(serviceFrame, false, false, 0)

	keysFrame, err := w.createKeysSection()
	if err != nil {
		return fmt.Errorf("failed to create keys section: %w", err)
	}
	mainBox.PackStart(keysFrame, false, false, 0)

	layoutsFrame, err := w.createLayoutsSection()
	if err != nil {
		return fmt.Errorf("failed to create layouts section: %w", err)
	}
	mainBox.PackStart(layoutsFrame, false, false, 0)

	trayFrame, err := w.createTraySection()
	if err != nil {
		return fmt.Errorf("failed to create tray section: %w", err)
	}
	mainBox.PackStart(trayFrame, false, false, 0)

	delaysFrame, err := w.createDelaysSection()
	if err != nil {
		return fmt.Errorf("failed to create delays section: %w", err)
	}
	mainBox.PackStart(delaysFrame, false, false, 0)

	devicesFrame, err := w.createDevicesSection()
	if err != nil {
		return fmt.Errorf("failed to create devices section: %w", err)
	}
	mainBox.PackStart(devicesFrame, false, false, 0)

	scrolledWindow.Add(mainBox)

	// Create outer box for scrolled content + buttons
	outerBox, err := gtk.BoxNew(gtk.ORIENTATION_VERTICAL, 0)
	if err != nil {
		return fmt.Errorf("failed to create outer box: %w", err)
	}

	outerBox.PackStart(scrolledWindow, true, true, 0)

	// Create button box
	buttonBox, err := w.createButtonBox()
	if err != nil {
		return fmt.Errorf("failed to create button box: %w", err)
	}
	outerBox.PackEnd(buttonBox, false, false, 10)

	w.window.Add(outerBox)

	return nil
}

// createServiceSection creates the "Service" section.
func (w *SettingsWindow) createServiceSection() (*gtk.Frame, error) {
	frame, err := gtk.FrameNew(strSectionService)
	if err != nil {
		return nil, err
	}

	grid, err := gtk.GridNew()
	if err != nil {
		return nil, err
	}
	grid.SetColumnSpacing(10)
	grid.SetRowSpacing(8)
	grid.SetMarginTop(10)
	grid.SetMarginBottom(10)
	grid.SetMarginStart(10)
	grid.SetMarginEnd(10)

	// Row 0: Status label + value + restart button
	statusLabel, err := gtk.LabelNew(strLabelStatus)
	if err != nil {
		return nil, err
	}
	statusLabel.SetHAlign(gtk.ALIGN_START)
	grid.Attach(statusLabel, 0, 0, 1, 1)

	w.statusLabel, err = gtk.LabelNew(strStatusUnknown)
	if err != nil {
		return nil, err
	}
	w.statusLabel.SetHAlign(gtk.ALIGN_START)
	// Add colored bullet (placeholder - will be updated in task 05)
	w.statusLabel.SetMarkup("<span foreground='gray'>\u25CF</span> " + strStatusUnknown)
	grid.Attach(w.statusLabel, 1, 0, 1, 1)

	w.restartButton, err = gtk.ButtonNewWithLabel(strButtonRestart)
	if err != nil {
		return nil, err
	}
	w.restartButton.Connect("clicked", w.onRestartClicked)
	grid.Attach(w.restartButton, 2, 0, 1, 1)

	// Row 1: Autostart checkbox
	w.autostartCheck, err = gtk.CheckButtonNewWithLabel(strCheckAutostart)
	if err != nil {
		return nil, err
	}
	w.autostartCheck.Connect("toggled", w.onAutostartToggled)
	grid.Attach(w.autostartCheck, 0, 1, 3, 1)

	frame.Add(grid)
	return frame, nil
}

// createKeysSection creates the "Keys" section.
func (w *SettingsWindow) createKeysSection() (*gtk.Frame, error) {
	frame, err := gtk.FrameNew(strSectionKeys)
	if err != nil {
		return nil, err
	}

	grid, err := gtk.GridNew()
	if err != nil {
		return nil, err
	}
	grid.SetColumnSpacing(10)
	grid.SetRowSpacing(8)
	grid.SetMarginTop(10)
	grid.SetMarginBottom(10)
	grid.SetMarginStart(10)
	grid.SetMarginEnd(10)

	// Row 0: Layout switch combo
	layoutSwitchLabel, err := gtk.LabelNew(strLabelLayoutSwitch)
	if err != nil {
		return nil, err
	}
	layoutSwitchLabel.SetHAlign(gtk.ALIGN_START)
	grid.Attach(layoutSwitchLabel, 0, 0, 1, 1)

	w.layoutSwitchCombo, err = gtk.ComboBoxTextNew()
	if err != nil {
		return nil, err
	}
	for _, opt := range layoutSwitchOptions {
		w.layoutSwitchCombo.AppendText(opt.Label)
	}
	w.layoutSwitchCombo.SetActive(0)
	w.layoutSwitchCombo.SetHExpand(true)
	w.layoutSwitchCombo.Connect("changed", w.onLayoutSwitchChanged)
	grid.Attach(w.layoutSwitchCombo, 1, 0, 1, 1)

	// Row 1: Hint for layout switch
	layoutSwitchHint, err := gtk.LabelNew("")
	if err != nil {
		return nil, err
	}
	layoutSwitchHint.SetUseMarkup(true)
	layoutSwitchHintText := glib.MarkupEscapeText(strLayoutSwitchHint)
	layoutSwitchHint.SetMarkup("<small><span foreground='gray'>" + layoutSwitchHintText + "</span></small>")
	layoutSwitchHint.SetLineWrap(true)
	layoutSwitchHint.SetHAlign(gtk.ALIGN_START)
	layoutSwitchHint.SetMarginStart(5)
	grid.Attach(layoutSwitchHint, 0, 1, 2, 1)

	// Row 2: Detection status label (shown only for "auto")
	w.layoutSwitchStatus, err = gtk.LabelNew("")
	if err != nil {
		return nil, err
	}
	w.layoutSwitchStatus.SetUseMarkup(true)
	w.layoutSwitchStatus.SetLineWrap(true)
	w.layoutSwitchStatus.SetHAlign(gtk.ALIGN_START)
	w.layoutSwitchStatus.SetMarginStart(5)
	grid.Attach(w.layoutSwitchStatus, 0, 2, 2, 1)

	// Row 3: Convert key combo
	convertKeyLabel, err := gtk.LabelNew(strLabelConvertKey)
	if err != nil {
		return nil, err
	}
	convertKeyLabel.SetHAlign(gtk.ALIGN_START)
	grid.Attach(convertKeyLabel, 0, 3, 1, 1)

	w.convertKeyCombo, err = gtk.ComboBoxTextNew()
	if err != nil {
		return nil, err
	}
	for _, opt := range convertKeyOptions {
		w.convertKeyCombo.AppendText(opt.Label)
	}
	w.convertKeyCombo.SetActive(0)
	w.convertKeyCombo.SetHExpand(true)
	w.convertKeyCombo.Connect("changed", w.onConvertKeyChanged)
	grid.Attach(w.convertKeyCombo, 1, 3, 1, 1)

	// Row 4: Conversion shortcut hint
	convertKeyHint, err := gtk.LabelNew("")
	if err != nil {
		return nil, err
	}
	convertKeyHint.SetUseMarkup(true)
	convertKeyHintText := glib.MarkupEscapeText(strConvertKeyHint)
	convertKeyHint.SetMarkup("<small><span foreground='gray'>" + convertKeyHintText + "</span></small>")
	convertKeyHint.SetLineWrap(true)
	convertKeyHint.SetHAlign(gtk.ALIGN_START)
	convertKeyHint.SetMarginStart(5)
	grid.Attach(convertKeyHint, 0, 4, 2, 1)

	w.conversionRecoveryWarning, err = gtk.LabelNew("")
	if err != nil {
		return nil, err
	}
	w.conversionRecoveryWarning.SetLineWrap(true)
	w.conversionRecoveryWarning.SetMaxWidthChars(55)
	w.conversionRecoveryWarning.SetHAlign(gtk.ALIGN_START)
	w.conversionRecoveryWarning.SetNoShowAll(true)
	if style, _ := w.conversionRecoveryWarning.GetStyleContext(); style != nil {
		style.AddClass("warning")
	}
	grid.Attach(w.conversionRecoveryWarning, 0, 5, 2, 1)

	frame.Add(grid)
	return frame, nil
}

// createLayoutsSection creates the "Layouts" section.
func (w *SettingsWindow) createLayoutsSection() (*gtk.Frame, error) {
	frame, err := gtk.FrameNew(strSectionLayouts)
	if err != nil {
		return nil, err
	}

	grid, err := gtk.GridNew()
	if err != nil {
		return nil, err
	}
	grid.SetColumnSpacing(10)
	grid.SetRowSpacing(8)
	grid.SetMarginTop(10)
	grid.SetMarginBottom(10)
	grid.SetMarginStart(10)
	grid.SetMarginEnd(10)

	// Row 0: Auto-detect checkbox
	w.autoDetectCheck, err = gtk.CheckButtonNewWithLabel(strCheckAutoDetect)
	if err != nil {
		return nil, err
	}
	w.autoDetectCheck.SetActive(true)
	grid.Attach(w.autoDetectCheck, 0, 0, 2, 1)

	// Row 1: Layout 1 combo
	layout1Label, err := gtk.LabelNew(strLabelLayout1)
	if err != nil {
		return nil, err
	}
	layout1Label.SetHAlign(gtk.ALIGN_START)
	grid.Attach(layout1Label, 0, 1, 1, 1)

	w.layout1Combo, err = gtk.ComboBoxTextNew()
	if err != nil {
		return nil, err
	}
	for _, layout := range layoutOptions {
		w.layout1Combo.AppendText(layout)
	}
	w.layout1Combo.SetActive(0) // "us"
	w.layout1Combo.SetHExpand(true)
	grid.Attach(w.layout1Combo, 1, 1, 1, 1)

	// Row 2: Layout 2 combo
	layout2Label, err := gtk.LabelNew(strLabelLayout2)
	if err != nil {
		return nil, err
	}
	layout2Label.SetHAlign(gtk.ALIGN_START)
	grid.Attach(layout2Label, 0, 2, 1, 1)

	w.layout2Combo, err = gtk.ComboBoxTextNew()
	if err != nil {
		return nil, err
	}
	for _, layout := range layoutOptions {
		w.layout2Combo.AppendText(layout)
	}
	w.layout2Combo.SetActive(1) // "ru"
	w.layout2Combo.SetHExpand(true)
	grid.Attach(w.layout2Combo, 1, 2, 1, 1)

	frame.Add(grid)
	return frame, nil
}

func (w *SettingsWindow) createTraySection() (*gtk.Frame, error) {
	frame, err := gtk.FrameNew(strSectionTray)
	if err != nil {
		return nil, err
	}
	grid, err := gtk.GridNew()
	if err != nil {
		return nil, err
	}
	grid.SetColumnSpacing(10)
	grid.SetMarginTop(10)
	grid.SetMarginBottom(10)
	grid.SetMarginStart(10)
	grid.SetMarginEnd(10)

	label, err := gtk.LabelNew(strLabelTrayIcon)
	if err != nil {
		return nil, err
	}
	label.SetHAlign(gtk.ALIGN_START)
	grid.Attach(label, 0, 0, 1, 1)

	w.trayIconModeCombo, err = gtk.ComboBoxTextNew()
	if err != nil {
		return nil, err
	}
	for _, option := range trayIconModeOptions {
		w.trayIconModeCombo.AppendText(option.Label)
	}
	w.trayIconModeCombo.SetActive(0)
	w.trayIconModeCombo.SetHExpand(true)
	grid.Attach(w.trayIconModeCombo, 1, 0, 1, 1)

	frame.Add(grid)
	return frame, nil
}

// createDelaysSection creates the "Delays" section.
// newSpinButton wraps gtk.SpinButtonNew and keeps the adjustment reachable
// until the spin button holds its own reference; see the field comment.
func newSpinButton(adjustment *gtk.Adjustment) (*gtk.SpinButton, error) {
	spin, err := gtk.SpinButtonNew(adjustment, 1, 0)
	runtime.KeepAlive(adjustment)
	return spin, err
}

func (w *SettingsWindow) createDelaysSection() (*gtk.Frame, error) {
	frame, err := gtk.FrameNew(strSectionDelays)
	if err != nil {
		return nil, err
	}

	grid, err := gtk.GridNew()
	if err != nil {
		return nil, err
	}
	grid.SetColumnSpacing(10)
	grid.SetRowSpacing(8)
	grid.SetMarginTop(10)
	grid.SetMarginBottom(10)
	grid.SetMarginStart(10)
	grid.SetMarginEnd(10)

	// Row 0: Delay between keys
	delayBetweenLabel, err := gtk.LabelNew(strLabelDelayBetween)
	if err != nil {
		return nil, err
	}
	delayBetweenLabel.SetHAlign(gtk.ALIGN_START)
	grid.Attach(delayBetweenLabel, 0, 0, 1, 1)

	// SpinButton: value, min, max, step, page, pageSize
	w.delayBetweenAdj, err = gtk.AdjustmentNew(10, 0, 100, 1, 10, 0)
	if err != nil {
		return nil, err
	}
	w.delayBetweenSpin, err = newSpinButton(w.delayBetweenAdj)
	if err != nil {
		return nil, err
	}
	w.delayBetweenSpin.SetNumeric(true)
	grid.Attach(w.delayBetweenSpin, 1, 0, 1, 1)

	msLabel1, err := gtk.LabelNew(strLabelMs)
	if err != nil {
		return nil, err
	}
	msLabel1.SetHAlign(gtk.ALIGN_START)
	grid.Attach(msLabel1, 2, 0, 1, 1)

	// Row 1: Delay after switch
	delaySwitchLabel, err := gtk.LabelNew(strLabelDelaySwitch)
	if err != nil {
		return nil, err
	}
	delaySwitchLabel.SetHAlign(gtk.ALIGN_START)
	grid.Attach(delaySwitchLabel, 0, 1, 1, 1)

	w.delaySwitchAdj, err = gtk.AdjustmentNew(100, 0, 500, 1, 10, 0)
	if err != nil {
		return nil, err
	}
	w.delaySwitchSpin, err = newSpinButton(w.delaySwitchAdj)
	if err != nil {
		return nil, err
	}
	w.delaySwitchSpin.SetNumeric(true)
	grid.Attach(w.delaySwitchSpin, 1, 1, 1, 1)

	msLabel2, err := gtk.LabelNew(strLabelMs)
	if err != nil {
		return nil, err
	}
	msLabel2.SetHAlign(gtk.ALIGN_START)
	grid.Attach(msLabel2, 2, 1, 1, 1)

	frame.Add(grid)
	return frame, nil
}

// createDevicesSection creates the "Devices" section.
func (w *SettingsWindow) createDevicesSection() (*gtk.Frame, error) {
	frame, err := gtk.FrameNew(strSectionDevices)
	if err != nil {
		return nil, err
	}

	w.devicesBox, err = gtk.BoxNew(gtk.ORIENTATION_VERTICAL, 5)
	if err != nil {
		return nil, err
	}
	w.devicesBox.SetMarginTop(10)
	w.devicesBox.SetMarginBottom(10)
	w.devicesBox.SetMarginStart(10)
	w.devicesBox.SetMarginEnd(10)

	// Initial loading label (will be replaced when window is shown)
	loadingLabel, err := gtk.LabelNew(strDevicesLoading)
	if err != nil {
		return nil, err
	}
	loadingLabel.SetHAlign(gtk.ALIGN_START)
	w.devicesBox.PackStart(loadingLabel, false, false, 0)

	frame.Add(w.devicesBox)
	return frame, nil
}

// createButtonBox creates the bottom bar: version label and Cancel/Apply buttons.
func (w *SettingsWindow) createButtonBox() (*gtk.Box, error) {
	buttonBox, err := gtk.BoxNew(gtk.ORIENTATION_HORIZONTAL, 10)
	if err != nil {
		return nil, err
	}
	buttonBox.SetMarginStart(10)
	buttonBox.SetMarginEnd(10)

	versionLabel, err := gtk.LabelNew("gswitch " + appVersion)
	if err != nil {
		return nil, err
	}
	if ctx, err := versionLabel.GetStyleContext(); err == nil {
		ctx.AddClass("dim-label")
	}
	buttonBox.PackStart(versionLabel, false, false, 0)

	applyButton, err := gtk.ButtonNewWithLabel(strButtonApply)
	if err != nil {
		return nil, err
	}
	applyButton.Connect("clicked", w.onApplyClicked)
	buttonBox.PackEnd(applyButton, false, false, 0)

	cancelButton, err := gtk.ButtonNewWithLabel(strButtonCancel)
	if err != nil {
		return nil, err
	}
	cancelButton.Connect("clicked", w.onCancelClicked)
	buttonBox.PackEnd(cancelButton, false, false, 0)

	return buttonBox, nil
}

// Show displays the settings window.
func (w *SettingsWindow) Show() {
	// Must run GTK operations on main thread
	glib.IdleAdd(func() {
		if w.window != nil {
			w.loadConfig()
			w.updateServiceStatus()
			w.window.ShowAll()
			w.window.Present()
		}
	})
}

// loadConfig reads the configuration file and populates widgets.
func (w *SettingsWindow) loadConfig() {
	cfg := LoadTrayConfig()

	// Use ignoreComboChanged to prevent triggering detection during programmatic SetActive
	w.ignoreComboChanged = true
	defer func() { w.ignoreComboChanged = false }()

	w.loadKeyConfig(cfg)

	// Set layout combos
	w.setLayoutComboByValue(w.layout1Combo, cfg.Layout1)
	w.setLayoutComboByValue(w.layout2Combo, cfg.Layout2)
	w.setTrayIconMode(LoadTrayIconMode())

	// Set delay values
	w.delayBetweenSpin.SetValue(float64(cfg.Delay))
	w.delaySwitchSpin.SetValue(float64(cfg.LayoutSwitchDelay))

	// Load devices with blacklist info
	w.loadDevices(cfg)

	// Trigger detection status update after loading config
	// (ignoreComboChanged is reset by defer, so we call async method after)
	glib.IdleAdd(func() {
		w.updateDetectionStatusAsync()
	})
}

// loadKeyConfig retains the disk snapshot while proposing a safe conversion
// key. Apply must detect the difference even without a user changing widgets.
func (w *SettingsWindow) loadKeyConfig(cfg *TrayConfig) {
	w.loadedConfig = cloneTrayConfig(cfg)
	w.setLayoutSwitchFromValue(cfg.LayoutSwitch)
	value := cfg.ConvertKey
	message := ""
	if code, err := strconv.ParseUint(value, 10, 16); err == nil {
		if effective := gsconfig.EffectiveConvertKey(uint16(code)); uint64(effective) != code {
			value = strconv.FormatUint(uint64(effective), 10)
			message = fmt.Sprintf(strConversionKeyRecovery, formatCustomKeyLabel(cfg.ConvertKey))
		}
	}
	w.setConvertKeyFromValue(value)
	w.conversionRecoveryWarning.SetText(message)
	w.conversionRecoveryWarning.SetVisible(message != "")
}

// recordConfigSave runs on the GTK thread after the save attempt. A failed or
// canceled write must retain both the disk snapshot and the recovery warning.
func (w *SettingsWindow) recordConfigSave(cfg *TrayConfig, saveErr error) {
	if saveErr != nil {
		return
	}
	w.loadedConfig = cloneTrayConfig(cfg)
	w.conversionRecoveryWarning.SetText("")
	w.conversionRecoveryWarning.Hide()
}

func (w *SettingsWindow) setTrayIconMode(mode TrayIconMode) {
	mode = normalizeTrayIconMode(mode)
	for i, option := range trayIconModeOptions {
		if option.Value == mode {
			w.trayIconModeCombo.SetActive(i)
			return
		}
	}
	w.trayIconModeCombo.SetActive(0)
}

func (w *SettingsWindow) trayIconMode() (TrayIconMode, error) {
	index := w.trayIconModeCombo.GetActive()
	if index < 0 || index >= len(trayIconModeOptions) {
		return DefaultTrayIconMode, errors.New("invalid tray icon selection")
	}
	return trayIconModeOptions[index].Value, nil
}

// loadDevices populates the devices section with checkboxes.
func (w *SettingsWindow) loadDevices(cfg *TrayConfig) {
	// Clear existing content
	w.devicesBox.GetChildren().Foreach(func(item any) {
		if widget, ok := item.(*gtk.Widget); ok {
			widget.Destroy()
		}
	})
	w.deviceCheckboxes = make(map[string]*gtk.CheckButton)

	// Get list of keyboards
	keyboards, err := w.deviceManager.GetKeyboards()
	if err != nil {
		// Show appropriate error message
		var errMsg string
		if errors.Is(err, ErrNoAccess) {
			errMsg = strDevicesNoAccess
		} else {
			errMsg = fmt.Sprintf("Error: %v", err)
		}
		label, _ := gtk.LabelNew(errMsg)
		if label != nil {
			label.SetHAlign(gtk.ALIGN_START)
			w.devicesBox.PackStart(label, false, false, 0)
		}
		w.devicesBox.ShowAll()
		return
	}

	if len(keyboards) == 0 {
		// No keyboards found
		label, _ := gtk.LabelNew(strDevicesEmpty)
		if label != nil {
			label.SetHAlign(gtk.ALIGN_START)
			w.devicesBox.PackStart(label, false, false, 0)
		}
		w.devicesBox.ShowAll()
		return
	}

	// Count active devices
	activeCount := 0
	for _, kb := range keyboards {
		if !cfg.IsBlacklisted(kb.UID) {
			activeCount++
		}
	}

	// Show warning if all devices are blocked
	if activeCount == 0 {
		warningLabel, _ := gtk.LabelNew("")
		if warningLabel != nil {
			warningLabel.SetMarkup("<span foreground='red'>" + strDevicesAllBlocked + "</span>")
			warningLabel.SetHAlign(gtk.ALIGN_START)
			w.devicesBox.PackStart(warningLabel, false, false, 0)
		}
	}

	// Create checkboxes for each keyboard
	for _, kb := range keyboards {
		checkbox, err := w.createDeviceCheckbox(kb, cfg.IsBlacklisted(kb.UID))
		if err != nil {
			continue
		}
		w.devicesBox.PackStart(checkbox, false, false, 0)
		w.deviceCheckboxes[kb.UID] = checkbox
	}

	w.devicesBox.ShowAll()
}

// createDeviceCheckbox creates a checkbox for a keyboard device.
func (w *SettingsWindow) createDeviceCheckbox(device KeyboardDevice, isBlacklisted bool) (*gtk.CheckButton, error) {
	// Format label: "Device Name" or "Device Name (blocked)"
	label := device.Name
	if isBlacklisted {
		label = device.Name + " " + strDeviceBlocked
	}

	checkbox, err := gtk.CheckButtonNewWithLabel(label)
	if err != nil {
		return nil, err
	}

	// Checked = active (NOT in blacklist)
	checkbox.SetActive(!isBlacklisted)

	// Store UID in widget name for later retrieval
	checkbox.SetName(device.UID)

	// Connect signal to update label when toggled
	checkbox.Connect("toggled", func() {
		w.onDeviceToggled(checkbox, device)
	})

	return checkbox, nil
}

// onDeviceToggled handles device checkbox toggle.
func (w *SettingsWindow) onDeviceToggled(checkbox *gtk.CheckButton, device KeyboardDevice) {
	isActive := checkbox.GetActive()

	// Update label to reflect state
	if isActive {
		checkbox.SetLabel(device.Name)
	} else {
		checkbox.SetLabel(device.Name + " " + strDeviceBlocked)
	}

	// Check if all devices are now blocked
	allBlocked := true
	for _, cb := range w.deviceCheckboxes {
		if cb.GetActive() {
			allBlocked = false
			break
		}
	}

	// Show/hide warning (would need to track warning label, simplified for now)
	if allBlocked {
		// Could show a dialog warning here
		fmt.Println("Warning: All keyboards are blocked!")
	}
}

// setLayoutSwitchFromValue sets the layout switch combo based on config value.
func (w *SettingsWindow) setLayoutSwitchFromValue(value string) {
	for i, opt := range layoutSwitchOptions {
		if opt.Value == value {
			w.layoutSwitchCombo.SetActive(i)
			w.prevLayoutSwitchIdx = i
			return
		}
	}
	// Custom value not in predefined options - add it to combo
	w.customLayoutSwitch = value
	w.layoutSwitchCombo.RemoveAll()
	for _, opt := range layoutSwitchOptions {
		w.layoutSwitchCombo.AppendText(opt.Label)
	}
	// Add custom entry with the value
	customLabel := formatCustomKeyLabel(value)
	w.layoutSwitchCombo.AppendText(customLabel)
	w.layoutSwitchCombo.SetActive(len(layoutSwitchOptions))
	w.prevLayoutSwitchIdx = 0 // fallback to first option if custom is canceled
}

// setConvertKeyFromValue sets the convert key combo based on config value.
func (w *SettingsWindow) setConvertKeyFromValue(value string) {
	for i, opt := range convertKeyOptions {
		if opt.Value == value {
			w.convertKeyCombo.SetActive(i)
			w.prevConvertKeyIdx = i
			return
		}
	}
	// Custom value not in predefined options - add it to combo
	w.customConvertKey = value
	w.convertKeyCombo.RemoveAll()
	for _, opt := range convertKeyOptions {
		w.convertKeyCombo.AppendText(opt.Label)
	}
	// Add custom entry with the value
	customLabel := formatCustomKeyLabel(value)
	w.convertKeyCombo.AppendText(customLabel)
	w.convertKeyCombo.SetActive(len(convertKeyOptions))
	w.prevConvertKeyIdx = 0 // fallback to first option if custom is canceled
}

// setLayoutComboByValue sets a layout combo to match a layout name.
func (w *SettingsWindow) setLayoutComboByValue(combo *gtk.ComboBoxText, value string) {
	// Extract layout name without variant: "ua(unicode)" -> "ua"
	layoutName := value
	if idx := strings.Index(value, "("); idx != -1 {
		layoutName = strings.TrimSpace(value[:idx])
	}

	for i, layout := range layoutOptions {
		if layout == layoutName {
			combo.SetActive(i)
			return
		}
	}
	// Default to first option if not found
	combo.SetActive(0)
}

// Hide hides the settings window.
func (w *SettingsWindow) Hide() {
	// Increment generation to cancel any pending detection UI updates
	w.detectionGeneration++
	glib.IdleAdd(func() {
		if w.window != nil {
			w.window.Hide()
		}
	})
}

// updateDetectionStatusAsync runs detection in background and updates status label.
// Only runs when "auto" is selected in layout switch combo.
func (w *SettingsWindow) updateDetectionStatusAsync() {
	// Check if auto-detect is selected
	idx := w.layoutSwitchCombo.GetActive()
	if idx < 0 || idx >= len(layoutSwitchOptions) {
		return
	}

	// If not "auto", cancel pending updates and clear the status label
	if layoutSwitchOptions[idx].Value != "auto" {
		w.detectionGeneration++ // Cancel any pending detection UI updates
		w.layoutSwitchStatus.SetText("")
		w.lastDetectionResult = nil
		return
	}

	// Guard against concurrent detection runs
	if w.detectionInFlight {
		w.detectionPendingRerun = true // Schedule rerun after current detection completes
		return
	}
	w.detectionInFlight = true
	w.detectionPendingRerun = false // Clear pending flag since we're starting

	// Increment generation and capture it for stale check
	w.detectionGeneration++
	currentGen := w.detectionGeneration

	// Show "Detecting..." message
	w.layoutSwitchStatus.SetMarkup("<small><span foreground='gray'>" + strDetecting + "</span></small>")

	// Run detection in goroutine
	go func() {
		info := runDetection()

		glib.IdleAdd(func() {
			// Clear in-flight flag
			w.detectionInFlight = false

			// Check if window is still valid
			if w.window == nil {
				w.detectionPendingRerun = false
				return
			}

			// Check for pending rerun request (handles auto ↔ other ↔ auto during in-flight)
			if w.detectionPendingRerun {
				w.detectionPendingRerun = false
				// Check if we should rerun (auto still selected)
				idx := w.layoutSwitchCombo.GetActive()
				if idx >= 0 && idx < len(layoutSwitchOptions) && layoutSwitchOptions[idx].Value == "auto" {
					w.updateDetectionStatusAsync()
				}
				return
			}

			// Check for stale result
			if w.detectionGeneration != currentGen {
				return
			}

			// Additional check: ComboBox must still be on "auto"
			idx := w.layoutSwitchCombo.GetActive()
			if idx < 0 || idx >= len(layoutSwitchOptions) || layoutSwitchOptions[idx].Value != "auto" {
				return
			}

			// Cache the result for validation in task 04
			w.lastDetectionResult = &info

			// Update the status label
			w.updateStatusLabel(info)
		})
	}()
}

// updateStatusLabel updates the detection status label based on DetectionInfo.
func (w *SettingsWindow) updateStatusLabel(info DetectionInfo) {
	var markup string

	switch info.Status {
	case TrayStatusOK:
		// Escape user strings for Pango
		escapedKeys := glib.MarkupEscapeText(info.KeyNames)
		escapedSource := glib.MarkupEscapeText(info.Source)

		if info.Warning != "" {
			// Warning with orange color
			escapedWarning := glib.MarkupEscapeText(info.Warning)
			text := fmt.Sprintf(strDetectedWarning, escapedKeys, escapedSource, escapedWarning)
			markup = "<small><span foreground='#CC7000'>" + text + "</span></small>"
		} else {
			// Success with green color
			text := fmt.Sprintf(strDetectedOK, escapedKeys, escapedSource)
			markup = "<small><span foreground='green'>" + text + "</span></small>"
		}

	case TrayStatusNeedsConfig:
		// Orange "not found" message
		markup = "<small><span foreground='#CC7000'>" + strDetectNeedsConfig + "</span></small>"

	case TrayStatusDetectError:
		// Red error message with fallback for empty error
		errText := info.Error
		if errText == "" {
			errText = strDetectErrorUnknown
		}
		escapedError := glib.MarkupEscapeText(errText)
		text := fmt.Sprintf(strDetectError, escapedError)
		markup = "<small><span foreground='red'>" + text + "</span></small>"

	default:
		// Service error or other - show error in red
		if info.Error != "" {
			escapedError := glib.MarkupEscapeText(info.Error)
			text := fmt.Sprintf(strDetectError, escapedError)
			markup = "<small><span foreground='red'>" + text + "</span></small>"
		}
	}

	w.layoutSwitchStatus.SetMarkup(markup)
}

// IsVisible returns true if the window is currently visible.
func (w *SettingsWindow) IsVisible() bool {
	if w.window == nil {
		return false
	}
	return w.window.IsVisible()
}

// onRestartClicked handles the Restart button click.
func (w *SettingsWindow) onRestartClicked() {
	// Run in goroutine to avoid blocking UI
	go func() {
		err := w.serviceManager.Restart()
		glib.IdleAdd(func() {
			if err != nil {
				if !errors.Is(err, ErrUserCanceled) {
					w.showErrorDialog(strErrorRestartFailed, err.Error())
				}
			}
			w.updateServiceStatus()
			// Notify tray to update status
			if w.app != nil {
				w.app.UpdateServiceStatus()
			}
		})
	}()
}

// onCancelClicked handles the Cancel button click.
func (w *SettingsWindow) onCancelClicked() {
	w.Hide()
}

// onApplyClicked handles the Apply button click.
func (w *SettingsWindow) onApplyClicked() {
	// Collect values from widgets
	cfg, err := w.collectConfigValues()
	if err != nil {
		w.showErrorDialog(strErrorValidation, err.Error())
		return
	}

	// Validate values
	if err := w.validateConfig(cfg); err != nil {
		w.showErrorDialog(strErrorValidation, err.Error())
		return
	}
	iconMode, err := w.trayIconMode()
	if err != nil {
		w.showErrorDialog(strErrorValidation, err.Error())
		return
	}
	daemonConfigChanged := w.loadedConfig == nil || !sameTrayConfig(cfg, w.loadedConfig)

	// Check auto-detect status before saving
	layoutSwitchIdx := w.layoutSwitchCombo.GetActive()
	if daemonConfigChanged && layoutSwitchIdx >= 0 && layoutSwitchIdx < len(layoutSwitchOptions) &&
		layoutSwitchOptions[layoutSwitchIdx].Value == "auto" {
		// Get detection info from cache or run detection
		var info DetectionInfo
		if w.lastDetectionResult != nil {
			info = *w.lastDetectionResult
		} else {
			info = runDetection()
		}

		// Show dialog only if status is not OK (Warning within OK is still OK)
		if info.Status != TrayStatusOK {
			action := w.showAutoDetectFailDialog(info)
			switch action {
			case "manual":
				w.selectManualKey()
				return
			case "cancel":
				return
				// case "save": continue with saving
			}
		}
	}

	go func() {
		traySaveErr := SaveTrayIconMode(iconMode)
		if traySaveErr != nil {
			glib.IdleAdd(func() {
				w.showErrorDialog(strErrorTraySaveFailed, traySaveErr.Error())
			})
			return
		}
		if w.app != nil {
			w.app.UpdateTrayIconMode(iconMode)
		}

		if !daemonConfigChanged {
			glib.IdleAdd(func() {
				w.Hide()
			})
			return
		}

		// Save daemon settings only when they changed.
		saveErr := w.saveConfig(cfg)
		var restartErr error
		if saveErr == nil {
			// Keep restart out of GTK main loop to avoid UI freeze while waiting for polkit.
			restartErr = w.serviceManager.Restart()
		}

		glib.IdleAdd(func() {
			w.recordConfigSave(cfg, saveErr)
			if saveErr != nil {
				if !errors.Is(saveErr, ErrUserCanceled) {
					w.showErrorDialog(strErrorSaveFailed, saveErr.Error())
				}
				return
			}
			if restartErr != nil {
				// Cancellation also leaves the saved configuration unapplied.
				w.showWarningDialog(strWarnRestartFailed, restartErr.Error())
			}

			w.updateServiceStatus()
			if w.app != nil {
				w.app.UpdateServiceStatus()
			}

			// Close window on success
			w.Hide()
		})
	}()
}

func cloneTrayConfig(cfg *TrayConfig) *TrayConfig {
	clone := *cfg
	clone.Blacklist = append([]string(nil), cfg.Blacklist...)
	return &clone
}

func sameTrayConfig(left, right *TrayConfig) bool {
	if left == nil || right == nil {
		return left == right
	}
	if left.LayoutSwitch != right.LayoutSwitch ||
		left.ConvertKey != right.ConvertKey ||
		left.Layout1 != right.Layout1 ||
		left.Layout2 != right.Layout2 ||
		left.Delay != right.Delay ||
		left.LayoutSwitchDelay != right.LayoutSwitchDelay ||
		len(left.Blacklist) != len(right.Blacklist) {
		return false
	}
	entries := make(map[string]struct{}, len(left.Blacklist))
	for _, entry := range left.Blacklist {
		entries[entry] = struct{}{}
	}
	for _, entry := range right.Blacklist {
		if _, ok := entries[entry]; !ok {
			return false
		}
	}
	return true
}

// collectConfigValues collects configuration values from widgets.
func (w *SettingsWindow) collectConfigValues() (*TrayConfig, error) {
	cfg := &TrayConfig{}

	// Get layout switch value
	layoutSwitchIdx := w.layoutSwitchCombo.GetActive()
	if layoutSwitchIdx < 0 {
		return nil, errors.New("invalid layout switch selection")
	}
	// Check if custom value is selected (index beyond predefined options)
	if layoutSwitchIdx >= len(layoutSwitchOptions) {
		if w.customLayoutSwitch != "" {
			cfg.LayoutSwitch = w.customLayoutSwitch
		} else {
			return nil, errors.New("invalid layout switch selection")
		}
	} else {
		cfg.LayoutSwitch = layoutSwitchOptions[layoutSwitchIdx].Value
	}

	// Get convert key value
	convertKeyIdx := w.convertKeyCombo.GetActive()
	if convertKeyIdx < 0 {
		return nil, errors.New("invalid convert key selection")
	}
	// Check if custom value is selected (index beyond predefined options)
	if convertKeyIdx >= len(convertKeyOptions) {
		if w.customConvertKey != "" {
			cfg.ConvertKey = w.customConvertKey
		} else {
			return nil, errors.New("invalid convert key selection")
		}
	} else {
		cfg.ConvertKey = convertKeyOptions[convertKeyIdx].Value
	}

	// Get layout values
	layout1Idx := w.layout1Combo.GetActive()
	if layout1Idx < 0 || layout1Idx >= len(layoutOptions) {
		return nil, errors.New("invalid layout 1 selection")
	}
	cfg.Layout1 = layoutOptions[layout1Idx]

	layout2Idx := w.layout2Combo.GetActive()
	if layout2Idx < 0 || layout2Idx >= len(layoutOptions) {
		return nil, errors.New("invalid layout 2 selection")
	}
	cfg.Layout2 = layoutOptions[layout2Idx]

	// Get delay values
	cfg.Delay = int(w.delayBetweenSpin.GetValue())
	cfg.LayoutSwitchDelay = int(w.delaySwitchSpin.GetValue())

	// Collect blacklist from device checkboxes
	// Unchecked devices go to blacklist
	cfg.Blacklist = nil
	for uid, checkbox := range w.deviceCheckboxes {
		if !checkbox.GetActive() {
			cfg.Blacklist = append(cfg.Blacklist, uid)
		}
	}

	return cfg, nil
}

// validateConfig validates configuration values.
func (w *SettingsWindow) validateConfig(cfg *TrayConfig) error {
	// Validate delay (0-100 ms)
	if cfg.Delay < 0 || cfg.Delay > 100 {
		return errors.New("delay between keystrokes must be between 0 and 100 ms")
	}

	// Validate layout switch delay (0-500 ms)
	if cfg.LayoutSwitchDelay < 0 || cfg.LayoutSwitchDelay > 500 {
		return errors.New("delay after switching must be between 0 and 500 ms")
	}

	// Validate layout switch key (must not be "custom" placeholder)
	if cfg.LayoutSwitch == "custom" {
		return errors.New("select a layout switch key or specify a key code")
	}

	// Validate convert key (must not be "custom" placeholder)
	if cfg.ConvertKey == "custom" {
		return errors.New("select a conversion key or specify a key code")
	}
	convertKey, err := strconv.ParseUint(cfg.ConvertKey, 10, 16)
	if err != nil {
		return errors.New("conversion key must be one numeric evdev scancode")
	}
	return gsconfig.ValidateConvertKey(uint16(convertKey))
}

func trayConfigWriteArgs(cfg *TrayConfig) string {
	// Build config string
	configStr := fmt.Sprintf(
		"layout-switch=%s,convert-key=%s,delay=%d,layout-switch-delay=%d,layout1=%s,layout2=%s",
		cfg.LayoutSwitch, cfg.ConvertKey, cfg.Delay, cfg.LayoutSwitchDelay, cfg.Layout1, cfg.Layout2,
	)

	// Add blacklist if not empty
	if len(cfg.Blacklist) > 0 {
		configStr += ",blacklist=" + strings.Join(cfg.Blacklist, ";")
	}

	return configStr
}

// saveConfig saves configuration using pkexec gswitch --write-config.
func (w *SettingsWindow) saveConfig(cfg *TrayConfig) error {
	configStr := trayConfigWriteArgs(cfg)

	// Find gswitch binary
	gswitchPath, err := findGswitchBinaryForPkexec()
	if err != nil {
		return fmt.Errorf("failed to find gswitch: %w", err)
	}

	// Run pkexec gswitch --write-config "..."
	// #nosec G204 -- gswitchPath is validated by findGswitchBinary to be a safe executable path
	cmd := exec.Command("pkexec", gswitchPath, "--write-config", configStr)
	output, err := cmd.CombinedOutput()
	if err != nil {
		// Check for user cancellation (exit code 126)
		exitErr := &exec.ExitError{}
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 126 {
			return ErrUserCanceled
		}
		return fmt.Errorf("%s: %s", err.Error(), strings.TrimSpace(string(output)))
	}

	return nil
}

// showWarningDialog shows a warning dialog with the given title and message.
func (w *SettingsWindow) showWarningDialog(title, message string) {
	dialog := gtk.MessageDialogNew(
		w.window,
		gtk.DIALOG_MODAL|gtk.DIALOG_DESTROY_WITH_PARENT,
		gtk.MESSAGE_WARNING,
		gtk.BUTTONS_OK,
		"%s",
		title,
	)
	dialog.FormatSecondaryText("%s", message)
	dialog.Run()
	dialog.Destroy()
}

// showAutoDetectFailDialog shows a dialog when auto-detect failed.
// Returns "save" to continue saving, "manual" to select key manually, or "cancel".
func (w *SettingsWindow) showAutoDetectFailDialog(info DetectionInfo) string {
	dialog := gtk.MessageDialogNew(
		w.window,
		gtk.DIALOG_MODAL|gtk.DIALOG_DESTROY_WITH_PARENT,
		gtk.MESSAGE_WARNING,
		gtk.BUTTONS_NONE,
		"%s",
		strAutoDetectFailTitle,
	)

	// Build secondary text with reason and attempts
	reason := info.Error
	if reason == "" {
		reason = strAutoDetectFailDefault
	}
	secondaryText := fmt.Sprintf(strAutoDetectFailReason, reason)

	// Add attempts section if present
	if len(info.Attempts) > 0 {
		secondaryText += "\n\n" + strAutoDetectFailSources
		var b strings.Builder
		for i, att := range info.Attempts {
			if i >= 3 {
				b.WriteString(fmt.Sprintf("\n  ... and %d more", len(info.Attempts)-3))
				break
			}
			b.WriteString("\n  • " + att)
		}
		secondaryText += b.String()
	}

	dialog.FormatSecondaryText("%s", secondaryText)

	// Add buttons: Cancel, Manual, Save
	_, _ = dialog.AddButton(strButtonCancel, gtk.RESPONSE_CANCEL)
	_, _ = dialog.AddButton(strAutoDetectFailManual, gtk.RESPONSE_NO)
	_, _ = dialog.AddButton(strAutoDetectFailSave, gtk.RESPONSE_YES)

	response := dialog.Run()
	dialog.Destroy()

	switch response {
	case gtk.RESPONSE_YES:
		return "save"
	case gtk.RESPONSE_NO:
		return "manual"
	default:
		return "cancel"
	}
}

// selectManualKey switches to "Other..." option and triggers the key picker.
// This reuses the existing flow in onLayoutSwitchChanged() which handles
// key picker dialog, revert on cancel, and combo update.
func (w *SettingsWindow) selectManualKey() {
	// Find index of "custom" option in layoutSwitchOptions
	customIdx := -1
	for i, opt := range layoutSwitchOptions {
		if opt.Value == "custom" {
			customIdx = i
			break
		}
	}
	if customIdx < 0 {
		return
	}

	// Set combo to "Other..." - this triggers onLayoutSwitchChanged()
	// which shows the key picker dialog and handles revert on cancel
	w.layoutSwitchCombo.SetActive(customIdx)
}

// onAutostartToggled handles the autostart checkbox toggle.
func (w *SettingsWindow) onAutostartToggled() {
	// Prevent handling during programmatic updates
	if w.autostartUpdating {
		return
	}

	isActive := w.autostartCheck.GetActive()

	// Run in goroutine to avoid blocking UI
	go func() {
		var err error
		if isActive {
			err = w.serviceManager.Enable()
		} else {
			err = w.serviceManager.Disable()
		}

		glib.IdleAdd(func() {
			if err != nil {
				if !errors.Is(err, ErrUserCanceled) {
					if isActive {
						w.showErrorDialog(strErrorEnableFailed, err.Error())
					} else {
						w.showErrorDialog(strErrorDisableFailed, err.Error())
					}
				}
				// Revert checkbox to previous state
				w.autostartUpdating = true
				w.autostartCheck.SetActive(!isActive)
				w.autostartUpdating = false
			}
		})
	}()
}

// updateServiceStatus updates the service status label and autostart checkbox.
func (w *SettingsWindow) updateServiceStatus() {
	status, err := w.serviceManager.GetStatus()
	enabled, _ := w.serviceManager.IsEnabled()

	// Update status label with colored bullet
	markup := fmt.Sprintf("<span foreground='%s'>\u25CF</span> %s",
		status.StatusColor(), status.String())
	if err != nil {
		escapedErr := glib.MarkupEscapeText(err.Error())
		markup = fmt.Sprintf("<span foreground='red'>\u25CF</span> %s (%s)",
			strStatusUnknown, escapedErr)
	}
	w.statusLabel.SetMarkup(markup)

	// Update autostart checkbox without triggering the toggle handler.
	// Enabling autostart requires an installed unit and a readable service state.
	w.autostartUpdating = true
	w.autostartCheck.SetActive(enabled)
	if err != nil || status == StatusNotInstalled || status == StatusUnknown {
		w.autostartCheck.SetSensitive(false)
	} else {
		w.autostartCheck.SetSensitive(true)
	}
	w.autostartUpdating = false
}

// showErrorDialog shows an error dialog with the given title and message.
func (w *SettingsWindow) showErrorDialog(title, message string) {
	dialog := gtk.MessageDialogNew(
		w.window,
		gtk.DIALOG_MODAL|gtk.DIALOG_DESTROY_WITH_PARENT,
		gtk.MESSAGE_ERROR,
		gtk.BUTTONS_OK,
		"%s",
		title,
	)
	dialog.FormatSecondaryText("%s", message)
	dialog.Run()
	dialog.Destroy()
}

// onLayoutSwitchChanged handles layout switch combo selection change.
func (w *SettingsWindow) onLayoutSwitchChanged() {
	// Ignore changes triggered programmatically (e.g., in loadConfig)
	if w.ignoreComboChanged {
		return
	}

	idx := w.layoutSwitchCombo.GetActive()
	if idx < 0 || idx >= len(layoutSwitchOptions) {
		return
	}

	// Check if "Other..." (custom) is selected
	if layoutSwitchOptions[idx].Value != "custom" {
		// Save current index as previous (for reverting if dialog is canceled)
		w.prevLayoutSwitchIdx = idx
		// Update detection status based on new selection
		w.updateDetectionStatusAsync()
		return
	}

	// Get current convert key to show warning (to avoid conflicts)
	currentConvertKey := w.getConvertKeyValueForWarning()

	result, ok := ShowKeyPickerDialog(w.window, KeyPickerForLayoutSwitch, currentConvertKey)
	if !ok || result.Value == "" {
		// User canceled - revert to previous selection
		w.revertLayoutSwitchCombo()
		return
	}

	w.customLayoutSwitch = result.Value
	w.updateLayoutSwitchComboWithCustom(result)
}

// revertLayoutSwitchCombo reverts layout switch combo to previous selection.
func (w *SettingsWindow) revertLayoutSwitchCombo() {
	if w.customLayoutSwitch != "" {
		// Revert to custom value (added as last item)
		w.layoutSwitchCombo.SetActive(len(layoutSwitchOptions))
	} else {
		// Revert to previous selection
		w.layoutSwitchCombo.SetActive(w.prevLayoutSwitchIdx)
	}
}

// updateLayoutSwitchComboWithCustom updates combo with custom key result.
func (w *SettingsWindow) updateLayoutSwitchComboWithCustom(result KeyPickerResult) {
	w.layoutSwitchCombo.RemoveAll()
	for _, opt := range layoutSwitchOptions {
		w.layoutSwitchCombo.AppendText(opt.Label)
	}
	customLabel := formatCustomKeyLabel(result.Value)
	w.layoutSwitchCombo.AppendText(customLabel)
	w.layoutSwitchCombo.SetActive(len(layoutSwitchOptions))
}

// onConvertKeyChanged handles convert key combo selection change.
func (w *SettingsWindow) onConvertKeyChanged() {
	idx := w.convertKeyCombo.GetActive()
	if idx < 0 || idx >= len(convertKeyOptions) {
		return
	}

	// Check if "Other..." (custom) is selected
	if convertKeyOptions[idx].Value != "custom" {
		// Save current index as previous (for reverting if dialog is canceled)
		w.prevConvertKeyIdx = idx
		return
	}

	// Get current layout switch key to show warning (to avoid conflicts)
	currentLayoutSwitch := w.getLayoutSwitchValueForWarning()

	result, ok := ShowKeyPickerDialog(w.window, KeyPickerForConvertKey, currentLayoutSwitch)
	if !ok || result.Value == "" {
		// User canceled - revert to previous selection
		w.revertConvertKeyCombo()
		return
	}

	w.customConvertKey = result.Value
	w.updateConvertKeyComboWithCustom(result)
}

// revertConvertKeyCombo reverts convert key combo to previous selection.
func (w *SettingsWindow) revertConvertKeyCombo() {
	if w.customConvertKey != "" {
		// Revert to custom value (added as last item)
		w.convertKeyCombo.SetActive(len(convertKeyOptions))
	} else {
		// Revert to previous selection
		w.convertKeyCombo.SetActive(w.prevConvertKeyIdx)
	}
}

// updateConvertKeyComboWithCustom updates combo with custom key result.
func (w *SettingsWindow) updateConvertKeyComboWithCustom(result KeyPickerResult) {
	w.convertKeyCombo.RemoveAll()
	for _, opt := range convertKeyOptions {
		w.convertKeyCombo.AppendText(opt.Label)
	}
	customLabel := formatCustomKeyLabel(result.Value)
	w.convertKeyCombo.AppendText(customLabel)
	w.convertKeyCombo.SetActive(len(convertKeyOptions))
}

// getCurrentConvertKeyValue returns the currently configured convert key value from config file.
func (w *SettingsWindow) getCurrentConvertKeyValue() string {
	// Always load from config to get the actual active value
	cfg := LoadTrayConfig()
	return cfg.ConvertKey
}

// getConvertKeyValueForWarning returns the convert key value for conflict warnings.
// Prefer the current UI selection (including custom), falling back to config.
func (w *SettingsWindow) getConvertKeyValueForWarning() string {
	idx := w.convertKeyCombo.GetActive()
	if idx < 0 {
		return w.getCurrentConvertKeyValue()
	}

	// Custom value appended to the end of the combo box.
	if idx == len(convertKeyOptions) && w.customConvertKey != "" {
		return w.customConvertKey
	}

	if idx >= 0 && idx < len(convertKeyOptions) {
		value := convertKeyOptions[idx].Value
		if value != "" && value != "custom" {
			return value
		}
		if value == "custom" && w.customConvertKey != "" {
			return w.customConvertKey
		}
	}

	return w.getCurrentConvertKeyValue()
}

// getCurrentLayoutSwitchValue returns the currently configured layout switch key value from config file.
func (w *SettingsWindow) getCurrentLayoutSwitchValue() string {
	// Always load from config to get the actual active value
	cfg := LoadTrayConfig()
	return cfg.LayoutSwitch
}

// getLayoutSwitchValueForWarning returns the layout switch key value for conflict warnings.
// Prefer the current UI selection (including custom), falling back to config.
func (w *SettingsWindow) getLayoutSwitchValueForWarning() string {
	idx := w.layoutSwitchCombo.GetActive()
	if idx < 0 {
		return w.getCurrentLayoutSwitchValue()
	}

	// Custom value appended to the end of the combo box.
	if idx == len(layoutSwitchOptions) && w.customLayoutSwitch != "" {
		return w.customLayoutSwitch
	}

	if idx >= 0 && idx < len(layoutSwitchOptions) {
		value := layoutSwitchOptions[idx].Value
		if value != "" && value != "custom" {
			return value
		}
		if value == "custom" && w.customLayoutSwitch != "" {
			return w.customLayoutSwitch
		}
	}

	return w.getCurrentLayoutSwitchValue()
}
