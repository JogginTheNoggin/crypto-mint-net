package shell

import (
	"bytes"
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/cli"
)

// TestRedact is GUI-006: a seed on a command line is hidden wherever the
// GUI shows the line.
func TestRedact(t *testing.T) {
	seed := strings.Repeat("ab", 32)
	cases := []struct {
		args, want []string
		redacted   bool
	}{
		{[]string{"wallet", "create", "-restore", seed, "-wallet", "w.json"}, []string{"wallet", "create", "-restore", Hidden, "-wallet", "w.json"}, true},
		{[]string{"keygen", "--seed=" + seed}, []string{"keygen", "--seed=" + Hidden}, true},
		{[]string{"-root", "/a", "keygen", "-seed", seed, "-alg", "ed25519"}, []string{"-root", "/a", "keygen", "-seed", Hidden, "-alg", "ed25519"}, true},
		{[]string{"keygen", "-seed-file", "s.hex"}, []string{"keygen", "-seed-file", "s.hex"}, false},
		{[]string{"sim", "-seed", "7"}, []string{"sim", "-seed", "7"}, false},
		{[]string{"status"}, []string{"status"}, false},
		{[]string{"keygen", "-seed"}, []string{"keygen", "-seed"}, true},
	}
	for _, c := range cases {
		orig := append([]string(nil), c.args...)
		got, red := Redact(c.args)
		if !reflect.DeepEqual(got, c.want) || red != c.redacted {
			t.Errorf("Redact(%q) = %q, %v; want %q, %v", c.args, got, red, c.want, c.redacted)
		}
		if !reflect.DeepEqual(c.args, orig) {
			t.Errorf("Redact changed its argument %q", orig)
		}
	}
}

// TestSecretFlagsExist fails if poo renames a secret flag, so the list
// cannot silently go stale.
func TestSecretFlagsExist(t *testing.T) {
	for _, c := range []struct {
		args []string
		flag string
	}{
		{[]string{"wallet", "create", "-h"}, "-restore"},
		{[]string{"keygen", "-h"}, "-seed"},
	} {
		var out bytes.Buffer
		cli.Main(context.Background(), c.args, cli.Stdio{Out: &out, Err: &out})
		if !strings.Contains(out.String(), c.flag+" ") && !strings.Contains(out.String(), c.flag+"\n") {
			t.Errorf("poo %s has no %s flag:\n%s", strings.Join(c.args, " "), c.flag, out.String())
		}
	}
}

// TestSecretOutput is GUI-009: the commands that print a seed.
func TestSecretOutput(t *testing.T) {
	for _, c := range []struct {
		args []string
		want bool
	}{
		{[]string{"keygen"}, true},
		{[]string{"keygen", "-alg", "ed25519"}, true},
		{[]string{"wallet", "seed"}, true},
		{[]string{"wallet", "-wallet", "w.json", "seed"}, false}, // -wallet's value reads as a word; poo takes flags after the subcommand
		{[]string{"wallet", "seed", "-wallet", "w.json"}, true},
		{[]string{"wallet", "balance"}, false},
		{[]string{"status"}, false},
		{nil, false},
	} {
		if got := SecretOutput(c.args); got != c.want {
			t.Errorf("SecretOutput(%q) = %v, want %v", c.args, got, c.want)
		}
	}
}
