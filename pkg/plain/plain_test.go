package plain

import "testing"

// RED-061, D158: one check for labels, settings and table cells.
func TestTextAndSafe(t *testing.T) {
	for in, want := range map[string]string{
		"friend":          "friend",
		"gr\u00fc\u00dfe": "gr\u00fc\u00dfe",
		"x\x1b[8A":        `x\x1b[8A`,
		"a\nb\rc":         `a\nb\rc`,
		"a\u202eb":        `a\u202eb`,
		"a\u2028b":        `a\u2028b`,
		"a\u200bb":        `a\u200bb`,
		"\u061c":          `\u061c`,
		"\ufeffa":         `\ufeffa`,
		"bad\xffbyte":     `bad\xffbyte`,
	} {
		if got := Safe(in); got != want {
			t.Errorf("Safe(%q) = %q, want %q", in, got, want)
		}
		if Text(in) != (in == want) {
			t.Errorf("Text(%q) = %v", in, Text(in))
		}
	}
}
