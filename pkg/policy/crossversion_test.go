package policy

import (
	"encoding/hex"
	"encoding/json"
	"flag"
	"os"
	"testing"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/encoding"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
)

var updateCross = flag.Bool("update-cross", false, "rewrite testdata/crossversion.json's vectors (the old build's verdicts must then be made again, see the file's note)")

// crossFile is the pinned cross-version vector of D170 (Red's condition 4,
// Black's condition 2). Each vector is an encoded witness for an address
// and tid, judged under three series witness ceilings. "old" holds the
// verdicts of a real build of the code before D170 (main @ 439b369, with
// the network-wide caps), made once with the program in
// testdata/crossversion-old/ and pinned. "new" holds this build's.
type crossFile struct {
	Note     string        `json:"note"`
	OldBuild string        `json:"old_build"`
	TID      string        `json:"tid"`
	Ceilings []int         `json:"ceilings"`
	Vectors  []crossVector `json:"vectors"`
}

type crossVector struct {
	Name    string   `json:"name"`
	Address string   `json:"address"`
	Witness string   `json:"witness"`
	Old     []string `json:"old"`
	New     []string `json:"new"`
	// OldUnreadable says why a series that allows this witness cannot be
	// built by the old build at all, so the builds never judge it side by
	// side: a new algorithm number (PoON 16.7).
	OldUnreadable string `json:"old_unreadable,omitempty"`
}

type crossRules struct{ max int }

func (crossRules) AllowsPolicyType(TypeID) bool { return true }
func (crossRules) AllowsKeyAlg(AlgID) bool      { return true }
func (c crossRules) MaxWitnessSize() int        { return c.max }

const crossPath = "testdata/crossversion.json"

func crossVerdict(h hash.Func, b []byte, addr, tid hash.Hash, max int) string {
	r := encoding.NewReader(b)
	w := DecodeWitness(r)
	if r.Finish() != nil {
		return "invalid"
	}
	if Default().Check(h, crossRules{max}, w, addr, tid) != nil {
		return "invalid"
	}
	return "valid"
}

// TestCrossVersion is the two-half cross-version test:
//   - every witness the old build takes, this build takes, under every
//     ceiling;
//   - under a ceiling at or below 64 KiB, the two builds decide every
//     witness the same;
//   - a witness over the old caps, under no ceiling or a larger one, is
//     refused by the old build and taken by this one.
func TestCrossVersion(t *testing.T) {
	h := sha(t)
	if *updateCross {
		writeCrossVectors(t, h)
	}
	b, err := os.ReadFile(crossPath)
	if err != nil {
		t.Fatal(err)
	}
	var f crossFile
	if err := json.Unmarshal(b, &f); err != nil {
		t.Fatal(err)
	}
	tid, err := hash.Parse(f.TID)
	if err != nil {
		t.Fatal(err)
	}
	loosened := 0
	for _, v := range f.Vectors {
		if len(v.Old) != len(f.Ceilings) || len(v.New) != len(f.Ceilings) {
			t.Fatalf("%s: the old build's verdicts are missing (see the file's note)", v.Name)
		}
		addr, err := hash.Parse(v.Address)
		if err != nil {
			t.Fatal(err)
		}
		wb, err := hex.DecodeString(v.Witness)
		if err != nil {
			t.Fatal(err)
		}
		for i, max := range f.Ceilings {
			got := crossVerdict(h, wb, addr, tid, max)
			if got != v.New[i] {
				t.Errorf("%s, ceiling %d: this build says %s, the vector %s", v.Name, max, got, v.New[i])
			}
			if v.OldUnreadable != "" {
				continue
			}
			if v.Old[i] == "valid" && got != "valid" {
				t.Errorf("%s, ceiling %d: the old build took it and this one refuses it", v.Name, max)
			}
			if max > 0 && max <= 64*1024 && got != v.Old[i] {
				t.Errorf("%s, ceiling %d: the builds differ under a ceiling at or below 64 KiB", v.Name, max)
			}
			if v.Old[i] == "invalid" && got == "valid" {
				loosened++
				if max > 0 && max <= 64*1024 {
					t.Errorf("%s: loosened under ceiling %d", v.Name, max)
				}
			}
		}
	}
	if loosened == 0 {
		t.Fatal("no vector shows the loosening half")
	}
}

func writeCrossVectors(t *testing.T, h hash.Func) {
	tid := h([]byte("D170 cross-version"))
	msg := Message(tid)
	sigs := func(p ThresholdParams, signers []Signer, idx ...int) []byte {
		var d ThresholdData
		for _, i := range idx {
			d.Sigs = append(d.Sigs, Sig{Index: uint16(i), Signature: signers[i].Sign(msg)})
		}
		return d.Encode()
	}
	thr := func(alg AlgID, label string, n, k int) (ThresholdParams, []Signer) {
		var p ThresholdParams
		var ss []Signer
		for i := 0; i < n; i++ {
			s := newSigner(t, alg, label+string(rune('a'+i)))
			ss = append(ss, s)
			p.Keys = append(p.Keys, Key{Alg: alg, Public: s.Public()})
		}
		p.K = uint16(k)
		return p, ss
	}
	all := func(n int) []int {
		out := make([]int, n)
		for i := range out {
			out[i] = i
		}
		return out
	}
	var ws []struct {
		name string
		w    Witness
	}
	add := func(name string, w Witness) {
		ws = append(ws, struct {
			name string
			w    Witness
		}{name, w})
	}

	p, ss := thr(AlgEd25519, "ed", 1, 1)
	add("ed25519 1 of 1", Witness{p.Policy(), sigs(p, ss, 0)})
	bad := sigs(p, ss, 0)
	bad[len(bad)-1] ^= 1
	add("ed25519 1 of 1, signature changed", Witness{p.Policy(), bad})
	short := ThresholdData{Sigs: []Sig{{0, ss[0].Sign(msg)[:63]}}}.Encode()
	add("ed25519 1 of 1, signature one byte short", Witness{p.Policy(), short})

	p, ss = thr(AlgMLDSA87, "m87", 16, 14)
	add("ml-dsa-87 14 of 16 (fits the old caps)", Witness{p.Policy(), sigs(p, ss, all(14)...)})
	p.K = 16
	add("ml-dsa-87 16 of 16 (data over 64 KiB, RED-077)", Witness{p.Policy(), sigs(p, ss, all(16)...)})

	p, ss = thr(AlgSLHDSASHA2128s, "slh", 9, 9)
	add("slh-dsa-sha2-128s 9 of 9 (data over 64 KiB, RED-079)", Witness{p.Policy(), sigs(p, ss, all(9)...)})

	p, ss = thr(AlgSLHDSASHA2256s, "slh256", 1, 1)
	add("slh-dsa-sha2-256s 1 of 1 (a signature over 8,192 bytes, algorithm 11)", Witness{p.Policy(), sigs(p, ss, 0)})

	// k-of-policies with params over 64 KiB: two 16-key ML-DSA-87
	// thresholds, one signature from the first.
	p1, ss1 := thr(AlgMLDSA87, "big1", 16, 1)
	p2, _ := thr(AlgMLDSA87, "big2", 16, 1)
	kp := KOfPoliciesParams{K: 1, Policies: []Policy{p1.Policy(), p2.Policy()}}
	add("any of two 16-key ml-dsa-87 policies (params over 64 KiB)", Witness{kp.Policy(), KOfPoliciesData{Parts: []Part{{0, sigs(p1, ss1, 0)}}}.Encode()})

	pe, se := thr(AlgEd25519, "edh", 1, 1)
	pre := []byte("D170 preimage")
	hl, err := NewHashlock(hash.SHA256, pre)
	if err != nil {
		t.Fatal(err)
	}
	both := AllOf(pe.Policy(), hl.Policy())
	add("ed25519 and a hashlock", Witness{both, KOfPoliciesData{Parts: []Part{{0, sigs(pe, se, 0)}, {1, HashlockData(pre)}}}.Encode()})

	f := crossFile{
		Note:     "D170 cross-version vector. \"old\" was made by a real build of main @ 439b369 (go run ./crossversion-old in a worktree of that commit, see testdata/crossversion-old/main.go) and must never be edited by hand. Rewriting the vectors (-update-cross) empties \"old\", which must then be made again from 439b369.",
		OldBuild: "439b369",
		TID:      tid.String(),
		Ceilings: []int{0, 64 * 1024, 128 * 1024},
	}
	for _, x := range ws {
		e := encoding.NewWriter(x.w.Size())
		x.w.Encode(e)
		v := crossVector{Name: x.name, Address: Address(h, x.w.Policy).String(), Witness: hex.EncodeToString(e.Bytes())}
		if usesAlg(x.w.Policy, AlgSLHDSASHA2256s) {
			v.OldUnreadable = "the old build does not know algorithm 11, so it cannot build a series that lists it: unreadable, not a disagreement (PoON 16.7)"
		}
		for _, max := range f.Ceilings {
			v.New = append(v.New, crossVerdict(h, e.Bytes(), Address(h, x.w.Policy), tid, max))
		}
		f.Vectors = append(f.Vectors, v)
	}
	b, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(crossPath, append(b, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
}

func usesAlg(p Policy, alg AlgID) bool {
	if p.Type != TypeThreshold {
		return false
	}
	tp, err := DecodeThresholdParams(Default(), p.Params)
	if err != nil {
		return false
	}
	for _, k := range tp.Keys {
		if k.Alg == alg {
			return true
		}
	}
	return false
}
