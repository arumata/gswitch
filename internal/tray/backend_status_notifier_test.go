package tray

import (
	"reflect"
	"testing"
)

type recordingStatusNotifierRuntime struct {
	calls []string
}

func (r *recordingStatusNotifierRuntime) Run(onReady, onExit func()) {
	r.calls = append(r.calls, "run")
	onReady()
	onExit()
}

func (r *recordingStatusNotifierRuntime) SetTitle(title string) {
	r.calls = append(r.calls, "title:"+title)
}

func TestStatusNotifierBackendSeedsStableIDBeforeRun(t *testing.T) {
	t.Parallel()

	runtime := &recordingStatusNotifierRuntime{}
	ready := false
	exited := false
	backend := statusNotifierBackend{runtime: runtime}

	err := backend.Run(
		func() { ready = true },
		func() { exited = true },
	)

	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	wantCalls := []string{"title:gswitch", "run"}
	if !reflect.DeepEqual(runtime.calls, wantCalls) {
		t.Fatalf("runtime calls = %v, want %v", runtime.calls, wantCalls)
	}
	if !ready || !exited {
		t.Fatalf("callbacks called = ready:%t exit:%t, want both true", ready, exited)
	}
}

func TestStatusNotifierBackendKeepsTitleDynamicAfterRun(t *testing.T) {
	t.Parallel()

	runtime := &recordingStatusNotifierRuntime{}
	backend := statusNotifierBackend{runtime: runtime}

	if err := backend.Run(func() {}, func() {}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	backend.SetTitle("gswitch: CapsLock (xkb)")

	wantCalls := []string{
		"title:gswitch",
		"run",
		"title:gswitch: CapsLock (xkb)",
	}
	if !reflect.DeepEqual(runtime.calls, wantCalls) {
		t.Fatalf("runtime calls = %v, want %v", runtime.calls, wantCalls)
	}
}
