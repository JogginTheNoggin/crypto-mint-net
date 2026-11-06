package keepjson

import (
	"bytes"
	"encoding/json"
	"reflect"
	"slices"
	"strings"
	"testing"
)

// splitOld and joinOld are Split and Join as Round 36 wrote them, decoding
// the whole object into a map: the scan of ORANGE-046 P4 must keep and
// write exactly what they did (White's condition).
func splitOld(raw []byte, v any) (Kept, error) {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, nil
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
		if out == nil {
			out = Kept{}
		}
		out[k] = c.Bytes()
	}
	return out, nil
}

func joinOld(b []byte, k Kept) ([]byte, error) {
	if len(k) == 0 {
		return b, nil
	}
	var have map[string]json.RawMessage
	if err := json.Unmarshal(b, &have); err != nil {
		return nil, err
	}
	end := bytes.LastIndexByte(b, '}')
	out := append([]byte(nil), b[:end]...)
	more := len(have) > 0
	names := make([]string, 0, len(k))
	for n := range k {
		names = append(names, n)
	}
	slices.Sort(names)
	for _, name := range names {
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

func TestOrange046ScanMatchesTheMap(t *testing.T) {
	type rec struct {
		Name  string `json:"name"`
		Items []int  `json:"items"`
		Plain int
		Skip  int `json:"-"`
	}
	inputs := []string{
		`{}`,
		` { } `,
		`null`,
		`[1,2]`,
		`"text"`,
		`{"name":"a","items":[1,2,3]}`,
		`{"NAME":"a","Items":[],"plain":3,"later":{"a":[1,{"b":"}"}]},"z":true}`,
		"{\n  \"name\" : \"x\" ,\n  \"new\" : [ 1 , 2 ] ,\n  \"n\" : -1.5e3 ,\n  \"nul\":null\n}",
		`{"a":1,"a":2,"b":"x\"y\\","a":3}`,
		`{"kéy":"é","esc\"aped":{"q":"]["},"Skip":4,"-":5}`,
		`{"s":"a,b}c","t":"{[","u":""}`,
		"{\"bad\xffutf\":1}",
		`{"name":"a"`,
		`{"a":1}{"b":2}`,
	}
	for _, in := range inputs {
		got, gerr := Split([]byte(in), rec{})
		want, werr := splitOld([]byte(in), rec{})
		if (gerr != nil) != (werr != nil) || !reflect.DeepEqual(got, want) {
			t.Fatalf("Split(%q) = %v, %v; the map gave %v, %v", in, got, gerr, want, werr)
		}
	}
	kept := Kept{"z": json.RawMessage(`1`), "a": json.RawMessage(`{"x":[1]}`), "name": json.RawMessage(`"dup"`)}
	for _, b := range []string{`{}`, `{"name":"n"}`, `{"name":"n","items":[1,2],"Plain":0}`, "{\n  \"name\": \"n\"\n}"} {
		got, gerr := Join([]byte(b), kept)
		want, werr := joinOld([]byte(b), kept)
		if (gerr != nil) != (werr != nil) || !bytes.Equal(got, want) {
			t.Fatalf("Join(%q) = %s, %v; the map gave %s, %v", b, got, gerr, want, werr)
		}
	}
}
