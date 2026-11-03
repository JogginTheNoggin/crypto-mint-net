// Package plain decides which text may reach a terminal as it is
// (RED-053, RED-061, D158). Every check of text from a file or a peer
// uses it, so a label, a setting and a table cell refuse and escape the
// same characters.
package plain

import (
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Unsafe reports whether r must not reach a terminal as it is: a control
// character (Cc: an escape, a newline, a carriage return), a format
// character (Cf: the bidirectional overrides and marks, zero-width spaces
// and joiners, the byte order mark), a line or paragraph separator (Zl,
// Zp), or the replacement for bytes that were not UTF-8.
func Unsafe(r rune) bool {
	return r == utf8.RuneError || unicode.In(r, unicode.Cc, unicode.Cf, unicode.Zl, unicode.Zp)
}

// Text reports whether s is UTF-8 with no rune Unsafe names.
func Text(s string) bool {
	if !utf8.ValidString(s) {
		return false
	}
	for _, r := range s {
		if Unsafe(r) {
			return false
		}
	}
	return true
}

// Safe returns s with every rune Unsafe names as its Go escape (\x1b, \n,
// \u202e, \u200b), and bytes that are not UTF-8 as \x escapes, so text
// can neither move the cursor, erase what was printed, start a line of
// its own, reorder what is shown nor hide in plain sight.
func Safe(s string) string {
	if Text(s) {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); {
		r, n := utf8.DecodeRuneInString(s[i:])
		switch {
		case r == utf8.RuneError && n <= 1:
			b.WriteString(`\x` + strconv.FormatUint(uint64(s[i])|0x100, 16)[1:])
		case Unsafe(r):
			q := strconv.QuoteRune(r)
			b.WriteString(q[1 : len(q)-1])
		default:
			b.WriteString(s[i : i+n])
		}
		i += n
	}
	return b.String()
}
