package plain

import (
	"testing"
	"unicode"
	"unicode/utf8"
)

// The ASCII fast path in Unsafe gives what the tables give (ORANGE-036).
func TestUnsafeASCII(t *testing.T) {
	for r := rune(0); r < utf8.RuneSelf; r++ {
		want := unicode.In(r, unicode.Cc, unicode.Cf, unicode.Zl, unicode.Zp)
		if Unsafe(r) != want {
			t.Errorf("%U: Unsafe %v, tables %v", r, Unsafe(r), want)
		}
	}
}
