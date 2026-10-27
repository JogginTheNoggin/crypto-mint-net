package wire_test

import (
	"bufio"
	"bytes"
	"errors"
	"reflect"
	"testing"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/entry"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/testutil"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/tx"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/weight"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/wire"
)

var (
	iss = hash.Hash{1}
	s1  = hash.Hash{2}
	s2  = hash.Hash{3}
	p1  = hash.Hash{4}
	p2  = hash.Hash{5}
)

func TestCovers(t *testing.T) {
	cases := []struct {
		follow, msg wire.Section
		want        wire.Coverage
	}{
		{wire.SeriesSection(iss, s1), wire.SeriesSection(iss, s1), wire.Full},
		{wire.SeriesSection(iss, s1), wire.PoolSection(iss, s1, p1), wire.Full},
		{wire.SeriesSection(iss, s1), wire.MintSection(iss, s1, p1, 7), wire.Full},
		{wire.PoolSection(iss, s1, p1), wire.SeriesSection(iss, s1), wire.Partial},
		{wire.SeriesSection(iss, s2), wire.SeriesSection(iss, s1), wire.None},
		{wire.PoolSection(iss, s1, p2), wire.PoolSection(iss, s1, p1), wire.None},
		{wire.MintSection(iss, s1, p1, 3), wire.PoolSection(iss, s1, p1), wire.Partial},
		{wire.MintSection(iss, s1, p1, 3), wire.MintSection(iss, s1, p1, 4), wire.None},
		{wire.All, wire.MintSection(iss, s1, p1, 4), wire.Full},
		{wire.IssuerSection(iss), wire.All, wire.Partial},
		{wire.IssuerSection(hash.Hash{9}), wire.PoolSection(iss, s1, p1), wire.None},
	}
	for i, c := range cases {
		if got := c.follow.Covers(c.msg); got != c.want {
			t.Errorf("%d: %s covers %s = %d, want %d", i, c.follow, c.msg, got, c.want)
		}
	}
	if !wire.SeriesSection(iss, s1).Contains(wire.PoolSection(iss, s1, p1)) {
		t.Fatal("Contains")
	}
	if wire.All.String() != "*/*/*/*" || wire.MintSection(iss, s1, p1, 4).String() == "" {
		t.Fatal("String")
	}
}

func roundTrip(t *testing.T, m wire.Message) wire.Message {
	t.Helper()
	var buf bytes.Buffer
	if err := wire.WriteFrame(&buf, m); err != nil {
		t.Fatal(err)
	}
	got, n, err := wire.ReadFrame(bufio.NewReader(&buf), 0)
	if err != nil {
		t.Fatal(err)
	}
	if n != buf.Cap()-buf.Len() && n <= 0 {
		t.Fatalf("read %d bytes", n)
	}
	return got
}

func TestHeaderRoundTrip(t *testing.T) {
	for _, s := range []wire.Section{wire.All, wire.IssuerSection(iss), wire.SeriesSection(iss, s1), wire.PoolSection(iss, s1, p1), wire.MintSection(iss, s1, p1, 99)} {
		m := wire.Message{Header: wire.Header{
			Type: wire.TypeExtension, Section: s, RequestID: hash.Hash{7}, Chunk: 3,
			Flags: wire.FlagLast | wire.FlagMore, Stated: weight.Credit(200), Hops: 0,
		}, Body: []byte("body")}
		got := roundTrip(t, m)
		if got.Type != m.Type || got.Section != s || got.RequestID != m.RequestID || got.Chunk != 3 ||
			!got.Last() || !got.More() || got.Stated.Cmp(m.Stated) != 0 || string(got.Body) != "body" {
			t.Fatalf("got %+v", got)
		}
	}
}

func TestMalformedHeaders(t *testing.T) {
	good := wire.Message{Header: wire.Header{Type: wire.TypeTx, Section: wire.PoolSection(iss, s1, p1)}}.Encode()
	bad := [][]byte{
		{},
		append([]byte{2}, good[1:]...),           // wire version
		append(append([]byte{}, good[:3]...), 5), // section depth 5
		good[:20],                                // cut in the section
	}
	flags := append([]byte{}, good...)
	flags[3+1+96+32+4] = 0x80 // unknown flag
	bad = append(bad, flags)
	lead := append([]byte{}, good...)
	// Stated weight with a leading zero byte is not minimal.
	off := 3 + 1 + 96 + 32 + 4 + 1
	lead = append(append(append([]byte{}, lead[:off]...), 0, 0, 0, 2, 0, 1), lead[off+4:]...)
	bad = append(bad, lead)
	for i, b := range bad {
		if _, err := wire.Decode(b); !errors.Is(err, wire.ErrMalformed) {
			t.Errorf("%d: err = %v", i, err)
		}
	}
	if _, err := wire.Decode(good); err != nil {
		t.Fatal(err)
	}
}

func TestFrameLimit(t *testing.T) {
	var buf bytes.Buffer
	m := wire.Message{Header: wire.Header{Type: wire.TypeTx}, Body: make([]byte, 1000)}
	wire.WriteFrame(&buf, m)
	if _, _, err := wire.ReadFrame(bufio.NewReader(&buf), 500); !errors.Is(err, wire.ErrMalformed) {
		t.Fatalf("err = %v", err)
	}
	if err := wire.WriteFrame(&buf, wire.Message{Body: make([]byte, wire.MaxFrame)}); err == nil {
		t.Fatal("oversized frame written")
	}
}

func TestHello(t *testing.T) {
	h := wire.Hello{Protocol: 1, NodeID: hash.Hash{8}, Listen: "1.2.3.4:8646", Agent: "poo/test",
		Coverage: []wire.Section{wire.SeriesSection(iss, s1), wire.MintSection(iss, s2, p2, 1)}}
	got, err := wire.DecodeHello(h.Encode())
	if err != nil || !reflect.DeepEqual(got, h) {
		t.Fatalf("%+v %v", got, err)
	}
	if _, err := wire.DecodeHello(append(h.Encode(), 0)); !errors.Is(err, wire.ErrMalformed) {
		t.Fatal("trailing byte accepted")
	}
}

func TestRequests(t *testing.T) {
	c := wire.RequestCommon{Nonce: 42, Time: 1700000000, Limits: wire.Limits{ChunkSize: 1 << 20, EntryLimit: 10, HashLimit: 100, LayerLimit: 1000}}
	reqs := []wire.Request{
		wire.ExtensionReq{RequestCommon: c, From: hash.Hash{1}, Beat: weight.Credit(70), Variant: wire.EntriesWithLayers},
		wire.ExtensionReq{RequestCommon: c, Beat: weight.Zero},
		wire.HashChainReq{RequestCommon: c},
		wire.HashChainReq{RequestCommon: c, HasAnchor: true, Anchor: hash.Hash{3}, Dir: wire.After, Count: 9},
		wire.EntriesReq{RequestCommon: c, Variant: wire.EntriesOnly, Hashes: []hash.Hash{{1}, {2}}},
		wire.FullLedgerReq{RequestCommon: c, Variant: wire.EntriesWithLayers},
		wire.LayersReq{RequestCommon: c, Hashes: []hash.Hash{{5}}},
		wire.WeightReq{RequestCommon: c},
	}
	for _, q := range reqs {
		got, err := wire.DecodeRequest(q.Type(), q.Encode())
		if err != nil {
			t.Fatalf("%s: %v", q.Type(), err)
		}
		if !bytes.Equal(got.Encode(), q.Encode()) || got.Common() != c {
			t.Fatalf("%s: round trip differs", q.Type())
		}
		if !q.Type().IsRequest() || q.Type().IsReply() {
			t.Fatalf("%s: classification", q.Type())
		}
		if _, err := wire.DecodeRequest(q.Type(), append(q.Encode(), 1)); !errors.Is(err, wire.ErrMalformed) {
			t.Fatalf("%s: trailing byte accepted", q.Type())
		}
	}
	if _, err := wire.DecodeRequest(wire.TypeTx, nil); !errors.Is(err, wire.ErrMalformed) {
		t.Fatal("non-request decoded")
	}
	bad := wire.FullLedgerReq{RequestCommon: c, Variant: 7}.Encode()
	if _, err := wire.DecodeRequest(wire.TypeFullLedgerReq, bad); !errors.Is(err, wire.ErrMalformed) {
		t.Fatal("bad variant accepted")
	}
	// Ids differ with any field and ignore nothing in the body.
	a := wire.RequestID(wire.TypeWeightReq, wire.PoolSection(iss, s1, p1), reqs[7].Encode())
	c2 := c
	c2.Nonce++
	b := wire.RequestID(wire.TypeWeightReq, wire.PoolSection(iss, s1, p1), wire.WeightReq{RequestCommon: c2}.Encode())
	d := wire.RequestID(wire.TypeWeightReq, wire.PoolSection(iss, s1, p2), reqs[7].Encode())
	if a == b || a == d || a != wire.RequestID(wire.TypeWeightReq, wire.PoolSection(iss, s1, p1), reqs[7].Encode()) {
		t.Fatal("request ids")
	}
}

func sampleEntry(t *testing.T) *entry.Entry {
	s := testutil.ExampleSeries()
	k := testutil.NewKeys(s)
	e, _, err := entry.Build(s, s.PoolID(1), []*tx.Tx{k.Claim(4, testutil.P("alice", 100))}, 0)
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func TestReplyChunks(t *testing.T) {
	e := sampleEntry(t)
	ec := wire.EntriesChunk{Items: []wire.EntryItem{{Entry: e}, {Entry: e, HasLayers: true, Nonces: []uint64{1, 5, 9}}}}
	got, err := wire.DecodeEntriesChunk(ec.Encode())
	if err != nil || !bytes.Equal(got.Encode(), ec.Encode()) || len(got.Items[1].Nonces) != 3 {
		t.Fatalf("entries chunk: %v", err)
	}
	if ec.Items[1].Size() != len(wire.EntriesChunk{Items: ec.Items[1:]}.Encode())-4 {
		t.Fatal("EntryItem.Size")
	}
	hc := wire.HashChainChunk{FromStart: true, Hashes: []hash.Hash{{1}, {2}}}
	if got, err := wire.DecodeHashChainChunk(hc.Encode()); err != nil || !reflect.DeepEqual(got, hc) {
		t.Fatalf("hash chain chunk: %v", err)
	}
	he := wire.HeldEntriesChunk{Items: []wire.HeldEntry{{Hash: hash.Hash{1}}, {Hash: e.Hash(testutil.ExampleSeries()), Held: true, Item: wire.EntryItem{Entry: e}}}}
	if got, err := wire.DecodeHeldEntriesChunk(he.Encode()); err != nil || !bytes.Equal(got.Encode(), he.Encode()) {
		t.Fatalf("held entries chunk: %v", err)
	}
	lc := wire.LayersChunk{Items: []wire.HeldLayers{{Entry: hash.Hash{1}}, {Entry: hash.Hash{2}, Held: true, Nonces: []uint64{3}}}}
	if got, err := wire.DecodeLayersChunk(lc.Encode()); err != nil || !reflect.DeepEqual(got, lc) {
		t.Fatalf("layers chunk: %v", err)
	}
	n := wire.Notice{Tip: hash.Hash{4}}
	if got, err := wire.DecodeNotice(n.Encode()); err != nil || got != n {
		t.Fatalf("notice: %v", err)
	}
	if wire.DecodeEmpty(wire.TypeNotHeld, nil) != nil || wire.DecodeEmpty(wire.TypeNotHeld, []byte{1}) == nil {
		t.Fatal("DecodeEmpty")
	}
	for _, f := range []func([]byte) error{
		func(b []byte) error { _, err := wire.DecodeEntriesChunk(b); return err },
		func(b []byte) error { _, err := wire.DecodeHashChainChunk(b); return err },
		func(b []byte) error { _, err := wire.DecodeHeldEntriesChunk(b); return err },
		func(b []byte) error { _, err := wire.DecodeLayersChunk(b); return err },
		func(b []byte) error { _, err := wire.DecodeTx(b); return err },
		func(b []byte) error { _, err := wire.DecodeEntry(b); return err },
		func(b []byte) error { _, err := wire.DecodeLayer(b); return err },
		func(b []byte) error { _, err := wire.DecodeNotice(b); return err },
	} {
		if err := f([]byte{0xff, 0xff, 0xff, 0xff, 1}); !errors.Is(err, wire.ErrMalformed) {
			t.Errorf("garbage accepted: %v", err)
		}
	}
}

func TestTypeNames(t *testing.T) {
	if wire.TypeHello.String() != "hello" || wire.Type(999).String() != "type(999)" {
		t.Fatal("names")
	}
}

// Fuzz targets: every decoder must return an error, never panic, on any
// input. `go test` runs the seeds; `go test -fuzz` explores.
func FuzzDecode(f *testing.F) {
	f.Add(wire.Message{Header: wire.Header{Type: wire.TypeEntry, Section: wire.PoolSection(iss, s1, p1), Stated: weight.Credit(9)}, Body: []byte{1, 2}}.Encode())
	f.Add([]byte{1, 0, 1, 4})
	f.Fuzz(func(t *testing.T, b []byte) {
		m, err := wire.Decode(b)
		if err != nil {
			return
		}
		// Whatever decodes must re-encode to the same bytes.
		if !bytes.Equal(m.Encode(), b) {
			t.Fatalf("re-encoding differs")
		}
	})
}

func FuzzBodies(f *testing.F) {
	f.Add(uint16(wire.TypeHello), wire.Hello{Coverage: []wire.Section{wire.All}}.Encode())
	f.Add(uint16(wire.TypeExtensionReq), wire.ExtensionReq{Beat: weight.Credit(3)}.Encode())
	f.Add(uint16(wire.TypeHashChain), wire.HashChainChunk{Hashes: []hash.Hash{{1}}}.Encode())
	f.Add(uint16(wire.TypeLayers), wire.LayersChunk{Items: []wire.HeldLayers{{Held: true, Nonces: []uint64{1}}}}.Encode())
	f.Fuzz(func(t *testing.T, typ uint16, b []byte) {
		switch wire.Type(typ) {
		case wire.TypeHello:
			wire.DecodeHello(b)
		case wire.TypeExtension:
			wire.DecodeEntriesChunk(b)
		case wire.TypeHashChain:
			wire.DecodeHashChainChunk(b)
		case wire.TypeEntries:
			wire.DecodeHeldEntriesChunk(b)
		case wire.TypeLayers:
			wire.DecodeLayersChunk(b)
		case wire.TypeNotice:
			wire.DecodeNotice(b)
		default:
			wire.DecodeRequest(wire.Type(typ), b)
		}
	})
}
