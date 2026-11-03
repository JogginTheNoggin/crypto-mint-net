package cli

import (
	"bytes"
	"context"
	"os"
	"slices"
	"strings"
	"testing"
)

func TestComplete(t *testing.T) {
	root := t.TempDir()
	t.Setenv("POO_HOME", root)
	cli(t, nil, "init", "-example", "-root", root)
	for _, c := range []struct {
		words []string
		want  []string
	}{
		{[]string{"sta"}, []string{"status"}},
		{[]string{"comp"}, []string{"completion"}},
		{[]string{"completion", ""}, []string{"bash", "zsh", "fish"}},
		{[]string{"help", "led"}, []string{"ledger"}},
		{[]string{"policy", "un"}, []string{"unarchive"}},
		{[]string{"peer", "bl"}, []string{"block"}},
		{[]string{"series", ""}, []string{"example", "new", "info"}},
		{[]string{"wallet", "pa"}, []string{"passwd", "pay"}},
		{[]string{"sim", "a"}, []string{"attack"}},
		{[]string{"ledger", "-po"}, []string{"-policy", "-pool"}},
		{[]string{"wallet", "pay", "-am"}, []string{"-amount"}},
		{[]string{"status", "-policy", ""}, []string{"default"}},
		{[]string{"ledger", "-pool", ""}, nil}, // a value nothing knows
		{[]string{"status", "-o"}, []string{"-o"}},
	} {
		got := complete(c.words[:len(c.words)-1], c.words[len(c.words)-1])
		if !slices.Equal(got, c.want) {
			t.Errorf("complete %q: %q, want %q", c.words, got, c.want)
		}
	}
}

// TestProbeOnlyReads runs every command and subcommand with -h under the
// probe, as completion does, and checks none wrote a file or failed to
// return.
func TestProbeOnlyReads(t *testing.T) {
	root, cwd := t.TempDir(), t.TempDir()
	t.Setenv("POO_HOME", root)
	t.Setenv("HOME", root)
	t.Chdir(cwd)
	n := 0
	for _, c := range commandNames() {
		paths := [][]string{{c}}
		for _, s := range subcommands(c) {
			paths = append(paths, []string{c, s})
		}
		for _, p := range paths {
			if len(commandFlags(p)) > 0 {
				n++
			}
		}
	}
	if n < 30 {
		t.Errorf("only %d commands showed flags to the probe", n)
	}
	for _, d := range []string{root, cwd} {
		if es, _ := os.ReadDir(d); len(es) != 0 {
			t.Errorf("the probe wrote into %s: %v", d, es)
		}
	}
}

func TestFlagErrorsGoToErr(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := Main(context.Background(), []string{"work", "-nope"}, Stdio{Out: &out, Err: &errOut}); code != 1 {
		t.Fatalf("code %d", code)
	}
	if !strings.Contains(errOut.String(), "Usage of work") || !strings.Contains(errOut.String(), "-tries") {
		t.Fatalf("flag usage did not reach Err:\n%s", errOut.String())
	}
}

func TestCompletionScripts(t *testing.T) {
	for _, sh := range []string{"bash", "zsh", "fish"} {
		var b bytes.Buffer
		if err := run([]string{"completion", sh}, &b); err != nil || !strings.Contains(b.String(), "poo __complete") {
			t.Errorf("%s: %v\n%s", sh, err, b.String())
		}
	}
	if err := run([]string{"completion", "tcsh"}, &bytes.Buffer{}); err == nil {
		t.Error("tcsh")
	}
}

// The one line poo help gives a command with its own usage names every
// subcommand that usage lists: "poo grind" left out schedule, and
// completion, which read that line, did not offer it (GREEN-044).
func TestHelpLineNamesEverySubcommand(t *testing.T) {
	for _, cmd := range []string{"grind", "sim"} {
		var line string
		for _, l := range strings.Split(usage, "\n") {
			if rest, ok := strings.CutPrefix(l, "  poo "+cmd+" "); ok {
				line, _, _ = strings.Cut(rest, " ")
			}
		}
		names := strings.Split(line, "|")
		for _, sub := range subcommands(cmd) {
			if sub != "help" && !slices.Contains(names, sub) {
				t.Errorf("poo help: the %s line %q leaves out %s", cmd, line, sub)
			}
		}
	}
	if got := complete([]string{"grind"}, "sch"); !slices.Equal(got, []string{"schedule"}) {
		t.Errorf("complete grind sch: %q", got)
	}
}

// "poo CMD help", "-h" and "--help" print a command's usage lines and
// exit 0 for every command made of subcommands, as poo help CMD does
// (GREEN-045). Before, policy took "-h" for a NAME, series for a
// subcommand, and settings, peer, submit and completion failed.
func TestSubcommandHelpWords(t *testing.T) {
	for _, cmd := range commandNames() {
		if len(subcommands(cmd)) == 0 {
			continue
		}
		var want bytes.Buffer
		if err := helpCmd([]string{cmd}, &want); err != nil {
			t.Fatal(err)
		}
		for _, w := range []string{"help", "-h", "--help"} {
			var out bytes.Buffer
			if err := Run(context.Background(), []string{cmd, w}, Stdio{Out: &out, Err: &out}); err != nil {
				t.Errorf("poo %s %s: %v\n%s", cmd, w, err, out.String())
				continue
			}
			if runsBare[cmd] && w != "help" {
				// Its usage lines, then its own flags (D157).
				if !strings.HasPrefix(out.String(), want.String()) || !strings.Contains(out.String(), "Usage of "+cmd) {
					t.Errorf("poo %s %s printed\n%s", cmd, w, out.String())
				}
				continue
			}
			if !ownHelp[cmd] && out.String() != want.String() {
				t.Errorf("poo %s %s printed\n%s\nwant poo help %s:\n%s", cmd, w, out.String(), cmd, want.String())
			}
		}
	}
}
