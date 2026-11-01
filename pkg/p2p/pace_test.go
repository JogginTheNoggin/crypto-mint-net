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
