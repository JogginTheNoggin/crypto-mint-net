package rpc

import (
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

func (h *Handler) ledgerImport(w http.ResponseWriter, r *http.Request) {
	var req LedgerImportRequest
	if e := decodeBody(r, &req); e != nil {
		writeError(w, e)
		return
	}
	if len(req.Items) > node.MaxImportItems {
		writeError(w, badRequest("items: %d, at most %d in one call", len(req.Items), node.MaxImportItems))
		return
	}
	out, err := h.n.ImportItems(engine.PoolKey{Series: req.Series, Index: req.Pool}, req.Items)
	if err != nil {
		writeError(w, classify(err))
		return
	}
	writeJSON(w, out)
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
