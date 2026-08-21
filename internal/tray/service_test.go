package tray

import (
	"slices"
	"testing"
)

func TestServiceManagerUsesUserScope(t *testing.T) {
	sm := NewServiceManager("gswitch.service")
	cmd := sm.command("restart")
	want := []string{"systemctl", "--user", "restart", "gswitch.service"}
	if !slices.Equal(cmd.Args, want) {
		t.Fatalf("command args = %q, want %q", cmd.Args, want)
	}
}

func TestIsUnitNotFoundOutput(t *testing.T) {
	for _, output := range []string{
		"no files found for gswitch.service",
		"unit gswitch.service could not be found",
		"loadstate=not-found",
	} {
		if !isUnitNotFoundOutput(output) {
			t.Errorf("isUnitNotFoundOutput(%q) = false, want true", output)
		}
	}
}
