package wire_test

import (
	"bytes"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/wire"
)

// The hello carries release, features and extensions; the ping and pong
// carry extensions (D186).
func TestHelloReleaseFeaturesExt(t *testing.T) {
	h := wire.Hello{Protocol: 1, NodeID: hash.Hash{3}, Agent: "poo", Release: "0.1.0-alpha.1+abc",
		Features: wire.OwnFeatures, Ext: []wire.Ext{{Tag: 1, Value: []byte("x")}, {Tag: 900, Value: nil}}}
	if err := h.Check(); err != nil {
		t.Fatal(err)
	}
	got, err := wire.DecodeHello(h.Encode())
	if err != nil {
		t.Fatal(err)
	}
	if got.Release != h.Release || !reflect.DeepEqual(got.Features, h.Features) || len(got.Ext) != 2 || string(got.Ext[0].Value) != "x" || got.Ext[1].Tag != 900 {
		t.Fatalf("got %+v", got)
	}
	if !wire.Has(got.Features, wire.FeatureParts, 1) || wire.Has(got.Features, wire.FeatureParts, 2) || wire.Has(got.Features, "nope", 0) {
		t.Fatal("Has")
	}
	p := wire.Ping{Nonce: 1, Time: 2, Ext: []wire.Ext{{Tag: 7, Value: []byte{1, 2}}}}
	gp, err := wire.DecodePing(p.Encode())
	if err != nil || !reflect.DeepEqual(gp, p) {
		t.Fatalf("ping %+v %v", gp, err)
	}
	q := wire.PongReply{Contact: wire.Contact{Addr: "a:1"}, Ext: []wire.Ext{{Tag: 2, Value: []byte("y")}}}
	gq, err := wire.DecodePongReply(q.Encode())
	if err != nil || gq.Ext[0].Tag != 2 || string(gq.Ext[0].Value) != "y" {
		t.Fatalf("pong %+v %v", gq, err)
	}
	// An empty hello's new fields cost six bytes: a 4-byte length and two counts.
	if n := len(wire.Hello{}.Encode()); n != 2+32+4+4+1+1 {
		t.Fatalf("empty hello %d bytes", n)
	}
}

// Every malformed new field closes the connection as malformed, and Check
// refuses to send one.
func TestHelloMalformed(t *testing.T) {
	long := strings.Repeat("a", wire.MaxRelease+1)
	many := make([]wire.Feature, wire.MaxFeatures+1)
	for i := range many {
		many[i] = wire.Feature{Name: string(rune('a'+i/26)) + string(rune('a'+i%26)), Ver: 1}
	}
	manyExt := make([]wire.Ext, wire.MaxExt+1)
	for i := range manyExt {
		manyExt[i] = wire.Ext{Tag: uint16(i)}
	}
	for name, h := range map[string]wire.Hello{
		"release chars":    {Release: "1.0 beta"},
		"release control":  {Release: "1.0\x1b[31m"},
		"release too long": {Release: long},
		"feature unsorted": {Features: []wire.Feature{{Name: "b"}, {Name: "a"}}},
		"feature twice":    {Features: []wire.Feature{{Name: "a"}, {Name: "a", Ver: 2}}},
		"feature empty":    {Features: []wire.Feature{{Name: ""}}},
		"feature upper":    {Features: []wire.Feature{{Name: "Parts"}}},
		"feature long":     {Features: []wire.Feature{{Name: strings.Repeat("a", wire.MaxFeatureName+1)}}},
		"too many":         {Features: many},
		"ext order":        {Ext: []wire.Ext{{Tag: 2}, {Tag: 1}}},
		"ext twice":        {Ext: []wire.Ext{{Tag: 2}, {Tag: 2}}},
		"ext value":        {Ext: []wire.Ext{{Tag: 1, Value: make([]byte, wire.MaxExtValue+1)}}},
		"ext many":         {Ext: manyExt},
	} {
		if h.Check() == nil {
			t.Errorf("%s: Check took it", name)
		}
		if _, err := wire.DecodeHello(h.Encode()); !errors.Is(err, wire.ErrMalformed) {
			t.Errorf("%s: decoded (%v)", name, err)
		}
	}
	// Bytes after the extensions are still refused.
	if _, err := wire.DecodeHello(append(wire.Hello{}.Encode(), 0)); !errors.Is(err, wire.ErrMalformed) {
		t.Fatal("took a byte after the extensions")
	}
	if _, err := wire.DecodePing(append(wire.Ping{}.Encode(), 0)); !errors.Is(err, wire.ErrMalformed) {
		t.Fatal("ping took a byte after the extensions")
	}
	// The old layout (before D186) is refused: nothing is released, so no
	// node speaks it.
	old := append([]byte{0, 1}, bytes.Repeat([]byte{0}, 32)...)
	old = append(old, 3, 'p', 'o', 'o')
	if _, err := wire.DecodeHello(old); err == nil {
		t.Fatal("took a hello without the new fields")
	}
}
