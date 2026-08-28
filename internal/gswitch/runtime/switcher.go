package runtime

import (
	"context"
	"errors"
	"fmt"
	"log/syslog"
	"os"
	"os/signal"
	"slices"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

var version = "dev"

// SetVersion configures runtime version for logging.
func SetVersion(buildVersion string) {
	version = buildVersion
}

var getActiveSessionEnv = GetActiveSessionEnv

// Switcher handles the keyboard layout switching logic with multi-device support
type Switcher struct {
	config        *Config
	vKeyboard     *VirtualKeyboard
	deviceManager *DeviceManager
	inputReader   *InputReader
	converter     *Converter

	stopAndExit atomic.Bool
	sessionEnv  *SessionEnv // env of the active graphical session (may be nil)
	debug       bool
	logger      *syslog.Writer
	wg          sync.WaitGroup

	ctx    context.Context
	cancel context.CancelFunc

	// Signal handling for graceful shutdown
	sigChan chan os.Signal
	sigStop chan struct{}
	sigOnce sync.Once

	// Selection conversion
	clipboard            *Clipboard
	layoutConverter      *LayoutConverter
	ctrlPressed          atomic.Bool
	inConversion         atomic.Bool
	lastConvertedPrimary string
	lastConvertedMu      sync.Mutex

	eventChan      chan *InputEvent
	overflowEvents []*InputEvent
	overflowMu     sync.Mutex

	// Degraded mode: layout-switch=auto but no grp:* option detected.
	// In this mode: selection convert works, buffer convert is disabled.
	degradedMode bool
}

// NewSwitcher creates a new Switcher instance
func NewSwitcher(config *Config, debug bool) (*Switcher, error) {
	var logger *syslog.Writer
	var err error

	if !debug {
		logger, err = syslog.New(syslog.LOG_INFO|syslog.LOG_DAEMON, "gswitch")
		if err != nil {
			return nil, fmt.Errorf("failed to connect to syslog: %w", err)
		}
	}

	// Cleanup logger on early return; disabled on successful exit
	cleanupLogger := true
	defer func() {
		if cleanupLogger && logger != nil {
			logger.Close()
		}
	}()

	ctx, cancel := context.WithCancel(context.Background())

	s := &Switcher{
		config:      config,
		inputReader: NewInputReader(),
		converter:   NewConverter(),
		debug:       debug,
		logger:      logger,
		ctx:         ctx,
		cancel:      cancel,
	}

	s.converter.ConvKey = config.ConvertKey

	// Setup signal handler early for graceful shutdown during waitForSession
	s.setupSignalHandler()
	cleanupSignalHandler := true
	defer func() {
		if cleanupSignalHandler {
			s.stopSignalHandler()
		}
	}()

	// A user service already runs inside the target graphical session and must
	// keep its own environment. Root compatibility mode has to discover that
	// session and drop privileges for session helpers.
	autoDetect := config.LayoutSwitchAuto && len(config.LayoutSwitchKey) == 0
	env, err := s.prepareSessionEnvironment(autoDetect, os.Geteuid() == 0)
	if err != nil {
		return nil, err
	}

	// Apply session environment before X11/Wayland setup
	s.sessionEnv = env
	if env != nil {
		if err := ApplySessionEnv(env); err != nil {
			s.log("Warning: failed to apply session env: %v", err)
		}
	}

	// Setup X11 environment before auto-detection (needed for setxkbmap, X11 property reads)
	setupX11Environment()

	// Perform auto-detection of layout switch keys if configured
	if autoDetect {
		result, detectErr := DetectLayoutSwitchKeys(nil)
		scancodes, degraded, err := applyAutoDetectResult(result, detectErr)

		switch {
		case degraded:
			// Degraded mode: no layout switch detected, but don't fail.
			// Selection convert works, buffer convert (double-shift) is disabled.
			s.log("Layout switch not detected, running in degraded mode")
			s.log("Configure layout switch via gswitch-tray or gswitch -c")
			s.degradedMode = true
		case err != nil:
			return nil, fmt.Errorf("layout-switch=auto: failed to auto-detect layout switch keys: %w\n"+
				"please specify layout-switch manually in %s\n"+
				"run 'sudo showkey' to find your key scancodes", err, ConfigFile)
		default:
			config.LayoutSwitchKey = scancodes
			if debug {
				fmt.Printf("Auto-detected layout switch keys: %s (%v) from %s\n",
					result.KeyNames, result.Scancodes, result.Source)
			}
			if result.Warning != "" && debug {
				fmt.Printf("WARNING: %s\n", result.Warning)
			}

			// Validate ConvertKey doesn't conflict with detected keys
			if config.ConvertKey != 0 && slices.Contains(config.LayoutSwitchKey, config.ConvertKey) {
				return nil, fmt.Errorf("convert key (%d) conflicts with auto-detected layout switch key (%s)",
					config.ConvertKey, result.KeyNames)
			}
		}
	}
	s.converter.LSKeys = config.LayoutSwitchKey
	s.converter.SetDebugLogger(s.logDebug)

	for _, uid := range config.Blacklist {
		s.inputReader.AddToBlacklist(uid)
		s.logDebug("Added to blacklist: %s", uid)
	}

	if display := os.Getenv("DISPLAY"); display != "" {
		s.logDebug("Using DISPLAY=%s", display)
	}
	if xauth := os.Getenv("XAUTHORITY"); xauth != "" {
		s.logDebug("Using XAUTHORITY=%s", xauth)
	}

	s.clipboard, err = NewClipboard(s.sessionEnv)
	if err != nil {
		s.log("Warning: clipboard not available: %v", err)
		s.log("Selection conversion will be disabled")
	} else {
		s.log("Clipboard backend: %s", s.clipboard.BackendName())
	}

	if err := s.initLayoutConverter(); err != nil {
		s.log("Warning: layout converter not available: %v", err)
		s.log("Selection conversion will be disabled")
	}

	cleanupLogger = false
	cleanupSignalHandler = false
	return s, nil
}

func (s *Switcher) prepareSessionEnvironment(autoDetect, runningAsRoot bool) (*SessionEnv, error) {
	if !runningAsRoot {
		return nil, nil //nolint:nilnil // nil selects the current user's environment.
	}

	env, err := getActiveSessionEnv()
	switch {
	case errors.Is(err, ErrNoActiveSession):
		if autoDetect {
			// Compatibility mode may start before login and must wait for the
			// graphical user whose environment and credentials it needs.
			env, err = s.waitForSession(s.ctx)
			if err != nil {
				return nil, fmt.Errorf("failed while waiting for session: %w", err)
			}
		} else {
			s.log("Warning: no active graphical session, clipboard may be unavailable")
		}
	case errors.Is(err, ErrNoSystemd):
		// Non-systemd root foreground mode is best-effort.
		s.log("Warning: %v, continuing with current environment", err)
	case err != nil:
		if autoDetect {
			return nil, fmt.Errorf("failed to get session env: %w", err)
		}
		s.log("Warning: failed to get session env: %v", err)
	}

	return env, nil
}

func (s *Switcher) initLayoutConverter() error {
	layouts, err := s.resolveLayouts()
	if err != nil {
		return err
	}

	layout1, layout2, err := loadLayoutPair(layouts)
	if err != nil {
		return err
	}

	s.layoutConverter = NewLayoutConverter(layout1, layout2)
	s.log("Layout converter initialized: %s <-> %s", formatLayout(layouts[0]), formatLayout(layouts[1]))

	return nil
}

// resolveLayouts returns the two layouts to use for conversion.
// Priority: explicit config (layout1/layout2) > auto-detect.
func (s *Switcher) resolveLayouts() ([]LayoutSpec, error) {
	// Explicit config wins
	if len(s.config.Layouts) == 2 {
		s.logDebug("Using layouts from config: %v", s.config.Layouts)
		return s.config.Layouts, nil
	}

	// Auto-detect
	layouts, err := GetCurrentLayouts(s.sessionEnv)
	if err != nil {
		return nil, fmt.Errorf("failed to get layouts: %w", err)
	}

	if len(layouts) < 2 {
		return nil, fmt.Errorf("need at least 2 layouts, detected %d: %v (set layout1/layout2 in config)", len(layouts), layouts)
	}

	if len(layouts) > 2 {
		return nil, fmt.Errorf("detected %d layouts: %v\n"+
			"gswitch supports only 2 layouts for conversion.\n"+
			"Please set layout1 and layout2 in %s\n"+
			"or run 'sudo gswitch -c' to configure",
			len(layouts), layouts, ConfigFile)
	}

	s.logDebug("Detected layouts: %v", layouts)
	return layouts, nil
}

// loadLayoutPair loads XKB data for two layouts.
func loadLayoutPair(layouts []LayoutSpec) (*LayoutInfo, *LayoutInfo, error) {
	if len(layouts) < 2 {
		return nil, nil, fmt.Errorf("expected 2 layouts, got %d", len(layouts))
	}

	layout1, err := LoadLayout(layouts[0].Name, layouts[0].Variant)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load layout %s: %w", formatLayout(layouts[0]), err)
	}

	layout2, err := LoadLayout(layouts[1].Name, layouts[1].Variant)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load layout %s: %w", formatLayout(layouts[1]), err)
	}

	return layout1, layout2, nil
}

// applyAutoDetectResult processes the detection result and determines the operating mode.
// Returns:
//   - scancodes: detected scancodes (nil if not found or error)
//   - degradedMode: true if running in degraded mode (ErrNoLayoutSwitchOption)
//   - error: fatal error that should stop the daemon
//
// Contract: on success (err == nil), result must not be nil.
// Extracted for unit testing without system side effects.
func applyAutoDetectResult(result *DetectionResult, err error) ([]uint16, bool, error) {
	if errors.Is(err, ErrNoLayoutSwitchOption) {
		// Degraded mode: no layout switch option found, but not fatal.
		return nil, true, nil
	}
	if err != nil {
		// Fatal error
		return nil, false, err
	}
	// Success: validate result and return detected scancodes
	if result == nil {
		return nil, false, errors.New("detection returned nil result without error")
	}
	return result.Scancodes, false, nil
}

func (s *Switcher) log(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	if s.debug {
		fmt.Println(msg)
	}
	if s.logger != nil {
		s.logger.Info(msg)
	}
}

func (s *Switcher) logDebug(format string, args ...any) {
	if s.debug {
		timestamp := time.Now().Format("15:04:05.000")
		fmt.Printf("%s %s\n", timestamp, fmt.Sprintf(format, args...))
	}
}

func (s *Switcher) logError(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	if s.debug {
		fmt.Println("ERROR: " + msg)
	}
	if s.logger != nil {
		s.logger.Err(msg)
	}
}

// setupSignalHandler configures signal handling for graceful shutdown.
// Must be called before any blocking operation (e.g., waitForSession).
func (s *Switcher) setupSignalHandler() {
	s.sigChan = make(chan os.Signal, 1)
	s.sigStop = make(chan struct{})
	signal.Notify(s.sigChan, syscall.SIGTERM, syscall.SIGINT, syscall.SIGQUIT, syscall.SIGHUP)
	go func() {
		select {
		case sig := <-s.sigChan:
			s.log("Got signal to exit (%v). Bye.", sig)
			s.stopAndExit.Store(true)
			s.cancel()
		case <-s.sigStop:
		}
	}()
}

func (s *Switcher) stopSignalHandler() {
	s.sigOnce.Do(func() {
		if s.sigChan != nil {
			signal.Stop(s.sigChan)
		}
		if s.sigStop != nil {
			close(s.sigStop)
		}
	})
}

// Run starts the main event loop
func (s *Switcher) Run() error {
	s.log("Starting gswitch %s...", version)

	s.logDebug("Installing virtual keyboard...")
	var err error
	s.vKeyboard, err = NewVirtualKeyboard()
	if err != nil {
		return fmt.Errorf("cannot install virtual keyboard: %w", err)
	}
	defer s.vKeyboard.Close()

	vkUID := s.getVirtualKeyboardUID()
	s.inputReader.AddToBlacklist(vkUID)
	s.logDebug("Virtual keyboard UID: %s (blacklisted)", vkUID)

	s.logDebug("Initializing device manager...")
	s.deviceManager, err = NewDeviceManager()
	if err != nil {
		return fmt.Errorf("cannot initialize device manager: %w", err)
	}
	defer s.deviceManager.Close()

	if err := s.deviceManager.ScanExisting(); err != nil {
		return fmt.Errorf("failed to scan existing devices: %w", err)
	}

	s.log("gswitch started successfully (auto-detect mode)")

	s.eventChan = make(chan *InputEvent, 256)

	s.wg.Add(3)
	go s.processDeviceEvents()
	go s.processKeyEvents()
	go s.processInotifyEvents()

	// Main event loop
	for !s.stopAndExit.Load() {
		select {
		case <-s.ctx.Done():
		case event := <-s.eventChan:
			s.handleIncomingEvent(event)
			s.flushOverflowEvents()
		case <-time.After(EventLoopTimeoutMs * time.Millisecond):
			s.flushOverflowEvents()
		}
	}

	// Stop signal handling before cleanup
	s.stopSignalHandler()

	s.inputReader.Close()
	s.logDebug("Waiting for goroutines to finish...")

	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		s.logDebug("All goroutines finished.")
	case <-time.After(ShutdownTimeoutMs * time.Millisecond):
		s.logDebug("Timeout waiting for goroutines, forcing exit.")
	}

	return nil
}

func (s *Switcher) processDeviceEvents() {
	defer s.wg.Done()
	deviceChan := s.deviceManager.Watch()

	for {
		select {
		case <-s.ctx.Done():
			return
		case devEvent, ok := <-deviceChan:
			if !ok {
				return
			}
			s.handleDeviceEvent(devEvent)
		}
	}
}

func (s *Switcher) processKeyEvents() {
	defer s.wg.Done()

	// Use epoll if available, otherwise fall back to polling
	if s.inputReader.HasEpoll() {
		s.processKeyEventsEpoll()
	} else {
		s.processKeyEventsPolling()
	}
}

func (s *Switcher) processKeyEventsEpoll() {
	for {
		select {
		case <-s.ctx.Done():
			return
		default:
		}

		// Wait for events with 100ms timeout to check ctx.Done() periodically
		readyFds := s.inputReader.WaitForEvents(100)
		if len(readyFds) == 0 {
			continue
		}

		// Read events only from ready file descriptors
		for _, fd := range readyFds {
			for {
				event, ok := s.inputReader.ReadEvent(fd)
				if !ok {
					break
				}
				if event.Type == EV_KEY {
					s.enqueueEvent(event)
				}
			}
		}
	}
}

func (s *Switcher) processKeyEventsPolling() {
	ticker := time.NewTicker(PollingIntervalMs * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			s.readAllDeviceEvents()
		}
	}
}

func (s *Switcher) processInotifyEvents() {
	defer s.wg.Done()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			if err := s.deviceManager.ProcessEvents(); err != nil {
				s.logError("inotify error: %v", err)
			}
		}
	}
}

func (s *Switcher) handleDeviceEvent(event DeviceEvent) {
	if event.Connected {
		time.Sleep(DeviceSettleMs * time.Millisecond)

		device, err := addDeviceWithRetry(
			s.ctx,
			event.Path,
			DeviceOpenRetryAttempts,
			DeviceOpenRetryDelayMs*time.Millisecond,
			s.inputReader.AddDevice,
		)
		if err != nil {
			s.logDebug("Skipped device %s: %v", event.Path, err)
			return
		}
		s.log("Added device %s: %s (UID=%s)", event.Path, device.Name, device.UID)
	} else {
		device := s.inputReader.GetDeviceByPath(event.Path)
		if device != nil {
			s.log("Removed device: %s (%s)", event.Path, device.Name)
			if err := s.inputReader.RemoveDevice(event.Path); err != nil {
				s.logDebug("Failed to remove device %s: %v", event.Path, err)
			}
		}
	}
}

type addDeviceFunc func(string) (*Device, error)

func addDeviceWithRetry(
	ctx context.Context,
	path string,
	attempts int,
	delay time.Duration,
	add addDeviceFunc,
) (*Device, error) {
	if attempts <= 0 {
		return nil, errors.New("device open retry requires at least one attempt")
	}

	var err error
	for attempt := range attempts {
		var device *Device
		device, err = add(path)
		if err == nil {
			return device, nil
		}
		if !isRetryableDeviceOpenError(err) || attempt == attempts-1 {
			return nil, err
		}

		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}

	return nil, err
}

func isRetryableDeviceOpenError(err error) bool {
	return errors.Is(err, os.ErrPermission) ||
		errors.Is(err, syscall.EACCES) ||
		errors.Is(err, syscall.EPERM) ||
		errors.Is(err, os.ErrNotExist)
}

func (s *Switcher) readAllDeviceEvents() {
	fds := s.inputReader.GetAllFDs()
	for _, fd := range fds {
		for {
			event, ok := s.inputReader.ReadEvent(fd)
			if !ok {
				break
			}
			if event.Type == EV_KEY {
				s.enqueueEvent(event)
			}
		}
	}
}

func (s *Switcher) processKeyEvent(event *InputEvent) {
	s.logDebug("input event received")

	// Track Ctrl state (ignore autorepeat)
	if event.Code == KEY_LEFTCTRL || event.Code == KEY_RIGHTCTRL {
		switch event.Value {
		case 1:
			s.ctrlPressed.Store(true)
		case 0:
			s.ctrlPressed.Store(false)
		}
	}

	// Ctrl+ConvertKey -> selection conversion (custom key mode)
	if s.config.ConvertKey != 0 && event.Code == s.config.ConvertKey && event.Value == 1 && s.ctrlPressed.Load() {
		s.logDebug("Ctrl+ConvertKey detected - converting selection")
		s.convertSelection()
		return
	}

	// Skip regular keys when Ctrl is pressed (Ctrl+key is a hotkey, not text input)
	if s.ctrlPressed.Load() && Letters[event.Code] {
		s.logDebug("skipping key (Ctrl pressed)")
		return
	}

	// Push to converter buffer
	if s.converter.Push(event.Code, event.Value) {
		s.logDebug("buffer length=%d", s.converter.BufferLen())

		action := s.converter.Process()
		if action != ActionNone {
			s.logDebug("Convert pattern detected, processing...")

			// Ctrl+DoubleShift -> selection conversion (double-shift mode)
			if s.config.ConvertKey == 0 && s.ctrlPressed.Load() {
				s.logDebug("Ctrl+DoubleShift detected - converting selection")
				s.converter.ClearBuffer()
				s.convertSelection()
				return
			}

			// Bare double-shift without Ctrl and without text: nothing to do
			if action == ActionDoubleShiftNoText {
				s.logDebug("Double-shift without text ignored")
				return
			}

			// In degraded mode, buffer convert is disabled (no layout-switch key)
			if s.degradedMode {
				s.logDebug("Degraded mode: buffer convert disabled")
				s.converter.ClearBuffer()
				return
			}

			if s.converter.HasText() {
				s.performConversion(action)
			} else {
				s.logDebug("Buffer has no text, skipping conversion")
			}
		}
	}
}

func (s *Switcher) performConversion(action Action) {
	if s.config.ReverseMode {
		switch action {
		case ActionConvertWord:
			action = ActionConvertAll
		case ActionConvertAll:
			action = ActionConvertWord
		}
	}

	if action == ActionConvertAll {
		s.logDebug("convert all")
	} else {
		s.logDebug("convert word")
	}

	events := s.converter.Convert(action)

	// Release shift keys before layout switch
	for shiftKey := range Shifts {
		if err := s.vKeyboard.KeyUp(shiftKey); err != nil {
			s.logDebug("failed to release shift key %d: %v", shiftKey, err)
		}
	}
	time.Sleep(time.Duration(s.config.Delay) * time.Millisecond)

	// The DE applies the layout switch asynchronously (GNOME routes it through
	// the shell/ibus) and key events arriving mid-switch get dropped, so pause
	// right after the switch keys before emitting backspaces and the replay.
	switchEvents := len(s.converter.LSKeys) * 2
	for i, ev := range events {
		if err := s.vKeyboard.EmitKey(ev.Code, ev.Value); err != nil {
			s.logError("failed to emit key: %v", err)
			return
		}
		time.Sleep(time.Duration(s.config.Delay) * time.Millisecond)
		if i == switchEvents-1 {
			time.Sleep(time.Duration(s.config.LayoutSwitchDelay) * time.Millisecond)
		}
	}

	time.Sleep(time.Duration(s.config.LayoutSwitchDelay) * time.Millisecond)
	s.inputReader.Flush()
	s.logDebug("buffer length after conversion=%d", s.converter.BufferLen())
}

func textDebugSummary(label, text string) string {
	return fmt.Sprintf("%s length=%d runes", label, len([]rune(text)))
}

// convertSelection converts selected text via clipboard
func (s *Switcher) convertSelection() {
	if s.clipboard == nil || s.layoutConverter == nil {
		s.logError("Selection conversion not available")
		return
	}

	s.inConversion.Store(true)
	defer func() {
		s.drainEventChan()
		s.ctrlPressed.Store(false)
		s.converter.ClearBuffer()
		s.inConversion.Store(false)
	}()

	time.Sleep(SelectionConversionInitDelayMs * time.Millisecond)

	// Read PRIMARY selection (what's currently selected)
	if !s.clipboard.HasPrimarySelection() {
		s.logDebug("PRIMARY selection not available")
		return
	}

	text, err := s.clipboard.ReadPrimarySelection()
	if err != nil {
		s.logDebug("failed to read PRIMARY selection: %v", err)
		return
	}

	if text == "" {
		s.logDebug("No text selected")
		return
	}

	s.lastConvertedMu.Lock()
	lastConverted := s.lastConvertedPrimary
	s.lastConvertedMu.Unlock()

	if text == lastConverted {
		s.logDebug("Same selection as last time, skipping")
		return
	}

	s.logDebug("%s", textDebugSummary("selected text", text))

	// Convert text
	isLayout1 := s.layoutConverter.DetectLayout(text)
	var converted string
	if isLayout1 {
		converted = s.layoutConverter.Convert(text, true)
		s.logDebug("Converting from %s to %s", s.layoutConverter.Layout1.Name, s.layoutConverter.Layout2.Name)
	} else {
		converted = s.layoutConverter.Convert(text, false)
		s.logDebug("Converting from %s to %s", s.layoutConverter.Layout2.Name, s.layoutConverter.Layout1.Name)
	}

	s.logDebug("%s", textDebugSummary("converted text", converted))

	if err := s.clipboard.Write(converted); err != nil {
		s.logError("failed to write to clipboard: %v", err)
		return
	}

	time.Sleep(ClipboardWriteDelayMs * time.Millisecond)

	s.lastConvertedMu.Lock()
	s.lastConvertedPrimary = text
	s.lastConvertedMu.Unlock()

	// Paste with Ctrl+V
	s.logDebug("Emitting Ctrl+V")
	if err := s.vKeyboard.KeyDown(KEY_LEFTCTRL); err != nil {
		s.logError("failed to press left ctrl: %v", err)
		return
	}
	time.Sleep(time.Duration(s.config.Delay) * time.Millisecond)
	if err := s.vKeyboard.PressKey(KEY_V, s.config.Delay); err != nil {
		s.logError("failed to press V key: %v", err)
		return
	}
	if err := s.vKeyboard.KeyUp(KEY_LEFTCTRL); err != nil {
		s.logDebug("failed to release left ctrl after paste: %v", err)
	}

	time.Sleep(time.Duration(s.config.LayoutSwitchDelay) * time.Millisecond)
	s.inputReader.Flush()
	s.logDebug("Selection conversion complete")
}

func (s *Switcher) drainEventChan() {
	for {
		select {
		case <-s.eventChan:
		default:
			s.drainOverflowEvents()
			return
		}
	}
}

// enqueueEvent delivers events without dropping; if the channel is full,
// the event is placed into an overflow queue to be flushed by the main loop.
func (s *Switcher) enqueueEvent(event *InputEvent) {
	select {
	case s.eventChan <- event:
	default:
		s.pushOverflowEvent(event)
	}
}

func (s *Switcher) pushOverflowEvent(event *InputEvent) {
	s.overflowMu.Lock()
	defer s.overflowMu.Unlock()
	s.overflowEvents = append(s.overflowEvents, event)
}

func (s *Switcher) popOverflowEvent() *InputEvent {
	s.overflowMu.Lock()
	defer s.overflowMu.Unlock()
	if len(s.overflowEvents) == 0 {
		return nil
	}
	ev := s.overflowEvents[0]
	s.overflowEvents[0] = nil
	s.overflowEvents = s.overflowEvents[1:]
	return ev
}

func (s *Switcher) flushOverflowEvents() {
	for {
		ev := s.popOverflowEvent()
		if ev == nil {
			return
		}
		s.handleIncomingEvent(ev)
	}
}

func (s *Switcher) drainOverflowEvents() {
	s.overflowMu.Lock()
	s.overflowEvents = s.overflowEvents[:0]
	s.overflowMu.Unlock()
}

func (s *Switcher) handleIncomingEvent(event *InputEvent) {
	if event != nil && !s.inConversion.Load() {
		s.processKeyEvent(event)
	}
}

func (s *Switcher) getVirtualKeyboardUID() string {
	return fmt.Sprintf("%04x:%04x:%04x:%04x:%016x",
		BUS_USB, 0x0777, 0x0777, 0, hashString("gswitch virtual input device"))
}

func hashString(s string) uint64 {
	h := uint64(FNV1aOffset)
	for i := range len(s) {
		h ^= uint64(s[i])
		h *= FNV1aPrime
	}
	return h
}

func (s *Switcher) Close() {
	if s.cancel != nil {
		s.cancel()
	}
	s.stopSignalHandler()
	if s.logger != nil {
		s.logger.Close()
	}
	if s.inputReader != nil {
		s.inputReader.Close()
	}
	if s.clipboard != nil {
		s.clipboard.Close()
	}
}

// WaitConfig holds configuration for waitForSession timing.
// Used for testing with shorter intervals.
type WaitConfig struct {
	InitialInterval time.Duration
	MaxInterval     time.Duration
	LogInterval     time.Duration
}

// DefaultWaitConfig returns the default production configuration.
func DefaultWaitConfig() WaitConfig {
	return WaitConfig{
		InitialInterval: 5 * time.Second,
		MaxInterval:     60 * time.Second,
		LogInterval:     time.Minute,
	}
}

// waitForSession waits for an active graphical user session to appear.
// It uses exponential backoff and logs waiting status periodically (~once per minute).
// Returns the SessionEnv when found, or ctx.Err() if context is canceled.
func (s *Switcher) waitForSession(ctx context.Context) (*SessionEnv, error) {
	return s.waitForSessionWithConfig(ctx, DefaultWaitConfig())
}

// waitForSessionWithConfig is the configurable version of waitForSession.
// Allows custom timing for testing.
func (s *Switcher) waitForSessionWithConfig(ctx context.Context, cfg WaitConfig) (*SessionEnv, error) {
	interval := cfg.InitialInterval
	attempt := 0
	lastLog := time.Time{} // zero value, will trigger first log

	for {
		env, err := getActiveSessionEnv()
		if err == nil {
			// Guard against unexpected nil env
			if env == nil {
				return nil, errors.New("GetActiveSessionEnv returned nil env without error")
			}
			s.log("Found active session for user %s", env.User)
			return env, nil
		}

		if !errors.Is(err, ErrNoActiveSession) {
			return nil, err // Real error (e.g., loginctl failed)
		}

		attempt++
		now := time.Now()
		if attempt == 1 {
			s.log("No active user session yet, waiting...")
			lastLog = now
		} else if now.Sub(lastLog) >= cfg.LogInterval {
			s.log("Still waiting for user session (attempt %d)...", attempt)
			lastLog = now
		}

		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
			if interval < cfg.MaxInterval {
				interval = min(interval*2, cfg.MaxInterval)
			}
		}
	}
}
