package cli

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/filter"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/nodepolicy"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/rpc"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/watch"
)

// viewFlags are the flags of the view and watch commands: where the node
// is, and the filter (pkg/filter), whose -series also picks the series of
// commands that need one.
type viewFlags struct {
	w      *where
	filter func() (filter.Filter, error)
	f      filter.Filter
}

func addView(fs *flag.FlagSet) *viewFlags {
	return &viewFlags{w: addWhere(fs), filter: filter.Flags(fs)}
}

// parse parses the flags and reads the filter.
func (v *viewFlags) parse(fs *flag.FlagSet, args []string) error {
	if err := fs.Parse(args); err != nil {
		return err
	}
	f, err := v.filter()
	v.f = f
	return err
}

func (v *viewFlags) dial(out io.Writer) (*rpc.Client, error) { return dialNode(out, v.w.resolve()) }

// oneSeries returns the filter's series when it names exactly one, or nil
// for the node to take the only followed series.
func (v *viewFlags) oneSeries() (*hash.Hash, error) {
	switch len(v.f.Series) {
	case 0:
		return nil, nil
	case 1:
		return &v.f.Series[0], nil
	}
	return nil, errors.New("name one -series for this command")
}

func viewCmd(name string, args []string, out io.Writer) error {
	fs := newFlagSet(name, out)
	v := addView(fs)
	var below, limit *int
	var serial *string
	if name == "ledger" || name == "address" {
		limit = fs.Int("limit", 0, fmt.Sprintf("at most this many items (default %d)", watch.DefaultLimit))
	}
	if name == "ledger" || name == "address" {
		below = fs.Int("below", 0, "start at this height and go back (default: the tip)")
	}
	if name == "tx" {
		serial = fs.String("in-serial", "", "look only in this mint's pool")
	}
	o := addOutput(fs)
	if err := v.parse(fs, args); err != nil {
		return err
	}
	if err := o.check(); err != nil {
		return err
	}
	if name == "tx" || name == "address" {
		if fs.NArg() != 1 {
			return fmt.Errorf("usage: poo %s [flags] %s", name, map[string]string{"tx": "TID", "address": "ADDRESS"}[name])
		}
	} else if fs.NArg() != 0 {
		return fmt.Errorf("poo %s takes flags only", name)
	}
	cl, err := v.dial(out)
	if err != nil {
		return err
	}
	switch name {
	case "pools":
		r, err := cl.Pools(v.f)
		if err != nil {
			return err
		}
		return o.write(out, r)
	case "pending":
		r, err := cl.Pending(v.f)
		if err != nil {
			return err
		}
		return o.write(out, r)
	case "unspent":
		r, err := cl.Unspent(v.f)
		if err != nil {
			return err
		}
		return o.write(out, r)
	}
	sid, err := v.oneSeries()
	if err != nil {
		return err
	}
	switch name {
	case "ledger":
		if len(v.f.Pools) != 1 {
			return errors.New("poo ledger needs one -pool")
		}
		f := v.f
		f.Pools = nil
		r, err := cl.Ledger(sid, v.f.Pools[0], *below, *limit, f)
		if err != nil {
			return err
		}
		return o.write(out, r)
	case "tx":
		tid, err := hash.Parse(fs.Arg(0))
		if err != nil {
			return fmt.Errorf("tid: %w", err)
		}
		var sp *uint64
		if *serial != "" {
			n, err := strconv.ParseUint(*serial, 10, 64)
			if err != nil {
				return fmt.Errorf("-in-serial: %w", err)
			}
			sp = &n
		}
		r, err := cl.Tx(sid, tid, sp)
		if err != nil {
			return err
		}
		return o.write(out, r)
	default: // address
		a, err := hash.Parse(fs.Arg(0))
		if err != nil {
			return fmt.Errorf("address: %w", err)
		}
		r, err := cl.AddressPage(sid, a, *below, *limit, v.f)
		if err != nil {
			return err
		}
		return o.write(out, r)
	}
}

// watchCmd prints the events the filter selects, one JSON object per
// line, until cancelled. When the node reloads its policy the stream
// ends; watch connects again (reading the node's address anew) and goes
// on, so a watcher is set and forget too. Events while the node reloads
// are not replayed: a hello line after an end line marks the gap, and the
// views (ledger, pending, tx, address) show where things stand. Stream notices (hello, ping,
// end) are left out unless -notices is given; missed lines always show.
func watchCmd(args []string, out io.Writer) error {
	fs := newFlagSet("watch", out)
	v := addView(fs)
	notices := fs.Bool("notices", false, "also print hello, ping and end lines")
	once := fs.Bool("once", false, "stop when the stream ends instead of connecting again")
	if err := v.parse(fs, args); err != nil {
		return err
	}
	ctx := contextOf(out)
	enc := json.NewEncoder(out)
	print := func(ev watch.Event) error {
		switch ev.Kind {
		case "hello", "ping", "end":
			if !*notices {
				return nil
			}
		}
		return enc.Encode(ev)
	}
	for {
		cl, err := v.dial(out)
		if err == nil {
			err = cl.Watch(ctx, v.f, print)
		}
		if ctx.Err() != nil {
			return nil
		}
		if *once {
			return err
		}
		// The node is restarting its interface, or not up yet: try again.
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(100 * time.Millisecond):
		}
	}
}

// followCmd shows or changes what the node follows. With a running node
// the change goes through it and applies at once; otherwise node.json is
// edited and the node applies it when it starts.
func followCmd(remove bool, args []string, out io.Writer) error {
	name := "follow"
	if remove {
		name = "unfollow"
	}
	fs := newFlagSet(name, out)
	w := addWhere(fs)
	issuer := fs.String("issuer", "", "an issuer's address (hex)")
	seriesID := fs.String("series", "", "a series id (hex)")
	var pools, mints []uint64
	nums := func(dst *[]uint64) func(string) error {
		return func(s string) error {
			for _, p := range strings.Split(s, ",") {
				if p = strings.TrimSpace(p); p == "" {
					continue
				}
				n, err := strconv.ParseUint(p, 10, 64)
				if err != nil {
					return err
				}
				*dst = append(*dst, n)
			}
			return nil
		}
	}
	fs.Func("pool", "pool indices of the series (repeatable, comma-separated; none: the whole series)", nums(&pools))
	fs.Func("mint", "mint serials of the series, each followed with its pool (repeatable, comma-separated)", nums(&mints))
	noReload := fs.Bool("no-reload", false, "change node.json only; a running node applies it on its next reload or start")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("poo %s takes flags only", name)
	}
	dir := w.resolve()
	if *issuer == "" && *seriesID == "" {
		if remove {
			return errors.New("poo unfollow needs -issuer or -series")
		}
		// No change: show what is followed.
		if cl, err := dialNode(out, dir); err == nil {
			if r, err := cl.Follows(); err == nil {
				return writeJSON(out, r)
			}
		}
		p, err := nodepolicy.Load(filepath.Join(dir, nodepolicy.FileName))
		if err != nil {
			return err
		}
		return writeJSON(out, rpc.FollowReply{Follow: p.Follow, Pools: []watch.Pool{}})
	}
	if viewOnlyOf(out) {
		return errViewOnly(name)
	}
	req := rpc.FollowRequest{Remove: remove, Pools: pools, Mints: mints, NoReload: *noReload}
	for _, x := range []struct {
		flag string
		v    string
		dst  **hash.Hash
	}{{"-issuer", *issuer, &req.Issuer}, {"-series", *seriesID, &req.Series}} {
		if x.v == "" {
			continue
		}
		h, err := hash.Parse(x.v)
		if err != nil {
			return fmt.Errorf("%s: %w", x.flag, err)
		}
		*x.dst = &h
	}
	if cl, err := dialNode(out, dir); err == nil {
		r, err := cl.Follow(req)
		if err == nil {
			if r.Reloading {
				waitBack(out, dir)
				if cl, err := dialNode(out, dir); err == nil {
					if now, err := cl.Follows(); err == nil {
						r = now
					}
				}
			}
			return writeJSON(out, r)
		}
		var re *rpc.Error
		if errors.As(err, &re) {
			return err
		}
		// The node did not answer: change the file instead.
	}
	path := filepath.Join(dir, nodepolicy.FileName)
	p, err := nodepolicy.Load(path)
	if err != nil {
		return err
	}
	f := nodepolicy.Follow{Issuer: req.Issuer, Series: req.Series, Pools: pools, Mints: mints}
	if remove {
		err = p.RemoveFollow(f)
	} else {
		err = p.AddFollow(f)
	}
	if err == nil {
		err = p.Validate()
	}
	if err != nil {
		return err
	}
	if err := nodepolicy.Save(path, p); err != nil {
		return err
	}
	return writeJSON(out, rpc.FollowReply{Follow: p.Follow, Pools: []watch.Pool{}})
}

// waitBack waits, for ten seconds at most, until the node in dir has gone
// down for its reload and answers again.
func waitBack(out io.Writer, dir string) {
	ctx := contextOf(out)
	answers := func() bool {
		cl, err := dialNode(out, dir)
		if err != nil {
			return false
		}
		_, err = cl.Status()
		return err == nil
	}
	deadline := time.Now().Add(10 * time.Second)
	down := time.Now().Add(2 * time.Second)
	for time.Now().Before(down) && ctx.Err() == nil && answers() {
		time.Sleep(20 * time.Millisecond)
	}
	for time.Now().Before(deadline) && ctx.Err() == nil && !answers() {
		time.Sleep(50 * time.Millisecond)
	}
}

// reloadCmd makes a running node apply its node policy again.
func reloadCmd(args []string, out io.Writer) error {
	fs := newFlagSet("reload", out)
	w := addWhere(fs)
	if err := fs.Parse(args); err != nil {
		return err
	}
	cl, err := dialNode(out, w.resolve())
	if err != nil {
		return err
	}
	if err := cl.Reload(); err != nil {
		return err
	}
	waitBack(out, w.resolve())
	fmt.Fprintln(out, "reloaded")
	return nil
}
