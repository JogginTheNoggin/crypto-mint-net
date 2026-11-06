package rpc

import (
	"io"
	"strings"
	"testing"
)

// endless is a long base64 string body, read up to max bytes.
type endless struct{ n, max int64 }

func (e *endless) Read(p []byte) (int, error) {
	if e.n >= e.max {
		return 0, io.EOF
	}
	if left := e.max - e.n; int64(len(p)) > left {
		p = p[:left]
	}
	for i := range p {
		p[i] = 'A'
	}
	e.n += int64(len(p))
	return len(p), nil
}

// RED-101: an item past the first that passes the batch bound is refused
// while it is read, not after it is held whole.
func TestBlue035ImportReadStops(t *testing.T) {
	tail := &endless{max: 64 << 20}
	body := io.MultiReader(strings.NewReader(`{"pool":1,"items":["AAAA","`), tail)
	_, e := decodeImport(body)
	if e == nil || !strings.Contains(e.Message, "past the first item") {
		t.Fatalf("err %v", e)
	}
	if bound := int64(MaxImportBatchBytes)*4/3 + 1<<20; tail.n > bound {
		t.Fatalf("read %d bytes of the second item, past %d", tail.n, bound)
	}
}
