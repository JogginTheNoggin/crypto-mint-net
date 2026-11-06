// Package keepjson keeps the keys of a JSON object that a struct has no
// field for, so a file a later release wrote survives being read and
// written back by this one (W-23, W-24, D195). It works on one level: a
// type that wants its nested objects kept too keeps them itself.
//
// A type uses it from its own UnmarshalJSON and MarshalJSON:
//
//	func (f *File) UnmarshalJSON(b []byte) error {
//		type plain File
//		var p plain
//		if err := json.Unmarshal(b, &p); err != nil {
//			return err
//		}
//		kept, err := keepjson.Split(b, p)
//		...
//	}
package keepjson

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"slices"
	"strings"
)

// The limits node.json uses for the settings it keeps (D184).
const (
	MaxKeys  = 256
	MaxBytes = 64 << 10 // each value, compacted
	MaxDepth = 64
)

// Kept holds an object's keys that its struct has no field for, each
// value compacted.
type Kept map[string]json.RawMessage

// Split returns the keys of the JSON object raw that the struct v (or the
// struct v points to) has no field for, as encoding/json matches them:
// a key that is a field's name in another letter case is that field. A
// value that is not an object keeps nothing. Over a limit, it fails.
func Split(raw []byte, v any) (Kept, error) {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, nil // not an object: the caller's own decode says so
	}
	names := fieldNames(reflect.TypeOf(v))
	var out Kept
	for k, val := range m {
		if slices.ContainsFunc(names, func(n string) bool { return strings.EqualFold(n, k) }) {
			continue
		}
		var c bytes.Buffer
		if err := json.Compact(&c, val); err != nil {
			return nil, err
		}
		switch {
		case len(out) >= MaxKeys:
			return nil, fmt.Errorf("more than %d fields this release does not know", MaxKeys)
		case c.Len() > MaxBytes:
			return nil, fmt.Errorf("the field %q, which this release does not know, is over %d bytes", k, MaxBytes)
		case depth(c.Bytes()) > MaxDepth:
			return nil, fmt.Errorf("the field %q, which this release does not know, is nested over %d levels", k, MaxDepth)
		}
		if out == nil {
			out = Kept{}
		}
		out[k] = c.Bytes()
	}
	return out, nil
}

// Join adds the kept keys, in name order, at the end of b, a JSON object
// as json.Marshal writes it. A key b already holds is not added again.
func Join(b []byte, k Kept) ([]byte, error) {
	if len(k) == 0 {
		return b, nil
	}
	var have map[string]json.RawMessage
	if err := json.Unmarshal(b, &have); err != nil {
		return nil, err
	}
	end := bytes.LastIndexByte(b, '}')
	if end < 0 {
		return nil, fmt.Errorf("keepjson: not an object")
	}
	out := append([]byte(nil), b[:end]...)
	more := len(have) > 0
	for _, name := range slices.Sorted(func(yield func(string) bool) {
		for n := range k {
			if !yield(n) {
				return
			}
		}
	}) {
		if _, ok := have[name]; ok {
			continue
		}
		if more {
			out = append(out, ',')
		}
		more = true
		key, _ := json.Marshal(name)
		out = append(out, key...)
		out = append(out, ':')
		out = append(out, k[name]...)
	}
	return append(out, b[end:]...), nil
}

// fieldNames lists the JSON names of struct t's fields, as encoding/json
// reads them (embedded structs included).
func fieldNames(t reflect.Type) []string {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return nil
	}
	var out []string
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		tag := f.Tag.Get("json")
		if tag == "-" {
			continue
		}
		name, _, _ := strings.Cut(tag, ",")
		if f.Anonymous && name == "" {
			out = append(out, fieldNames(f.Type)...)
			continue
		}
		if !f.IsExported() {
			continue
		}
		if name == "" {
			name = f.Name
		}
		out = append(out, name)
	}
	return out
}

// depth is how deeply b's arrays and objects nest.
func depth(b []byte) int {
	d, most, inStr, esc := 0, 0, false, false
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
			d++
			most = max(most, d)
		case c == ']' || c == '}':
			d--
		}
	}
	return most
}
