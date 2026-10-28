package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/testutil"
)

func TestPoolBySerial(t *testing.T) {
	s := testutil.ExampleSeries() // 4 mints per pool
	for _, c := range []struct {
		args []string
		want uint64
		err  string
	}{
		{nil, 0, ""},
		{[]string{"-pool", "1"}, 1, ""},
		{[]string{"-serial", "5"}, 1, ""},
		{[]string{"-serial", "5", "-pool", "1"}, 1, ""},
		{[]string{"-serial", "0"}, 0, ""},
		{[]string{"-serial", "5", "-pool", "0"}, 0, "is in pool 1"},
		{[]string{"-serial", "1000000"}, 0, "mints"},
	} {
		fs := newFlagSet("t", &bytes.Buffer{})
		pf := addPool(fs)
		if err := fs.Parse(c.args); err != nil {
			t.Fatal(err)
		}
		got, err := pf.resolve(fs, s)
		if c.err != "" {
			if err == nil || !strings.Contains(err.Error(), c.err) {
				t.Errorf("%v: %v, want %q", c.args, err, c.err)
			}
			continue
		}
		if err != nil || got != c.want {
			t.Errorf("%v: %d %v, want %d", c.args, got, err, c.want)
		}
	}
}
