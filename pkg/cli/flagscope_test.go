package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestFlagsForRefusesUnusedFlags(t *testing.T) {
	t.Setenv("POON_HOME", t.TempDir())
	for _, c := range []struct {
		args []string
		want string
	}{
		{[]string{"peer", "unblock", "-range", "203.0.113.5:8646"}, "-range does nothing for unblock; it is for block"},
		{[]string{"peer", "delete", "-seconds", "5", "203.0.113.5:8646"}, "-seconds does nothing for delete; it is for timeout and block"},
		{[]string{"policy", "list", "-fresh"}, "-fresh does nothing for list; it is for unarchive"},
		{[]string{"sim", "load", "-share", "0.4"}, "-share does nothing for load; it is for attack and sizing"},
		{[]string{"sim", "partition", "-duration", "1000"}, "-duration does nothing for partition"},
		{[]string{"sim", "attack", "-split", "10"}, "-split does nothing for attack; it is for partition"},
	} {
		err := run(c.args, &bytes.Buffer{})
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%v: %v, want %q", c.args, err, c.want)
		}
	}
	// A flag for the subcommand still passes the check: the error is the
	// next step's (no running node), not the flag's.
	for _, args := range [][]string{
		{"peer", "block", "-range", "-seconds", "-1", "203.0.113.5:8646"},
		{"peer", "clear", "-what", "flags", "203.0.113.5:8646"},
	} {
		if err := run(args, &bytes.Buffer{}); err == nil || strings.Contains(err.Error(), "does nothing") {
			t.Errorf("%v: %v", args, err)
		}
	}
}
