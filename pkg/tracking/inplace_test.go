package tracking_test

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/tracking"
)

// A tracker given new retention in place (SetRetention) keeps and trims
// what a tracker opened from the same file with that retention does, for
// the same events after, lowered and raised, at the floors and the
// ceilings of node policy (ORANGE-035's in-place rule, D168).
func TestSetRetentionMatchesReopen(t *testing.T) {
	t0 := time.Unix(1_800_000_000, 0)
	events := func(tr *tracking.Tracker, from, n int, at time.Time) {
		for i := from; i < from+n; i++ {
			a := fmt.Sprintf("10.%d.%d.1:1", i/250, i%250)
			for j := 0; j < 20; j++ {
				tr.Message(a, "tx", 100, at.Add(time.Duration(j)*time.Second))
			}
			if i%7 == 0 {
				tr.Offend(a, tracking.InvalidItem, "test", at)
			}
		}
	}
	start := tracking.Retention{FlagHistory: 24 * time.Hour, Log: time.Hour, LogSize: 256, MaxAddresses: 4096}
	src := filepath.Join(t.TempDir(), "tracking.json")
	tr, err := tracking.New(tracking.Config{Retain: start}, src)
	if err != nil {
		t.Fatal(err)
	}
	events(tr, 0, 1500, t0)
	if err := tr.Save(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range []tracking.Retention{
		{FlagHistory: time.Hour, Log: time.Minute, LogSize: 16, MaxAddresses: 1024},                   // the floors
		{FlagHistory: 365 * 24 * time.Hour, Log: 24 * time.Hour, LogSize: 4096, MaxAddresses: 131072}, // the ceilings
		{FlagHistory: 2 * time.Hour, Log: 10 * time.Minute, LogSize: 32, MaxAddresses: 1200},          // lowered
		{FlagHistory: 48 * time.Hour, Log: 2 * time.Hour, LogSize: 512, MaxAddresses: 8192},           // raised
	} {
		dir := t.TempDir()
		a, b := filepath.Join(dir, "a.json"), filepath.Join(dir, "b.json")
		for _, p := range []string{a, b} {
			if err := os.WriteFile(p, data, 0o600); err != nil {
				t.Fatal(err)
			}
		}
		running, err := tracking.New(tracking.Config{Retain: start}, a)
		if err != nil {
			t.Fatal(err)
		}
		running.SetRetention(r)
		reopened, err := tracking.New(tracking.Config{Retain: r}, b)
		if err != nil {
			t.Fatal(err)
		}
		if running.Retention() != reopened.Retention() {
			t.Fatalf("%+v: retention %+v in place, %+v reopened", r, running.Retention(), reopened.Retention())
		}
		later := t0.Add(3 * time.Hour)
		for _, tr := range []*tracking.Tracker{running, reopened} {
			events(tr, 1000, 1000, later)
			if err := tr.Save(); err != nil {
				t.Fatal(err)
			}
		}
		x, _ := os.ReadFile(a)
		y, _ := os.ReadFile(b)
		if !bytes.Equal(x, y) {
			t.Fatalf("%+v: the trackers differ after the same events (%d and %d bytes)", r, len(x), len(y))
		}
	}
}
