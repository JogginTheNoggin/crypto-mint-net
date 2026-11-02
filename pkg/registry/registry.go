// Package registry reads address registries: public lists of node
// addresses for mint policies, where a node with no connections finds its
// first candidates (the design's "Address Registries"). A registry is a
// JSON document at a URL; the design leaves its format open, so this one
// is provisional (decision D44):
//
//	{
//	  "format": 1,
//	  "nodes": [
//	    {"addr": "203.0.113.5:8646", "follows": ["<issuer hex>/<series hex>/*/*"]}
//	  ]
//	}
//
// "follows" uses the text form of wire.Section. Everything read is a
// candidate only: the node checks each address with a follow request
// before peering with it.
package registry

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/wire"
)

// Format is the registry format this package reads.
const Format = 1

// MaxSize bounds a registry document.
const MaxSize = 1 << 20

// Entry is one listed node.
type Entry struct {
	Addr    string         `json:"addr"`
	Follows []wire.Section `json:"-"`
	Text    []string       `json:"follows"`
}

type document struct {
	Format int     `json:"format"`
	Nodes  []Entry `json:"nodes"`
}

// Fetch loads the registry at loc: an http or https URL, or a file path
// (file:// or plain). Entries with a bad address or section are skipped;
// a document that is not a registry is an error.
func Fetch(ctx context.Context, c *http.Client, loc string) ([]Entry, error) {
	var body io.ReadCloser
	switch {
	case strings.HasPrefix(loc, "http://") || strings.HasPrefix(loc, "https://"):
		req, err := http.NewRequestWithContext(ctx, "GET", loc, nil)
		if err != nil {
			return nil, err
		}
		resp, err := c.Do(req)
		if err != nil {
			return nil, err
		}
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			return nil, fmt.Errorf("registry %s: %s", loc, resp.Status)
		}
		body = resp.Body
	default:
		f, err := os.Open(strings.TrimPrefix(loc, "file://"))
		if err != nil {
			return nil, err
		}
		body = f
	}
	defer body.Close()
	data, err := io.ReadAll(io.LimitReader(body, MaxSize+1))
	if err != nil {
		return nil, err
	}
	if len(data) > MaxSize {
		return nil, fmt.Errorf("registry %s: larger than %d bytes", loc, MaxSize)
	}
	return Parse(data)
}

// ErrFormat marks a document that is not a registry this node reads.
var ErrFormat = errors.New("registry: not a registry of a known format")

// Parse reads a registry document.
func Parse(data []byte) ([]Entry, error) {
	var d document
	if err := json.Unmarshal(data, &d); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrFormat, err)
	}
	if d.Format != Format {
		return nil, fmt.Errorf("%w: format %d", ErrFormat, d.Format)
	}
	var out []Entry
	for _, e := range d.Nodes {
		if wire.CheckAddr(e.Addr) != nil {
			continue
		}
		for _, t := range e.Text {
			if s, err := wire.ParseSection(t); err == nil {
				e.Follows = append(e.Follows, s)
			}
		}
		if len(e.Follows) > 0 {
			out = append(out, e)
		}
	}
	return out, nil
}

// Encode writes a registry document listing entries (by their Follows).
func Encode(entries []Entry) ([]byte, error) {
	d := document{Format: Format, Nodes: []Entry{}}
	for _, e := range entries {
		e.Text = nil
		for _, s := range e.Follows {
			e.Text = append(e.Text, s.Text())
		}
		d.Nodes = append(d.Nodes, e)
	}
	return json.MarshalIndent(d, "", "  ")
}
