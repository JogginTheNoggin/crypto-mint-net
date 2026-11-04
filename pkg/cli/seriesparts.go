package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/nodepolicy"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/policy"
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
	var p series.Params
	if err := json.Unmarshal(b, &p); err != nil {
		return nil, fmt.Errorf("%s: %w", file, err)
	}
	return series.New(p)
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
	return writeJSON(out, struct {
		ID      string `json:"series_id"`
		Witness string `json:"issuer_witness"`
	}{s.ID().String(), nodepolicy.EncodeWitness(w)})
}

// seriesRelease is poo series release: sign part K of a variable series as
// its issuer, or check a release file.
func seriesRelease(args []string, out io.Writer) error {
	fs := newFlagSet("series release", out)
	part := fs.Uint64("part", 0, "the part to release, from 2 on (part 1 is released by the issuer witness on the series)")
	check := fs.String("check", "", "check this release file instead of signing one")
	seed := addSeed(fs, "the issuer's key's seed")
	pos, err := parseMixed(fs, args)
	if err != nil {
		return err
	}
	if len(pos) != 1 || (*part == 0) == (*check == "") {
		return errors.New("usage: poo series release FILE -part K [-seed-file F], or poo series release FILE -check RELEASE")
	}
	s, err := readSeriesFile(pos[0])
	if err != nil {
		return err
	}
	if !s.IsVariable() {
		return fmt.Errorf("series %s is fixed: it has no parts to release", s.ID().Short())
	}
	if *check != "" {
		b, err := os.ReadFile(*check)
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
	return writeJSON(out, r)
}

// releaseInfo is what a valid release proves.
type releaseInfo struct {
	Valid   bool   `json:"valid"`
	Part    uint64 `json:"part"`
	PartID  string `json:"part_id"`
	First   uint64 `json:"first_serial"`
	Last    uint64 `json:"last_serial"`
	Mints   uint64 `json:"mints_released"`
	Summary string `json:"summary"`
}

func releaseReport(s *series.Series, r series.Release) releaseInfo {
	first, last, _ := s.PartRange(r.Part)
	n, _ := s.MintsThrough(r.Part)
	return releaseInfo{Valid: true, Part: r.Part, PartID: r.ID(s.Hash()).String(), First: first, Last: last, Mints: n,
		Summary: fmt.Sprintf("releases part %d, serials %d to %d, and with it every part before: serials 0 to %d exist", r.Part, first, last, n-1)}
}

// seriesParts is poo series parts: the parts of a variable series, worked
// out from the series alone, released or not.
func seriesParts(args []string, out io.Writer) error {
	fs := newFlagSet("series parts", out)
	from := fs.Uint64("from", 1, "the first part to list")
	to := fs.Uint64("to", 0, "the last part to list (default: from + 9, or the largest number of parts)")
	pos, err := parseMixed(fs, args)
	if err != nil {
		return err
	}
	if len(pos) != 1 || *from == 0 {
		return errors.New("usage: poo series parts FILE [-from K] [-to K] (parts count from 1)")
	}
	s, err := readSeriesFile(pos[0])
	if err != nil {
		return err
	}
	if !s.IsVariable() {
		return fmt.Errorf("series %s is fixed: its %d mints are set for life", s.ID().Short(), s.Mints())
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
	type part struct {
		Part      uint64 `json:"part"`
		First     uint64 `json:"first_serial"`
		Last      uint64 `json:"last_serial"`
		FirstPool uint64 `json:"first_pool"`
		LastPool  uint64 `json:"last_pool"`
		ID        string `json:"part_id"`
	}
	info := struct {
		ID       string `json:"series_id"`
		PartSize uint64 `json:"part_size"`
		MaxParts uint64 `json:"max_parts,omitempty"`
		Parts    []part `json:"parts"`
	}{ID: s.ID().String(), PartSize: s.PartSize(), MaxParts: s.MaxParts()}
	for k := *from; k <= last; k++ {
		f, l, err := s.PartRange(k)
		if err != nil {
			if len(info.Parts) == 0 {
				return err
			}
			break
		}
		id, _ := s.PartID(k)
		info.Parts = append(info.Parts, part{k, f, l, s.PoolOf(f), s.PoolOf(l), id.String()})
	}
	return writeJSON(out, info)
}
