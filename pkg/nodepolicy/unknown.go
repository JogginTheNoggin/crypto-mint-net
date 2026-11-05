package nodepolicy

import (
	"bytes"
	"encoding/json"
	"fmt"
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

// UnknownDropped lists the settings this release does not know that sit
// inside a list entry or a map value, such as series[0].x: Load cannot
// keep them, so the next Save drops them. Paths name entries by index or
// key (GREEN-057).
func (p Policy) UnknownDropped() []string { return slices.Clone(p.dropped) }

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
			ft, ok := fields[k]
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
	for _, e := range extras {
		if nb, ok := insertSetting(b, e.path, e.val); ok {
			b = nb
		}
	}
	return b
}

func insertSetting(obj []byte, path []string, val json.RawMessage) ([]byte, bool) {
	var m map[string]json.RawMessage
	if json.Unmarshal(obj, &m) != nil || m == nil {
		return nil, false
	}
	k := path[0]
	if len(path) == 1 {
		if _, ok := m[k]; ok {
			return nil, false
		}
		m[k] = val
	} else {
		child, ok := m[k]
		if !ok || bytes.Equal(bytes.TrimSpace(child), []byte("null")) {
			child = json.RawMessage("{}")
		}
		nc, ok := insertSetting(child, path[1:], val)
		if !ok {
			return nil, false
		}
		m[k] = nc
	}
	out, err := json.Marshal(m)
	return out, err == nil
}
