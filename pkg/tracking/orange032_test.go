package tracking

import (
	"testing"
	"time"
)

// ORANGE-032: the event log keeps the last LogSize events in order, and
// its array stays within about twice LogSize, without copying the log for
// each event.
func TestLogKeepsLastEvents(t *testing.T) {
	const size = 64
	tr, err := New(Config{Default: DetailedName, Retain: Retention{LogSize: size, Log: 24 * time.Hour}}, "")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1_800_000_000, 0)
	const n = 10 * size
	for i := range n {
		tr.Item("8.8.8.8:1", "sec", "tx", now.Add(time.Duration(i)*time.Second))
	}
	tr.mu.Lock()
	defer tr.mu.Unlock()
	r := tr.recs[tr.Key("8.8.8.8:1")]
	if len(r.Log) != size {
		t.Fatalf("%d events kept, want %d", len(r.Log), size)
	}
	for i, e := range r.Log {
		if want := now.Add(time.Duration(n-size+i) * time.Second); !e.At.Equal(want) {
			t.Fatalf("event %d at %v, want %v", i, e.At, want)
		}
	}
	if c := cap(r.Log); c > 2*size+8 {
		t.Fatalf("the log's array holds %d events for %d kept", c, size)
	}
	// Events older than Retention.Log are cut too.
	tr.recs = map[string]*record{}
	tr.mu.Unlock()
	tr.Item("8.8.8.8:1", "sec", "tx", now)
	tr.Item("8.8.8.8:1", "sec", "tx", now.Add(48*time.Hour))
	tr.mu.Lock()
	if r := tr.recs[tr.Key("8.8.8.8:1")]; len(r.Log) != 1 {
		t.Fatalf("%d events kept past the log's time", len(r.Log))
	}
}
