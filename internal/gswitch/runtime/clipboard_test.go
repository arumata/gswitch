package runtime

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestWriteWaylandDoesNotWaitForDaemonizedStderr(t *testing.T) {
	dir := t.TempDir()
	wlCopy := filepath.Join(dir, "wl-copy")
	script := "#!/bin/sh\ncat >/dev/null\n(sleep 2) &\n"
	if err := os.WriteFile(wlCopy, []byte(script), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(wlCopy, 0o700); err != nil { //nolint:gosec // Private executable test fixture.
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	clipboard := &Clipboard{useWayland: true}
	started := time.Now()
	if err := clipboard.writeWayland("converted text"); err != nil {
		t.Fatalf("writeWayland() error = %v", err)
	}
	if elapsed := time.Since(started); elapsed > 500*time.Millisecond {
		t.Fatalf("writeWayland() blocked for %v on daemonized stderr", elapsed)
	}
}

func TestClearPrimarySelectionWayland(t *testing.T) {
	dir := t.TempDir()
	argsFile := filepath.Join(dir, "args")
	wlCopy := filepath.Join(dir, "wl-copy")
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" >\"$GSWITCH_TEST_ARGS\"\n"
	if err := os.WriteFile(wlCopy, []byte(script), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(wlCopy, 0o700); err != nil { //nolint:gosec // Private executable test fixture.
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("GSWITCH_TEST_ARGS", argsFile)

	clipboard := &Clipboard{useWayland: true}
	if err := clipboard.ClearPrimarySelection(); err != nil {
		t.Fatalf("ClearPrimarySelection() error = %v", err)
	}

	args, err := os.ReadFile(argsFile) //nolint:gosec // Private test fixture path.
	if err != nil {
		t.Fatal(err)
	}
	if got, want := strings.Fields(string(args)), []string{"--primary", "--clear"}; !slices.Equal(got, want) {
		t.Fatalf("wl-copy args = %q, want %q", got, want)
	}
}
