package nodepolicy

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/brand"
	"reflect"
	"slices"
	"strconv"
	"strings"
)

// A node.json written by a newer release can hold settings this release
// does not know. Load keeps them and Save writes them back, so going back a
// release and changing a setting loses nothing the newer release set
// (White's W-1, D184). They are kept inside objects only: an unknown field
// inside a list entry (a series, a follow, a grind job) is not.

// extra is one setting this release does not know, by its path of keys.
type extra struct {
	path []string
	val  json.RawMessage
}

var unmarshalerType = reflect.TypeFor[json.Unmarshaler]()

// Limits on what Load keeps (RED-094): a kept value is written back as it
// is, so one deeply nested or very large would make every later save,
// load and status request pay for it. A setting over a limit is not kept
// and is named in UnknownDropped.
const (
	MaxKeptSettings = 256
	MaxKeptBytes    = 64 << 10 // each value, compacted
	MaxKeptDepth    = 64
)

// found is what Load learns of the keys in node.json that are not
// exactly a setting's name.
type found struct {
	kept   []extra  // unknown, kept and written back
	folded []string // a setting's name in another case or folding
	over   []string // unknown, over a limit, so not kept
}

// unknownSettings returns the settings in raw that Policy has no field
// for.
func unknownSettings(raw []byte) found {
	var f found
	collectUnknown(raw, reflect.TypeFor[Policy](), nil, &f)
	slices.Sort(f.folded)
	slices.Sort(f.over)
	return f
}

// fieldFor finds the field encoding/json fills from key k: the one named
// exactly k, or else one whose name equals k under Unicode case folding,
// as the decoder matches (RED-093). It returns the field's own name.
func fieldFor(fields map[string]reflect.Type, k string) (reflect.Type, string, bool) {
	if t, ok := fields[k]; ok {
		return t, k, true
	}
	names := make([]string, 0, len(fields))
	for name := range fields {
		names = append(names, name)
	}
	slices.Sort(names)
	for _, name := range names {
		if strings.EqualFold(name, k) {
			return fields[name], name, true
		}
	}
	return nil, "", false
}

func collectUnknown(raw []byte, t reflect.Type, path []string, f *found) {
	var m map[string]json.RawMessage
	if json.Unmarshal(raw, &m) != nil {
		return
	}
	fields := jsonFields(t)
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	for _, k := range keys {
		v := m[k]
		at := append(append([]string(nil), path...), k)
		ft, name, ok := fieldFor(fields, k)
		if !ok {
			var c bytes.Buffer
			switch {
			case len(f.kept) >= MaxKeptSettings:
				f.over = append(f.over, strings.Join(at, ".")+" (over "+fmt.Sprint(MaxKeptSettings)+" kept settings)")
			case json.Compact(&c, v) != nil:
			case c.Len() > MaxKeptBytes:
				f.over = append(f.over, strings.Join(at, ".")+fmt.Sprintf(" (over %d KiB)", MaxKeptBytes>>10))
			case jsonDepth(c.Bytes()) > MaxKeptDepth:
				f.over = append(f.over, strings.Join(at, ".")+fmt.Sprintf(" (nested over %d deep)", MaxKeptDepth))
			default:
				f.kept = append(f.kept, extra{path: at, val: json.RawMessage(c.Bytes())})
			}
			continue
		}
		if name != k {
			// The decoder applied it to the setting: it is that setting,
			// written back under its own name, never kept beside it.
			f.folded = append(f.folded, strings.Join(at, ".")+" (read as "+name+")")
		}
		for ft.Kind() == reflect.Pointer {
			ft = ft.Elem()
		}
		if ft.Kind() == reflect.Struct && !reflect.PointerTo(ft).Implements(unmarshalerType) {
			collectUnknown(v, ft, append(append([]string(nil), path...), name), f)
		}
	}
}

// jsonDepth returns how deeply the arrays and objects of the valid JSON
// value b nest.
func jsonDepth(b []byte) int {
	depth, most, inStr, esc := 0, 0, false, false
	for _, c := range b {
		switch {
		case inStr:
			switch {
			case esc:
				esc = false
			case c == '\\':
				esc = true
			case c == '"':
				inStr = false
			}
		case c == '"':
			inStr = true
		case c == '[' || c == '{':
			depth++
			most = max(most, depth)
		case c == ']' || c == '}':
			depth--
		}
	}
	return most
}

// jsonFields maps the JSON names of struct t's fields to their types, as
// encoding/json reads them (embedded structs included).
func jsonFields(t reflect.Type) map[string]reflect.Type {
	out := map[string]reflect.Type{}
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		tag := f.Tag.Get("json")
		if tag == "-" {
			continue
		}
		name, _, _ := strings.Cut(tag, ",")
		if f.Anonymous && name == "" {
			et := f.Type
			if et.Kind() == reflect.Pointer {
				et = et.Elem()
			}
			if et.Kind() == reflect.Struct {
				for k, v := range jsonFields(et) {
					out[k] = v
				}
				continue
			}
		}
		if !f.IsExported() {
			continue
		}
		if name == "" {
			name = f.Name
		}
		out[name] = f.Type
	}
	return out
}

// UnknownSettings lists, by dotted path, the settings Load found that this
// release does not know and Save keeps (W-1, D184), for a warning at start.
func (p Policy) UnknownSettings() []string {
	var out []string
	for _, e := range p.unknown {
		out = append(out, strings.Join(e.path, "."))
	}
	slices.Sort(out)
	return out
}

// UnknownDropped lists the settings this release does not know that the
// next Save drops: those inside a list entry or a map value, such as
// series[0].x, which Load cannot keep (paths name entries by index or
// key, GREEN-057), and those over a keeping limit, with the limit named
// (RED-094).
func (p Policy) UnknownDropped() []string { return slices.Clone(p.dropped) }

// FoldedSettings lists the keys in node.json that name a setting in
// another letter case or Unicode folding: encoding/json reads them as
// that setting, so Load applies them, and Save writes the setting under
// its own name in their place (RED-093).
func (p Policy) FoldedSettings() []string { return slices.Clone(p.folded) }

// NameSome joins the first n of names, adding how many more there are, so
// a warning stays one short line however many names there are (RED-095).
func NameSome(names []string, n int) string {
	if len(names) <= n {
		return strings.Join(names, ", ")
	}
	return strings.Join(names[:n], ", ") + fmt.Sprintf(" and %d more", len(names)-n)
}

// droppedSettings returns the paths of the unknown settings in raw that
// sit inside a list entry or a map value, at any depth.
func droppedSettings(raw []byte) []string {
	var out []string
	collectDropped(raw, reflect.TypeFor[Policy](), "", false, &out)
	slices.Sort(out)
	return out
}

func collectDropped(raw []byte, t reflect.Type, path string, inEntry bool, out *[]string) {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	join := func(k string) string {
		if path == "" {
			return k
		}
		return path + "." + k
	}
	switch {
	case t.Kind() == reflect.Struct && !reflect.PointerTo(t).Implements(unmarshalerType):
		var m map[string]json.RawMessage
		if json.Unmarshal(raw, &m) != nil {
			return
		}
		fields := jsonFields(t)
		for k, v := range m {
			ft, _, ok := fieldFor(fields, k)
			switch {
			case !ok && inEntry:
				*out = append(*out, join(k))
			case ok:
				collectDropped(v, ft, join(k), inEntry, out)
			}
		}
	case t.Kind() == reflect.Slice || t.Kind() == reflect.Array:
		var l []json.RawMessage
		if json.Unmarshal(raw, &l) != nil {
			return
		}
		for i, v := range l {
			collectDropped(v, t.Elem(), fmt.Sprintf("%s[%d]", path, i), true, out)
		}
	case t.Kind() == reflect.Map:
		var m map[string]json.RawMessage
		if json.Unmarshal(raw, &m) != nil {
			return
		}
		for k, v := range m {
			collectDropped(v, t.Elem(), join(k), true, out)
		}
	}
}

// withUnknown returns the encoded policy b with the kept settings put
// back where they were. A setting whose key this release now writes, or
// whose parent is no longer an object, is left out.
func withUnknown(b []byte, extras []extra) []byte {
	root := &extraTree{}
	for _, e := range extras {
		root.add(e.path, e.val)
	}
	// As before, b comes back as it was when no setting went in.
	if nb, n := root.insert(b); n > 0 {
		return nb
	}
	return b
}

// extraTree groups the kept settings by their path, so withUnknown
// decodes and encodes each object once however many settings go into it
// (ORANGE-042: one pass per setting made Save quadratic in their number).
type extraTree struct {
	vals map[string]json.RawMessage // settings to put in this object
	kids map[string]*extraTree      // settings to put in its child objects
}

func (t *extraTree) add(path []string, val json.RawMessage) {
	k := path[0]
	if len(path) == 1 {
		if t.vals == nil {
			t.vals = map[string]json.RawMessage{}
		}
		t.vals[k] = val
		return
	}
	if t.kids == nil {
		t.kids = map[string]*extraTree{}
	}
	c := t.kids[k]
	if c == nil {
		c = &extraTree{}
		t.kids[k] = c
	}
	c.add(path[1:], val)
}

// insert returns obj with t's settings put in, and how many went in;
// with none, obj is to be left as it was. Nothing goes into an obj that
// is not an object. A setting whose key obj already holds is left out,
// and a child that is missing or null is made an empty object first.
func (t *extraTree) insert(obj []byte) ([]byte, int) {
	var m map[string]json.RawMessage
	if json.Unmarshal(obj, &m) != nil || m == nil {
		return nil, 0
	}
	n := 0
	for k, v := range t.vals {
		if !holdsFolded(m, k) {
			m[k] = v
			n++
		}
	}
	for k, c := range t.kids {
		child, ok := m[k]
		if !ok || bytes.Equal(bytes.TrimSpace(child), []byte("null")) {
			child = json.RawMessage("{}")
		}
		if nc, cn := c.insert(child); cn > 0 {
			m[k] = nc
			n += cn
		}
	}
	if n == 0 {
		return nil, 0
	}
	out, err := json.Marshal(m)
	if err != nil {
		return nil, 0
	}
	return out, n
}

// holdsFolded says m holds k, or a key the decoder would take for k
// (RED-093): a kept setting never goes in beside one.
func holdsFolded(m map[string]json.RawMessage, k string) bool {
	if _, ok := m[k]; ok {
		return true
	}
	for x := range m {
		if strings.EqualFold(x, k) {
			return true
		}
	}
	return false
}

// indentKept returns the indented policy b with the kept settings put
// back, each value written compacted on one line: indenting a nested
// value would multiply its size by its depth (RED-094). Each value goes
// in first as a marker string no file can hold (a fresh random part),
// the whole is indented, and the markers are then replaced in one pass.
func indentKept(b []byte, extras []extra) []byte {
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return b
	}
	prefix := brand.TempPrefix + "kept-" + hex.EncodeToString(nonce[:]) + "-"
	marked := make([]extra, len(extras))
	pairs := make([]string, 0, 2*len(extras))
	for i, e := range extras {
		mk := prefix + strconv.Itoa(i)
		marked[i] = extra{path: e.path, val: json.RawMessage(strconv.Quote(mk))}
		pairs = append(pairs, strconv.Quote(mk), string(e.val))
	}
	var out bytes.Buffer
	if json.Indent(&out, withUnknown(b, marked), "", "  ") != nil {
		return b
	}
	return []byte(strings.NewReplacer(pairs...).Replace(out.String()))
}
