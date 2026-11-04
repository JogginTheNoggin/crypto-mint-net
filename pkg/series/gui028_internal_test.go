package series

import (
	"context"
	"errors"
	"testing"
	"time"
)

// GUI-028: working out a far part id stops when its context is done, both
// while the chain grows and while walking from a mark, and keeps what it
// worked out; a later call gives the exact id.
func TestPartIDContextCancels(t *testing.T) {
	defer func(a, b uint64, m int) { firstPartStride, partChunk, maxPartMarks = a, b, m }(firstPartStride, partChunk, maxPartMarks)
	firstPartStride, partChunk, maxPartMarks = 4, 64, 16
	s := variableForTest(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := s.PartIDContext(ctx, 1<<40); !errors.Is(err, context.Canceled) {
		t.Fatalf("grow: %v", err)
	}
	if s.parts.next == 0 || s.parts.next > 1+2*partChunk {
		t.Fatalf("worked out %d before stopping", s.parts.next)
	}
	want := naive(s, 3000)
	if id, err := s.PartID(3000); err != nil || id != want[2999] {
		t.Fatalf("after a stop: %v", err)
	}
	// A walk from a mark far behind (the stride is large by now).
	if s.parts.stride > 64 {
		if _, err := s.PartIDContext(ctx, 2999-1); err != nil && !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	}
	if id, _ := s.PartID(2998); id != want[2997] {
		t.Fatal("part 2998 differs")
	}

	// A real deadline stops a very far part in good time.
	partChunk = 1 << 10
	s = variableForTest(t)
	ctx, cancel = context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	start := time.Now()
	if _, err := s.PartIDContext(ctx, 1<<40); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("deadline: %v", err)
	}
	if d := time.Since(start); d > 5*time.Second {
		t.Fatalf("took %v to stop", d)
	}
	if _, err := s.UnsignedContext(ctx, 1<<40); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Unsigned: %v", err)
	}
}
