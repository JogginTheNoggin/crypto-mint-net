// Command crossversion-old fills in the "old" verdicts of
// pkg/policy/testdata/crossversion.json (D170) with a build of the code
// before D170. It lives under testdata, so this build never compiles it.
//
// To run it, from a checkout of this repo:
//
//	git worktree add /tmp/old 439b369
//	mkdir /tmp/old/crossversion-old
//	cp pkg/policy/testdata/crossversion-old/main.go /tmp/old/crossversion-old/
//	(cd /tmp/old && go run ./crossversion-old $PWD/pkg/policy/testdata/crossversion.json)
package main

import (
	"encoding/hex"
	"encoding/json"
	"log"
	"os"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/encoding"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/policy"
)

type rules struct{ max int }

func (rules) AllowsPolicyType(policy.TypeID) bool { return true }
func (rules) AllowsKeyAlg(policy.AlgID) bool      { return true }
func (r rules) MaxWitnessSize() int               { return r.max }

func main() {
	path := os.Args[1]
	b, err := os.ReadFile(path)
	if err != nil {
		log.Fatal(err)
	}
	var f map[string]json.RawMessage
	if err := json.Unmarshal(b, &f); err != nil {
		log.Fatal(err)
	}
	var tidS string
	var ceilings []int
	var vectors []map[string]any
	json.Unmarshal(f["tid"], &tidS)
	json.Unmarshal(f["ceilings"], &ceilings)
	json.Unmarshal(f["vectors"], &vectors)
	tid, err := hash.Parse(tidS)
	if err != nil {
		log.Fatal(err)
	}
	h, _ := hash.Lookup(hash.SHA256)
	for _, v := range vectors {
		addr, err := hash.Parse(v["address"].(string))
		if err != nil {
			log.Fatal(err)
		}
		wb, err := hex.DecodeString(v["witness"].(string))
		if err != nil {
			log.Fatal(err)
		}
		var old []string
		for _, max := range ceilings {
			r := encoding.NewReader(wb)
			w := policy.DecodeWitness(r)
			verdict := "valid"
			if r.Finish() != nil || policy.Default().Check(h, rules{max}, w, addr, tid) != nil {
				verdict = "invalid"
			}
			old = append(old, verdict)
		}
		v["old"] = old
	}
	out, _ := json.Marshal(vectors)
	f["vectors"] = out
	// Keep the field order of the file.
	var ordered struct {
		Note     json.RawMessage `json:"note"`
		OldBuild json.RawMessage `json:"old_build"`
		TID      json.RawMessage `json:"tid"`
		Ceilings json.RawMessage `json:"ceilings"`
		Vectors  json.RawMessage `json:"vectors"`
	}
	ordered.Note, ordered.OldBuild, ordered.TID, ordered.Ceilings, ordered.Vectors = f["note"], f["old_build"], f["tid"], f["ceilings"], f["vectors"]
	res, err := json.Marshal(ordered)
	if err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile(path, append(res, '\n'), 0o644); err != nil {
		log.Fatal(err)
	}
}
