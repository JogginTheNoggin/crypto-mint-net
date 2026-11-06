package nodepolicy

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/fsutil"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/series"
)

// CheckedFileName is the node's own cache of the releases it checked in
// full, beside node.json (RED-075, D167). Checking the release of part k
// takes k hashes; with this cache, the next process (a reload, a restart,
// any poon command) checks only the release's witness. It is never part of
// a policy export, so a release another node wrote is always checked in
// full. A cache that is missing, damaged or does not match the release
// node.json keeps is not used: it costs time, never a result.
const CheckedFileName = "releases-checked.json"

// checkedEntry is one release the node checked: the release's part and
// the previous part's id it checked against the chain.
type checkedEntry struct {
	Series   hash.Hash `json:"series"`
	Part     uint64    `json:"part"`
	Previous hash.Hash `json:"previous"`
}

// trustChecked takes the cache in dir for the releases p keeps.
func trustChecked(dir string, p Policy) {
	kept := map[hash.Hash]series.Release{}
	for _, si := range p.Series {
		if si.Release != nil {
			kept[si.Release.Series] = *si.Release
		}
	}
	if len(kept) == 0 {
		return
	}
	b, err := os.ReadFile(filepath.Join(dir, CheckedFileName))
	if err != nil {
		return
	}
	var list []checkedEntry
	if json.Unmarshal(b, &list) != nil {
		return
	}
	for _, e := range list {
		if r, ok := kept[e.Series]; ok && r.Part == e.Part && r.Previous == e.Previous {
			series.TrustRelease(r)
		}
	}
}

// SaveChecked writes the cache in dir for the releases p keeps that this
// process checked in full (or took from the cache). Save calls it; a node
// calls it after building its series, so a node.json written before the
// cache gets one. A failure is ignored: the cache only saves time.
func SaveChecked(dir string, p Policy) {
	list := []checkedEntry{}
	for _, si := range p.Series {
		if r := si.Release; r != nil && series.ReleaseChecked(*r) {
			list = append(list, checkedEntry{r.Series, r.Part, r.Previous})
		}
	}
	path := filepath.Join(dir, CheckedFileName)
	if len(list) == 0 {
		if _, err := os.Stat(path); err != nil {
			return
		}
	}
	b, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return
	}
	b = append(b, '\n')
	if old, err := os.ReadFile(path); err == nil && string(old) == string(b) {
		return
	}
	_ = fsutil.WriteFile(path, b, 0o600)
}
