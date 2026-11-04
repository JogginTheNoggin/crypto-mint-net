package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/nodepolicy"
)

// RED-083: the text plan names a declined series and a series with no
// witness ceiling, and does not say "nothing to change" alone when it
// declined one.
func TestPlanTextNamesDeclinedAndUnbounded(t *testing.T) {
	var out bytes.Buffer
	printPlan(&out, importResult{Mode: "merge", Plan: nodepolicy.Plan{
		Declined:  []string{"abcd: declined (its 1 follow rule(s) are left out too)"},
		Unbounded: []string{"ef01: the series sets no witness ceiling"},
	}}, false, false)
	got := out.String()
	for _, want := range []string{"declined: series abcd", "1 follow rule(s)", "series_ceiling=N", "warning: series ef01", "nothing else to change"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	if strings.Contains(got, "  nothing to change") {
		t.Errorf("said nothing to change:\n%s", got)
	}
}
