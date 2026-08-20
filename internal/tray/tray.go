package tray

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"fyne.io/systray"
)

// Tray represents the system tray icon and menu.
type Tray struct {
	app *App

	// Service manager for status updates
	serviceManager *ServiceManager

	// Menu items
	mServiceStatus *systray.MenuItem
	mServiceAction *systray.MenuItem
	mRefresh       *systray.MenuItem
	mSettings      *systray.MenuItem
	mQuit          *systray.MenuItem

	// Current layout code (to avoid unnecessary icon updates)
	currentCode string

	// Current service status (to avoid unnecessary menu updates)
	currentServiceStatus ServiceStatus

	// Current detection info
	detectionInfo DetectionInfo

	layoutQueueMu      sync.Mutex
	pendingLayout      *LayoutInfo
	layoutUpdateQueued bool

	// Service status polling
	stopChan chan struct{}
	stopOnce sync.Once

	// Service worker queue (systemctl/pkexec operations)
	serviceOps chan serviceRequest

	statusRefreshPending atomic.Bool
	togglePending        atomic.Bool

	// UI update queue to serialize systray API calls and state mutations.
	uiOps chan func()
}

type serviceRequestKind int

const (
	serviceRequestRefresh serviceRequestKind = iota
	serviceRequestToggle
)

type serviceRequest struct {
	kind serviceRequestKind
}

var (
	beforeApplyLayoutHookMu sync.RWMutex
	beforeApplyLayoutHook   func()
)

func runBeforeApplyLayoutHook() {
	beforeApplyLayoutHookMu.RLock()
	hook := beforeApplyLayoutHook
	beforeApplyLayoutHookMu.RUnlock()
	if hook != nil {
		hook()
	}
}

func setBeforeApplyLayoutHookForTest(hook func()) func() {
	beforeApplyLayoutHookMu.Lock()
	prev := beforeApplyLayoutHook
	beforeApplyLayoutHook = hook
	beforeApplyLayoutHookMu.Unlock()
	return func() {
		beforeApplyLayoutHookMu.Lock()
		beforeApplyLayoutHook = prev
		beforeApplyLayoutHookMu.Unlock()
	}
}

// NewTray creates a new system tray instance.
func NewTray(app *App) *Tray {
	return &Tray{
		app:            app,
		serviceManager: NewServiceManager("gswitch"),
		stopChan:       make(chan struct{}),
		serviceOps:     make(chan serviceRequest, 8),
		uiOps:          make(chan func(), 64),
	}
}

// onReady is called when systray is ready.
func (t *Tray) onReady() {
	// Set initial icon (will be updated based on detection status and layout)
	systray.SetIcon(kbIcon)
	systray.SetTitle("")
	systray.SetTooltip("gswitch - Layout switcher")

	// Create service status menu items
	t.mServiceStatus = systray.AddMenuItem(strTrayServiceUnknown, "Service status")
	t.mServiceStatus.Disable() // Status is display-only
	t.mServiceAction = systray.AddMenuItem(strButtonStop, "Stop/Start service")
	systray.AddSeparator()

	// Create menu items
	t.mRefresh = systray.AddMenuItem(strTrayRefresh, "Refresh detection status")
	t.mSettings = systray.AddMenuItem("Settings...", "Open settings")
	systray.AddSeparator()
	t.mQuit = systray.AddMenuItem("Quit", "Close application")

	// Handle menu clicks and queued UI updates in a single goroutine.
	go t.handleEvents()

	// Start service worker.
	go t.runServiceWorker()

	// Update service status in menu.
	t.UpdateServiceStatus()

	// Start service status polling.
	go t.pollServiceStatus()

	// Notify app that tray is ready
	t.app.onTrayReady()
}

// onExit is called when systray is exiting.
func (t *Tray) onExit() {
	t.stopOnce.Do(func() {
		close(t.stopChan)
	})
	fmt.Println("Tray application exited")
}

// pollServiceStatus periodically checks service status.
func (t *Tray) pollServiceStatus() {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			t.requestServiceStatusRefresh()
		case <-t.stopChan:
			return
		}
	}
}

func (t *Tray) runServiceWorker() {
	for {
		select {
		case req := <-t.serviceOps:
			t.handleServiceRequest(req)
		case <-t.stopChan:
			return
		}
	}
}

func (t *Tray) handleServiceRequest(req serviceRequest) {
	switch req.kind {
	case serviceRequestRefresh:
		defer t.statusRefreshPending.Store(false)
		status, err := t.serviceManager.GetStatus()
		if err != nil {
			status = StatusUnknown
		}
		t.enqueueUIReliable(func() {
			t.applyServiceStatus(status)
		})
	case serviceRequestToggle:
		defer t.togglePending.Store(false)

		status, err := t.serviceManager.GetStatus()
		if err != nil {
			status = StatusUnknown
		}

		if status == StatusRunning {
			err = t.serviceManager.Stop()
		} else {
			err = t.serviceManager.Start()
		}
		if err != nil && !errors.Is(err, ErrUserCanceled) {
			fmt.Printf("Service action failed: %v\n", err)
		}

		updatedStatus, statusErr := t.serviceManager.GetStatus()
		if statusErr != nil {
			updatedStatus = StatusUnknown
		}
		t.enqueueUIReliable(func() {
			t.applyServiceStatus(updatedStatus)
		})
	}
}

// handleEvents handles menu clicks and queued UI operations.
func (t *Tray) handleEvents() {
	for {
		select {
		case fn := <-t.uiOps:
			if fn != nil {
				fn()
			}
		case <-t.mServiceAction.ClickedCh:
			t.requestServiceToggle()
		case <-t.mRefresh.ClickedCh:
			go t.app.RefreshDetectionStatus()
		case <-t.mSettings.ClickedCh:
			t.app.OnSettingsClicked()
		case <-t.mQuit.ClickedCh:
			t.app.Quit()
		case <-t.stopChan:
			return
		}
	}
}

// enqueueUI schedules a UI update on the tray event loop without blocking callers.
// Returns false if queue is full or tray is stopping.
func (t *Tray) enqueueUI(fn func()) bool {
	if fn == nil {
		return false
	}

	select {
	case <-t.stopChan:
		return false
	case t.uiOps <- fn:
		return true
	default:
		return false
	}
}

// enqueueUIReliable guarantees eventual enqueue while keeping caller non-blocking.
func (t *Tray) enqueueUIReliable(fn func()) {
	if t.enqueueUI(fn) {
		return
	}
	go func() {
		select {
		case <-t.stopChan:
		case t.uiOps <- fn:
		}
	}()
}

func (t *Tray) enqueueServiceRequest(req serviceRequest) bool {
	select {
	case <-t.stopChan:
		return false
	case t.serviceOps <- req:
		return true
	default:
		return false
	}
}

func (t *Tray) requestServiceStatusRefresh() {
	if !t.statusRefreshPending.CompareAndSwap(false, true) {
		return
	}
	if !t.enqueueServiceRequest(serviceRequest{kind: serviceRequestRefresh}) {
		t.statusRefreshPending.Store(false)
	}
}

func (t *Tray) requestServiceToggle() {
	if !t.togglePending.CompareAndSwap(false, true) {
		return
	}
	if !t.enqueueServiceRequest(serviceRequest{kind: serviceRequestToggle}) {
		t.togglePending.Store(false)
	}
}

// UpdateServiceStatus updates the service status menu items.
func (t *Tray) UpdateServiceStatus() {
	t.requestServiceStatusRefresh()
}

func (t *Tray) applyServiceStatus(status ServiceStatus) {
	if t.mServiceStatus == nil || t.mServiceAction == nil {
		return
	}
	if status == t.currentServiceStatus {
		return
	}
	t.currentServiceStatus = status

	// Update status label and service action button.
	switch status {
	case StatusRunning:
		t.mServiceStatus.SetTitle(strTrayServiceRunning)
		t.mServiceAction.SetTitle(strButtonStop)
		t.mServiceAction.Enable()
	case StatusStopped:
		t.mServiceStatus.SetTitle(strTrayServiceStopped)
		t.mServiceAction.SetTitle(strButtonStart)
		t.mServiceAction.Enable()
	case StatusFailed:
		t.mServiceStatus.SetTitle(strTrayServiceFailed)
		t.mServiceAction.SetTitle(strButtonStart)
		t.mServiceAction.Enable()
	case StatusNotInstalled:
		t.mServiceStatus.SetTitle(strTrayServiceNotInstalled)
		t.mServiceAction.SetTitle(strButtonStart)
		t.mServiceAction.Disable() // Cannot start a non-installed service
	default:
		t.mServiceStatus.SetTitle(strTrayServiceUnknown)
		t.mServiceAction.SetTitle(strButtonStart)
		t.mServiceAction.Disable()
	}
}

// UpdateLayout updates the tray icon and tooltip for the given layout.
func (t *Tray) UpdateLayout(layout LayoutInfo) {
	t.layoutQueueMu.Lock()
	layoutCopy := layout
	t.pendingLayout = &layoutCopy
	if t.layoutUpdateQueued {
		t.layoutQueueMu.Unlock()
		return
	}
	t.layoutUpdateQueued = true
	t.layoutQueueMu.Unlock()

	t.enqueueUIReliable(func() {
		t.flushPendingLayout()
	})
}

// flushPendingLayout applies the most recent layout update.
// If multiple updates arrive before processing, only the latest is applied.
func (t *Tray) flushPendingLayout() {
	for {
		t.layoutQueueMu.Lock()
		pending := t.pendingLayout
		if pending == nil {
			t.layoutUpdateQueued = false
			t.layoutQueueMu.Unlock()
			return
		}
		layout := *pending
		t.pendingLayout = nil
		t.layoutQueueMu.Unlock()

		runBeforeApplyLayoutHook()
		t.applyLayout(layout)
	}
}

func (t *Tray) applyLayout(layout LayoutInfo) {
	// Skip if layout hasn't changed
	if t.currentCode == layout.ShortCode {
		return
	}
	t.currentCode = layout.ShortCode

	// Don't change icon if in warning/error state
	if t.detectionInfo.Status != TrayStatusOK {
		return
	}

	// Use cached icon to avoid regeneration
	icon := GetLayoutIcon(layout.ShortCode)
	systray.SetIcon(icon)

	// Build tooltip: layout info + detection keybinding (if available)
	tooltip := layout.ShortCode + " - " + layout.LongName
	if t.detectionInfo.KeyNames != "" {
		tooltip += "\n" + t.detectionInfo.KeyNames
		if t.detectionInfo.Source != "" {
			tooltip += " (" + t.detectionInfo.Source + ")"
		}
	}
	systray.SetTooltip(tooltip)
}

// UpdateDetectionStatus updates the tray icon and tooltip based on detection status.
func (t *Tray) UpdateDetectionStatus(info DetectionInfo) {
	t.enqueueUIReliable(func() {
		t.applyDetectionStatus(info)
	})
}

func (t *Tray) applyDetectionStatus(info DetectionInfo) {
	t.detectionInfo = info

	switch info.Status {
	case TrayStatusOK:
		// Normal operation - restore layout icon if available, otherwise use KB icon
		// Reset currentCode to force UpdateLayout to set proper icon
		t.currentCode = ""
		systray.SetIcon(kbIcon) // Default, will be updated by UpdateLayout if called
		switch {
		case info.KeyNames != "" && info.Source != "":
			tooltip := fmt.Sprintf(strTooltipOK, info.KeyNames, info.Source)
			systray.SetTitle(tooltip) // KDE shows Title as tooltip
			systray.SetTooltip(tooltip)
		case info.KeyNames != "":
			tooltip := "gswitch: " + info.KeyNames
			systray.SetTitle(tooltip)
			systray.SetTooltip(tooltip)
		default:
			systray.SetTitle("gswitch")
			systray.SetTooltip("gswitch - Layout switcher")
		}

	case TrayStatusNeedsConfig:
		// Warning - show yellow icon
		systray.SetIcon(GetWarningIcon())
		systray.SetTitle(strTooltipNeedsConfig)
		systray.SetTooltip(strTooltipNeedsConfig + "\n" + strTooltipClickConfig)

	case TrayStatusServiceError:
		// Service error - show red icon
		systray.SetIcon(GetErrorIcon())
		if info.Error != "" {
			tooltip := "gswitch: " + info.Error
			systray.SetTitle(tooltip)
			systray.SetTooltip(tooltip)
		} else {
			systray.SetTitle(strTooltipServiceError)
			systray.SetTooltip(strTooltipServiceError)
		}

	case TrayStatusDetectError:
		// Detection error - show red icon
		systray.SetIcon(GetErrorIcon())
		if info.Error != "" {
			tooltip := "gswitch: " + info.Error
			systray.SetTitle(tooltip)
			systray.SetTooltip(tooltip)
		} else {
			systray.SetTitle(strTooltipDetectError)
			systray.SetTooltip(strTooltipDetectError)
		}
	}
}

// GetServiceManager returns the service manager instance.
func (t *Tray) GetServiceManager() *ServiceManager {
	return t.serviceManager
}
