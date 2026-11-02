package p2p

import (
	"testing"
	"time"
)

// D139 and D140 (replacing D137's turns): a waiting chase may cut a kept
// parts file when that file's sender left, its last chase took nothing
// or it has not asked for a while, its sender delivers at least 1.5
// times more slowly, or, within that margin, it has more still to come.
// A claimed size never decides against a much faster sender.
func TestTakeFrom(t *testing.T) {
	h := &Host{cfg: Config{ChaseSlice: time.Minute}}
	h.cfg.Chase.Backoff = 30 * time.Second
	now := time.Now()
	const mib = 1 << 20
	file := func(rate float64, size, off uint64) *progress {
		return &progress{partsRate: rate, partsAt: now, part: &partial{size: size, off: off}}
	}
	me := func(size, off uint64) *progress { return &progress{part: &partial{size: size, off: off}} }
	cases := []struct {
		name   string
		holder *progress
		away   bool
		me     *progress
		rate   float64 // the waiting chase's, in bytes a second
		take   bool
	}{
		{"a departed sender's file", file(64*mib, 100*mib, 99*mib), true, me(100*mib, 0), 1, true},
		{"a holder whose last chase took nothing", file(0, 100*mib, 99*mib), false, me(100*mib, 0), 1, true},
		{"a holder not chased for a while", &progress{partsRate: 64 * mib, partsAt: now.Add(-time.Hour), part: &partial{size: 100 * mib, off: 99 * mib}}, false, me(100*mib, 0), 1, true},
		{"RED-031: a trickler just under, at 1 MiB a minute", file(mib/60, 99*mib, 10*mib), false, me(100*mib, 0), 4 * mib, true},
		{"a huge claim trickled (B4)", file(mib/10, 1<<40, 90*mib), false, me(100*mib, 0), mib, true},
		{"B6: a slower fetch never cuts a faster holder, whatever it claims", file(mib, 1<<40, 90*mib), false, me(100*mib, 0), mib / 10, false},
		{"B6: a colluder near its end, far slower than the honest fetch", file(4<<10, 34*mib, 32*mib), false, me(64*mib, 0), 13 * mib, true},
		{"B6: 1.4 times faster but further from its end", file(mib, 100*mib, 90*mib), false, me(100*mib, 0), 1.4 * mib, false},
		{"B6: 1.5 times faster takes it, whatever is to come", file(mib, 100*mib, 90*mib), false, me(100*mib, 0), 1.5 * mib, true},
		{"a holder as fast and nearer its end keeps it", file(4*mib, 100*mib, 90*mib), false, me(100*mib, 0), 4 * mib, false},
		{"RED-032: a fresh sender slower than the holder", file(64<<10, 100*mib, 50*mib), false, me(100*mib, 0), 32 << 10, false},
		{"RED-033: a slow honest link faster than a slow trickler", file(32<<10, 100*mib, 10*mib), false, me(100*mib, 50*mib), 64 << 10, true},
		{"ORANGE-022: same pace, the one nearer its end keeps it", file(64<<10, 100*mib, 90*mib), false, me(100*mib, 50*mib), 64 << 10, false},
		{"ORANGE-022: same pace, the one nearer its end takes it", file(64<<10, 100*mib, 10*mib), false, me(100*mib, 50*mib), 64 << 10, true},
		{"an empty file is not cut", file(0, 100*mib, 0), true, me(100*mib, 0), 1, false},
	}
	for _, c := range cases {
		if _, _, got := h.takeFrom(c.holder, c.away, c.me, c.rate, now); got != c.take {
			t.Errorf("%s: take %v, want %v", c.name, got, c.take)
		}
	}
}

// RED-037 (D142): a waiting chase whose measured probes found no room
// asks again less often each time, up to eight times partsStale; one
// that wrote resets it.
func TestMeasureLastsLongerAfterMisses(t *testing.T) {
	h := &Host{cfg: Config{ChaseSlice: time.Minute}}
	h.cfg.Chase.Backoff = 30 * time.Second
	stale := h.partsStale()
	for miss, want := range []time.Duration{stale, stale, 2 * stale, 4 * stale, 8 * stale, 8 * stale, 8 * stale} {
		if got := h.measureLasts(&progress{partsMiss: miss}); got != want {
			t.Errorf("after %d misses: %v, want %v", miss, got, want)
		}
	}
}
