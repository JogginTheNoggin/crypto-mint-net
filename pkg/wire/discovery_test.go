package wire_test

import (
	"errors"
	"reflect"
	"testing"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/wire"
)

func TestDiscoveryRoundTrips(t *testing.T) {
	lim := wire.DiscoveryLimits{ChunkSize: 4096, ListLimit: 10, PeerListLimit: 5}
	peers := []wire.PeerInfo{
		{Contact: wire.Contact{Addr: "10.0.0.1:8646"}},
		{Contact: wire.Contact{Addr: "[::1]:9"}, Policies: []wire.Section{wire.PoolSection(iss, s1, p1)}},
	}
	type codec struct {
		v      any
		decode func([]byte) (any, error)
	}
	cases := []codec{
		{wire.FollowReq{Nonce: 1, Time: 2, Variant: wire.AllPeers, Broadcast: true, Contact: wire.Contact{Addr: "a.example:1"}, Limits: lim},
			func(b []byte) (any, error) { return wire.DecodeFollowReq(b) }},
		{wire.FollowReq{Nonce: 3, Time: 4},
			func(b []byte) (any, error) { return wire.DecodeFollowReq(b) }},
		{wire.PeerListReq{Nonce: 5, Time: 6, Variant: wire.FullPeers, Limits: lim},
			func(b []byte) (any, error) { return wire.DecodePeerListReq(b) }},
		{wire.Ping{Nonce: 7, Time: 8},
			func(b []byte) (any, error) { return wire.DecodePing(b) }},
		{wire.MissingChunksReq{Positions: []uint32{2, 5}},
			func(b []byte) (any, error) { return wire.DecodeMissingChunksReq(b) }},
		{wire.FollowReply{Contact: wire.Contact{Addr: "b.example:2"}, Policies: []wire.Section{wire.PoolSection(iss, s1, p2)}, More: true, Peers: peers},
			func(b []byte) (any, error) { return wire.DecodeFollowReply(b) }},
		{wire.FollowReply{},
			func(b []byte) (any, error) { return wire.DecodeFollowReply(b) }},
		{wire.PongReply{Contact: wire.Contact{Addr: "c.example:3"}, Policies: []wire.Section{wire.All}},
			func(b []byte) (any, error) { return wire.DecodePongReply(b) }},
		{wire.PeerListChunk{Peers: peers},
			func(b []byte) (any, error) { return wire.DecodePeerListChunk(b) }},
	}
	for _, c := range cases {
		b := c.v.(interface{ Encode() []byte }).Encode()
		got, err := c.decode(b)
		if err != nil {
			t.Fatalf("%T: %v", c.v, err)
		}
		if !reflect.DeepEqual(normalize(got), normalize(c.v)) {
			t.Fatalf("%T: got %+v, want %+v", c.v, got, c.v)
		}
		if _, err := c.decode(append(b, 0)); !errors.Is(err, wire.ErrMalformed) {
			t.Fatalf("%T: trailing byte accepted", c.v)
		}
		for i := 0; i < len(b); i++ {
			c.decode(b[:i]) // never panics
		}
	}
}

// normalize treats nil and empty lists alike.
func normalize(v any) any {
	switch x := v.(type) {
	case wire.FollowReply:
		if len(x.Policies) == 0 {
			x.Policies = nil
		}
		if len(x.Peers) == 0 {
			x.Peers = nil
		}
		return x
	}
	return v
}

func TestDiscoveryMalformed(t *testing.T) {
	bad := wire.FollowReq{Contact: wire.Contact{Addr: "no-port"}}.Encode()
	if _, err := wire.DecodeFollowReq(bad); err == nil {
		t.Fatal("contact without a port accepted")
	}
	bad = wire.FollowReq{Variant: 3}.Encode()
	if _, err := wire.DecodeFollowReq(bad); err == nil {
		t.Fatal("unknown variant accepted")
	}
	if _, err := wire.DecodePeerListReq(wire.PeerListReq{Variant: wire.NoPeers}.Encode()); err == nil {
		t.Fatal("peer list request for no peers accepted")
	}
	if _, err := wire.DecodeMissingChunksReq(wire.MissingChunksReq{Positions: []uint32{0}}.Encode()); err == nil {
		t.Fatal("chunk position 0 accepted")
	}
	if _, err := wire.DecodePeerListChunk(wire.PeerListChunk{Peers: []wire.PeerInfo{{}}}.Encode()); err == nil {
		t.Fatal("listed peer without contact accepted")
	}
	for _, a := range []string{"x:0", "x:70000", ":80", "x"} {
		if wire.CheckAddr(a) == nil {
			t.Errorf("%q accepted", a)
		}
	}
}

func TestSectionText(t *testing.T) {
	for _, s := range []wire.Section{wire.All, wire.IssuerSection(iss), wire.SeriesSection(iss, s1),
		wire.PoolSection(iss, s1, p1), wire.MintSection(iss, s1, p1, 77)} {
		got, err := wire.ParseSection(s.Text())
		if err != nil || got != s {
			t.Fatalf("%s: %+v %v", s.Text(), got, err)
		}
	}
	for _, bad := range []string{"", "*/*/*", "*/" + s1.String() + "/*/*", "zz/*/*/*", hash.Hash{}.String() + "/*/*/-1"} {
		if _, err := wire.ParseSection(bad); err == nil {
			t.Errorf("%q parsed", bad)
		}
	}
}

func TestSortSections(t *testing.T) {
	ss := []wire.Section{
		wire.PoolSection(iss, s2, p1), wire.MintSection(iss, s1, p1, 2), wire.SeriesSection(iss, s1),
		wire.MintSection(iss, s1, p1, 1), wire.PoolSection(iss, s1, p1), wire.All,
	}
	wire.SortSections(ss)
	want := []wire.Section{
		wire.All, wire.SeriesSection(iss, s1), wire.PoolSection(iss, s1, p1),
		wire.MintSection(iss, s1, p1, 1), wire.MintSection(iss, s1, p1, 2), wire.PoolSection(iss, s2, p1),
	}
	if !reflect.DeepEqual(ss, want) {
		t.Fatalf("%v", ss)
	}
	if wire.TypeFollowReq.String() != "follow-request" || !wire.TypePeerList.IsDiscoveryReply() || wire.TypeFollowReq.IsRequest() {
		t.Fatal("type helpers")
	}
}
