package node

import (
	"bufio"
	"bytes"
	"cmp"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"io"
	"os"
	"runtime/debug"
	"slices"
	"sync"
	"time"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/encoding"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/engine"
	phash "github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/ledger"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/nodepolicy"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/store"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/tx"
)

// Pool snapshots (D178). Beside each pool's log the node keeps one
// snapshot file, <index>.snap, written through a temporary file and a
// rename, so it is whole or absent. It holds:
//
//   - a header: the build that wrote it, the pool, how far into the log it
//     covers (bytes and records), a tag of the log's bytes there
//     (store.Tag), when it was written, and the pool's fingerprint;
//   - the sidecar index: where in the log each held entry's record is, so
//     a start reads no record the snapshot covers;
//   - the pool's skeleton and tip state (ledger.Pool.Snapshot);
//   - the pending transactions;
//   - a SHA-256 of everything before it.
//
// At start a node uses the snapshot only if every check passes: the
// checksum (a torn or damaged file), the pool (a file swapped from another
// pool), the build (a snapshot from another version is replayed over, so
// an upgrade never trusts state an older build computed), and the tag (a
// log compacted, cut short or replaced since). It then replays only the
// records after it. Anything else, or -reverify, or no_snapshots, means a
// full replay, as before. A snapshot is the node's own data: it never
// comes from a peer, and nothing it holds is a consensus rule.
//
// A compaction removes the snapshot before it rewrites the log and writes
// a new one after, and Close writes one when the log grew since the last;
// while running, one is written every snapshot_every records.

var snapMagic = []byte("PoO-snap\x01")

// Record kinds of a snapshot file.
const (
	snapKindHeader  = 'h'
	snapKindIndex   = 'i'
	snapKindLedger  = 'l'
	snapKindPending = 'p'
	snapKindEnd     = 'z'
)

// indexPerRecord is how many entry locations one index record holds.
const indexPerRecord = 4096

// maxSnapRecord bounds one record of a snapshot file when it is read.
const maxSnapRecord = 64 << 20

// errDamaged: a record of the pool's log failed to read back, so no
// snapshot of it is written (poolBodies.damaged).
var errDamaged = errors.New("node: a record of the pool log failed to read back; no snapshot is written until a start replays the log whole")

// errSnapshot wraps every reason a snapshot is not used.
var errSnapshot = errors.New("snapshot not used")

// snapshotSemantics is bumped by hand in the same commit as any change to
// what a pool is or what a snapshot of it holds (a selection or tie rule,
// the state after a new kind of transaction, a field of the skeleton,
// Snapshot or Restore), even one that leaves the file format alone. It is
// part of buildID, so such a change retires every snapshot written before
// it, whatever the build is called (RED-087, D181).
const snapshotSemantics = 1

// release is the version and commit the command line was built as, set
// by SetRelease (cli sets them by -X, which a release build uses, since it
// records no VCS information).
var release struct{ version, commit string }

// SetRelease names the release this binary is, for buildID. The cli
// package calls it at init with its Version and Commit (RED-087, D181).
func SetRelease(version, commit string) { release.version, release.commit = version, commit }

// Release is the release this binary is, as SetRelease named it, for
// GET /v1/status (W-10, D184).
type Release struct {
	Version string `json:"version"`
	Commit  string `json:"commit,omitempty"`
}

// RunningRelease returns the release this binary is.
func RunningRelease() Release { return Release{Version: release.version, Commit: release.commit} }

// buildID names the code that wrote a snapshot: the release version and
// commit (set by -X in a release build), the module version and VCS
// revision when the binary records them, a hash of the executable when
// its tree was modified (so two dirty builds of one commit differ), and
// snapshotSemantics. A snapshot from another build is not used, so an
// upgrade replays once (D178, RED-087).
var buildID = sync.OnceValue(func() string {
	return makeBuildID(release.version, release.commit, executableHash)
})

func makeBuildID(version, commit string, exe func() string) string {
	id := fmt.Sprintf("poo snap%d", snapshotSemantics)
	if version != "" {
		id += " " + version
	}
	if commit != "" {
		id += " " + commit
	}
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return id
	}
	if v := bi.Main.Version; v != "" && v != "(devel)" {
		id += " " + v
	}
	for _, s := range bi.Settings {
		switch s.Key {
		case "vcs.revision":
			if s.Value != commit {
				id += " " + s.Value
			}
		case "vcs.modified":
			if s.Value == "true" {
				id += "+modified " + exe()
			}
		}
	}
	return id
}

// executableHash is a short SHA-256 of this executable's file, or "" when
// it cannot be read.
func executableHash() string {
	p, err := os.Executable()
	if err != nil {
		return ""
	}
	f, err := os.Open(p)
	if err != nil {
		return ""
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return ""
	}
	return hex.EncodeToString(h.Sum(nil)[:8])
}

// snapHeader is what a snapshot says about itself.
type snapHeader struct {
	build   string
	key     engine.PoolKey
	off     int64 // log bytes covered
	records int   // log records covered
	tag     [32]byte
	written time.Time
	print   phash.Hash
}

func (h snapHeader) encode() []byte {
	w := encoding.NewWriter(256)
	w.VarBytes([]byte(h.build)).Raw(h.key.Series[:]).U64(h.key.Index).U64(uint64(h.off)).U64(uint64(h.records)).
		Raw(h.tag[:]).U64(uint64(h.written.UnixNano())).Raw(h.print[:])
	return w.Bytes()
}

func decodeSnapHeader(b []byte) (snapHeader, error) {
	var h snapHeader
	r := encoding.NewReader(b)
	h.build = string(r.VarBytes(1024))
	r.Into(h.key.Series[:])
	h.key.Index = r.U64()
	h.off = int64(r.U64())
	h.records = int(r.U64())
	r.Into(h.tag[:])
	h.written = time.Unix(0, int64(r.U64()))
	r.Into(h.print[:])
	return h, r.Finish()
}

// snapWriter frames snapshot records and sums them.
type snapWriter struct {
	w   *bufio.Writer
	sum hash.Hash
}

func (s *snapWriter) write(kind byte, data []byte) error {
	var hdr [5]byte
	binary.BigEndian.PutUint32(hdr[:4], uint32(len(data)+1))
	hdr[4] = kind
	s.sum.Write(hdr[:])
	s.sum.Write(data)
	if _, err := s.w.Write(hdr[:]); err != nil {
		return err
	}
	_, err := s.w.Write(data)
	return err
}

// snapReader reads the records snapWriter framed, checking the sum at the
// end record.
type snapReader struct {
	r    *bufio.Reader
	sum  hash.Hash
	peek *snapRec
	done bool
}

type snapRec struct {
	kind byte
	data []byte
}

func (s *snapReader) next() (snapRec, error) {
	if s.peek != nil {
		r := *s.peek
		s.peek = nil
		return r, nil
	}
	if s.done {
		return snapRec{}, fmt.Errorf("%w: records after the end", errSnapshot)
	}
	var hdr [5]byte
	if _, err := io.ReadFull(s.r, hdr[:]); err != nil {
		return snapRec{}, fmt.Errorf("%w: cut short", errSnapshot)
	}
	n := binary.BigEndian.Uint32(hdr[:4])
	if n < 1 || n > maxSnapRecord {
		return snapRec{}, fmt.Errorf("%w: a record of %d bytes", errSnapshot, n)
	}
	data := make([]byte, n-1)
	if _, err := io.ReadFull(s.r, data); err != nil {
		return snapRec{}, fmt.Errorf("%w: cut short", errSnapshot)
	}
	if hdr[4] == snapKindEnd {
		want := s.sum.Sum(nil)
		if !bytes.Equal(data, want) {
			return snapRec{}, fmt.Errorf("%w: checksum differs (a damaged file)", errSnapshot)
		}
		if _, err := s.r.ReadByte(); err != io.EOF {
			return snapRec{}, fmt.Errorf("%w: bytes after the end", errSnapshot)
		}
		s.done = true
		return snapRec{kind: snapKindEnd}, nil
	}
	s.sum.Write(hdr[:])
	s.sum.Write(data)
	return snapRec{kind: hdr[4], data: data}, nil
}

func (s *snapReader) unread(r snapRec) { s.peek = &r }

func (n *Node) snapPath(k engine.PoolKey) string { return n.logPath(k) + ".snap" }

// snapState is what the node knows of one pool's snapshot.
type snapState struct {
	written time.Time
	records int // log records it covers
	bytes   int64
	// since counts records appended since it was written (or since start
	// when there is none).
	since int
	// writing is set while a snapshot of the pool is being written.
	writing bool
	// epoch counts compactions; a snapshot collected before one is not
	// put in place after it.
	epoch int
}

// snap returns pool k's snapshot state, making it. n.mu is held.
func (n *Node) snap(k engine.PoolKey) *snapState {
	s := n.snaps[k]
	if s == nil {
		s = &snapState{}
		n.snaps[k] = s
	}
	return s
}

// snapshotsOn reports whether the node writes and uses snapshots. n.mu
// is held, or the node is opening.
func (n *Node) snapshotsOn() bool { return !n.run.NoSnapshots }

// collected is a snapshot gathered under the node's lock, to be written
// out without it.
type collected struct {
	key     engine.PoolKey
	header  snapHeader
	ledger  [][]byte
	pending [][]byte
	index   []indexed
	epoch   int
}

type indexed struct {
	h   phash.Hash
	loc store.Loc
}

// collect gathers pool k's snapshot: the engine's skeleton and pending
// transactions and the body index, all as of the log's current end.
// n.mu is held, so the log and the engine describe the same moment.
func (n *Node) collect(k engine.PoolKey) (*collected, error) {
	l := n.logs[k]
	b := n.poolBodies(k)
	if l == nil || b == nil {
		return nil, ledger.ErrNotSnapshottable
	}
	if b.damaged.Load() {
		return nil, errDamaged
	}
	c := &collected{key: k, epoch: n.snap(k).epoch}
	print, err := n.eng.Snapshot(k, func(rec []byte) error {
		c.ledger = append(c.ledger, append([]byte(nil), rec...))
		return nil
	}, func(t *tx.Tx) error {
		c.pending = append(c.pending, t.Encode())
		return nil
	})
	if err != nil {
		return nil, err
	}
	var held []phash.Hash
	n.eng.Ledger(k, func(p *ledger.Pool) { held = p.ReplayOrder() })
	b.mu.Lock()
	for _, h := range held {
		loc, ok := b.locs[h]
		if !ok {
			b.mu.Unlock()
			return nil, ledger.ErrNotSnapshottable
		}
		c.index = append(c.index, indexed{h, loc})
	}
	b.mu.Unlock()
	c.header = snapHeader{build: buildID(), key: k, off: l.Size(), records: l.Records(), written: time.Now(), print: print}
	return c, nil
}

// writeCollected writes c to a temporary file beside the log and returns
// its path and size; the caller renames it into place.
func (n *Node) writeCollected(c *collected) (string, int64, error) {
	tag, err := store.Tag(n.logPath(c.key), c.header.off)
	if err != nil {
		return "", 0, err
	}
	c.header.tag = tag
	tmp := n.snapPath(c.key) + ".tmp"
	f, err := os.OpenFile(tmp, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return "", 0, err
	}
	sw := &snapWriter{w: bufio.NewWriterSize(f, 1<<16), sum: sha256.New()}
	sw.sum.Write(snapMagic)
	err = func() error {
		if _, err := sw.w.Write(snapMagic); err != nil {
			return err
		}
		if err := sw.write(snapKindHeader, c.header.encode()); err != nil {
			return err
		}
		for i := 0; i < len(c.index); i += indexPerRecord {
			part := c.index[i:min(len(c.index), i+indexPerRecord)]
			w := encoding.NewWriter(4 + 48*len(part))
			w.Count(len(part))
			for _, x := range part {
				w.Raw(x.h[:]).U64(uint64(x.loc.Off)).U64(uint64(x.loc.Len))
			}
			if err := sw.write(snapKindIndex, w.Bytes()); err != nil {
				return err
			}
		}
		for _, rec := range c.ledger {
			if err := sw.write(snapKindLedger, rec); err != nil {
				return err
			}
		}
		for _, rec := range c.pending {
			if err := sw.write(snapKindPending, rec); err != nil {
				return err
			}
		}
		var hdr [5]byte
		binary.BigEndian.PutUint32(hdr[:4], uint32(sha256.Size+1))
		hdr[4] = snapKindEnd
		if _, err := sw.w.Write(hdr[:]); err != nil {
			return err
		}
		if _, err := sw.w.Write(sw.sum.Sum(nil)); err != nil {
			return err
		}
		if err := sw.w.Flush(); err != nil {
			return err
		}
		if !n.pol.NoSync {
			return f.Sync()
		}
		return nil
	}()
	size, _ := f.Seek(0, io.SeekCurrent)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(tmp)
		return "", 0, err
	}
	return tmp, size, nil
}

// snapshotLocked writes pool k's snapshot with n.mu held throughout, as
// Close and a compaction do. A pool that cannot be snapshotted now (a
// body not written yet) is skipped quietly.
func (n *Node) snapshotLocked(k engine.PoolKey) error {
	if !n.snapshotsOn() {
		return nil
	}
	c, err := n.collect(k)
	if errors.Is(err, errDamaged) {
		return n.dropSnapshot(k)
	}
	if errors.Is(err, ledger.ErrNotSnapshottable) {
		return nil
	}
	if err != nil {
		return err
	}
	tmp, size, err := n.writeCollected(c)
	if err != nil {
		return fmt.Errorf("node: snapshot %s: %w", k, err)
	}
	return n.placeSnapshot(c, tmp, size)
}

// placeSnapshot renames a written snapshot into place. n.mu is held.
func (n *Node) placeSnapshot(c *collected, tmp string, size int64) error {
	s := n.snap(c.key)
	if s.epoch != c.epoch || n.logs[c.key] == nil {
		os.Remove(tmp) // a compaction came in between
		return nil
	}
	if err := os.Rename(tmp, n.snapPath(c.key)); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("node: snapshot %s: %w", c.key, err)
	}
	s.written, s.records, s.bytes = c.header.written, c.header.records, size
	s.since = n.logs[c.key].Records() - c.header.records
	return nil
}

// noteAppend counts a record appended to pool k's log and reports whether
// a snapshot is due. n.mu is held.
func (n *Node) noteAppend(k engine.PoolKey) bool {
	if !n.snapshotsOn() {
		return false
	}
	s := n.snap(k)
	s.since++
	if s.writing || s.since < n.snapshotDue(k) {
		return false
	}
	s.writing = true
	return true
}

// snapshotDue is how many records pool k's log takes between running
// snapshots: snapshot_every, or a tenth of the log when that is more, so
// the time a gather holds the node's lock, which grows with the pool,
// stays about a tenth of the time spent taking records in (ORANGE-040
// proposal 2). A start after a crash replays at most that many records.
// n.mu is held.
func (n *Node) snapshotDue(k engine.PoolKey) int {
	every := nodepolicy.BoundSnapshotEvery.Or(n.run.SnapshotEvery)
	if l := n.logs[k]; l != nil {
		every = max(every, l.Records()/10)
	}
	return every
}

// snapshotLater writes pool k's snapshot while the node runs: gathered
// under the node's lock, written to disk without it, and put in place
// under it again unless a compaction came in between. It is called after
// the lock is released, by the goroutine whose record made it due.
func (n *Node) snapshotLater(k engine.PoolKey) {
	n.mu.Lock()
	if n.closed {
		n.mu.Unlock()
		return
	}
	c, err := n.collect(k)
	if errors.Is(err, errDamaged) {
		err = n.dropSnapshot(k)
	}
	if c == nil {
		n.snap(k).writing = false
		n.mu.Unlock()
		if err == nil {
			return
		}
		if !errors.Is(err, ledger.ErrNotSnapshottable) && !errors.Is(err, engine.ErrFailed) {
			n.logger().Warn("pool snapshot not written", "pool", k.String(), "err", err)
		}
		return
	}
	n.mu.Unlock()
	tmp, size, err := n.writeCollected(c)
	n.mu.Lock()
	defer n.mu.Unlock()
	n.snap(k).writing = false
	if err == nil && n.closed {
		os.Remove(tmp)
		return
	}
	if err == nil {
		err = n.placeSnapshot(c, tmp, size)
	}
	if err != nil {
		n.logger().Warn("pool snapshot not written", "pool", k.String(), "err", err)
	}
}

// dropSnapshot removes pool k's snapshot, before its log is rewritten.
// n.mu is held.
func (n *Node) dropSnapshot(k engine.PoolKey) error {
	s := n.snap(k)
	s.epoch++
	s.written, s.records, s.bytes = time.Time{}, 0, 0
	if err := os.Remove(n.snapPath(k)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("node: remove snapshot %s: %w", k, err)
	}
	return nil
}

// restored is what loadSnapshot read: where the log's replay starts.
type restored struct {
	header  snapHeader
	pending []*tx.Tx
	bytes   int64
}

// loadSnapshot restores pool k from its snapshot, if there is one it can
// use. On any failure the pool and its body index are left empty, and
// the reason is returned wrapping errSnapshot (or os.ErrNotExist when
// there is none).
func (n *Node) loadSnapshot(k engine.PoolKey, b *poolBodies) (*restored, error) {
	// A first pass checks the whole file's checksum, the pool and the
	// log, so nothing from a damaged or stale file is ever restored.
	h, err := n.readSnapHeader(k)
	if err != nil {
		return nil, err
	}
	if h.build != buildID() {
		return nil, fmt.Errorf("%w: written by %q, this is %q", errSnapshot, h.build, buildID())
	}
	f, err := os.Open(n.snapPath(k))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	sr := &snapReader{r: bufio.NewReaderSize(f, 1<<16), sum: sha256.New()}
	if _, err := io.ReadFull(sr.r, make([]byte, len(snapMagic))); err != nil {
		return nil, fmt.Errorf("%w: cut short", errSnapshot)
	}
	sr.sum.Write(snapMagic)
	if _, err := sr.next(); err != nil { // the header, read above
		return nil, err
	}
	// The index, then the skeleton, which reads bodies through it.
	reset := func() {
		b.mu.Lock()
		b.locs = map[phash.Hash]store.Loc{}
		b.mu.Unlock()
	}
	fail := func(err error) (*restored, error) {
		reset()
		if !errors.Is(err, errSnapshot) {
			err = fmt.Errorf("%w: %v", errSnapshot, err)
		}
		return nil, err
	}
	for {
		rec, err := sr.next()
		if err != nil {
			return fail(err)
		}
		if rec.kind != snapKindIndex {
			sr.unread(rec)
			break
		}
		r := encoding.NewReader(rec.data)
		m := map[phash.Hash]store.Loc{}
		for range r.Count(indexPerRecord, 48) {
			var x phash.Hash
			r.Into(x[:])
			loc := store.Loc{Off: int64(r.U64()), Len: int64(r.U64())}
			if loc.Off < 0 || loc.Len <= 0 || loc.Off+loc.Len > h.off {
				r.Fail(fmt.Errorf("entry %s's record lies outside what it covers", x.Short()))
			}
			m[x] = loc
		}
		if err := r.Finish(); err != nil {
			return fail(fmt.Errorf("index: %v", err))
		}
		b.mu.Lock()
		for x, loc := range m {
			b.locs[x] = loc
		}
		b.mu.Unlock()
	}
	var readErr error
	err = n.eng.Restore(k, func() ([]byte, error) {
		rec, err := sr.next()
		if err != nil {
			readErr = err
			return nil, err
		}
		if rec.kind != snapKindLedger {
			sr.unread(rec)
			return nil, nil
		}
		return rec.data, nil
	})
	if err != nil {
		if readErr != nil {
			err = readErr
		}
		return fail(err)
	}
	// The restored pool must be the pool the snapshot was written from:
	// every start is a differential of Restore against Snapshot, so a pair
	// that does not round-trip is caught and the log replayed whole
	// (RED-088, D181). It costs one pass over what the pool holds.
	if fp, err := n.eng.Fingerprint(k); err != nil || fp != h.print {
		n.eng.Forget(k)
		return fail(fmt.Errorf("%w: the restored pool differs from the one it was written from", errSnapshot))
	}
	out := &restored{header: h, bytes: info.Size()}
	for {
		rec, err := sr.next()
		if err != nil {
			n.eng.Forget(k)
			return fail(err)
		}
		if rec.kind == snapKindEnd {
			break
		}
		if rec.kind != snapKindPending {
			n.eng.Forget(k)
			return fail(fmt.Errorf("%w: a record of kind %q out of place", errSnapshot, rec.kind))
		}
		t, err := tx.Decode(rec.data)
		if err != nil {
			n.eng.Forget(k)
			return fail(fmt.Errorf("pending transaction: %v", err))
		}
		out.pending = append(out.pending, t)
	}
	return out, nil
}

// SnapshotInfo describes one pool's snapshot, for status and doctor.
type SnapshotInfo struct {
	Pool string `json:"pool"`
	// Written is when it was written; zero when the pool has none.
	Written time.Time `json:"written,omitzero"`
	// Records is how many log records it covers, and Since how many the
	// log took after it: what a start would replay.
	Records int   `json:"records"`
	Since   int   `json:"since"`
	Bytes   int64 `json:"bytes"`
}

// Snapshots describes the snapshot of every pool with a log, by pool.
func (n *Node) Snapshots() []SnapshotInfo {
	n.mu.Lock()
	defer n.mu.Unlock()
	var out []SnapshotInfo
	for _, k := range n.sortedLogKeys() {
		s := n.snap(k)
		out = append(out, SnapshotInfo{Pool: k.String(), Written: s.written, Records: s.records, Since: s.since, Bytes: s.bytes})
	}
	return out
}

// readSnapHeader reads pool k's snapshot whole, checking its checksum,
// and returns its header if the log still holds what it covers. It
// restores nothing (-reverify compares a full replay with it).
func (n *Node) readSnapHeader(k engine.PoolKey) (snapHeader, error) {
	f, err := os.Open(n.snapPath(k))
	if err != nil {
		return snapHeader{}, err
	}
	defer f.Close()
	sr := &snapReader{r: bufio.NewReaderSize(f, 1<<16), sum: sha256.New()}
	head := make([]byte, len(snapMagic))
	if _, err := io.ReadFull(sr.r, head); err != nil || !bytes.Equal(head, snapMagic) {
		return snapHeader{}, fmt.Errorf("%w: not a snapshot file", errSnapshot)
	}
	sr.sum.Write(snapMagic)
	var h snapHeader
	for first := true; ; first = false {
		rec, err := sr.next()
		if err != nil {
			return snapHeader{}, err
		}
		if rec.kind == snapKindEnd {
			break
		}
		if first {
			if rec.kind != snapKindHeader {
				return snapHeader{}, fmt.Errorf("%w: no header", errSnapshot)
			}
			if h, err = decodeSnapHeader(rec.data); err != nil {
				return snapHeader{}, fmt.Errorf("%w: header: %v", errSnapshot, err)
			}
		}
	}
	if h.key != k {
		return snapHeader{}, fmt.Errorf("%w: it is pool %s's", errSnapshot, h.key)
	}
	if tag, err := store.Tag(n.logPath(k), h.off); err != nil || tag != h.tag {
		return snapHeader{}, fmt.Errorf("%w: the log no longer holds what it covers", errSnapshot)
	}
	return h, nil
}

// sortedLogKeys returns the pools with an open log, by series and index.
// n.mu is held.
func (n *Node) sortedLogKeys() []engine.PoolKey {
	out := make([]engine.PoolKey, 0, len(n.logs))
	for k := range n.logs {
		out = append(out, k)
	}
	slices.SortFunc(out, func(a, b engine.PoolKey) int {
		if c := a.Series.Compare(b.Series); c != 0 {
			return c
		}
		return cmp.Compare(a.Index, b.Index)
	})
	return out
}

// SnapshotSummary sums up the pools' snapshots for poo status and doctor
// (D178): one line however many pools are followed.
type SnapshotSummary struct {
	// Off is set when no_snapshots is on.
	Off bool `json:"off,omitempty"`
	// Pools counts the pools with a log, and Missing those of them with
	// no snapshot yet.
	Pools   int `json:"pools"`
	Missing int `json:"missing"`
	// Oldest is when the oldest snapshot was written, and AgeSeconds its
	// age now; zero when there is none.
	Oldest     time.Time `json:"oldest,omitzero"`
	AgeSeconds int64     `json:"age_seconds"`
	// Behind is the most records any pool's log took since its snapshot
	// (all of them for a pool with none): the most a start would replay.
	Behind int   `json:"behind"`
	Bytes  int64 `json:"bytes"`
	// Every is snapshot_every as used, and Due the most records any pool
	// takes between running snapshots: Every, or a tenth of a large log
	// (ORANGE-040).
	Every int `json:"every"`
	Due   int `json:"due"`
}

// SnapshotSummary returns the summary of every pool's snapshot.
func (n *Node) SnapshotSummary() SnapshotSummary {
	n.mu.Lock()
	defer n.mu.Unlock()
	out := SnapshotSummary{Off: !n.snapshotsOn(), Every: nodepolicy.BoundSnapshotEvery.Or(n.run.SnapshotEvery)}
	out.Due = out.Every
	for k := range n.logs {
		out.Due = max(out.Due, n.snapshotDue(k))
	}
	for k := range n.logs {
		s := n.snap(k)
		out.Pools++
		out.Behind = max(out.Behind, s.since)
		out.Bytes += s.bytes
		if s.written.IsZero() {
			out.Missing++
			continue
		}
		if out.Oldest.IsZero() || s.written.Before(out.Oldest) {
			out.Oldest = s.written
		}
	}
	if !out.Oldest.IsZero() {
		out.AgeSeconds = int64(time.Since(out.Oldest) / time.Second)
	}
	return out
}
