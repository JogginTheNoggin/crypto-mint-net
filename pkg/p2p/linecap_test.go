package p2p

import (
	"testing"
	"time"
)

func TestLineCap(t *testing.T) {
	var c lineCap
	now := time.Unix(1000, 0)
	for i := 0; i < 3; i++ {
		if ok, _ := c.allow(now, 3); !ok {
			t.Fatalf("line %d refused under the cap", i)
		}
	}
	for i := 0; i < 5; i++ {
		if ok, _ := c.allow(now.Add(500*time.Millisecond), 3); ok {
			t.Fatal("line over the cap allowed")
		}
	}
	ok, dropped := c.allow(now.Add(time.Second), 3)
	if !ok || dropped != 5 {
		t.Fatalf("next second: ok %v, dropped %d, want true, 5", ok, dropped)
	}
	if _, dropped := c.allow(now.Add(time.Second), 3); dropped != 0 {
		t.Fatalf("dropped reported twice: %d", dropped)
	}
}
