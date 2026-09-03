package tray

import "testing"

func TestSameTrayConfig(t *testing.T) {
	base := &TrayConfig{
		LayoutSwitch:      "56+42",
		ConvertKey:        "0",
		Layout1:           "us",
		Layout2:           "ru",
		Delay:             10,
		LayoutSwitchDelay: 100,
		Blacklist:         []string{"keyboard-a", "keyboard-b"},
	}
	if !sameTrayConfig(base, cloneTrayConfig(base)) {
		t.Fatal("cloned configuration should match")
	}

	reordered := cloneTrayConfig(base)
	reordered.Blacklist = []string{"keyboard-b", "keyboard-a"}
	if !sameTrayConfig(base, reordered) {
		t.Fatal("blacklist order should not require a daemon save")
	}

	changed := cloneTrayConfig(base)
	changed.Delay = 20
	if sameTrayConfig(base, changed) {
		t.Fatal("changed daemon setting should require a save")
	}
}
