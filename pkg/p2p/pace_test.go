package p2p

import (
	"testing"
	"time"
)

func TestReqWindowCounts(t *testing.T) {
	var w reqWindow
	t0 := time.Unix(1_000_000, 0)
	for i := 0; i < 90; i++ {
		w.add(t0.Add(time.Duration(i)*time.Second), time.Minute)
	}
	// The last 60 seconds hold requests 30 to 89.
	if n := w.count(t0.Add(89*time.Second), time.Minute); n != 60 {
		t.Fatalf("count %d, want 60", n)
	}
	if n := w.count(t0.Add(89*time.Second+time.Minute), time.Minute); n != 0 {
		t.Fatalf("count a window later %d, want 0", n)
	}
	var s reqWindow
	for i := 0; i < 5; i++ {
		s.add(t0, 3*time.Second)
	}
	if n := s.count(t0.Add(time.Second), 3*time.Second); n != 5 {
		t.Fatalf("short window count %d, want 5", n)
	}
	if n := s.count(t0.Add(3*time.Second), 3*time.Second); n != 0 {
		t.Fatalf("short window count later %d, want 0", n)
	}
}

// D133: a second unanswered request in a row sets the pace to three
// quarters of what was sent in the window; a lower one replaces it, it
// lasts paceLearn windows from the last drop, and a request sent before
// the peer last replied is never counted.
func TestReqWindowLearnsThePace(t *testing.T) {
	var w reqWindow
	t0 := time.Unix(1_000_000, 0)
	if w.learned(t0) != 0 {
		t.Fatal("a pace was learned from nothing")
	}
	for i := 0; i < 40; i++ {
		w.add(t0, time.Minute)
	}
	early := t0.Add(-time.Second)
	w.dropped(early, t0, time.Minute)
	w.answered(t0, 0, time.Minute)
	w.dropped(early, t0, time.Minute) // sent before the reply
	if got := w.learned(t0); got != 0 {
		t.Fatalf("learned %d from a drop sent before a reply", got)
	}
	w.dropped(t0, t0, time.Minute)
	if got := w.learned(t0); got != 0 {
		t.Fatalf("learned %d from one drop", got)
	}
	w.dropped(t0, t0, time.Minute)
	if got := w.learned(t0); got != 30 {
		t.Fatalf("learned %d, want 30", got)
	}
	w.dropped(t0, t0, time.Minute) // 40 sent still: 30 again, not higher
	if got := w.learned(t0); got != 30 {
		t.Fatalf("learned %d after a second drop, want 30", got)
	}
	later := t0.Add(paceLearn*time.Minute + time.Second)
	if got := w.learned(later); got != 0 {
		t.Fatalf("still %d after paceLearn windows", got)
	}
	w.dropped(later, later, time.Minute) // nothing sent in the window: at least 1
	if got := w.learned(later); got != 1 {
		t.Fatalf("learned %d from an empty window, want 1", got)
	}
}

// RED-028 (D136): two requests lost while little was sent teach a pace to
// probe up from, not one to hold: it doubles each window the peer
// replies in and ends at the configured pace. Drops again while probing
// show the budget, which then holds, is not lowered by drops with little
// sent, and is known for later.
func TestLearnedPaceRisesUnlessShown(t *testing.T) {
	const pace = 480
	win := time.Minute
	var w reqWindow
	t0 := time.Unix(2_000_000, 0)
	w.add(t0, win)
	w.add(t0, win)
	w.dropped(t0, t0, win)
	w.dropped(t0, t0, win)
	if got := w.learned(t0); got != 1 {
		t.Fatalf("learned %d, want 1", got)
	}
	w.answered(t0, pace, win)
	if got := w.learned(t0); got != 1 {
		t.Fatalf("rose to %d within the window", got)
	}
	now, windows := t0, 0
	for w.learned(now) != 0 {
		now = now.Add(win)
		w.answered(now, pace, win)
		if windows++; windows > 12 {
			t.Fatalf("the pace is still %d after %d windows", w.learned(now), windows)
		}
	}
	if windows > 9 {
		t.Fatalf("%d windows to get back to the pace", windows)
	}

	// A peer whose budget is 100 a window: learned at 75 (probing), it
	// drops again, which shows the budget.
	w = reqWindow{}
	for i := 0; i < 100; i++ {
		w.add(t0, win)
	}
	w.dropped(t0, t0, win)
	w.dropped(t0, t0, win)
	t1 := t0.Add(win)
	w.answered(t1, pace, win) // probing: 150
	for i := 0; i < 150; i++ {
		w.add(t1, win)
	}
	w.dropped(t1, t1, win)
	w.dropped(t1, t1, win)
	if got := w.learned(t1); got != 112 {
		t.Fatalf("learned %d once shown, want 112", got)
	}
	w.answered(t1.Add(2*win), pace, win)
	if got := w.learned(t1.Add(2 * win)); got != 112 {
		t.Fatalf("a shown budget rose to %d", got)
	}
	// Two requests lost with little sent do not lower it.
	t2 := t1.Add(5 * win)
	w.add(t2, win)
	w.dropped(t2, t2, win)
	w.dropped(t2, t2, win)
	if got := w.learned(t2); got != 112 {
		t.Fatalf("lowered to %d by drops with one request sent", got)
	}
	// After it lapses, a drop with as much sent holds at once.
	later := t2.Add(paceLearn*win + time.Second)
	for i := 0; i < 150; i++ {
		w.add(later, win)
	}
	w.dropped(later, later, win)
	w.dropped(later, later, win)
	w.answered(later.Add(2*win), pace, win)
	if got := w.learned(later.Add(2 * win)); got != 112 {
		t.Fatalf("learned %d after the lapse, want 112 held", got)
	}
}

// RED-034 (D139): a second outage while probing, with the probe holding
// the asker below what it sent at the first, halves the probe (not below
// where it began) and the pace still rises back to the configured one;
// drops with as much sent as at the first show the budget and hold.
func TestSecondOutageWhileProbingKeepsProbing(t *testing.T) {
	const pace = 480
	win := time.Minute
	var w reqWindow
	t0 := time.Unix(3_000_000, 0)
	for i := 0; i < 40; i++ {
		w.add(t0, win)
	}
	w.dropped(t0, t0, win)
	w.dropped(t0, t0, win) // probing from 30
	now := t0
	for i := 0; i < 2; i++ {
		now = now.Add(win)
		w.answered(now, pace, win)
	}
	if got := w.learned(now); got != 120 {
		t.Fatalf("probe at %d, want 120", got)
	}
	// The second outage, with 3 sent in its window.
	now = now.Add(win)
	for i := 0; i < 3; i++ {
		w.add(now, win)
	}
	w.dropped(now, now, win)
	w.dropped(now, now, win)
	if got := w.learned(now); got != 60 {
		t.Fatalf("after the second outage %d, want the probe halved to 60", got)
	}
	// Again, at the probe's start: it does not fall below it.
	w.dropped(now, now, win)
	w.dropped(now, now, win)
	if got := w.learned(now); got != 30 {
		t.Fatalf("after a third outage %d, want 30", got)
	}
	w.dropped(now, now, win)
	if got := w.learned(now); got != 30 {
		t.Fatalf("fell to %d, below where the probe began", got)
	}
	windows := 0
	for w.learned(now) != 0 {
		now = now.Add(win)
		w.answered(now, pace, win)
		if windows++; windows > 6 {
			t.Fatalf("the pace is still %d after %d windows", w.learned(now), windows)
		}
	}
	// Drops while probing with as much sent as at the first hold.
	w = reqWindow{}
	for i := 0; i < 40; i++ {
		w.add(t0, win)
	}
	w.dropped(t0, t0, win)
	w.dropped(t0, t0, win)
	t1 := t0.Add(win)
	w.answered(t1, pace, win) // 60
	for i := 0; i < 36; i++ {
		w.add(t1, win)
	}
	w.dropped(t1, t1, win)
	w.dropped(t1, t1, win)
	if got := w.learned(t1.Add(3 * win)); got != 27 {
		t.Fatalf("learned %d with 36 sent, want 27 held", got)
	}
}

// RED-036 (D142): at low load a second outage while probing came with as
// little sent as the first (a peak of 2), so the peak alone let it hold
// at 1. Drops with nothing sent in the window answered are an outage, not
// a budget: the probe steps back and rises to the configured pace. A
// peer that served some of the window before dropping still holds.
func TestOutageAtLowLoadNeverHolds(t *testing.T) {
	const pace = 480
	win := time.Minute
	var w reqWindow
	t0 := time.Unix(4_000_000, 0)
	w.add(t0, win)
	w.add(t0, win)
	w.dropped(t0, t0, win)
	w.dropped(t0, t0, win) // probing from 1, peak 2
	now := t0.Add(win)
	w.answered(now, pace, win) // 2
	now = now.Add(win)
	w.add(now, win)
	w.add(now, win)
	w.dropped(now, now, win)
	w.dropped(now, now, win) // 2 sent, both lost
	if got := w.learned(now); got != 1 || !w.probing {
		t.Fatalf("after the second outage %d (probing %v), want the probe at 1", got, w.probing)
	}
	windows := 0
	for w.learned(now) != 0 {
		now = now.Add(win)
		w.answered(now, pace, win)
		if windows++; windows > 12 {
			t.Fatalf("the pace is still %d after %d windows", w.learned(now), windows)
		}
	}
	// A peer that answered some of the window, then dropped: a budget.
	w = reqWindow{}
	for i := 0; i < 40; i++ {
		w.add(t0, win)
	}
	w.dropped(t0, t0, win)
	w.dropped(t0, t0, win)
	t1 := t0.Add(win)
	w.answered(t1, pace, win) // 60
	for i := 0; i < 40; i++ {
		w.add(t1, win)
	}
	w.dropped(t1, t1, win)
	w.dropped(t1, t1, win)
	if got := w.learned(t1); got != 30 || w.probing {
		t.Fatalf("learned %d (probing %v) from a peer that served 38 of 40, want 30 held", got, w.probing)
	}
	// Drops at the known pace with nothing in the window answered do
	// not hold either.
	later := t1.Add(paceLearn*win + time.Second)
	w.answered(later.Add(-time.Second), pace, win)
	for i := 0; i < 2; i++ {
		w.add(later, win)
	}
	w.known = 2
	w.dropped(later, later, win)
	w.dropped(later, later, win)
	if !w.probing {
		t.Fatal("an outage at the known pace held")
	}
}
