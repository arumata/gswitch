package tray

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestEnqueueUINonBlockingWhenQueueFull(t *testing.T) {
	tray := &Tray{
		stopChan: make(chan struct{}),
		uiOps:    make(chan func(), 1),
	}
	tray.uiOps <- func() {}

	start := time.Now()
	ok := tray.enqueueUI(func() {})
	elapsed := time.Since(start)

	if ok {
		t.Fatal("enqueueUI() returned true with full queue, want false")
	}
	if elapsed > 50*time.Millisecond {
		t.Fatalf("enqueueUI() blocked for %v with full queue", elapsed)
	}
}

func TestServiceRequestDedup(t *testing.T) {
	tray := &Tray{
		stopChan:   make(chan struct{}),
		serviceOps: make(chan serviceRequest, 8),
	}

	tray.requestServiceStatusRefresh()
	tray.requestServiceStatusRefresh()
	if got := len(tray.serviceOps); got != 1 {
		t.Fatalf("status refresh requests queued = %d, want 1", got)
	}

	// Drain queued refresh and clear pending flag as worker would do.
	req := <-tray.serviceOps
	if req.kind != serviceRequestRefresh {
		t.Fatalf("queued request kind = %v, want refresh", req.kind)
	}
	tray.statusRefreshPending.Store(false)

	tray.requestServiceToggle()
	tray.requestServiceToggle()
	if got := len(tray.serviceOps); got != 1 {
		t.Fatalf("toggle requests queued = %d, want 1", got)
	}
	req = <-tray.serviceOps
	if req.kind != serviceRequestToggle {
		t.Fatalf("queued request kind = %v, want toggle", req.kind)
	}
}

func TestUpdateLayoutLatestWins(t *testing.T) {
	tray := &Tray{
		stopChan:      make(chan struct{}),
		uiOps:         make(chan func(), 1),
		detectionInfo: DetectionInfo{Status: TrayStatusNeedsConfig},
	}

	tray.UpdateLayout(LayoutInfo{ShortCode: "US", LongName: "English (US)"})
	tray.UpdateLayout(LayoutInfo{ShortCode: "RU", LongName: "Russian"})

	fn := <-tray.uiOps
	if fn == nil {
		t.Fatal("queued UI operation is nil")
	}
	fn()

	if tray.currentCode != "RU" {
		t.Fatalf("currentCode = %q, want %q", tray.currentCode, "RU")
	}
}

func TestUpdateLayoutDuringFlushPending(t *testing.T) {
	entered := make(chan struct{}, 1)
	release := make(chan struct{})
	restore := setBeforeApplyLayoutHookForTest(func() {
		select {
		case entered <- struct{}{}:
		default:
		}
		<-release
	})
	defer restore()

	tray := &Tray{
		stopChan:      make(chan struct{}),
		uiOps:         make(chan func(), 1),
		detectionInfo: DetectionInfo{Status: TrayStatusNeedsConfig},
	}

	tray.UpdateLayout(LayoutInfo{ShortCode: "US", LongName: "English (US)"})
	fn := <-tray.uiOps
	flushDone := make(chan struct{})
	go func() {
		defer close(flushDone)
		fn()
	}()

	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("flushPendingLayout did not reach apply barrier")
	}

	// Deterministic update while flushPendingLayout is in progress.
	tray.UpdateLayout(LayoutInfo{ShortCode: "RU", LongName: "Russian"})
	close(release)

	select {
	case <-flushDone:
	case <-time.After(2 * time.Second):
		t.Fatal("flushPendingLayout did not complete")
	}

	deadline := time.After(2 * time.Second)
	for {
		select {
		case op := <-tray.uiOps:
			if op != nil {
				op()
			}
		default:
			goto drained
		case <-deadline:
			t.Fatal("timed out draining uiOps")
		}
	}

drained:
	if tray.currentCode != "RU" {
		t.Fatalf("currentCode = %q, want %q", tray.currentCode, "RU")
	}
	if got := len(tray.uiOps); got != 0 {
		t.Fatalf("uiOps len = %d, want 0", got)
	}
	tray.layoutQueueMu.Lock()
	defer tray.layoutQueueMu.Unlock()
	if tray.pendingLayout != nil {
		t.Fatal("pendingLayout should be nil after drain")
	}
	if tray.layoutUpdateQueued {
		t.Fatal("layoutUpdateQueued should be false after drain")
	}
}

func TestUpdateLayoutDuringFlushPendingRepeated(t *testing.T) {
	for i := range 200 {
		t.Run(fmt.Sprintf("iter-%d", i), func(t *testing.T) {
			entered := make(chan struct{}, 1)
			release := make(chan struct{})
			restore := setBeforeApplyLayoutHookForTest(func() {
				select {
				case entered <- struct{}{}:
				default:
				}
				<-release
			})
			defer restore()

			tray := &Tray{
				stopChan:      make(chan struct{}),
				uiOps:         make(chan func(), 1),
				detectionInfo: DetectionInfo{Status: TrayStatusNeedsConfig},
			}

			tray.UpdateLayout(LayoutInfo{ShortCode: "US", LongName: "English (US)"})
			fn := <-tray.uiOps
			flushDone := make(chan struct{})
			go func() {
				defer close(flushDone)
				fn()
			}()

			select {
			case <-entered:
			case <-time.After(2 * time.Second):
				t.Fatal("flushPendingLayout did not reach apply barrier")
			}

			tray.UpdateLayout(LayoutInfo{ShortCode: "RU", LongName: "Russian"})
			close(release)

			select {
			case <-flushDone:
			case <-time.After(2 * time.Second):
				t.Fatal("flushPendingLayout did not complete")
			}

			for {
				select {
				case op := <-tray.uiOps:
					if op != nil {
						op()
					}
				default:
					goto drained
				}
			}

		drained:
			if tray.currentCode != "RU" {
				t.Fatalf("currentCode = %q, want %q", tray.currentCode, "RU")
			}
		})
	}
}

func TestUpdateLayoutStressConcurrent(t *testing.T) {
	tray := &Tray{
		stopChan:      make(chan struct{}),
		uiOps:         make(chan func(), 64),
		detectionInfo: DetectionInfo{Status: TrayStatusNeedsConfig},
	}

	workerStop := make(chan struct{})
	var workerWG sync.WaitGroup
	workerWG.Go(func() {
		for {
			select {
			case fn := <-tray.uiOps:
				if fn != nil {
					fn()
				}
			case <-workerStop:
				return
			}
		}
	})

	const producers = 8
	const updatesPerProducer = 1500

	var producersWG sync.WaitGroup
	for p := range producers {
		producersWG.Add(1)
		go func(id int) {
			defer producersWG.Done()
			for i := range updatesPerProducer {
				code := fmt.Sprintf("P%d-%d", id, i)
				tray.UpdateLayout(LayoutInfo{ShortCode: code, LongName: code})
			}
		}(p)
	}
	producersWG.Wait()

	finalLayout := LayoutInfo{ShortCode: "FINAL", LongName: "Final"}
	tray.UpdateLayout(finalLayout)

	barrier := make(chan struct{})
	tray.enqueueUIReliable(func() {
		close(barrier)
	})

	select {
	case <-barrier:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for UI queue drain barrier")
	}

	close(workerStop)
	workerWG.Wait()

	if tray.currentCode != finalLayout.ShortCode {
		t.Fatalf("currentCode = %q, want %q", tray.currentCode, finalLayout.ShortCode)
	}
}
