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
	"maps"
	"reflect"
	"slices"
	"strings"
	"unicode/utf8"
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
// a key that is a field's name in another letter case is that field, and
// of a key given twice the last one counts. A value that is not an object
// keeps nothing. Over a limit, it fails.
//
// raw is scanned once, at its top level only: the values of known fields
// are skipped, never copied or decoded (ORANGE-046 P4), so a large wallet
// costs one pass over its bytes.
func Split(raw []byte, v any) (Kept, error) { return split(raw, v, true) }

// SplitValid is Split for bytes known to be valid JSON, as an
// UnmarshalJSON method receives them (encoding/json checks the whole input
// first): it does not check them again (ORANGE-047 P2, D200).
func SplitValid(raw []byte, v any) (Kept, error) { return split(raw, v, false) }

func split(raw []byte, v any, check bool) (Kept, error) {
	names := fieldNames(reflect.TypeOf(v))
	var out Kept
	var err error
	ok := eachTop(raw, check, func(k string, val []byte) bool {
		if slices.ContainsFunc(names, func(n string) bool { return strings.EqualFold(n, k) }) {
			return true
		}
		var c bytes.Buffer
		if err = json.Compact(&c, val); err != nil {
			return false
		}
		_, again := out[k]
		switch {
		case !again && len(out) >= MaxKeys:
			err = fmt.Errorf("more than %d fields this release does not know", MaxKeys)
		case c.Len() > MaxBytes:
			err = fmt.Errorf("the field %q, which this release does not know, is over %d bytes", k, MaxBytes)
		case depth(c.Bytes()) > MaxDepth:
			err = fmt.Errorf("the field %q, which this release does not know, is nested over %d levels", k, MaxDepth)
		}
		if err != nil {
			return false
		}
		if out == nil {
			out = Kept{}
		}
		out[k] = c.Bytes()
		return true
	})
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, nil // not an object: the caller's own decode says so
	}
	return out, nil
}

// Join adds the kept keys, in name order, at the end of b, a JSON object
// as json.Marshal writes it. A key b already holds is not added again.
func Join(b []byte, k Kept) ([]byte, error) { return join(b, k, true) }

// JoinValid is Join for bytes known to be valid JSON, as json.Marshal
// returns them: it does not check them again (ORANGE-047 P2, D200).
func JoinValid(b []byte, k Kept) ([]byte, error) { return join(b, k, false) }

func join(b []byte, k Kept, check bool) ([]byte, error) {
	if len(k) == 0 {
		return b, nil
	}
	have := map[string]bool{}
	if !eachTop(b, check, func(key string, _ []byte) bool { have[key] = true; return true }) {
		return nil, fmt.Errorf("keepjson: not an object")
	}
	end := bytes.LastIndexByte(b, '}')
	out := append([]byte(nil), b[:end]...)
	more := len(have) > 0
	for _, name := range slices.Sorted(maps.Keys(k)) {
		if have[name] {
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

// eachTop calls f with each key of the JSON object b, decoded as
// encoding/json decodes it, and the bytes of its value, in order, until f
// returns false. It reports false when b is not a valid JSON object.
//
// With check false, b must be valid JSON already; it is not checked again.
func eachTop(b []byte, check bool, f func(key string, val []byte) bool) bool {
	if check && !json.Valid(b) {
		return false
	}
	i := skipSpace(b, 0)
	if i >= len(b) || b[i] != '{' {
		return false
	}
	i = skipSpace(b, i+1)
	if i < len(b) && b[i] == '}' {
		return true
	}
	for i < len(b) {
		ks := i
		i = endString(b, i)
		key, ok := decodeKey(b[ks:i])
		if !ok {
			return false
		}
		i = skipSpace(b, i)
		i = skipSpace(b, i+1) // the colon
		vs := i
		i = endValue(b, i)
		if !f(key, b[vs:i]) {
			return true
		}
		i = skipSpace(b, i)
		if i >= len(b) || b[i] == '}' {
			return true
		}
		i = skipSpace(b, i+1) // the comma
	}
	return true
}

func skipSpace(b []byte, i int) int {
	for i < len(b) && (b[i] == ' ' || b[i] == '\t' || b[i] == '\n' || b[i] == '\r') {
		i++
	}
	return i
}

// endString returns the index just past the string starting at b[i].
func endString(b []byte, i int) int {
	for i++; i < len(b); i++ {
		switch b[i] {
		case '\\':
			i++
		case '"':
			return i + 1
		}
	}
	return i
}

// endValue returns the index just past the value starting at b[i]; b is
// valid JSON.
func endValue(b []byte, i int) int {
	d := 0
	for i < len(b) {
		switch c := b[i]; {
		case c == '"':
			i = endString(b, i)
			if d == 0 {
				return i
			}
			continue
		case c == '{' || c == '[':
			d++
		case c == '}' || c == ']':
			if d == 0 {
				return i
			}
			d--
			if d == 0 {
				return i + 1
			}
		case d == 0 && (c == ',' || c == ' ' || c == '\t' || c == '\n' || c == '\r'):
			return i
		}
		i++
	}
	return i
}

// decodeKey decodes a key as encoding/json does (escapes, and invalid
// UTF-8 replaced).
func decodeKey(q []byte) (string, bool) {
	if len(q) < 2 {
		return "", false
	}
	if inner := q[1 : len(q)-1]; bytes.IndexByte(inner, '\\') < 0 && utf8.Valid(inner) {
		return string(inner), true
	}
	var s string
	if err := json.Unmarshal(q, &s); err != nil {
		return "", false
	}
	return s, true
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
