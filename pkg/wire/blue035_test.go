package wire_test

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"runtime"
	"testing"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/wire"
)

// RED-100: a frame that declares 16 MiB and sends four bytes costs what
// was sent, not what was declared; whole frames of every size still read.
func TestBlue035FrameMemoryFollowsBytes(t *testing.T) {
	var head [8]byte
	binary.BigEndian.PutUint32(head[:4], wire.MaxFrame)
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	for range 20 {
		if _, _, err := wire.ReadFrame(bufio.NewReader(bytes.NewReader(head[:])), wire.MaxFrame); err == nil {
			t.Fatal("a cut frame read")
		}
	}
	runtime.ReadMemStats(&after)
	if got := after.TotalAlloc - before.TotalAlloc; got > 20<<20 {
		t.Fatalf("20 cut frames declaring 16 MiB allocated %d MiB", got>>20)
	}
	for _, n := range []int{0, 1, 64<<10 - 1, 64 << 10, 64<<10 + 1, 3<<20 + 5} {
		m := wire.Message{Header: wire.Header{Type: wire.TypeTx}, Body: bytes.Repeat([]byte{0x35}, n)}
		var buf bytes.Buffer
		if err := wire.WriteFrame(&buf, m); err != nil {
			t.Fatal(err)
		}
		got, _, err := wire.ReadFrame(bufio.NewReader(&buf), wire.MaxFrame)
		if err != nil || !bytes.Equal(got.Body, m.Body) {
			t.Fatalf("%d bytes: %v", n, err)
		}
	}
	// The hello bound is above the largest hello and below MaxFrame.
	if wire.MaxHelloFrame >= wire.MaxFrame || wire.MaxHelloFrame < wire.MaxExt*(wire.MaxExtValue+8)+wire.MaxFeatures*48 {
		t.Fatal(wire.MaxHelloFrame)
	}
}
