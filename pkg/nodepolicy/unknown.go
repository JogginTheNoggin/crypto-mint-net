package nodepolicy

import (
	"bytes"
	"encoding/json"
	"reflect"
	"slices"
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

// unknownSettings returns the settings in raw that Policy has no field
// for.
func unknownSettings(raw []byte) []extra {
	var out []extra
	collectUnknown(raw, reflect.TypeFor[Policy](), nil, &out)
	return out
}

func collectUnknown(raw []byte, t reflect.Type, path []string, out *[]extra) {
	var m map[string]json.RawMessage
	if json.Unmarshal(raw, &m) != nil {
		return
	}
	fields := jsonFields(t)
	for k, v := range m {
		ft, ok := fields[k]
		if !ok {
			*out = append(*out, extra{path: append(append([]string(nil), path...), k), val: append(json.RawMessage(nil), v...)})
			continue
		}
		for ft.Kind() == reflect.Pointer {
			ft = ft.Elem()
		}
		if ft.Kind() == reflect.Struct && !reflect.PointerTo(ft).Implements(unmarshalerType) {
			collectUnknown(v, ft, append(path, k), out)
		}
	}
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
		if _, ok := m[k]; !ok {
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
