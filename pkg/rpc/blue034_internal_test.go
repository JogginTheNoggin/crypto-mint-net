package rpc

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/node"
)

// RED-098: an import call holds one item of any size, then more only
// while they total at most MaxImportBatchBytes; it stops reading at the
// item that passes the bound.
func TestBlue034ImportCallBounded(t *testing.T) {
	body := func(sizes ...int) string {
		items := make([][]byte, len(sizes))
		for i, n := range sizes {
			items[i] = make([]byte, n)
		}
		b, _ := json.Marshal(LedgerImportRequest{Pool: 3, Items: items})
		return string(b)
	}
	half := MaxImportBatchBytes / 2
	for name, tc := range map[string]struct {
		body string
		ok   bool
	}{
		"one large item":       {body(3 * MaxImportBatchBytes), true},
		"a batch":              {body(1, half, half), true},
		"past the batch bound": {body(1, half, half, 2), false},
		"too many items":       {body(make([]int, node.MaxImportItems+1)...), false},
		"unknown field":        {`{"pool":1,"items":[],"extra":1}`, false},
		"two values":           {`{"pool":1}{}`, false},
		"null items":           {`{"pool":1,"items":null}`, true},
	} {
		req, e := decodeImport(strings.NewReader(tc.body))
		if (e == nil) != tc.ok {
			t.Fatalf("%s: %v", name, e)
		}
		if e == nil && name == "a batch" && (req.Pool != 3 || len(req.Items) != 3 || len(req.Items[1]) != half) {
			t.Fatalf("%s: %+v", name, req.Pool)
		}
	}
}
