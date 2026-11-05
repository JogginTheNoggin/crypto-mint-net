package rpc

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strconv"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/engine"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/node"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/watch"
)

// Ledger export and import (D188). GET /v1/ledger/export gives a page of
// a pool's selected ledger as raw entry items with a summary; POST
// /v1/ledger/import puts items through the node's full checks.

// LedgerImportRequest is the body of POST /v1/ledger/import.
type LedgerImportRequest struct {
	Series hash.Hash `json:"series"`
	Pool   uint64    `json:"pool"`
	// Items are entry items' byte forms, oldest first (base64 in JSON).
	Items [][]byte `json:"items"`
}

func (h *Handler) ledgerExport(w http.ResponseWriter, r *http.Request) {
	id, e := h.seriesParam(r)
	if e != nil {
		writeError(w, e)
		return
	}
	pool, e := queryUint(r, "pool")
	if e != nil {
		writeError(w, e)
		return
	}
	from, e := optInt(r, "from")
	if e != nil {
		writeError(w, e)
		return
	}
	to, e := optInt(r, "to")
	if e != nil {
		writeError(w, e)
		return
	}
	out, err := watch.Export(h.n.Engine(), engine.PoolKey{Series: id, Index: pool}, from, to)
	if err != nil {
		writeError(w, classify(err))
		return
	}
	writeJSON(w, out)
}

// MaxImportBatchBytes bounds the items of one import call, beyond its
// first: a call holds one item of any size (an entry has no size bound,
// D144), and then more only while they total at most this. It is the size
// of an export page, and poo ledger import sends batches of it (RED-098).
const MaxImportBatchBytes = watch.ExportBytes

func (h *Handler) ledgerImport(w http.ResponseWriter, r *http.Request) {
	req, e := decodeImport(r.Body)
	if e != nil {
		writeError(w, e)
		return
	}
	out, err := h.n.ImportItems(engine.PoolKey{Series: req.Series, Index: req.Pool}, req.Items)
	if err != nil {
		writeError(w, classify(err))
		return
	}
	writeJSON(w, out)
}

// decodeImport reads an import call's body item by item, and stops at
// the item that takes the call over node.MaxImportItems or over
// MaxImportBatchBytes past its first item, so a call holds at most one
// batch and one item, never the whole body first (RED-098). It refuses
// unknown fields and anything after the object, as decodeBody does.
func decodeImport(body io.Reader) (LedgerImportRequest, *Error) {
	var req LedgerImportRequest
	bad := func(format string, a ...any) (LedgerImportRequest, *Error) {
		return LedgerImportRequest{}, badRequest("body: "+format, a...)
	}
	dec := json.NewDecoder(body)
	if t, err := dec.Token(); err != nil || t != json.Delim('{') {
		return bad("not a JSON object")
	}
	size := 0
	for dec.More() {
		t, err := dec.Token()
		if err != nil {
			return bad("%v", err)
		}
		switch t {
		case "series":
			err = dec.Decode(&req.Series)
		case "pool":
			err = dec.Decode(&req.Pool)
		case "items":
			if t, err = dec.Token(); err != nil {
				return bad("%v", err)
			}
			if t == nil {
				continue
			}
			if t != json.Delim('[') {
				return bad("items is not a list")
			}
			for dec.More() {
				if len(req.Items) == node.MaxImportItems {
					return bad("items: more than %d in one call", node.MaxImportItems)
				}
				var b []byte
				if err := dec.Decode(&b); err != nil {
					return bad("items: %v", err)
				}
				if len(req.Items) > 0 {
					if size+len(b) > MaxImportBatchBytes {
						return bad("items: over %d MiB past the first item in one call; send fewer, or a large item alone", MaxImportBatchBytes>>20)
					}
					size += len(b)
				}
				req.Items = append(req.Items, b)
			}
			_, err = dec.Token()
		default:
			return bad("json: unknown field %q", t)
		}
		if err != nil {
			return bad("%v", err)
		}
	}
	if _, err := dec.Token(); err != nil {
		return bad("%v", err)
	}
	if dec.More() {
		return bad("more than one JSON value")
	}
	if _, err := dec.Token(); err != io.EOF {
		return bad("more than one JSON value")
	}
	return req, nil
}

// LedgerExport asks for one page of an export.
func (c *Client) LedgerExport(series *hash.Hash, pool uint64, from, to int) (watch.ExportPage, error) {
	v := withSeries(url.Values{}, series)
	v.Set("pool", strconv.FormatUint(pool, 10))
	if from > 0 {
		v.Set("from", strconv.Itoa(from))
	}
	if to > 0 {
		v.Set("to", strconv.Itoa(to))
	}
	var out watch.ExportPage
	err := c.do(http.MethodGet, "/v1/ledger/export?"+v.Encode(), nil, &out)
	return out, err
}

// LedgerImport hands the node one batch of items.
func (c *Client) LedgerImport(req LedgerImportRequest) (node.ImportResult, error) {
	var out node.ImportResult
	err := c.do(http.MethodPost, "/v1/ledger/import", req, &out)
	return out, err
}
