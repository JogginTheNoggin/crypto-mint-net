package wallet

import (
	"bytes"
	"encoding/json"
	"reflect"
	"testing"
)

// Open and Save read and write a wallet file byte for byte as
// json.Unmarshal and json.MarshalIndent do, kept keys included: indented
// alike, matched without regard to case, the last of a repeated key
// counting, HTML characters escaped alike (ORANGE-047 P3, White's
// condition).
func TestOrange047FileBytesUnchanged(t *testing.T) {
	inputs := []string{
		`{"format":1,"gap":20}`,
		`{"format":1,"GAP":5,"later":{"a":[1,"<b>&"]},"z":"\u2028x","later":2}`,
		"{\n \"format\" : 1 ,\n \"new\" : [ 1 , { \"k\" : \"v\" } ]\n}",
	}
	for _, in := range inputs {
		var viaJSON, direct File
		if err := json.Unmarshal([]byte(in), &viaJSON); err != nil {
			t.Fatal(err)
		}
		if err := decodeFile([]byte(in), &direct); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(viaJSON, direct) {
			t.Fatalf("%s: read %+v and %+v", in, viaJSON, direct)
		}
		want, err := json.MarshalIndent(viaJSON, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		got, err := encodeFile(direct)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, want) {
			t.Fatalf("%s:\nencodeFile\n%s\nMarshalIndent\n%s", in, got, want)
		}
	}
}
