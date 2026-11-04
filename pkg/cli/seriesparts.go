package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"os"
	"path/filepath"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/nodepolicy"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/policy"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/rpc"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/series"
)

// An issuer's tools for a series (D163): sign it, and for a variable
// series list its parts and release the next one. All of them are offline:
// a release is published outside the network, as a series is (PoO section
// 16.8).

// readSeriesFile reads a series' params from a JSON file, as poo series new
// prints them, and builds the series.
func readSeriesFile(file string) (*series.Series, error) {
	b, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}
	// A params file or a signed series file (poo series sign) both read.
	info, err := readSeriesInfo(b)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", file, err)
	}
	return series.New(info.Params)
}

// issuerSigner reads the issuer's seed and checks that its 1-of-1
// address is the series' issuer, so a wrong key is refused before
// anything is printed.
func issuerSigner(seed *seedFlags, s *series.Series, out io.Writer) (policy.Signer, error) {
	iss := s.Params().Issuer
	if iss == nil {
		return nil, errors.New("the series names no issuer (poo series new -issuer ADDR)")
	}
	signer, err := seed.signer(out)
	if err != nil {
		return nil, err
	}
	if a := s.Address(policy.SingleKey(signer.Alg(), signer.Public())); a != *iss {
		return nil, fmt.Errorf("this key's 1-of-1 address is %s, not the series' issuer %s", a, iss)
	}
	return signer, nil
}

// seriesSign is poo series sign: the issuer witness over the series'
// issuer message, in the hex node.json takes as issuer_witness.
func seriesSign(args []string, out io.Writer) error {
	fs := newFlagSet("series sign", out)
	seed := addSeed(fs, "the issuer's key's seed")
	pos, err := parseMixed(fs, args)
	if err != nil {
		return err
	}
	if len(pos) != 1 {
		return errors.New("usage: poo series sign FILE [-seed-file F]")
	}
	s, err := readSeriesFile(pos[0])
	if err != nil {
		return err
	}
	signer, err := issuerSigner(seed, s, out)
	if err != nil {
		return err
	}
	w := policy.SingleKeyWitness(signer, s.IssuerMessageTID())
	// The series with its witness: poo init -series takes this file, and
	// node.json's issuer_witness is the witness.
	return writeJSON(out, signedSeries{ID: s.ID().String(), Params: s.Params(), IssuerWitness: nodepolicy.EncodeWitness(w)})
}

// seriesRelease is poo series release: sign part K of a variable series as
// its issuer, or check a release file.
func seriesRelease(args []string, out io.Writer) error {
	fs := newFlagSet("series release", out)
	part := fs.Uint64("part", 0, "the part to release, from 2 on (part 1 is released by the issuer witness on the series)")
	check := fs.String("check", "", "check this release file (- for standard input) instead of signing one")
	seed := addSeed(fs, "the issuer's key's seed")
	pos, err := parseMixed(fs, args)
	if err != nil {
		return err
	}
	if len(pos) != 1 || (*part == 0) == (*check == "") {
		return errors.New("usage: poo series release FILE -part K [-seed-file F], or poo series release FILE -check RELEASE|-")
	}
	s, err := readSeriesFile(pos[0])
	if err != nil {
		return err
	}
	if !s.IsVariable() {
		return fmt.Errorf("series %s is fixed: it has no parts to release", s.ID().Short())
	}
	if *check != "" {
		// - reads the release from standard input, so a caller can check
		// one it has just signed without writing it to disk (GUI-027).
		var b []byte
		if *check == "-" {
			b, err = io.ReadAll(inputOf(out))
		} else {
			b, err = os.ReadFile(*check)
		}
		if err != nil {
			return err
		}
		var r series.Release
		if err := json.Unmarshal(b, &r); err != nil {
			return fmt.Errorf("%s: %w", *check, err)
		}
		if err := s.CheckRelease(r); err != nil {
			return err
		}
		return writeJSON(out, releaseReport(s, r))
	}
	r, err := s.Unsigned(*part)
	if err != nil {
		return err
	}
	signer, err := issuerSigner(seed, s, out)
	if err != nil {
		return err
	}
	r.Witness = policy.SingleKeyWitness(signer, s.ReleaseMessage(r.ID(s.Hash())))
	if err := s.CheckRelease(r); err != nil {
		return err // unreachable for a key that matched the issuer
	}
	o, err := releaseOut(s, r)
	if err != nil {
		return err
	}
	return writeJSON(out, o)
}

// releaseOut is a release as poo series release prints it: the release's
// own fields, which are what -check and a node read, then the part's id and
// ranges for people and the GUI's preview. A reader ignores the extra keys
// and works them out again from the series.
func releaseOut(s *series.Series, r series.Release) (any, error) {
	b, err := json.Marshal(r)
	if err != nil {
		return nil, err
	}
	var rel struct {
		Series   string `json:"series"`
		Part     uint64 `json:"part"`
		Previous string `json:"previous"`
		Witness  string `json:"witness"`
	}
	if err := json.Unmarshal(b, &rel); err != nil {
		return nil, err
	}
	row, err := partRowOf(s, r.Part)
	if err != nil {
		return nil, err
	}
	return struct {
		Series    string `json:"series"`
		Part      uint64 `json:"part"`
		Previous  string `json:"previous"`
		Witness   string `json:"witness"`
		PartID    string `json:"part_id"`
		First     uint64 `json:"first_serial"`
		Last      uint64 `json:"last_serial"`
		FirstPool uint64 `json:"first_pool"`
		LastPool  uint64 `json:"last_pool"`
		Mints     uint64 `json:"mints"`
		Units     uint64 `json:"units"`
	}{rel.Series, rel.Part, rel.Previous, rel.Witness, row.ID, row.First, row.Last, row.FirstPool, row.LastPool, row.Mints, row.Units}, nil
}

// releaseInfo is what a valid release proves.
type releaseInfo struct {
	Valid   bool   `json:"valid"`
	Part    uint64 `json:"part"`
	PartID  string `json:"part_id"`
	First   uint64 `json:"first_serial"`
	Last    uint64 `json:"last_serial"`
	FirstP  uint64 `json:"first_pool"`
	LastP   uint64 `json:"last_pool"`
	Mints   uint64 `json:"mints_released"`
	Summary string `json:"summary"`
}

func releaseReport(s *series.Series, r series.Release) releaseInfo {
	first, last, _ := s.PartRange(r.Part)
	n, _ := s.MintsThrough(r.Part)
	return releaseInfo{Valid: true, Part: r.Part, PartID: r.ID(s.Hash()).String(), First: first, Last: last,
		FirstP: s.PoolOf(first), LastP: s.PoolOf(last), Mints: n,
		Summary: fmt.Sprintf("releases part %d, serials %d to %d, and with it every part before: serials 0 to %d exist", r.Part, first, last, n-1)}
}

// seriesParts is poo series parts: the parts of a variable series, worked
// out from the series alone, released or not.
func seriesParts(args []string, out io.Writer) error {
	fs := newFlagSet("series parts", out)
	from := fs.Uint64("from", 1, "the first part to list")
	to := fs.Uint64("to", 0, "the last part to list (default: from + 9, or the largest number of parts)")
	o := addOutput(fs)
	pos, err := parseMixed(fs, args)
	if err != nil {
		return err
	}
	if err := o.check(); err != nil {
		return err
	}
	if len(pos) != 1 || *from == 0 {
		return errors.New("usage: poo series parts FILE [-from K] [-to K] [-o json|table] (parts count from 1)")
	}
	s, err := readSeriesFile(pos[0])
	if err != nil {
		return err
	}
	if !s.IsVariable() {
		return fmt.Errorf("series %s is fixed: its %d mints are set for life", s.ID().Short(), s.Mints())
	}
	if m := s.MaxParts(); m != 0 && *from > m {
		return fmt.Errorf("-from %d is past the last part: series %s has at most %d parts", *from, s.ID().Short(), m)
	}
	last := *to
	if last == 0 {
		last = *from + 9
		if m := s.MaxParts(); m != 0 && last > m {
			last = m
		}
	}
	if last < *from || last-*from >= 1000 {
		return errors.New("-to must be from -from to -from + 999")
	}
	info := struct {
		ID       string    `json:"series_id"`
		PartSize uint64    `json:"part_size"`
		MaxParts uint64    `json:"max_parts,omitempty"`
		Parts    []partRow `json:"parts"`
	}{ID: s.ID().String(), PartSize: s.PartSize(), MaxParts: s.MaxParts()}
	if info.Parts, err = partRows(s, *from, last); err != nil {
		return err
	}
	return o.write(out, info)
}

// partRow is one part as the tools print it: its serials and pools as
// ranges, never as lists, so a part of any size is one short row (GUI
// ORANGE-018, ORANGE-034).
type partRow struct {
	Part      uint64 `json:"part"`
	First     uint64 `json:"first_serial"`
	Last      uint64 `json:"last_serial"`
	FirstPool uint64 `json:"first_pool"`
	LastPool  uint64 `json:"last_pool"`
	Mints     uint64 `json:"mints"`
	Units     uint64 `json:"units"`
	ID        string `json:"part_id"`
}

// partRowOf returns part k's row.
func partRowOf(s *series.Series, k uint64) (partRow, error) {
	f, l, err := s.PartRange(k)
	if err != nil {
		return partRow{}, err
	}
	id, err := s.PartID(k)
	if err != nil {
		return partRow{}, err
	}
	n := l - f + 1
	units := n * s.Supply()
	if n != 0 && units/n != s.Supply() {
		units = 0 // more than 64 bits hold; left out
	}
	return partRow{k, f, l, s.PoolOf(f), s.PoolOf(l), n, units, id.String()}, nil
}

// partRows returns the rows of parts from to last, stopping early at the
// last part the series can have; it fails only when there is none.
func partRows(s *series.Series, from, last uint64) ([]partRow, error) {
	var rows []partRow
	for k := from; k <= last; k++ {
		r, err := partRowOf(s, k)
		if err != nil {
			if len(rows) == 0 {
				return nil, err
			}
			break
		}
		rows = append(rows, r)
	}
	return rows, nil
}

// seriesAddRelease is poo series add-release: give the node a release of a
// variable series (D166). A running node checks it, keeps it in node.json
// when it is newer than the release kept, and reloads to follow the new
// parts; with the node stopped, node.json is changed under its lock.
func seriesAddRelease(args []string, out io.Writer) error {
	fs := newFlagSet("series add-release", out)
	w := addWhere(fs)
	o := addOutput(fs)
	pos, err := parseMixed(fs, args)
	if err != nil {
		return err
	}
	if len(pos) != 1 {
		return errors.New("usage: poo series add-release RELEASE (a file poo series release printed, - for standard input)")
	}
	if err := o.check(); err != nil {
		return err
	}
	if viewOnlyOf(out) {
		return errViewOnly("series add-release")
	}
	var b []byte
	if pos[0] == "-" {
		b, err = io.ReadAll(inputOf(out))
	} else {
		b, err = os.ReadFile(pos[0])
	}
	if err != nil {
		return err
	}
	var r series.Release
	if err := json.Unmarshal(b, &r); err != nil {
		return fmt.Errorf("%s: not a release: %w", pos[0], err)
	}
	dir := w.resolve()
	var nodeErr error
	if cl, err := dialThroughReload(out, dir); err == nil {
		req := rpc.ReleaseRequest{Release: r}
		rep, err := callThroughReload(out, dir, cl, func(c *rpc.Client) (rpc.ReleaseReply, error) { return c.AddRelease(req) })
		if err == nil {
			return o.write(out, releaseNote(rep))
		}
		var re *rpc.Error
		if errors.As(err, &re) {
			return err
		}
		nodeErr = err
	}
	unlock, err := lockPolicy(dir, "node policy", nodeErr)
	if err != nil {
		return err
	}
	defer unlock()
	path := filepath.Join(dir, nodepolicy.FileName)
	p, err := nodepolicy.Load(path)
	if err != nil {
		return err
	}
	res, err := p.AddRelease(r)
	if err != nil {
		return err
	}
	rep := rpc.ReleaseReply{ReleaseAdded: res}
	if res.Added {
		if err := nodepolicy.Save(path, p); err != nil {
			return err
		}
		rep.Saved = true
	}
	return o.write(out, releaseNote(rep))
}

// seriesFollowed is poo series followed: the series the running node
// follows, as GET /v1/series gives them. A variable series carries what
// this node has released: the newest part, the next part the issuer can
// release, the mints and units so far, and the newest release (D166). It
// reads and works in view-only mode.
func seriesFollowed(args []string, out io.Writer) error {
	fs := newFlagSet("series followed", out)
	w := addWhere(fs)
	o := addOutput(fs)
	pos, err := parseMixed(fs, args)
	if err != nil {
		return err
	}
	if len(pos) != 0 {
		return errors.New("usage: poo series followed [-o json|table] (the series the running node follows)")
	}
	if err := o.check(); err != nil {
		return err
	}
	cl, err := dialNode(out, w.resolve())
	if err != nil {
		return err
	}
	known, err := cl.Series()
	if err != nil {
		return err
	}
	if known == nil {
		known = []rpc.SeriesInfo{}
	}
	if *o.format == "table" {
		// The params are fourteen fields, which a table cell can only
		// count ({14}); a row says what a reader asks of a followed
		// series instead. The JSON, which the GUI reads, is unchanged
		// (GREEN-050).
		return o.write(out, struct {
			Series []followedRow `json:"series"`
		}{followedRows(known)})
	}
	return o.write(out, struct {
		Series []rpc.SeriesInfo `json:"series"`
	}{known})
}

// followedRow is one followed series in poo series followed -o table.
type followedRow struct {
	ID       hash.Hash `json:"id"`
	Supply   string    `json:"supply"`
	Pools    uint64    `json:"pools"`
	Mints    uint64    `json:"mints"`
	Units    string    `json:"units"`
	Released uint64    `json:"released_through,omitempty"`
	NextPart uint64    `json:"next_part,omitempty"`
}

func followedRows(known []rpc.SeriesInfo) []followedRow {
	rows := make([]followedRow, 0, len(known))
	for _, k := range known {
		r := followedRow{ID: k.ID, Supply: "fixed", Pools: k.Pools, Mints: k.Params.Mints}
		if v := k.Variable; v != nil {
			r.Supply, r.Mints, r.Units, r.Released, r.NextPart = "variable", v.Mints, v.Units, v.Released, v.NextPart
		} else {
			r.Units = new(big.Int).Mul(new(big.Int).SetUint64(k.Params.Mints), new(big.Int).SetUint64(k.Params.Supply)).String()
		}
		rows = append(rows, r)
	}
	return rows
}

// releaseOut is poo series add-release's reply, with a note when the
// release changed nothing, so "added": false is not read as a refusal
// (GREEN-050).
type addedOut struct {
	rpc.ReleaseReply
	Note string `json:"note,omitempty"`
}

func releaseNote(rep rpc.ReleaseReply) addedOut {
	o := addedOut{ReleaseReply: rep}
	switch {
	case rep.Added:
	case rep.Part < rep.Released:
		o.Note = fmt.Sprintf("part %d is already released here: the kept release of part %d proves every part before it; nothing changed", rep.Part, rep.Released)
	default:
		o.Note = fmt.Sprintf("part %d is the release this node already keeps; nothing changed", rep.Part)
	}
	return o
}
