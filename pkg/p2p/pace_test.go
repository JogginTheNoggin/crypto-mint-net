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
	w.answered(t0)
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
