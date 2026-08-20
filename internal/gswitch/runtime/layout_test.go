package runtime

import (
	"reflect"
	"strings"
	"testing"
)

func TestStringsToSpecs_NoVariantSplit(t *testing.T) {
	input := []string{"us", "ru", "gb-extd"}
	want := []LayoutSpec{
		{Name: "us"},
		{Name: "ru"},
		{Name: "gb-extd"},
	}

	got := stringsToSpecs(input)

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("stringsToSpecs() = %+v, want %+v", got, want)
	}
}

func TestGetLayoutsFromXkb_AlignVariants(t *testing.T) {
	// Simulate parsed output of setxkbmap -query:
	// layout:  us,ru
	// variant: ,phonetic
	layouts := stringsToSpecs([]string{"us", "ru"})
	variants := []string{"", "phonetic"}

	for i := range layouts {
		if i < len(variants) {
			layouts[i].Variant = variants[i]
		}
	}

	if layouts[0].Variant != "" {
		t.Fatalf("expected empty variant for first layout, got %q", layouts[0].Variant)
	}
	if layouts[1].Variant != "phonetic" {
		t.Fatalf("expected variant 'phonetic' for second layout, got %q", layouts[1].Variant)
	}
}

func TestParseIbusEngines_EmptyVariant(t *testing.T) {
	out := "['xkb:us::eng', 'xkb:ru:phonetic:rus']"
	got := parseIbusEngines(out)

	want := []LayoutSpec{
		{Name: "us", Variant: ""},
		{Name: "ru", Variant: "phonetic"},
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseIbusEngines() = %+v, want %+v", got, want)
	}
}

func TestParseIbusEngines_DedupAndVariant(t *testing.T) {
	out := "['xkb:us::eng', 'xkb:us::eng', 'xkb:gb:extd:eng', 'xkb:ru::rus']"
	got := parseIbusEngines(out)

	want := []LayoutSpec{
		{Name: "us", Variant: ""},
		{Name: "gb", Variant: "extd"},
		{Name: "ru", Variant: ""},
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseIbusEngines() dedup/variant = %+v, want %+v", got, want)
	}
}

func TestParseIbusEngines_IgnoreNonXkb(t *testing.T) {
	out := "['mozc-jp', 'xkb:us::eng']"
	got := parseIbusEngines(out)
	want := []LayoutSpec{{Name: "us", Variant: ""}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseIbusEngines() ignore non-xkb = %+v, want %+v", got, want)
	}
}

func TestParseIbusEngines_EmptyList(t *testing.T) {
	out := "[]"
	got := parseIbusEngines(out)
	if len(got) != 0 {
		t.Fatalf("expected empty slice for empty ibus list, got %+v", got)
	}
}

func TestSplitLayoutVariant(t *testing.T) {
	layout, variant := splitLayoutVariant("us")
	if layout != "us" || variant != "" {
		t.Fatalf("splitLayoutVariant('us') = %s, %s; want us, ''", layout, variant)
	}

	layout, variant = splitLayoutVariant("ru-phonetic")
	if layout != "ru" || variant != "phonetic" {
		t.Fatalf("splitLayoutVariant('ru-phonetic') = %s, %s; want ru, phonetic", layout, variant)
	}
}

func TestSplitLayoutVariant_MultiHyphen(t *testing.T) {
	layout, variant := splitLayoutVariant("gb-mac-intl")
	if layout != "gb" || variant != "mac-intl" {
		t.Fatalf("splitLayoutVariant('gb-mac-intl') = %s, %s; want gb, mac-intl", layout, variant)
	}
}

func TestSplitLayoutVariant_Parentheses(t *testing.T) {
	layout, variant := splitLayoutVariant("ua(unicode)")
	if layout != "ua" || variant != "unicode" {
		t.Fatalf("splitLayoutVariant('ua(unicode)') = %s, %s; want ua, unicode", layout, variant)
	}

	layout, variant = splitLayoutVariant("ru(phonetic)")
	if layout != "ru" || variant != "phonetic" {
		t.Fatalf("splitLayoutVariant('ru(phonetic)') = %s, %s; want ru, phonetic", layout, variant)
	}

	layout, variant = splitLayoutVariant("us")
	if layout != "us" || variant != "" {
		t.Fatalf("splitLayoutVariant('us') = %s, %s; want us, ''", layout, variant)
	}
}

func TestSplitLayoutVariant_WithSpaces(t *testing.T) {
	// Parentheses with spaces
	layout, variant := splitLayoutVariant("ua( unicode )")
	if layout != "ua" || variant != "unicode" {
		t.Fatalf("splitLayoutVariant('ua( unicode )') = %q, %q; want ua, unicode", layout, variant)
	}

	layout, variant = splitLayoutVariant(" ru (phonetic) ")
	if layout != "ru" || variant != "phonetic" {
		t.Fatalf("splitLayoutVariant(' ru (phonetic) ') = %q, %q; want ru, phonetic", layout, variant)
	}

	// Dash with spaces
	layout, variant = splitLayoutVariant("ru - phonetic")
	if layout != "ru" || variant != "phonetic" {
		t.Fatalf("splitLayoutVariant('ru - phonetic') = %q, %q; want ru, phonetic", layout, variant)
	}
}

func TestStringsToSpecs_SkipEmpty(t *testing.T) {
	input := []string{"us", "", "  ", "ru"}
	got := stringsToSpecs(input)
	want := []LayoutSpec{{Name: "us"}, {Name: "ru"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("stringsToSpecs skip empty = %+v, want %+v", got, want)
	}
}

func TestGetLayoutsFromFcitx5_ParsesVariantLine(t *testing.T) {
	// Lines as in fcitx5 profile: Name=keyboard-ru-phonetic
	specs := []LayoutSpec{}
	lines := []string{
		"Name=keyboard-us",
		"Name=keyboard-ru-phonetic",
	}

	for _, line := range lines {
		if after, ok := strings.CutPrefix(line, "Name=keyboard-"); ok {
			layout, variant := splitLayoutVariant(after)
			spec := LayoutSpec{Name: layout, Variant: variant}
			if !containsLayoutSpec(specs, spec) {
				specs = append(specs, spec)
			}
		}
	}

	want := []LayoutSpec{
		{Name: "us", Variant: ""},
		{Name: "ru", Variant: "phonetic"},
	}

	if !reflect.DeepEqual(specs, want) {
		t.Fatalf("fcitx parse = %+v, want %+v", specs, want)
	}
}
