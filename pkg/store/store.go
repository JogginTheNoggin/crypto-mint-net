// Package store keeps what a full node has accepted for one pool on disk, as
// an append-only log of records, so the node can rebuild its ledgers and
// pending transactions when it starts again.
//
// The store holds bytes, not meaning. Each record is a kind (entry, layer or
// transaction) and the item's canonical byte form. The node replays the log
// through the engine on start-up, so everything is checked again exactly as
// it was the first time; nothing read from disk is trusted.
//
// File format, all integers big-endian:
//
//	header : "PoO-log" 0x01                   (8 bytes)
//	record : U8 kind ∥ U32 length ∥ data ∥ U32 CRC-32C(kind ∥ length ∥ data)
//
// An item larger than MaxRecordSize is written as several records: parts
// of kind KindPart, then a last record of the item's own kind, whose data
// is the rest. The parts and the last record together make one record when
// read back (consensus audit P1, decision D117).
//
// A crash can leave the last record half written. Open keeps every record
// up to the first one that is incomplete or fails its checksum, and cuts the
// file there. What is lost can be fetched from peers again.
//
// Every record has a location (Loc): where its first frame starts and how
// many bytes its frames take, in one generation of the file. ReadAt reads a
// record back by its location, checking every frame's checksum, so a node
// can keep an item's bytes on disk and read them when it needs them
// (D172). A compaction starts a new generation: locations from before it
// are refused, never read at the wrong place.
//
// Compaction rewrites a log with only what the node still holds, through a
// temporary file that is renamed over the old one, so a crash during
// compaction leaves either the old log or the new one.
package store

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"os"
	"path/filepath"
	"sync"
)

// Kind is the type of a record.
type Kind uint8

// Record kinds. Numbers are never reused.
const (
	KindEntry Kind = 1
	KindLayer Kind = 2
	KindTx    Kind = 3
	// KindPart holds part of a larger record; the next record that is not
	// a part ends it.
	KindPart Kind = 4
)

// String names the kind.
func (k Kind) String() string {
	switch k {
	case KindEntry:
		return "entry"
	case KindLayer:
		return "layer"
	case KindTx:
		return "tx"
	case KindPart:
		return "part"
	}
	return fmt.Sprintf("kind(%d)", uint8(k))
}

// Record is one stored item.
type Record struct {
	Kind Kind
	Data []byte
}

// Loc is where a record sits in a log: the generation of the file (each
// compaction starts a new one), the offset of its first frame, and the
// bytes its frames take (D172).
type Loc struct {
	Gen uint64
	Off int64
	Len int64
}

// ErrStaleLoc is returned by ReadAt for a location from an older
// generation of the file, or one past its end.
var ErrStaleLoc = errors.New("store: location is not in this generation of the log")

// ErrBadRecord is returned by ReadAt when the bytes at a location are not
// one whole record with good checksums.
var ErrBadRecord = errors.New("store: no whole record at this location")

// MaxRecordSize bounds the data of one record on disk, so a corrupt length
// cannot make Open allocate without limit. A larger item is split.
const MaxRecordSize = 64 << 20

// pieceSize is the most data written in one record; a test lowers it.
var pieceSize = MaxRecordSize

var magic = []byte("PoO-log\x01")

var castagnoli = crc32.MakeTable(crc32.Castagnoli)

// ErrNotLog is returned for a file that does not start with the log header.
var ErrNotLog = errors.New("store: not a PoO log file")

// Log is an open log file. It is not safe for concurrent use; the node
// serializes access. The one exception is Durable, which may run beside
// the other methods, so a caller can wait for the disk without holding
// its own lock (ORANGE-005).
type Log struct {
	path    string
	f       *os.File
	w       *bufio.Writer
	sync    bool
	records int
	// size is the file's length: where the next record starts. Only the
	// appending goroutine changes it, under fmu.
	size int64
	// gen counts compactions, guarded by fmu; see Loc.
	gen uint64

	// fmu guards f against Durable, and the append counters.
	fmu     sync.Mutex
	written uint64 // appends so far
	synced  uint64 // appends known to be on disk
	// syncMu lets one Durable at a time sync; the others wait for it and
	// usually find their append already covered.
	syncMu sync.Mutex
	// err is the first failed write or sync, guarded by fmu. Once set the
	// log takes no more appends and reports nothing durable past synced:
	// after a failed fsync the system may drop the unwritten pages and let
	// a later fsync succeed, so a retry proves nothing (RED-010, D108).
	err error
}

// fileSync syncs a log's file; tests replace it to make a sync fail.
var fileSync = (*os.File).Sync

// ErrFailed wraps the error that stopped a log. The log takes no more
// records until it is opened again, which replays what is on disk.
var ErrFailed = errors.New("store: log stopped after a failed write")

// failLocked latches err as the log's first failure and returns it
// wrapped. The caller holds fmu.
func (l *Log) failLocked(err error) error {
	if l.err == nil {
		l.err = fmt.Errorf("%w: %w", ErrFailed, err)
	}
	return l.err
}

// Err returns the failure that stopped the log, or nil.
func (l *Log) Err() error {
	l.fmu.Lock()
	defer l.fmu.Unlock()
	return l.err
}

// Options control how a log writes.
type Options struct {
	// NoSync skips the fsync after each append. Faster, but a power loss
	// can lose the latest records (a process crash cannot).
	NoSync bool
}

// Opened reports what Open found.
type Opened struct {
	Records []Record
	// Cut is how many bytes at the end of the file were dropped because
	// they did not form a whole, valid record. Normally 0.
	Cut int64
}

// Open opens the log at path, creating it (and its directory) if needed,
// and returns every whole record in it.
func Open(path string, opt Options) (*Log, Opened, error) {
	var recs []Record
	l, op, err := OpenFunc(path, opt, func(_ *Log, r Record, _ Loc) error {
		recs = append(recs, r)
		return nil
	})
	op.Records = recs
	return l, op, err
}

// OpenFunc is Open that hands each whole record and its location to fn as
// it reads them, instead of holding them all (D172). Opened.Records is
// nil. An error from fn stops the read and is returned. fn is also given
// the log being opened, which it may only ReadAt, so a record already
// handed over can be read back before Open returns (a node drops bodies
// from memory while it replays).
func OpenFunc(path string, opt Options, fn func(*Log, Record, Loc) error) (*Log, Opened, error) {
	return OpenFuncAt(path, opt, From{}, fn)
}

// From says where OpenFuncAt starts reading: Ready, when set, is called
// once the file is open, before any record is read, with the log (which
// it may ReadAt), and returns the offset to read from and how many
// records come before it, as a snapshot that covers them says (D178).
// 0, 0 reads every record. The offset must be the end of a record, or of
// the header, within the file, or ErrStaleLoc is returned; the caller has
// checked that the log still holds what it skips (Tag).
type From struct {
	Ready func(*Log) (off int64, before int, err error)
}

// OpenFuncAt is OpenFunc that may skip the records a snapshot covers.
func OpenFuncAt(path string, opt Options, from From, fn func(*Log, Record, Loc) error) (*Log, Opened, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, Opened{}, err
	}
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, Opened{}, err
	}
	l := &Log{path: path, f: f, sync: !opt.NoSync}
	info, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, Opened{}, err
	}
	var op Opened
	if info.Size() == 0 {
		if _, err := f.Write(magic); err != nil {
			f.Close()
			return nil, Opened{}, err
		}
		if err := l.syncFile(); err != nil {
			f.Close()
			return nil, Opened{}, err
		}
	} else {
		l.size = info.Size() // for ReadAt from fn; set to the end below
		var off int64
		var before int
		if from.Ready != nil {
			if off, before, err = from.Ready(l); err == nil && off != 0 && (off < int64(len(magic)) || off > info.Size()) {
				err = ErrStaleLoc
			}
			if err != nil {
				f.Close()
				return nil, Opened{}, err
			}
		}
		n, good, err := read(f, info.Size(), off, func(r Record, loc Loc) error { return fn(l, r, loc) })
		if err != nil {
			f.Close()
			return nil, Opened{}, err
		}
		l.records = before + n
		op.Cut = info.Size() - good
		if op.Cut > 0 {
			if err := f.Truncate(good); err != nil {
				f.Close()
				return nil, Opened{}, err
			}
			if err := l.syncFile(); err != nil {
				f.Close()
				return nil, Opened{}, err
			}
		}
	}
	end, err := f.Seek(0, io.SeekEnd)
	if err != nil {
		f.Close()
		return nil, Opened{}, err
	}
	l.size = end
	l.w = bufio.NewWriter(f)
	return l, op, nil
}

// Size returns the log's length in bytes: where the next record starts.
func (l *Log) Size() int64 {
	l.fmu.Lock()
	defer l.fmu.Unlock()
	return l.size
}

// tagSpan is how many bytes at each end of a log's prefix Tag reads.
const tagSpan = 4096

// Tag returns a digest of the first bytes of the log at path and the
// bytes just before offset off, with off itself, so a snapshot that ends
// at off can tell whether the file still holds what it covers: a log
// rewritten by a compaction, cut short, or replaced by another file gives
// another tag (D178). A file shorter than off gives ErrStaleLoc.
func Tag(path string, off int64) ([32]byte, error) {
	var out [32]byte
	f, err := os.Open(path)
	if err != nil {
		return out, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return out, err
	}
	if off < int64(len(magic)) || info.Size() < off {
		return out, ErrStaleLoc
	}
	h := sha256.New()
	var n [8]byte
	binary.BigEndian.PutUint64(n[:], uint64(off))
	h.Write(n[:])
	head := make([]byte, min(off, tagSpan))
	if _, err := f.ReadAt(head, 0); err != nil {
		return out, err
	}
	h.Write(head)
	start := max(0, off-tagSpan)
	tail := make([]byte, off-start)
	if _, err := f.ReadAt(tail, start); err != nil {
		return out, err
	}
	h.Write(tail)
	copy(out[:], h.Sum(nil))
	return out, nil
}

// read hands every whole record of a log and its location to fn, and
// returns how many there were and the offset just after the last one.
func read(f *os.File, size, from int64, fn func(Record, Loc) error) (int, int64, error) {
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return 0, 0, err
	}
	r := bufio.NewReader(f)
	head := make([]byte, len(magic))
	if _, err := io.ReadFull(r, head); err != nil || !bytes.Equal(head, magic) {
		return 0, 0, ErrNotLog
	}
	count := 0
	off := int64(len(magic))
	if from > off {
		if _, err := f.Seek(from, io.SeekStart); err != nil {
			return 0, 0, err
		}
		r.Reset(f)
		off = from
	}
	good := off // just after the last whole item
	var parts []byte
	var hdr [5]byte
	for {
		if _, err := io.ReadFull(r, hdr[:]); err != nil {
			return count, good, nil
		}
		n := binary.BigEndian.Uint32(hdr[1:])
		if n > MaxRecordSize || int64(n) > size-off-9 {
			return count, good, nil
		}
		data := make([]byte, n)
		if _, err := io.ReadFull(r, data); err != nil {
			return count, good, nil
		}
		var sum [4]byte
		if _, err := io.ReadFull(r, sum[:]); err != nil {
			return count, good, nil
		}
		if binary.BigEndian.Uint32(sum[:]) != checksum(hdr[:], data) {
			return count, good, nil
		}
		off += 9 + int64(n)
		if Kind(hdr[0]) == KindPart {
			parts = append(parts, data...)
			continue
		}
		if parts != nil {
			data = append(parts, data...)
			parts = nil
		}
		if err := fn(Record{Kind: Kind(hdr[0]), Data: data}, Loc{Off: good, Len: off - good}); err != nil {
			return count, good, err
		}
		count++
		good = off
	}
}

func checksum(hdr, data []byte) uint32 {
	c := crc32.Update(0, castagnoli, hdr)
	return crc32.Update(c, castagnoli, data)
}

// frame writes rec, split into parts when it is larger than one record
// may be.
func frame(w io.Writer, rec Record) error {
	data := rec.Data
	for len(data) > pieceSize {
		if err := framePiece(w, KindPart, data[:pieceSize]); err != nil {
			return err
		}
		data = data[pieceSize:]
	}
	return framePiece(w, rec.Kind, data)
}

// framedSize is how many bytes frame writes for data of n bytes.
func framedSize(n int) int64 {
	pieces := int64(1)
	if n > pieceSize {
		pieces = int64((n + pieceSize - 1) / pieceSize)
	}
	return int64(n) + 9*pieces
}

func framePiece(w io.Writer, k Kind, data []byte) error {
	var hdr [5]byte
	hdr[0] = byte(k)
	binary.BigEndian.PutUint32(hdr[1:], uint32(len(data)))
	var sum [4]byte
	binary.BigEndian.PutUint32(sum[:], checksum(hdr[:], data))
	for _, b := range [][]byte{hdr[:], data, sum[:]} {
		if _, err := w.Write(b); err != nil {
			return err
		}
	}
	return nil
}

// Append writes one record and, unless NoSync is set, waits until it is on
// disk.
func (l *Log) Append(rec Record) error {
	t, err := l.AppendLater(rec)
	if err != nil {
		return err
	}
	return l.Durable(t)
}

func (l *Log) syncFile() error {
	if !l.sync {
		return nil
	}
	return l.f.Sync()
}

// AppendLater writes one record to the file, as Append does, but returns
// before syncing it. The record survives a crash of the process at once;
// pass the returned ticket to Durable to wait until it would also survive
// a power loss.
func (l *Log) AppendLater(rec Record) (uint64, error) {
	t, _, err := l.AppendLaterAt(rec)
	return t, err
}

// AppendLaterAt is AppendLater that also returns the record's location,
// for ReadAt (D172).
func (l *Log) AppendLaterAt(rec Record) (uint64, Loc, error) {
	if err := l.Err(); err != nil {
		return 0, Loc{}, err
	}
	err := frame(l.w, rec)
	if err == nil {
		err = l.w.Flush()
	}
	l.fmu.Lock()
	defer l.fmu.Unlock()
	if err != nil {
		return 0, Loc{}, l.failLocked(err)
	}
	n := framedSize(len(rec.Data))
	loc := Loc{Gen: l.gen, Off: l.size, Len: n}
	l.size += n
	l.records++
	l.written++
	return l.written, loc, nil
}

// ReadAt reads back the record at loc, checking each frame's checksum. It
// may run beside appends and Durable. A location from an older generation
// is refused (ErrStaleLoc), and bytes that are not one whole record give
// ErrBadRecord: neither is ever read as some other record.
func (l *Log) ReadAt(loc Loc) (Record, error) {
	l.fmu.Lock()
	defer l.fmu.Unlock()
	if loc.Gen != l.gen || loc.Off < int64(len(magic)) || loc.Len < 9 || loc.Off+loc.Len > l.size {
		return Record{}, ErrStaleLoc
	}
	buf := make([]byte, loc.Len)
	if _, err := l.f.ReadAt(buf, loc.Off); err != nil {
		return Record{}, fmt.Errorf("store: read %s at %d: %w", l.path, loc.Off, err)
	}
	var data []byte
	for len(buf) > 0 {
		if len(buf) < 9 {
			return Record{}, ErrBadRecord
		}
		n := int(binary.BigEndian.Uint32(buf[1:5]))
		if n > len(buf)-9 {
			return Record{}, ErrBadRecord
		}
		piece := buf[5 : 5+n]
		if binary.BigEndian.Uint32(buf[5+n:9+n]) != checksum(buf[:5], piece) {
			return Record{}, ErrBadRecord
		}
		k := Kind(buf[0])
		buf = buf[9+n:]
		if k == KindPart {
			if len(buf) == 0 {
				return Record{}, ErrBadRecord
			}
			data = append(data, piece...)
			continue
		}
		if len(buf) != 0 {
			return Record{}, ErrBadRecord
		}
		if data != nil {
			piece = append(data, piece...)
		} else {
			piece = append([]byte(nil), piece...)
		}
		return Record{Kind: k, Data: piece}, nil
	}
	return Record{}, ErrBadRecord
}

// Durable waits until the append with this ticket is on disk, unless
// NoSync is set. It may run while other goroutines append: callers waiting
// together share one fsync (a group commit), so a node that takes items
// from many peers syncs once for many of them.
func (l *Log) Durable(ticket uint64) error {
	if !l.sync {
		return nil
	}
	l.syncMu.Lock()
	defer l.syncMu.Unlock()
	l.fmu.Lock()
	if l.synced >= ticket {
		l.fmu.Unlock()
		return nil
	}
	if l.err != nil {
		// A sync already failed: never retry it and call it durable.
		defer l.fmu.Unlock()
		return l.err
	}
	f, upto := l.f, l.written
	l.fmu.Unlock()
	err := fileSync(f)
	l.fmu.Lock()
	defer l.fmu.Unlock()
	if l.synced >= ticket {
		// Rewrite or Close synced everything while this sync ran (and may
		// have closed f under it).
		return nil
	}
	if err != nil {
		return l.failLocked(err)
	}
	l.synced = upto
	return nil
}

// Records returns how many records the log holds, including ones the node
// no longer needs. Compare it with the live count to decide when to compact.
func (l *Log) Records() int { return l.records }

// Path returns the file's path.
func (l *Log) Path() string { return l.path }

// Rewrite replaces the log's contents with recs. It writes a temporary file
// beside the log, syncs it and renames it into place, then reopens the log
// for appending.
func (l *Log) Rewrite(recs []Record) error {
	_, err := l.RewriteFunc(len(recs), func(i int) (Record, error) { return recs[i], nil })
	return err
}

// RewriteFunc is Rewrite that asks next for the n records one at a time,
// so they need not all be in memory, and returns their locations in the
// new generation (D172). An error from next leaves the old log as it was.
// next may read the old log with ReadAt: the new file only replaces it
// at the end.
func (l *Log) RewriteFunc(n int, next func(i int) (Record, error)) ([]Loc, error) {
	if err := l.Err(); err != nil {
		return nil, err
	}
	if err := l.w.Flush(); err != nil {
		return nil, err
	}
	tmp := l.path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return nil, err
	}
	w := bufio.NewWriter(f)
	locs := make([]Loc, 0, n)
	size := int64(len(magic))
	err = func() error {
		if _, err := w.Write(magic); err != nil {
			return err
		}
		for i := 0; i < n; i++ {
			r, err := next(i)
			if err != nil {
				return err
			}
			if err := frame(w, r); err != nil {
				return err
			}
			fs := framedSize(len(r.Data))
			locs = append(locs, Loc{Off: size, Len: fs})
			size += fs
		}
		if err := w.Flush(); err != nil {
			return err
		}
		return f.Sync()
	}()
	if err != nil {
		f.Close()
		os.Remove(tmp)
		return nil, err
	}
	// Close the old file first: some systems cannot rename over a file
	// that is still open.
	l.fmu.Lock()
	l.f.Close()
	l.fmu.Unlock()
	if err := os.Rename(tmp, l.path); err != nil {
		f.Close()
		os.Remove(tmp)
		old, oerr := os.OpenFile(l.path, os.O_RDWR|os.O_APPEND, 0o600)
		if oerr != nil {
			return nil, errors.Join(err, oerr)
		}
		serr := old.Sync()
		l.fmu.Lock()
		l.f = old
		if serr == nil {
			l.synced = l.written
		}
		l.fmu.Unlock()
		l.w = bufio.NewWriter(old)
		return nil, err
	}
	syncDir(filepath.Dir(l.path))
	if _, err := f.Seek(0, io.SeekEnd); err != nil {
		f.Close()
		return nil, err
	}
	l.fmu.Lock()
	l.f = f
	l.synced = l.written // the new file holds every live record, synced
	l.gen++
	l.size = size
	for i := range locs {
		locs[i].Gen = l.gen
	}
	l.fmu.Unlock()
	l.w = bufio.NewWriter(f)
	l.records = n
	return locs, nil
}

// syncDir makes a rename durable where the platform supports it.
func syncDir(dir string) {
	if d, err := os.Open(dir); err == nil {
		d.Sync()
		d.Close()
	}
}

// Close flushes and closes the log.
func (l *Log) Close() error {
	ferr := l.w.Flush()
	serr := fileSync(l.f)
	l.fmu.Lock()
	if ferr == nil && serr == nil && l.err == nil {
		l.synced = l.written
	}
	cerr := l.f.Close()
	lerr := l.err
	l.fmu.Unlock()
	return errors.Join(lerr, ferr, serr, cerr)
}
