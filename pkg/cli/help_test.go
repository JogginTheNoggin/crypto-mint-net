package cli

import (
	"bytes"
	"slices"
	"strings"
	"testing"
)

func TestCommandNamesMatchDispatch(t *testing.T) {
	// Every command the usage names must run (not be unknown), so help and
	// suggestions never point at a command the tool lacks.
	for _, n := range commandNames() {
		if n == "help" {
			continue // help -x is help for the command "-x"
		}
		err := run([]string{n, "-no-such-flag"}, &bytes.Buffer{})
		if err != nil && strings.Contains(err.Error(), "unknown command") {
			t.Errorf("usage names %q but run does not know it", n)
		}
	}
	for _, n := range []string{"status", "wallet", "sim", "help", "watch"} {
		if !slices.Contains(commandNames(), n) {
			t.Errorf("commandNames lacks %q", n)
		}
	}
}

func TestSuggest(t *testing.T) {
	for in, want := range map[string]string{
		"stauts": "status", "statsu": "status", "peer": "", "pees": "peers",
		"walet": "wallet", "unfolow": "unfollow", "relaod": "reload", "led": "ledger",
	} {
		got := suggest(in, commandNames())
		if want == "" {
			continue
		}
		if len(got) == 0 || got[0] != want {
			t.Errorf("suggest(%q) = %v, want %q first", in, got, want)
		}
	}
	if got := suggest("xyzzy", commandNames()); len(got) != 0 {
		t.Errorf("suggest(xyzzy) = %v", got)
	}
}

func TestUnknownCommandIsShort(t *testing.T) {
	err := run([]string{"stauts"}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), `did you mean status?`) || strings.Contains(err.Error(), "Usage") {
		t.Fatalf("got %v", err)
	}
}

func TestHelpCommand(t *testing.T) {
	var out bytes.Buffer
	if err := run([]string{"help", "ledger"}, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "poon ledger -pool K") || strings.Contains(out.String(), "poon pending") {
		t.Fatalf("help ledger:\n%s", out.String())
	}
	out.Reset()
	if err := run([]string{"help", "watch"}, &out); err != nil || !strings.Contains(out.String(), "FILTER:") {
		t.Fatalf("help watch keeps its continuation lines: %v\n%s", err, out.String())
	}
	if err := run([]string{"help", "nope"}, &bytes.Buffer{}); err == nil {
		t.Fatal("help for an unknown command")
	}
}
