package rpc

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/filter"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/watch"
)

func withSeries(v url.Values, series *hash.Hash) url.Values {
	if series != nil {
		v.Set("series", series.String())
	}
	return v
}

// Pools lists the followed pools f covers.
func (c *Client) Pools(f filter.Filter) ([]watch.Pool, error) {
	var out []watch.Pool
	return out, c.do("GET", "/v1/pools?"+f.Values().Encode(), nil, &out)
}

// Ledger returns a page of a pool's selected ledger, newest first, from
// height below (0: the tip). series may be nil when the node follows one
// series.
func (c *Client) Ledger(series *hash.Hash, pool uint64, below, limit int, f filter.Filter) (watch.Ledger, error) {
	v := withSeries(f.Values(), series)
	v.Set("pool", strconv.FormatUint(pool, 10))
	if below > 0 {
		v.Set("below", strconv.Itoa(below))
	}
	if limit > 0 {
		v.Set("limit", strconv.Itoa(limit))
	}
	var out watch.Ledger
	return out, c.do("GET", "/v1/ledger?"+v.Encode(), nil, &out)
}

// Pending lists the pending transactions f selects.
func (c *Client) Pending(f filter.Filter) ([]watch.PendingTx, error) {
	var out []watch.PendingTx
	return out, c.do("GET", "/v1/pending?"+f.Values().Encode(), nil, &out)
}

// Unspent lists the unspent portions f selects.
func (c *Client) Unspent(f filter.Filter) ([]watch.Holding, error) {
	var out []watch.Holding
	return out, c.do("GET", "/v1/unspent?"+f.Values().Encode(), nil, &out)
}

// Tx returns one transaction and its status. serial, if not nil, names the
// mint, so only its pool is looked in.
func (c *Client) Tx(series *hash.Hash, tid hash.Hash, serial *uint64) (watch.TxStatus, error) {
	v := withSeries(url.Values{}, series)
	v.Set("id", tid.String())
	if serial != nil {
		v.Set("serial", strconv.FormatUint(*serial, 10))
	}
	var out watch.TxStatus
	return out, c.do("GET", "/v1/tx?"+v.Encode(), nil, &out)
}

// Address returns an address's balance, holdings, history (at most limit
// items, 0 for the default) and pending transactions, in the pools f's
// pool and serial fields cover.
func (c *Client) Address(series *hash.Hash, a hash.Hash, limit int, f filter.Filter) (watch.Address, error) {
	return c.AddressPage(series, a, 0, limit, f)
}

// AddressPage is Address for the history from height below down (0: the
// tips); the reply's Next says where each unfinished pool goes on.
func (c *Client) AddressPage(series *hash.Hash, a hash.Hash, below, limit int, f filter.Filter) (watch.Address, error) {
	v := withSeries(f.Values(), series)
	v.Set("id", a.String())
	if below > 0 {
		v.Set("below", strconv.Itoa(below))
	}
	if limit > 0 {
		v.Set("limit", strconv.Itoa(limit))
	}
	var out watch.Address
	return out, c.do("GET", "/v1/address?"+v.Encode(), nil, &out)
}

// Follows returns what the node policy follows and what the node follows
// now.
func (c *Client) Follows() (FollowReply, error) {
	var out FollowReply
	return out, c.do("GET", "/v1/follow", nil, &out)
}

// Follow changes the node policy's follow list and, unless NoReload, makes
// the node apply it. The node's interface restarts while it does, so the
// next call may need to wait for it (Dial again once rpc.addr is back).
func (c *Client) Follow(req FollowRequest) (FollowReply, error) {
	var out FollowReply
	return out, c.do("POST", "/v1/follow", req, &out)
}

// Reload makes the node apply its node policy again.
func (c *Client) Reload() error {
	var out map[string]bool
	return c.do("POST", "/v1/reload", struct{}{}, &out)
}

// Watch streams the events f selects to fn until ctx is done, fn returns
// an error, or the node ends the stream. Stream notices (hello, ping, end,
// missed) are passed to fn too; their Kind is not one of filter.Kinds. It
// returns nil when the node ended the stream (for instance to reload) and
// ctx's error when ctx was cancelled.
func (c *Client) Watch(ctx context.Context, f filter.Filter, fn func(watch.Event) error) error {
	req, err := http.NewRequestWithContext(ctx, "GET", c.base+"/v1/watch?"+f.Values().Encode(), nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	resp, err := (&http.Client{}).Do(req) // no timeout: the stream is long-lived
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		e := &Error{Status: resp.StatusCode}
		if err := json.NewDecoder(resp.Body).Decode(e); err != nil || e.Code == "" {
			return fmt.Errorf("rpc: watch: HTTP %d", resp.StatusCode)
		}
		return e
	}
	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 64<<10), 16<<20)
	for sc.Scan() {
		var ev watch.Event
		if err := json.Unmarshal(sc.Bytes(), &ev); err != nil {
			return fmt.Errorf("rpc: watch: %w", err)
		}
		if err := fn(ev); err != nil {
			return err
		}
		if ev.Kind == "end" {
			return nil
		}
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return sc.Err()
}
