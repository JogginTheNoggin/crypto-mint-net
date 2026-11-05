package p2p_test

import (
	"bufio"
	"crypto/rand"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/wire"
)

// maxExt is an extension list at the frozen maximum, of tags no release
// registers.
func maxExt() []wire.Ext {
	var es []wire.Ext
	for i := 0; i < wire.MaxExt; i++ {
		v := make([]byte, wire.MaxExtValue)
		rand.Read(v)
		es = append(es, wire.Ext{Tag: uint16(40000 + i), Value: v})
	}
	return es
}

// Black's Round 33 condition on D186 term 4: a hello and a ping at the
// frozen maximum (the longest release, 255 unknown features, 32 unknown
// extensions of the largest size) are answered, and a ping or pong whose
// extensions list is malformed ends that connection only, with no
// lasting offence.
func TestExtrasAtTheLimitsNoOffence(t *testing.T) {
	a := startNode(t, "a", nil)
	sec := poolSection()

	c, err := net.Dial("tcp", a.h.Addr())
	if err != nil {
		t.Fatal(err)
	}
	var id hash.Hash
	rand.Read(id[:])
	hello := wire.Hello{Protocol: 1, NodeID: id, Agent: "raw", Release: strings.Repeat("9", wire.MaxRelease), Ext: maxExt()}
	for i := 0; i < wire.MaxFeatures; i++ {
		hello.Features = append(hello.Features, wire.Feature{Name: fmt.Sprintf("x-unknown-%03d", i), Ver: 1})
	}
	if err := hello.Check(); err != nil {
		t.Fatal(err)
	}
	r := &rawPeer{t: t, c: c, r: bufio.NewReader(c), coverage: []wire.Section{sec}}
	t.Cleanup(func() { r.closing.Store(true); c.Close() })
	r.write(wire.Message{Header: wire.Header{Type: wire.TypeHello}, Body: hello.Encode()})
	if m, _, err := r.read(); err != nil || m.Type != wire.TypeHello {
		t.Fatalf("no hello back: %v %v", m.Type, err)
	}
	key := a.h.Tracker().Key(c.LocalAddr().String())

	ping := func(es []wire.Ext) hash.Hash {
		body := wire.Ping{Nonce: 7, Time: uint64(time.Now().Unix()), Ext: es}.Encode()
		rid := wire.RequestID(wire.TypePing, sec, body)
		r.write(wire.Message{Header: wire.Header{Type: wire.TypePing, Section: sec, RequestID: rid}, Body: body})
		return rid
	}
	rid := ping(maxExt())
	for {
		m, _, err := r.read()
		if err != nil {
			t.Fatalf("no pong to a ping at the limits: %v", err)
		}
		if m.RequestID == rid {
			if m.Type != wire.TypePong && m.Type != wire.TypePartialPong {
				t.Fatalf("answered %s", m.Type)
			}
			break
		}
	}
	if st := a.h.Tracker().Status(key, time.Now()); st.FlagTotal != 0 {
		t.Fatalf("an offence for messages at the limits: %+v", st)
	}

	// Tags out of order: the connection ends, and nothing is held against
	// the address.
	body := wire.Ping{Nonce: 8}.Encode()
	body = append(body[:len(body)-1], 2, 0, 9, 0, 0, 0, 0, 0, 1, 0, 0, 0, 0)
	if _, err := wire.DecodePing(body); err == nil {
		t.Fatal("the bad ping decodes")
	}
	r.write(wire.Message{Header: wire.Header{Type: wire.TypePing, Section: sec, RequestID: wire.RequestID(wire.TypePing, sec, body)}, Body: body})
	r.expectClosed()
	time.Sleep(50 * time.Millisecond)
	if st := a.h.Tracker().Status(key, time.Now()); st.FlagTotal != 0 {
		t.Fatalf("a lasting offence for a bad extensions list: %+v", st)
	}
}
