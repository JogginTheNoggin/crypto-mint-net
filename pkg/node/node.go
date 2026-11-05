// Package node is a full node with a data directory: the engine, the node
// policy that configures it, and a store for every followed pool.
//
// Opening a node reads its node policy, makes the accepted series known,
// applies what it follows, and replays each pool's store through the engine
// as Stored items, so everything on disk is checked again. Every item the
// engine then keeps is appended to its pool's store before the call
// returns. Logs are compacted on start-up and on close when most of their
// records are no longer needed.
//
// Data directory layout:
//
//	node.json                      the node policy
//	LOCK                           held while a node runs
//	pools/<series_id>/<k>.log      one store per followed pool
//	rpc.token, rpc.addr            written by the RPC server (package rpc)
//
// A Node is safe for concurrent use. Calls that change state are
// serialized, so each store is in the order the engine accepted items.
package node

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/engine"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/entry"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/layer"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/ledger"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/logging"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/nodepolicy"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/store"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/tx"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/weight"
)

// ErrClosed is returned by calls on a closed node.
var ErrClosed = errors.New("node: closed")

// A log is compacted when it holds at least CompactMin records and more
// than compactRatio times the records the node still needs. Provisional,
// like every storage default.
const (
	DefaultCompactMin = 1024
	compactRatio      = 2
)

// Options are settings that are not node policy.
type Options struct {
	// Log receives progress and warnings. Nil discards them.
	Log io.Writer
	// LogLevel and LogFormat override the node policy's log section for
	// this run ("" keeps the policy's; package logging).
	LogLevel, LogFormat string
	// Logger, if set, is used as it is instead of one made from Log.
	Logger *slog.Logger
	// CompactMin is the smallest log that is compacted automatically
	// (0: DefaultCompactMin).
	CompactMin int
	// Reverify ignores the pool snapshots and replays every log whole,
	// checking every record again (poo node -reverify, D178). Where a
	// snapshot covered the whole log, the pool the replay builds is
	// compared with it and the result reported (ReplayStats).
	Reverify bool
}

// Node is a running full node.
type Node struct {
	mu  sync.Mutex
	dir string
	pol nodepolicy.Policy
	// run is pol with the limits applied in place since Open (D168),
	// guarded by mu.
	run nodepolicy.Policy
	// policyMu: PolicyMu.
	policyMu sync.Mutex
	eng      *engine.Engine
	logs     map[engine.PoolKey]*store.Log
	// bodies holds each pool's entry record locations, and cache the
	// bodies read back from them (D172); bmu guards the map.
	bmu    sync.Mutex
	bodies map[engine.PoolKey]*poolBodies
	cache  *ledger.BodyCache
	log    *slog.Logger // the node's logger; Logger gives it to p2p and rpc
	failed sync.Map     // pool keys whose store stopped, logged once
	// kept paces the extra layers a layer floor keeps (RED-018).
	kept   keptBucket
	cmin   int
	unlock func()
	closed bool
	replay ReplayStats
	files  nodepolicy.FileLimit
	// snaps is what the node knows of each pool's snapshot (D178),
	// guarded by mu; reverify is Options.Reverify.
	snaps    map[engine.PoolKey]*snapState
	reverify bool

	observers []func(Event)
}

// ReplayStats reports what Open read back from the stores.
type ReplayStats struct {
	Pools   int `json:"pools"`
	Records int `json:"records"`
	// Kept counts records the engine kept again. The rest were valid but
	// no longer needed (a pruned branch, a transaction now in an entry) or
	// failed a check, which would mean the files were changed.
	Kept int `json:"kept"`
	// Cut is the bytes dropped from the ends of logs after a crash.
	Cut int64 `json:"cut_bytes"`
	// Snapshots counts the pools started from their snapshots, and
	// Covered the log records those snapshots covered, which were not read
	// (D178). Records counts only the records replayed after them.
	Snapshots int `json:"snapshots"`
	Covered   int `json:"snapshot_records"`
	// Unused lists, by pool, why a snapshot found was not used, so the
	// pool was replayed whole.
	Unused []string `json:"snapshots_unused,omitempty"`
	// Reverified and Differed count, under -reverify, the pools whose
	// snapshot covered the whole log, and those of them whose full replay
	// built a different pool than the snapshot held. Differed should be 0.
	Reverified int `json:"reverified,omitempty"`
	Differed   int `json:"reverify_differed,omitempty"`
}

// Open opens the node in dir. The directory must hold a node.json.
func Open(dir string, opt Options) (*Node, error) {
	pol, err := nodepolicy.Load(filepath.Join(dir, nodepolicy.FileName))
	if err != nil {
		return nil, err
	}
	lg := opt.Logger
	if lg == nil {
		level, format := pol.LogSettings()
		if opt.LogLevel != "" {
			level = opt.LogLevel
		}
		if opt.LogFormat != "" {
			format = opt.LogFormat
		}
		if lg, err = logging.New(opt.Log, level, format); err != nil {
			return nil, err
		}
	}
	for _, n := range pol.Lowered {
		lg.With(logging.Component, "node").Warn("policy value lowered", "file", nodepolicy.FileName, "note", n.String())
	}
	if opt.CompactMin <= 0 {
		opt.CompactMin = DefaultCompactMin
	}
	unlock, err := nodepolicy.Lock(dir)
	if err != nil {
		return nil, err
	}
	n := &Node{
		dir: dir, pol: pol, run: pol, log: lg, unlock: unlock, cmin: opt.CompactMin,
		logs:   map[engine.PoolKey]*store.Log{},
		bodies: map[engine.PoolKey]*poolBodies{},
		cache:  ledger.NewBodyCache(pol.EntryCacheBytes()),
		snaps:  map[engine.PoolKey]*snapState{}, reverify: opt.Reverify,
	}
	lc := pol.LedgerConfig()
	lc.Bodies = n.cache
	n.eng = engine.New(engine.Config{
		Ledger: lc, MaxPending: pol.MaxPending, SeenSize: pol.SeenSize,
		Source: func(k engine.PoolKey) ledger.BodySource { return source{n, k} },
	})
	if err := n.configure(); err != nil {
		unlock()
		return nil, err
	}
	if err := n.load(); err != nil {
		n.closeLogs()
		unlock()
		return nil, err
	}
	// One store file per pool in use: say so before it runs out
	// (ORANGE-037).
	n.files = nodepolicy.CheckFileLimit(n.eng.FollowedCount())
	if n.files.Low {
		n.logger().Warn("open file limit may be too low", "limit", n.files.Soft, "need", n.files.Need,
			"note", n.files.String()+"; raise it (ulimit -n, or LimitNOFILE= in a systemd unit) or follow fewer pools")
	}
	return n, nil
}

// Logger returns the node's logger. Packages that serve the node (p2p,
// rpc, daemon) log through it, each under its own component.
func (n *Node) Logger() *slog.Logger { return n.log }

func (n *Node) logger() *slog.Logger { return n.log.With(logging.Component, "node") }

// configure makes accepted series known and applies the follow rules.
func (n *Node) configure() error {
	accepted, refused := n.pol.Accepted()
	for _, r := range refused {
		n.logger().Warn("series not accepted", "index", r.Index, "err", r.Reason)
	}
	// The releases just checked, kept for the next process (RED-075).
	nodepolicy.SaveChecked(n.dir, n.pol)
	byIssuer := map[hash.Hash][]hash.Hash{}
	for _, a := range accepted {
		if a.ReleaseErr != nil {
			n.logger().Warn("release in node.json not used; the series is followed with the parts proven without it (give it a valid one with poo series add-release)", "series", a.Series.ID().Short(), "err", a.ReleaseErr)
		}
		n.eng.AddSeries(a.Series)
		if a.Issuer != nil {
			byIssuer[*a.Issuer] = append(byIssuer[*a.Issuer], a.Series.ID())
		}
	}
	for i, f := range n.pol.Follow {
		var err error
		switch {
		case f.Issuer != nil:
			ids := byIssuer[*f.Issuer]
			if len(ids) == 0 {
				n.logger().Warn("follow rule matches no accepted series", "follow", i, "issuer", f.Issuer.Short())
			}
			for _, id := range ids {
				if err = n.eng.FollowSeries(id); err != nil {
					break
				}
			}
		case len(f.Pools) == 0 && len(f.Mints) == 0 && len(f.Except) == 0:
			err = n.eng.FollowSeries(*f.Series)
		case len(f.Pools) == 0 && len(f.Mints) == 0:
			err = n.eng.FollowSeriesExcept(*f.Series, f.Except)
		default:
			for _, k := range f.Pools {
				if err = n.eng.FollowPool(*f.Series, k); err != nil {
					break
				}
			}
			for _, s := range f.Mints {
				if err == nil {
					err = n.eng.FollowMint(*f.Series, s)
				}
			}
		}
		if errors.Is(err, engine.ErrUnknownSeries) {
			// A series that was not accepted is not followed; the node
			// still starts.
			n.logger().Warn("follow rule names a series not known or not accepted", "follow", i, "series", f.Series.Short())
			continue
		}
		if err != nil {
			return fmt.Errorf("%w: follow %d: %v", nodepolicy.ErrInvalid, i, err)
		}
	}
	return nil
}

func (n *Node) logPath(k engine.PoolKey) string {
	return filepath.Join(n.dir, "pools", k.Series.String(), strconv.FormatUint(k.Index, 10)+".log")
}

// stored lists the pools that have a store on disk and are followed, by
// series id and then pool index. It reads the pools directory rather than
// asking about every followed pool, so a series of very many pools
// followed whole costs only what is stored (RED-046, D152).
func (n *Node) stored() ([]engine.PoolKey, error) {
	root := filepath.Join(n.dir, "pools")
	dirs, err := os.ReadDir(root)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("node: %w", err)
	}
	var out []engine.PoolKey
	for _, d := range dirs {
		id, err := hash.Parse(d.Name())
		if !d.IsDir() || err != nil || id.String() != d.Name() {
			continue
		}
		files, err := os.ReadDir(filepath.Join(root, d.Name()))
		if err != nil {
			return nil, fmt.Errorf("node: %w", err)
		}
		for _, f := range files {
			num, ok := strings.CutSuffix(f.Name(), ".log")
			if !ok || f.IsDir() {
				continue
			}
			i, err := strconv.ParseUint(num, 10, 64)
			if err != nil || strconv.FormatUint(i, 10) != num {
				continue
			}
			if k := (engine.PoolKey{Series: id, Index: i}); n.eng.Follows(k) {
				out = append(out, k)
			}
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if c := out[i].Series.Compare(out[j].Series); c != 0 {
			return c < 0
		}
		return out[i].Index < out[j].Index
	})
	return out, nil
}

// load opens the store of every followed pool that has one and replays
// it. Stores of pools no longer followed are left on disk untouched.
func (n *Node) load() error {
	keys, err := n.stored()
	if err != nil {
		return err
	}
	for _, k := range keys {
		if err := n.loadPool(k); err != nil {
			return err
		}
	}
	return nil
}

// storedRec is one record read back at start, with its place in the log.
type storedRec struct {
	rec store.Record
	loc store.Loc
}

// loadPool replays pool k's log. Records are read one window at a time:
// a window is decoded, its witnesses are checked on every core, and then
// it is replayed in order, finding them verified (ORANGE-021). The place
// of each entry the engine keeps is noted, and its body dropped from
// memory once the next entry comes, so start-up holds one window of
// records and the bodies the cache keeps, not the whole log (D172).
func (n *Node) loadPool(k engine.PoolKey) error {
	path := n.logPath(k)
	s, _ := n.eng.Series(k.Series)
	var b *poolBodies
	win := make([]storedRec, 0, replayWindow)
	flush := func() {
		recs := make([]store.Record, len(win))
		for i, r := range win {
			recs[i] = r.rec
		}
		ds := decodeAll(recs)
		var txs []*tx.Tx
		for _, d := range ds {
			switch {
			case d.t != nil:
				txs = append(txs, d.t)
			case d.e != nil:
				txs = append(txs, d.e.Txs...)
			}
		}
		n.eng.WarmStored(txs)
		for i, d := range ds {
			n.replay.Records++
			if !n.apply(k, d) {
				continue
			}
			n.replay.Kept++
			if d.e != nil {
				b.add(d.e.Hash(s), win[i].loc)
			}
		}
		win = win[:0]
	}
	var check *snapHeader // under -reverify, the snapshot to compare
	from := store.From{Ready: func(ol *store.Log) (int64, int, error) {
		b = n.openBodies(k, ol)
		if !n.snapshotsOn() {
			return 0, 0, nil
		}
		if n.reverify {
			h, err := n.readSnapHeader(k)
			switch {
			case err == nil:
				check = &h
			case !errors.Is(err, os.ErrNotExist):
				// A snapshot -reverify cannot read is reported as on any
				// start, so the flag that checks everything does not hide
				// a damaged file behind "ok" (GREEN-055).
				n.replay.Unused = append(n.replay.Unused, k.String()+": "+err.Error())
				n.logger().Warn("pool snapshot not compared under -reverify; the whole log is replayed", "pool", k.String(), "reason", err.Error())
			}
			return 0, 0, nil
		}
		res, err := n.loadSnapshot(k, b)
		if err != nil {
			if !errors.Is(err, os.ErrNotExist) {
				n.replay.Unused = append(n.replay.Unused, k.String()+": "+err.Error())
				n.logger().Warn("pool snapshot not used; replaying the whole log", "pool", k.String(), "reason", err.Error())
			}
			return 0, 0, nil
		}
		// The pending transactions come back as they were before the
		// records after the snapshot, which are then replayed.
		for _, t := range res.pending {
			n.eng.HandleTx(t, engine.Stored)
		}
		ss := n.snap(k)
		ss.written, ss.records, ss.bytes = res.header.written, res.header.records, res.bytes
		n.replay.Snapshots++
		n.replay.Covered += res.header.records
		return res.header.off, res.header.records, nil
	}}
	l, op, err := store.OpenFuncAt(path, store.Options{NoSync: n.pol.NoSync}, from, func(ol *store.Log, r store.Record, loc store.Loc) error {
		win = append(win, storedRec{r, loc})
		if len(win) == replayWindow {
			flush()
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("node: %s: %w", path, err)
	}
	if len(win) > 0 {
		flush()
	}
	n.openBodies(k, l)
	n.logs[k] = l
	n.replay.Pools++
	n.replay.Cut += op.Cut
	if op.Cut > 0 {
		n.logger().Warn("dropped an unfinished write", "pool", k.String(), "bytes", op.Cut)
	}
	ss := n.snap(k)
	ss.since = l.Records() - ss.records
	if check != nil && check.off == l.Size() && check.records == l.Records() {
		n.replay.Reverified++
		if fp, err := n.eng.Fingerprint(k); err != nil || fp != check.print {
			n.replay.Differed++
			n.logger().Error("a full replay built a different pool than its snapshot held; the replay is used", "pool", k.String())
		} else {
			n.logger().Info("full replay matches the pool snapshot", "pool", k.String())
		}
	}
	n.eng.Ledger(k, func(p *ledger.Pool) { p.ReleaseBodies() })
	if err := n.maybeCompact(k); err != nil {
		return err
	}
	// A start that replayed many records, or checked them all, writes a
	// snapshot now, so the next start need not.
	if ss.since >= nodepolicy.BoundSnapshotEvery.Or(n.pol.SnapshotEvery) || n.reverify && ss.since > 0 {
		return n.snapshotLocked(k)
	}
	return nil
}

// replayWindow is how many stored records are decoded and warmed at a
// time on start-up (ORANGE-021).
const replayWindow = 1024

// decoded is one stored record, decoded.
type decoded struct {
	kind store.Kind
	e    *entry.Entry
	l    layer.Layer
	t    *tx.Tx
	err  error
}

func decodeAll(rs []store.Record) []decoded {
	out := make([]decoded, len(rs))
	for i, r := range rs {
		d := decoded{kind: r.Kind}
		switch r.Kind {
		case store.KindEntry:
			d.e, d.err = entry.Decode(r.Data)
		case store.KindLayer:
			d.l, d.err = layer.Decode(r.Data)
		case store.KindTx:
			d.t, d.err = tx.Decode(r.Data)
		default:
			d.err = fmt.Errorf("unknown record kind %d", r.Kind)
		}
		out[i] = d
	}
	return out
}

// apply replays one decoded record and reports whether the engine kept it.
func (n *Node) apply(k engine.PoolKey, d decoded) bool {
	if d.err != nil {
		return false
	}
	var err error
	switch d.kind {
	case store.KindEntry:
		_, err = n.eng.HandleEntry(k, d.e, engine.Stored)
	case store.KindLayer:
		_, err = n.eng.HandleLayer(k, d.l, engine.Stored)
	case store.KindTx:
		_, err = n.eng.HandleTx(d.t, engine.Stored)
	}
	return err == nil
}

// liveItem is one record a compaction writes: an entry copied from its
// place in the old log, or bytes from memory.
type liveItem struct {
	kind store.Kind
	h    hash.Hash // for an entry
	loc  store.Loc
	at   bool // copy the record at loc
	data []byte
}

// liveRecords returns what pool k still needs: the entries it holds, each
// followed by its layers, in an order that rebuilds the same selected
// ledger, then the pending transactions. An entry whose record is in the
// log is copied from there when the log is rewritten, so a compaction
// never reads every body into memory (D172).
func (n *Node) liveRecords(k engine.PoolKey) ([]liveItem, error) {
	b := n.poolBodies(k)
	var items []liveItem
	var bad error
	err := n.eng.Ledger(k, func(p *ledger.Pool) {
		for _, h := range p.ReplayOrder() {
			it := liveItem{kind: store.KindEntry, h: h}
			if b != nil {
				it.loc, it.at = b.locOf(h)
			}
			if !it.at {
				e, err := p.Entry(h)
				if err != nil {
					bad = fmt.Errorf("node: compact %s: no body for entry %s", k, h.Short())
					return
				}
				it.data = e.Encode()
			}
			items = append(items, it)
			for _, l := range p.Layers(h) {
				items = append(items, liveItem{kind: store.KindLayer, data: l.Encode()})
			}
		}
	})
	if err == nil {
		err = bad
	}
	if err != nil {
		return nil, err
	}
	pending, err := n.eng.Pending(k)
	if err != nil {
		return nil, err
	}
	for _, it := range pending {
		items = append(items, liveItem{kind: store.KindTx, data: it.Tx.Encode()})
	}
	return items, nil
}

func (n *Node) maybeCompact(k engine.PoolKey) error {
	l := n.logs[k]
	if l == nil || l.Records() < n.cmin {
		return nil
	}
	items, err := n.liveRecords(k)
	if err != nil {
		return err
	}
	if l.Records() <= compactRatio*len(items) {
		return nil
	}
	return n.compact(k, items)
}

// compact rewrites pool k's log with items, copying entry records from
// the old file, and gives the pool the new locations. It holds the pool's
// location lock throughout, so body reads wait for the new locations.
func (n *Node) compact(k engine.PoolKey, items []liveItem) error {
	l := n.logs[k]
	b := n.openBodies(k, l)
	before := l.Records()
	// The snapshot covers the old file: it goes first, so it never
	// outlives the log it describes, and a new one follows (D178).
	if err := n.dropSnapshot(k); err != nil {
		return err
	}
	if err := n.rewrite(k, l, b, items); err != nil {
		return err
	}
	// The pool now runs with the first-sighting order a replay of the
	// new log gives it, so a restart builds the very pool that runs.
	n.eng.Ledger(k, func(p *ledger.Pool) { p.Renumber() })
	n.logger().Info("compacted store", "pool", k.String(), "from", before, "to", len(items))
	return n.snapshotLocked(k)
}

// rewrite rewrites pool k's log l with items and gives b the new
// locations, holding b's lock throughout.
func (n *Node) rewrite(k engine.PoolKey, l *store.Log, b *poolBodies, items []liveItem) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	locs, err := l.RewriteFunc(len(items), func(i int) (store.Record, error) {
		it := items[i]
		if !it.at {
			return store.Record{Kind: it.kind, Data: it.data}, nil
		}
		r, err := l.ReadAt(it.loc)
		if err != nil {
			return r, err
		}
		if r.Kind != store.KindEntry {
			return r, fmt.Errorf("%w: entry %s's record is of kind %d", store.ErrBadRecord, it.h.Short(), r.Kind)
		}
		return r, nil
	})
	if err != nil {
		return fmt.Errorf("node: compact %s: %w", k, err)
	}
	m := make(map[hash.Hash]store.Loc, len(items))
	for i, it := range items {
		if it.kind == store.KindEntry {
			m[it.h] = locs[i]
		}
	}
	b.locs = m
	return nil
}

// Compact rewrites the store of pool k with only what the node still holds.
func (n *Node) Compact(k engine.PoolKey) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.closed {
		return ErrClosed
	}
	if n.logs[k] == nil {
		return nil
	}
	if err := n.eng.Err(); err != nil {
		return err
	}
	items, err := n.liveRecords(k)
	if err != nil {
		return err
	}
	return n.compact(k, items)
}

// record appends an accepted item to the store of pool k, opening the
// store on first use. It returns once the item is written, with a function
// that waits until it is on disk. The caller calls that after releasing
// the node's lock, so items taken in from many peers share one fsync
// instead of each holding the lock through its own (ORANGE-005).
//
// An entry's record place is noted under its hash h, so the pool can drop
// the body from memory and read it back (D172).
func (n *Node) record(k engine.PoolKey, kind store.Kind, data []byte, h hash.Hash) (func() error, error) {
	l := n.logs[k]
	if l == nil {
		var err error
		// A log of a pool not replayed at start (followed since) is
		// appended to; its old records are not read (D172).
		l, _, err = store.OpenFunc(n.logPath(k), store.Options{NoSync: n.pol.NoSync}, func(*store.Log, store.Record, store.Loc) error { return nil })
		if err != nil {
			return nil, err
		}
		n.logs[k] = l
		n.openBodies(k, l)
	}
	t, loc, err := l.AppendLaterAt(store.Record{Kind: kind, Data: data})
	if err != nil {
		return nil, n.storeFailed(k, err)
	}
	if kind == store.KindEntry {
		if b := n.poolBodies(k); b != nil {
			b.add(h, loc)
		}
	}
	due := n.noteAppend(k)
	return func() error {
		if err := l.Durable(t); err != nil {
			return n.storeFailed(k, err)
		}
		if due {
			n.snapshotLater(k)
		}
		return nil
	}, nil
}

// storeFailed wraps an error of pool k's store. A store stopped by a
// failed write takes nothing more for the pool, so from then on every item
// of the pool fails with this error and is neither stored nor forwarded.
// The node says so once, as an error, since only a restart, which replays
// what is really on disk, brings the pool back (RED-010, decision D108).
func (n *Node) storeFailed(k engine.PoolKey, err error) error {
	if errors.Is(err, store.ErrFailed) {
		if _, told := n.failed.LoadOrStore(k, true); !told {
			n.logger().Error("pool store stopped after a failed write; restart the node to replay what is on disk", "pool", k.String(), "err", err)
		}
	}
	return fmt.Errorf("node: store %s: %w", k, err)
}

// Event reports an item the node kept, after it is written to the store.
// The fsync that makes it survive a power loss may still be running: the
// Handle method that kept the item returns only after it (ORANGE-005).
// Exactly one of Tx, Entry and Layer is set.
type Event struct {
	Key    engine.PoolKey
	Tx     *tx.Tx
	Entry  *entry.Entry
	Layer  *layer.Layer
	Origin engine.Origin
	// From names the peer that sent the item, or is empty.
	From string
	// Forward is the engine's answer: whether the item must be passed on.
	Forward bool
	// Switch is set when the item changed the selected ledger.
	Switch *ledger.Switch
	// Dropped lists pending transactions the item made invalid.
	Dropped []hash.Hash
	// Weight is the selected ledger's weight after the item: the node's
	// stated weight for the pool.
	Weight weight.Weight
}

// Observe registers f to receive an Event for every item the node keeps
// from now on, in the order they were kept. f runs while the node's lock
// is held: it must return quickly and must not call the node.
func (n *Node) Observe(f func(Event)) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.observers = append(n.observers, f)
}

func (n *Node) emit(ev Event) {
	if len(n.observers) == 0 {
		return
	}
	if _, _, w, err := n.eng.Tip(ev.Key); err == nil {
		ev.Weight = w
	}
	for _, f := range n.observers {
		f(ev)
	}
}

// HandleTx takes in a transaction and stores it if the engine keeps it.
func (n *Node) HandleTx(t *tx.Tx, origin engine.Origin) (engine.TxResult, error) {
	return n.HandleTxFrom(t, origin, "")
}

// HandleTxFrom is HandleTx for an item sent by the named peer.
func (n *Node) HandleTxFrom(t *tx.Tx, origin engine.Origin, from string) (engine.TxResult, error) {
	n.eng.PreverifyTx(t)
	var durable func() error
	res, err := func() (engine.TxResult, error) {
		n.mu.Lock()
		defer n.mu.Unlock()
		if n.closed {
			return engine.TxResult{}, ErrClosed
		}
		res, err := n.eng.HandleTx(t, origin)
		if err != nil {
			return res, err
		}
		if durable, err = n.record(res.Key, store.KindTx, t.Encode(), hash.Hash{}); err != nil {
			return res, err
		}
		n.emit(Event{Key: res.Key, Tx: t, Origin: origin, From: from, Forward: res.Forward})
		return res, nil
	}()
	if err != nil {
		return res, err
	}
	return res, durable()
}

// HandleEntry takes in an entry of pool k and stores it if kept.
func (n *Node) HandleEntry(k engine.PoolKey, e *entry.Entry, origin engine.Origin) (engine.EntryResult, error) {
	return n.HandleEntryFrom(k, e, origin, "")
}

// HandleEntryFrom is HandleEntry for an item sent by the named peer.
func (n *Node) HandleEntryFrom(k engine.PoolKey, e *entry.Entry, origin engine.Origin, from string) (engine.EntryResult, error) {
	n.eng.PreverifyEntry(k, e, origin)
	var durable func() error
	res, err := func() (engine.EntryResult, error) {
		n.mu.Lock()
		defer n.mu.Unlock()
		if n.closed {
			return engine.EntryResult{}, ErrClosed
		}
		res, err := n.eng.HandleEntry(k, e, origin)
		if err != nil {
			return res, err
		}
		if durable, err = n.record(k, store.KindEntry, e.Encode(), res.Hash); err != nil {
			return res, err
		}
		// The entry is written, so the pool can drop its body now, into
		// the bounded cache. Left to the pool's next AddEntry, the newest
		// entry of every pool stayed in memory outside entry_cache_mib,
		// up to one body per followed pool for as long as the pool is
		// quiet (GREEN-053).
		n.eng.Ledger(k, func(p *ledger.Pool) { p.ReleaseBodies() })
		n.emit(Event{Key: k, Entry: e, Origin: origin, From: from, Forward: res.Forward, Switch: res.Switch, Dropped: res.Dropped})
		return res, nil
	}()
	if err != nil {
		return res, err
	}
	return res, durable()
}

// HandleLayer takes in a layer of pool k and stores it if kept.
func (n *Node) HandleLayer(k engine.PoolKey, l layer.Layer, origin engine.Origin) (engine.LayerResult, error) {
	return n.HandleLayerFrom(k, l, origin, "")
}

// HandleLayerFrom is HandleLayer for an item sent by the named peer.
func (n *Node) HandleLayerFrom(k engine.PoolKey, l layer.Layer, origin engine.Origin, from string) (engine.LayerResult, error) {
	var durable func() error
	res, err := func() (engine.LayerResult, error) {
		n.mu.Lock()
		defer n.mu.Unlock()
		if n.closed {
			return engine.LayerResult{}, ErrClosed
		}
		res, err := n.eng.HandleLayer(k, l, origin)
		if err != nil {
			return res, err
		}
		if durable, err = n.record(k, store.KindLayer, l.Encode(), hash.Hash{}); err != nil {
			return res, err
		}
		n.emit(Event{Key: k, Layer: &l, Origin: origin, From: from, Forward: res.Forward, Switch: res.Switch, Dropped: res.Dropped})
		return res, nil
	}()
	if err != nil {
		return res, err
	}
	return res, durable()
}

// Build makes an entry from the pending transactions of pool k (see
// engine.Engine.Build) and stores it.
func (n *Node) Build(k engine.PoolKey, maxTries uint64) (*entry.Entry, engine.EntryResult, error) {
	return n.BuildContext(context.Background(), k, maxTries)
}

// BuildContext is Build that gives up, with ctx's error, once ctx is
// done. The search for a nonce holds neither the node's lock nor the
// engine's, so a long build never stalls peers or the RPC (RED-006).
func (n *Node) BuildContext(ctx context.Context, k engine.PoolKey, maxTries uint64) (*entry.Entry, engine.EntryResult, error) {
	return n.BuildWith(ctx, k, maxTries, engine.Grind{})
}

// BuildWith is BuildContext searching as g says (package grind).
func (n *Node) BuildWith(ctx context.Context, k engine.PoolKey, maxTries uint64, g engine.Grind) (*entry.Entry, engine.EntryResult, error) {
	n.mu.Lock()
	closed := n.closed
	n.mu.Unlock()
	if closed {
		return nil, engine.EntryResult{}, ErrClosed
	}
	e, err := n.eng.BuildEntryWith(ctx, k, maxTries, g)
	if err != nil {
		return e, engine.EntryResult{Key: k}, err
	}
	res, err := n.HandleEntry(k, e, engine.Local)
	return e, res, err
}

// BuildOn is BuildWith built as o says (engine.BuildOn, D161): on a
// chosen parent, from chosen transactions. It also returns the
// transactions left out and why.
func (n *Node) BuildOn(ctx context.Context, k engine.PoolKey, maxTries uint64, g engine.Grind, o engine.BuildOn) (*entry.Entry, engine.EntryResult, []ledger.Refused, error) {
	n.mu.Lock()
	closed := n.closed
	n.mu.Unlock()
	if closed {
		return nil, engine.EntryResult{}, nil, ErrClosed
	}
	e, refused, err := n.eng.BuildEntryOn(ctx, k, maxTries, g, o)
	if err != nil {
		return e, engine.EntryResult{Key: k}, refused, err
	}
	res, err := n.HandleEntry(k, e, engine.Local)
	return e, res, refused, err
}

// Work grinds a layer on an entry of pool k (see engine.Engine.Work) and
// stores it.
func (n *Node) Work(k engine.PoolKey, entryHash hash.Hash, start, tries uint64) (layer.Layer, engine.LayerResult, error) {
	if tries == 0 {
		return layer.Layer{}, engine.LayerResult{Key: k}, engine.ErrNoStrongerLayer
	}
	return n.WorkWith(context.Background(), k, entryHash, start, tries, engine.Grind{})
}

// WorkWith is Work searching as g says (package grind); tries 0 grinds
// until ctx is done, and the strongest layer found by then is kept (see
// engine.Engine.GrindLayerWith).
func (n *Node) WorkWith(ctx context.Context, k engine.PoolKey, entryHash hash.Hash, start, tries uint64, g engine.Grind) (layer.Layer, engine.LayerResult, error) {
	n.mu.Lock()
	closed := n.closed
	n.mu.Unlock()
	if closed {
		return layer.Layer{}, engine.LayerResult{}, ErrClosed
	}
	// Grinding holds neither lock, so it never stalls peers or the RPC.
	type found struct {
		l layer.Layer
		s int
	}
	var mu sync.Mutex
	var kept []found
	if g.Floor > 0 {
		g.Keep = func(l layer.Layer, s int) {
			mu.Lock()
			kept = append(kept, found{l, s})
			mu.Unlock()
		}
	}
	l, err := n.eng.GrindLayerWith(ctx, k, entryHash, start, tries, g)
	if err != nil {
		return l, engine.LayerResult{Key: k}, err
	}
	res, err := n.HandleLayer(k, l, engine.Local)
	// With a floor, the other layers found above it go in too: each adds
	// its own credit, and none takes any away (ORANGE-010, D118). The
	// strongest go first: at most KeepMax of a search (ORANGE-016's
	// layer_rate), at most MaxKeptLayers, and no more than KeptPerSecond
	// across searches, under the rate peers accept; the rest are dropped,
	// which loses only credit the node chose not to send (RED-018, D120).
	slices.SortFunc(kept, func(a, b found) int {
		if a.s != b.s {
			return b.s - a.s
		}
		return cmp.Compare(a.l.Nonce, b.l.Nonce)
	})
	took := 0
	for _, o := range kept {
		if g.KeepMax > 0 && took >= g.KeepMax {
			break
		}
		if o.l.Nonce == l.Nonce {
			continue
		}
		if took == MaxKeptLayers || !n.kept.take(time.Now()) {
			break
		}
		took++
		if _, kerr := n.HandleLayer(k, o.l, engine.Local); kerr == nil && g.KeptCount != nil {
			g.KeptCount.Add(1)
		}
	}
	return l, res, err
}

// MaxKeptLayers is the most layers above the floor one search sends
// besides its strongest, and KeptPerSecond the most a node sends a second
// across searches: half what a peer accepts from one address (100 a
// second, tracking's layer budget), so an honest grinder is never flagged.
const (
	MaxKeptLayers = 16
	KeptPerSecond = 50
)

// keptBucket is a token bucket of KeptPerSecond, holding a second's worth.
type keptBucket struct {
	mu     sync.Mutex
	tokens float64
	at     time.Time
}

func (b *keptBucket) take(now time.Time) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.at.IsZero() {
		b.tokens = KeptPerSecond
	} else {
		b.tokens = min(KeptPerSecond, b.tokens+now.Sub(b.at).Seconds()*KeptPerSecond)
	}
	b.at = now
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

// Engine returns the engine, for reading. Changes must go through the
// node's own Handle calls, or they are not stored.
func (n *Node) Engine() *engine.Engine { return n.eng }

// Policy returns the node policy the node was opened with.
func (n *Node) Policy() nodepolicy.Policy { return n.pol }

// Running returns the node policy the node runs with now: the one it was
// opened with, with the limits applied in place since (D168).
func (n *Node) Running() nodepolicy.Policy {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.run
}

// SetRunning records p as the policy the node runs with, after its limits
// were applied in place (D168).
func (n *Node) SetRunning(p nodepolicy.Policy) {
	n.mu.Lock()
	n.run = p
	n.mu.Unlock()
}

// UpdateRunning changes the policy the node runs with in place, for an
// RPC edit that the running node applied itself (a manual peer it added
// to its book, a grind schedule it started), so that a later difference
// between node.json and the running policy means an edit nothing applied
// (RED-080).
func (n *Node) UpdateRunning(f func(*nodepolicy.Policy)) {
	n.mu.Lock()
	defer n.mu.Unlock()
	run := n.run
	run.P2P.Peers = slices.Clone(run.P2P.Peers)
	f(&run)
	n.run = run
}

// Dir returns the data directory.
func (n *Node) Dir() string { return n.dir }

// FileLimit reports the open file limit checked at Open (ORANGE-037).
func (n *Node) FileLimit() nodepolicy.FileLimit { return n.files }

// Replayed reports what Open read back from the stores.
func (n *Node) Replayed() ReplayStats { return n.replay }

// StoreInfo describes one pool's store.
type StoreInfo struct {
	Key     engine.PoolKey
	Path    string
	Records int
}

// Stores lists the open stores, by pool.
func (n *Node) Stores() []StoreInfo {
	n.mu.Lock()
	defer n.mu.Unlock()
	out := make([]StoreInfo, 0, len(n.logs))
	for k, l := range n.logs {
		out = append(out, StoreInfo{Key: k, Path: l.Path(), Records: l.Records()})
	}
	sort.Slice(out, func(i, j int) bool { return strings.Compare(out[i].Path, out[j].Path) < 0 })
	return out
}

func (n *Node) closeLogs() error {
	var errs []error
	for _, l := range n.logs {
		errs = append(errs, l.Close())
	}
	return errors.Join(errs...)
}

// Close compacts stores that need it, closes them and releases the data
// directory.
func (n *Node) Close() error {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.closed {
		return nil
	}
	n.closed = true
	var errs []error
	// A failed engine may hold a half-changed pool: its stores are left
	// as written, never rewritten from it (D174). Its snapshots go too,
	// so the next start replays and checks every log whole: a failure on
	// reading a log back (a ledger.ReadFault) is then met at start, which
	// cuts that log at the damage, rather than skipped by a snapshot
	// (D178).
	if n.eng.Err() != nil {
		for k := range n.logs {
			errs = append(errs, n.dropSnapshot(k))
		}
	} else {
		for k := range n.logs {
			errs = append(errs, n.maybeCompact(k))
			// A snapshot as of the end of the log, unless the last one
			// (perhaps the compaction's) already is (D178).
			b := n.poolBodies(k)
			if s := n.snap(k); s.since > 0 || s.written.IsZero() || b != nil && b.damaged.Load() {
				errs = append(errs, n.snapshotLocked(k))
			}
		}
	}
	errs = append(errs, n.closeLogs())
	n.unlock()
	return errors.Join(errs...)
}
