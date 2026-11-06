package p2p_test

import (
	"encoding/binary"
	"io"
	"net"
	"testing"
	"time"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/p2p"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/wire"
)

// closedSoon says the node closed c within d.
func closedSoon(c net.Conn, d time.Duration) bool {
	c.SetReadDeadline(time.Now().Add(d))
	_, err := io.Copy(io.Discard, c) // the node may send its own hello first
	ne, ok := err.(net.Error)
	return !(ok && ne.Timeout())
}

// RED-100: a first frame over the hello bound is refused at once, and
// connections that have not said hello are capped at max_inbound.
func TestBlue035HelloBoundAndCap(t *testing.T) {
	tn := startNode(t, "a", func(c *p2p.Config) { c.MaxInbound = 3 })
	dial := func() net.Conn {
		c, err := net.Dial("tcp", tn.h.Addr())
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { c.Close() })
		return c
	}
	big := dial()
	var n [4]byte
	binary.BigEndian.PutUint32(n[:], wire.MaxHelloFrame+1)
	big.Write(n[:])
	if !closedSoon(big, 2*time.Second) {
		t.Fatal("a first frame over the hello bound was not refused")
	}
	var silent []net.Conn
	for range 3 {
		silent = append(silent, dial())
	}
	time.Sleep(200 * time.Millisecond) // let the node accept them
	if !closedSoon(dial(), 2*time.Second) {
		t.Fatal("a fourth connection still saying hello was taken")
	}
	if closedSoon(silent[0], 200*time.Millisecond) {
		t.Fatal("a connection within the cap was closed")
	}
	for _, c := range silent {
		c.Close()
	}
	// Once they are gone, a real peer still connects.
	deadline := time.Now().Add(5 * time.Second)
	for {
		if !closedSoon(dial(), 300*time.Millisecond) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("no connection taken after the silent ones closed")
		}
	}
}
