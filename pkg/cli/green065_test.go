package cli

import (
	"regexp"
	"strings"
	"testing"
)

// GREEN-065: after the rename to poon, eighteen lines of poon help put
// their description one column right of every other (written for the
// three-letter name), and the zsh completion asked to be saved as _poo.
// Every one-line entry of the usage starts its description in the
// column the continuation lines use, and the completion scripts name
// the tool.
func TestUsageColumnsAndCompletionName(t *testing.T) {
	one := regexp.MustCompile(`^(  poon \S.*?\S)( {2,})(\S.*)$`)
	cont := regexp.MustCompile(`^( +)\S`)
	col := -1
	for _, l := range strings.Split(usage, "\n") {
		if m := cont.FindStringSubmatch(l); m != nil && len(m[1]) >= 20 {
			if col == -1 {
				col = len(m[1])
			} else if len(m[1]) != col {
				t.Errorf("a continuation line starts at %d, not %d: %q", len(m[1]), col, l)
			}
		}
	}
	if col == -1 {
		t.Fatal("no continuation lines in usage")
	}
	for _, l := range strings.Split(usage, "\n") {
		if m := one.FindStringSubmatch(l); m != nil {
			if c := len(m[1]) + len(m[2]); c != col {
				t.Errorf("description at column %d, not %d: %q", c, col, l)
			}
		}
	}
	for name, script := range map[string]string{"bash": bashCompletion, "zsh": zshCompletion, "fish": fishCompletion} {
		if regexp.MustCompile(`\bpoo\b|_poo\b`).MatchString(script) {
			t.Errorf("the %s completion still names poo:\n%s", name, script)
		}
	}
}
