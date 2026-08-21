package tray

import "testing"

func TestParseGnomeSources(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []LayoutInfo
	}{
		{
			name:  "two xkb sources",
			input: "[('xkb', 'us'), ('xkb', 'ru')]",
			want:  []LayoutInfo{{ShortCode: "US", LongName: "us"}, {ShortCode: "RU", LongName: "ru"}},
		},
		{
			name:  "variant after plus",
			input: "[('xkb', 'ua+unicode')]",
			want:  []LayoutInfo{{ShortCode: "UA", LongName: "ua+unicode"}},
		},
		{
			name:  "ibus engine skipped",
			input: "[('ibus', 'mozc-jp'), ('xkb', 'de')]",
			want:  []LayoutInfo{{ShortCode: "DE", LongName: "de"}},
		},
		{
			name:  "empty mru",
			input: "@a(ss) []",
			want:  nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseGnomeSources(tt.input)
			if len(got) != len(tt.want) {
				t.Fatalf("parseGnomeSources() = %+v, want %+v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("layout[%d] = %+v, want %+v", i, got[i], tt.want[i])
				}
			}
		})
	}
}
