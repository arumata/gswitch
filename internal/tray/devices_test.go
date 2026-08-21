package tray

import (
	"os"
	"path/filepath"
	"testing"
)

// writeSysfsDevice creates a fake /sys/class/input/eventX layout in dir.
func writeSysfsDevice(t *testing.T, root, event string, files map[string]string) {
	t.Helper()
	base := filepath.Join(root, event, "device")
	if err := os.MkdirAll(filepath.Join(base, "id"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(base, "capabilities"), 0o750); err != nil {
		t.Fatal(err)
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(base, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

// keyboardSysfsFiles returns sysfs files describing a normal keyboard.
func keyboardSysfsFiles(name string) map[string]string {
	return map[string]string{
		"name":             name + "\n",
		"id/bustype":       "0003\n",
		"id/vendor":        "0db0\n",
		"id/product":       "0b7a\n",
		"id/version":       "0111\n",
		"capabilities/ev":  "120013\n",
		"capabilities/key": "1000000000007 ff800000000007ff febeffdfffefffff fffffffffffffffe\n",
	}
}

func TestGetKeyboardsFromSysfs(t *testing.T) {
	root := t.TempDir()
	writeSysfsDevice(t, root, "event0", keyboardSysfsFiles("Test Keyboard"))

	// A mouse: EV_KEY present (buttons) but no KEY_A
	mouse := keyboardSysfsFiles("Test Mouse")
	mouse["capabilities/key"] = "1f0000 0 0 0\n"
	writeSysfsDevice(t, root, "event1", mouse)

	// A power button: no EV_KEY... actually has EV_KEY only; no KEY_A
	power := keyboardSysfsFiles("Power Button")
	power["capabilities/ev"] = "3\n"
	power["capabilities/key"] = "10000000000000 0\n"
	writeSysfsDevice(t, root, "event2", power)

	// gswitch's own virtual keyboard must be filtered out
	virtual := keyboardSysfsFiles("gswitch virtual input device")
	virtual["id/vendor"] = "0777\n"
	virtual["id/product"] = "0777\n"
	writeSysfsDevice(t, root, "event3", virtual)

	// Same physical device, second event node - must deduplicate
	writeSysfsDevice(t, root, "event4", keyboardSysfsFiles("Test Keyboard"))

	m := NewDeviceManager()
	m.sysDir = root

	keyboards, err := m.GetKeyboards()
	if err != nil {
		t.Fatalf("GetKeyboards() error: %v", err)
	}
	if len(keyboards) != 1 {
		t.Fatalf("GetKeyboards() = %d devices %+v, want 1", len(keyboards), keyboards)
	}
	kb := keyboards[0]
	if kb.Name != "Test Keyboard" {
		t.Errorf("Name = %q, want Test Keyboard", kb.Name)
	}
	// UID format: bustype:vendor:product:version:fnv64a(name)
	if want := "0003:0db0:0b7a:0111:"; kb.UID[:len(want)] != want {
		t.Errorf("UID = %q, want prefix %q", kb.UID, want)
	}
	if kb.Path != "/dev/input/event0" {
		t.Errorf("Path = %q, want /dev/input/event0", kb.Path)
	}
}

func TestSysfsUIDMatchesRuntimeFormat(t *testing.T) {
	// The MSI GK71 keyboard from real hardware: UID must match the one
	// input_reader.go generates (verified against a live system).
	root := t.TempDir()
	files := keyboardSysfsFiles("Micro-Star INT'L CO., LTD. MSI GK71 Sonic Gaming Keyboard")
	writeSysfsDevice(t, root, "event0", files)

	m := NewDeviceManager()
	m.sysDir = root

	keyboards, err := m.GetKeyboards()
	if err != nil {
		t.Fatalf("GetKeyboards() error: %v", err)
	}
	if len(keyboards) != 1 {
		t.Fatalf("got %d devices, want 1", len(keyboards))
	}
	if keyboards[0].UID != "0003:0db0:0b7a:0111:5bf926b1bb389d63" {
		t.Errorf("UID = %q, want 0003:0db0:0b7a:0111:5bf926b1bb389d63", keyboards[0].UID)
	}
}

func TestParseHexBitmapKeyA(t *testing.T) {
	tests := []struct {
		name string
		key  string
		want bool
	}{
		{"real keyboard", "1000000000007 ff800000000007ff febeffdfffefffff fffffffffffffffe", true},
		{"mouse buttons only", "1f0000 0 0 0", false},
		{"single word with KEY_A", "40000000", true},
		{"empty", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := hexBitmapHasBit(tt.key, 30); got != tt.want {
				t.Errorf("hexBitmapHasBit(%q, 30) = %v, want %v", tt.key, got, tt.want)
			}
		})
	}
}
