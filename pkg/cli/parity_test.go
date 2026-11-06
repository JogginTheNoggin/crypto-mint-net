package cli

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// noCommandYet lists the rpc.Client methods that no poon command calls,
// each with why. The GUI reaches the node through the same client, so a
// method missing here and from the commands is something the GUI can do
// that the headless node cannot, which the owner's rule forbids
// (GREEN-038). Remove an entry when a command gains the call.
var noCommandYet = map[string]string{
	"GrindReplaceSchedule": "poon grind schedule sends edits (D125), which cover every replacement",
	"GrindSetSchedule":     "the older form of GrindReplaceSchedule",
	"SubmitEntry":          "poon submit entry sends the same request with SubmitEntryHex, from the hex as read (GUI ORANGE-012)",
}

// TestEveryClientCallHasACommand fails when an rpc.Client method is
// neither called by a poon command nor listed in noCommandYet.
func TestEveryClientCallHasACommand(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("..", "rpc", "client.go"))
	if err != nil {
		t.Fatal(err)
	}
	var src strings.Builder
	files, _ := filepath.Glob("*.go")
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		s, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		src.Write(s)
	}
	used := src.String()
	methods := regexp.MustCompile(`(?m)^func \(c \*Client\) ([A-Z]\w*)\(`).FindAllStringSubmatch(string(b), -1)
	if len(methods) == 0 {
		t.Fatal("no client methods found")
	}
	for _, m := range methods {
		name := m[1]
		called := strings.Contains(used, "."+name+"(")
		why, listed := noCommandYet[name]
		switch {
		case !called && !listed:
			t.Errorf("rpc.Client.%s has no poon command: add one, or list it in noCommandYet with why", name)
		case called && listed:
			t.Errorf("rpc.Client.%s is called by a command now: take it out of noCommandYet (%s)", name, why)
		}
	}
}
