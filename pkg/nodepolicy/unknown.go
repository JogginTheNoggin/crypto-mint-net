package nodepolicy

import (
	"bytes"
	"encoding/json"
	"reflect"
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
