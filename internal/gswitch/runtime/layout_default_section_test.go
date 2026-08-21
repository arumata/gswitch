package runtime

import "testing"

// TestLoadLayoutDefaultSection checks that layouts whose default xkb_symbols
// section is not named "basic" (like ua's "unicode") still load a usable
// conversion table.
func TestLoadLayoutDefaultSection(t *testing.T) {
	l1, err := LoadLayout("us", "")
	if err != nil {
		t.Skipf("XKB symbols not available: %v", err)
	}
	l2, err := LoadLayout("ua", "")
	if err != nil {
		t.Skipf("XKB symbols not available: %v", err)
	}
	lc := NewLayoutConverter(l1, l2)

	if !lc.DetectLayout("ghbdsn") {
		t.Error("DetectLayout(ghbdsn) = false, want true (us text)")
	}
	if got := lc.Convert("ghbdsn", true); got != "привіт" {
		t.Errorf("us->ua Convert(ghbdsn) = %q, want привіт", got)
	}
	if got := lc.Convert("привіт", false); got != "ghbdsn" {
		t.Errorf("ua->us Convert(привіт) = %q, want ghbdsn", got)
	}
}

// TestLoadLayoutRuDefault checks ru, whose default section is "winkeys" and
// which has no "basic" section at all (used to be a special case in the parser).
func TestLoadLayoutRuDefault(t *testing.T) {
	l1, err := LoadLayout("us", "")
	if err != nil {
		t.Skipf("XKB symbols not available: %v", err)
	}
	l2, err := LoadLayout("ru", "")
	if err != nil {
		t.Skipf("XKB symbols not available: %v", err)
	}
	lc := NewLayoutConverter(l1, l2)

	if got := lc.Convert("ghbdtn", true); got != "привет" {
		t.Errorf("us->ru Convert(ghbdtn) = %q, want привет", got)
	}
	if got := lc.Convert("привет", false); got != "ghbdtn" {
		t.Errorf("ru->us Convert(привет) = %q, want ghbdtn", got)
	}
}
