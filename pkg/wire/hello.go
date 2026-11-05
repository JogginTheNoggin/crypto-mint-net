package wire

import (
	"errors"
	"fmt"
	"slices"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/encoding"
)

// Release, features and extensions (D186, WHITE-002). The hello carries
// the sender's release and the optional abilities it supports; the hello,
// ping and pong each end with an extension list, so a later release can
// add fields to them without cutting off older nodes. All of it is the
// peer's own claim: it is read only for display and for optional extras
// both peers list, and never decides validity, weight, selection, or
// whether a peer is kept, ranked, asked or chased.

// Limits on the new fields. They are frozen with wire version 1 at the
// first public tag (D186). MaxExtValue fits a post-quantum key or
// signature (W-14, D190); MaxFeatures is what the U8 count can say.
const (
	MaxRelease     = 64
	MaxFeatures    = 255
	MaxFeatureName = 32
	MaxExt         = 32
	MaxExtValue    = 64 << 10
)

// ErrBadExtras marks a message refused for its release, features or
// extensions list. It also wraps ErrMalformed, but a node closes the
// connection for it without a lasting offence (D186 term 4, Black's
// Round 33 condition): a later release may read these lists in a way an
// older one does not expect.
var ErrBadExtras = errors.New("bad release, features or extensions")

// extras runs read, the reading of a message's release, features or
// extensions, and reports whether it was what failed the reader.
func extras(r *encoding.Reader, read func()) bool {
	if r.Err() != nil {
		return false
	}
	read()
	return r.Err() != nil
}

// finishExtras is finish for a message with extras: a failure inside them
// is marked with ErrBadExtras.
func finishExtras(r *encoding.Reader, what string, bad bool) error {
	if bad {
		return fmt.Errorf("%w: %w: %s: %v", ErrMalformed, ErrBadExtras, what, r.Err())
	}
	return finish(r, what)
}

// Feature is one optional ability a node says it supports, with the
// version of it.
type Feature struct {
	Name string `json:"name"`
	Ver  uint16 `json:"ver"`
}

// Ext is one extension field: a registered tag and its value.
type Ext struct {
	Tag   uint16 `json:"tag"`
	Value []byte `json:"value"`
}

// Registered features. A name is never reused or repurposed; a new one
// gets a line in docs/protocol/wire.md. The first ones name abilities
// every node of release 0.1.0 has, so a later release can tell old from
// new.
const (
	FeatureLayersFrom = "layers-from" // types 27 and 53
	FeatureManyLayers = "many-layers" // type 52
	FeatureParts      = "parts"       // types 26, 50 and 51
	FeatureRelease    = "release"     // type 14
)

// OwnFeatures is what this build lists in its hello, sorted.
var OwnFeatures = []Feature{{FeatureLayersFrom, 1}, {FeatureManyLayers, 1}, {FeatureParts, 1}, {FeatureRelease, 1}}

// Registered extension tags. None is defined yet; a tag, like a type
// number, is never reused.

// ValidRelease reports whether s may be sent as a release: empty (not
// shared), or 1 to MaxRelease bytes of 0-9 A-Z a-z . - +.
func ValidRelease(s string) bool {
	if len(s) > MaxRelease {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !(c >= '0' && c <= '9' || c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || c == '.' || c == '-' || c == '+') {
			return false
		}
	}
	return true
}

func validFeatureName(s string) bool {
	if s == "" || len(s) > MaxFeatureName {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-') {
			return false
		}
	}
	return true
}

// Has reports whether fs lists name at version ver or later.
func Has(fs []Feature, name string, ver uint16) bool {
	i, ok := slices.BinarySearchFunc(fs, name, func(f Feature, n string) int {
		switch {
		case f.Name < n:
			return -1
		case f.Name > n:
			return 1
		}
		return 0
	})
	return ok && fs[i].Ver >= ver
}

func encodeFeatures(w *encoding.Writer, fs []Feature) {
	w.U8(uint8(len(fs)))
	for _, f := range fs {
		w.VarBytes([]byte(f.Name)).U16(f.Ver)
	}
}

func decodeFeatures(r *encoding.Reader) []Feature {
	n := int(r.U8())
	if n > MaxFeatures {
		r.Fail(fmt.Errorf("%d features, more than %d", n, MaxFeatures))
		return nil
	}
	var out []Feature
	for i := 0; i < n && r.Err() == nil; i++ {
		f := Feature{Name: string(r.VarBytes(MaxFeatureName)), Ver: r.U16()}
		if r.Err() != nil {
			return nil
		}
		if !validFeatureName(f.Name) {
			r.Fail(fmt.Errorf("feature name %q", f.Name))
			return nil
		}
		if len(out) > 0 && out[len(out)-1].Name >= f.Name {
			r.Fail(fmt.Errorf("features not sorted and unique at %q", f.Name))
			return nil
		}
		out = append(out, f)
	}
	return out
}

func encodeExt(w *encoding.Writer, es []Ext) {
	w.U8(uint8(len(es)))
	for _, e := range es {
		w.U16(e.Tag).VarBytes(e.Value)
	}
}

func decodeExt(r *encoding.Reader) []Ext {
	m := int(r.U8())
	if m > MaxExt {
		r.Fail(fmt.Errorf("%d extensions, more than %d", m, MaxExt))
		return nil
	}
	var out []Ext
	for i := 0; i < m && r.Err() == nil; i++ {
		e := Ext{Tag: r.U16(), Value: r.VarBytes(MaxExtValue)}
		if r.Err() != nil {
			return nil
		}
		if len(out) > 0 && out[len(out)-1].Tag >= e.Tag {
			r.Fail(fmt.Errorf("extension tags not increasing at %d", e.Tag))
			return nil
		}
		out = append(out, e)
	}
	return out
}

// checkSend refuses what a decoder would refuse, so a node never sends
// a message its peers must drop.
func checkSend(release string, fs []Feature, es []Ext) error {
	if !ValidRelease(release) {
		return fmt.Errorf("release %q", release)
	}
	if len(fs) > MaxFeatures || len(es) > MaxExt {
		return fmt.Errorf("too many features or extensions")
	}
	for i, f := range fs {
		if !validFeatureName(f.Name) || i > 0 && fs[i-1].Name >= f.Name {
			return fmt.Errorf("feature %q", f.Name)
		}
	}
	for i, e := range es {
		if len(e.Value) > MaxExtValue || i > 0 && es[i-1].Tag >= e.Tag {
			return fmt.Errorf("extension %d", e.Tag)
		}
	}
	return nil
}
