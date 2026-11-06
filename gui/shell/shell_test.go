package shell

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

// The tables pin the grammar each terminal implements (docs/gui.md, The
// terminal's grammar). Each POSIX line splits as sh would split it;
// GUI-002 in security/gui/findings has the reasons.

func TestSplitPOSIX(t *testing.T) {
	cases := []struct {
		line string
		want []string
	}{
		{"", nil},
		{"   ", nil},
		{"status", []string{"status"}},
		{"poon status", []string{"status"}},
		{"poon", nil},
		{"  peer  block\t1.2.3.4:5 ", []string{"peer", "block", "1.2.3.4:5"}},
		{`wallet pay -memo 'for lunch' -amount 5`, []string{"wallet", "pay", "-memo", "for lunch", "-amount", "5"}},
		{`-memo "say \"hi\" \\ ok"`, []string{"-memo", `say "hi" \ ok`}},
		{`-memo "a\nb"`, []string{"-memo", `a\nb`}},
		{`-memo "cost \$5"`, []string{"-memo", "cost $5"}},
		{`a\ b c`, []string{"a b", "c"}},
		// Outside quotes a backslash escapes any character, as in sh.
		{`foo C:\new`, []string{"foo", "C:new"}},
		{`-dir 'C:\Users\a\.poon'`, []string{"-dir", `C:\Users\a\.poon`}},
		{`\|\$\*`, []string{"|$*"}},
		{`-memo ''`, []string{"-memo", ""}},
		{`x"y z"w`, []string{"xy zw"}},
		{`it'"'s`, []string{`it"s`}},
		{"status poon", []string{"status", "poon"}},
		{"status # the rest is a comment", []string{"status"}},
		{"a#b", []string{"a#b"}},
		{"-memo a~b", []string{"-memo", "a~b"}},
		{"'|&;<>()$`*?[~'", []string{"|&;<>()$`*?[~"}},
	}
	for _, c := range cases {
		got, err := POSIX.Split(c.line)
		if err != nil || len(got)+len(c.want) > 0 && !reflect.DeepEqual(got, c.want) {
			t.Errorf("Split(%q) = %q, %v; want %q", c.line, got, err, c.want)
		}
	}
	for _, bad := range []string{`'abc`, `"abc`, `a "b\"`, `abc\`} {
		if _, err := POSIX.Split(bad); err != ErrUnterminated {
			t.Errorf("Split(%q) = %v, want ErrUnterminated", bad, err)
		}
	}
	for _, bad := range []string{"a\nb c", "a\rb", "'a\nb'", "status\n"} {
		if _, err := POSIX.Split(bad); err != ErrNewline {
			t.Errorf("Split(%q) = %v, want ErrNewline", bad, err)
		}
	}
	// What a shell would run as more than one poon command, or expand, is
	// refused.
	for _, bad := range []string{"a | b", "a;b", "a & b", "a>f", "a<f", "(a)", "$HOME", "`id`", "a*", "a?", "[ab]", "~/x", `"$HOME"`, "\"`id`\""} {
		var ue *UnsupportedError
		if _, err := POSIX.Split(bad); !errors.As(err, &ue) {
			t.Errorf("Split(%q) = %v, want UnsupportedError", bad, err)
		}
	}
}

func TestSplitWindows(t *testing.T) {
	cases := []struct {
		line string
		want []string
	}{
		{"poon status", []string{"status"}},
		{`-dir C:\Users\a\.poon`, []string{"-dir", `C:\Users\a\.poon`}},
		{`-dir \\server\share`, []string{"-dir", `\\server\share`}},
		{`-memo "for lunch"`, []string{"-memo", "for lunch"}},
		{`-memo 'for lunch'`, []string{"-memo", "'for", "lunch'"}},
		{`-memo "say \"hi\""`, []string{"-memo", `say "hi"`}},
		{`-memo "a""b"`, []string{"-memo", `a"b`}},
		{`"C:\dir\\" x`, []string{`C:\dir\`, "x"}},
		{`a\\\"b`, []string{`a\"b`}},
		{`-memo "a & b"`, []string{"-memo", "a & b"}},
		{`50% $HOME *`, []string{"50%", "$HOME", "*"}},
		{`-memo ""`, []string{"-memo", ""}},
	}
	for _, c := range cases {
		got, err := Windows.Split(c.line)
		if err != nil || !reflect.DeepEqual(got, c.want) {
			t.Errorf("Split(%q) = %q, %v; want %q", c.line, got, err, c.want)
		}
	}
	if _, err := Windows.Split(`"abc`); err != ErrUnterminated {
		t.Errorf("unterminated: %v", err)
	}
	if _, err := Windows.Split("a\r\nb"); err != ErrNewline {
		t.Errorf("newline: %v", err)
	}
	for _, bad := range []string{"a & b", "a | b", "a > f", "a < f", "a^b"} {
		var ue *UnsupportedError
		if _, err := Windows.Split(bad); !errors.As(err, &ue) {
			t.Errorf("Split(%q) = %v, want UnsupportedError", bad, err)
		}
	}
}

var roundTrip = [][]string{
	{"status"},
	{"wallet", "pay", "-memo", "it's \"fine\" \\ ok", "-amount", "5"},
	{"-dir", `C:\Users\a`},
	{"-dir", `C:\dir\`},
	{"-memo", ""},
	{"-memo", "a | b; $HOME `id` * ? [x] ~ # & < > ( ) ^ %"},
	{"poon"},
	{"poon", "poon"},
}

func TestQuoteRoundTrip(t *testing.T) {
	for _, g := range []Grammar{POSIX, Windows} {
		for _, args := range roundTrip {
			got, err := g.Split(g.Quote(args))
			if err != nil || !reflect.DeepEqual(got, args) {
				t.Errorf("%v: Split(Quote(%q)) = %q, %v (line %s)", g, args, got, err, g.Quote(args))
			}
		}
	}
	if q := POSIX.Quote([]string{"status", "-root", "/home/a/.poon"}); q != "poon status -root /home/a/.poon" {
		t.Errorf("plain args are quoted: %s", q)
	}
}

func FuzzSplitQuote(f *testing.F) {
	f.Add("wallet", "it's \"x\"", `C:\a\`)
	f.Fuzz(func(t *testing.T, a, b, c string) {
		args := []string{a, b, c}
		for _, g := range []Grammar{POSIX, Windows} {
			got, err := g.Split(g.Quote(args))
			if strings.ContainsAny(a+b+c, "\r\n") {
				if err != ErrNewline {
					t.Fatalf("%v: %q: %v, want ErrNewline", g, args, err)
				}
				continue
			}
			if err != nil || !reflect.DeepEqual(got, args) {
				t.Fatalf("%v: Split(Quote(%q)) = %q, %v", g, args, got, err)
			}
		}
	})
}
