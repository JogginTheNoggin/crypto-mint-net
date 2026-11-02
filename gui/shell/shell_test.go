package shell

import (
	"reflect"
	"testing"
)

func TestSplit(t *testing.T) {
	cases := []struct {
		line string
		want []string
	}{
		{"", nil},
		{"   ", nil},
		{"status", []string{"status"}},
		{"poo status", []string{"status"}},
		{"poo", nil},
		{"  peer  block\t1.2.3.4:5 ", []string{"peer", "block", "1.2.3.4:5"}},
		{`wallet pay -memo 'for lunch' -amount 5`, []string{"wallet", "pay", "-memo", "for lunch", "-amount", "5"}},
		{`-memo "say \"hi\" \\ ok"`, []string{"-memo", `say "hi" \ ok`}},
		{`-memo "a\nb"`, []string{"-memo", `a\nb`}},
		{`a\ b c`, []string{"a b", "c"}},
		{`-dir C:\Users\a\.poo`, []string{"-dir", `C:\Users\a\.poo`}},
		{`-memo ''`, []string{"-memo", ""}},
		{`x"y z"w`, []string{"xy zw"}},
		{`it'"'s`, []string{`it"s`}},
		{"status poo", []string{"status", "poo"}},
	}
	for _, c := range cases {
		got, err := Split(c.line)
		if err != nil || len(got)+len(c.want) > 0 && !reflect.DeepEqual(got, c.want) {
			t.Errorf("Split(%q) = %q, %v; want %q", c.line, got, err, c.want)
		}
	}
	for _, bad := range []string{`'abc`, `"abc`, `a "b\"`} {
		if _, err := Split(bad); err != ErrUnterminated {
			t.Errorf("Split(%q) = %v, want ErrUnterminated", bad, err)
		}
	}
}

func TestQuoteRoundTrip(t *testing.T) {
	for _, args := range [][]string{
		{"status"},
		{"wallet", "pay", "-memo", "it's \"fine\" \\ ok", "-amount", "5"},
		{"-dir", `C:\Users\a`},
		{"-memo", ""},
		{"poo"},
	} {
		got, err := Split(Quote(args))
		if err != nil || !reflect.DeepEqual(got, args) {
			t.Errorf("Split(Quote(%q)) = %q, %v (line %s)", args, got, err, Quote(args))
		}
	}
}

func FuzzSplitQuote(f *testing.F) {
	f.Add("wallet", "it's \"x\"", `C:\a`)
	f.Fuzz(func(t *testing.T, a, b, c string) {
		args := []string{a, b, c}
		got, err := Split(Quote(args))
		if err != nil || !reflect.DeepEqual(got, args) {
			t.Fatalf("Split(Quote(%q)) = %q, %v", args, got, err)
		}
	})
}
