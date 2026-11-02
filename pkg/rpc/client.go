package rpc

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/entry"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/layer"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/tx"
)

// Client calls a node's interface.
type Client struct {
	base  string
	token string
	http  *http.Client
}

// NewClient returns a client for the node at addr (host:port).
func NewClient(addr, token string) *Client {
	return &Client{base: "http://" + addr, token: token, http: &http.Client{Timeout: 5 * time.Minute}}
}

// Dial returns a client for the node running on dir, reading its address
// and token from the data directory.
func Dial(dir string) (*Client, error) {
	a, err := os.ReadFile(filepath.Join(dir, AddrFile))
	if err != nil {
		return nil, fmt.Errorf("rpc: is the node running? %w", err)
	}
	t, err := os.ReadFile(filepath.Join(dir, TokenFile))
	if err != nil {
		return nil, err
	}
	return NewClient(strings.TrimSpace(string(a)), strings.TrimSpace(string(t))), nil
}

func (c *Client) do(method, path string, body, out any) error {
	var rd *bytes.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rd = bytes.NewReader(b)
	} else {
		rd = bytes.NewReader(nil)
	}
	req, err := http.NewRequest(method, c.base+path, rd)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		e := &Error{Status: resp.StatusCode}
		if err := json.NewDecoder(resp.Body).Decode(e); err != nil || e.Code == "" {
			return fmt.Errorf("rpc: %s %s: HTTP %d", method, path, resp.StatusCode)
		}
		return e
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// Status returns every followed pool.
func (c *Client) Status() (StatusReply, error) {
	var r StatusReply
	return r, c.do("GET", "/v1/status", nil, &r)
}

// Peers returns the node's connected peers.
func (c *Client) Peers() (PeersReply, error) {
	var r PeersReply
	return r, c.do("GET", "/v1/peers", nil, &r)
}

// Series returns the known series that are followed.
func (c *Client) Series() ([]SeriesInfo, error) {
	var r []SeriesInfo
	return r, c.do("GET", "/v1/series", nil, &r)
}

// SubmitTx submits a transaction.
func (c *Client) SubmitTx(t *tx.Tx) (TxReply, error) {
	var r TxReply
	return r, c.do("POST", "/v1/tx", TxRequest{Tx: hex.EncodeToString(t.Encode())}, &r)
}

// SubmitEntry submits an entry of a pool.
func (c *Client) SubmitEntry(p PoolRef, e *entry.Entry) (EntryReply, error) {
	var r EntryReply
	return r, c.do("POST", "/v1/entry", EntryRequest{PoolRef: p, Entry: hex.EncodeToString(e.Encode())}, &r)
}

// SubmitLayer submits a layer on an entry of a pool.
func (c *Client) SubmitLayer(p PoolRef, l layer.Layer) (LayerReply, error) {
	var r LayerReply
	return r, c.do("POST", "/v1/layer", LayerRequest{PoolRef: p, Layer: hex.EncodeToString(l.Encode())}, &r)
}

// Build asks the node to build an entry from its pending transactions.
func (c *Client) Build(p PoolRef, maxTries uint64) (EntryReply, error) {
	var r EntryReply
	return r, c.do("POST", "/v1/build", BuildRequest{PoolRef: p, MaxTries: maxTries}, &r)
}

// Work asks the node to add the strongest layer it finds in tries nonces.
func (c *Client) Work(p PoolRef, e hash.Hash, start, tries uint64) (LayerReply, error) {
	var r LayerReply
	return r, c.do("POST", "/v1/work", WorkRequest{PoolRef: p, Entry: e, Start: start, Tries: tries}, &r)
}

func mintQuery(series hash.Hash, serial uint64) url.Values {
	return url.Values{"series": {series.String()}, "serial": {strconv.FormatUint(serial, 10)}}
}

// Payment reports where a transaction of a mint stands.
func (c *Client) Payment(series hash.Hash, serial uint64, tid hash.Hash) (PaymentReply, error) {
	q := mintQuery(series, serial)
	q.Set("tid", tid.String())
	var r PaymentReply
	return r, c.do("GET", "/v1/payment?"+q.Encode(), nil, &r)
}

// Holdings returns the unspent portions of a mint.
func (c *Client) Holdings(series hash.Hash, serial uint64) ([]Holding, error) {
	var r []Holding
	return r, c.do("GET", "/v1/holdings?"+mintQuery(series, serial).Encode(), nil, &r)
}
