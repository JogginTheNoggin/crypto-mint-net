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
// A crash can leave the last record half written. Open keeps every record
// up to the first one that is incomplete or fails its checksum, and cuts the
// file there. What is lost can be fetched from peers again.
//
// Compaction rewrites a log with only what the node still holds, through a
// temporary file that is renamed over the old one, so a crash during
// compaction leaves either the old log or the new one.
package store

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"os"
	"path/filepath"
)

// Kind is the type of a record.
type Kind uint8

// Record kinds. Numbers are never reused.
const (
	KindEntry Kind = 1
	KindLayer Kind = 2
	KindTx    Kind = 3
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
	}
	return fmt.Sprintf("kind(%d)", uint8(k))
}

// Record is one stored item.
type Record struct {
	Kind Kind
	Data []byte
}

// MaxRecordSize bounds one record's data, so a corrupt length cannot make
// Open allocate without limit.
const MaxRecordSize = 64 << 20

var magic = []byte("PoO-log\x01")

var castagnoli = crc32.MakeTable(crc32.Castagnoli)

// ErrNotLog is returned for a file that does not start with the log header.
var ErrNotLog = errors.New("store: not a PoO log file")

// Log is an open log file. It is not safe for concurrent use; the node
// serializes access.
type Log struct {
	path    string
	f       *os.File
	w       *bufio.Writer
	sync    bool
	records int
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
		recs, good, err := read(f, info.Size())
		if err != nil {
			f.Close()
			return nil, Opened{}, err
		}
		op.Records = recs
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
	if _, err := f.Seek(0, io.SeekEnd); err != nil {
		f.Close()
		return nil, Opened{}, err
	}
	l.records = len(op.Records)
	l.w = bufio.NewWriter(f)
	return l, op, nil
}

// read returns the whole records of a log and the offset just after the
// last one.
func read(f *os.File, size int64) ([]Record, int64, error) {
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return nil, 0, err
	}
	r := bufio.NewReader(f)
	head := make([]byte, len(magic))
	if _, err := io.ReadFull(r, head); err != nil || !bytes.Equal(head, magic) {
		return nil, 0, ErrNotLog
	}
	var recs []Record
	off := int64(len(magic))
	var hdr [5]byte
	for {
		if _, err := io.ReadFull(r, hdr[:]); err != nil {
			return recs, off, nil
		}
		n := binary.BigEndian.Uint32(hdr[1:])
		if n > MaxRecordSize || int64(n) > size-off-9 {
			return recs, off, nil
		}
		data := make([]byte, n)
		if _, err := io.ReadFull(r, data); err != nil {
			return recs, off, nil
		}
		var sum [4]byte
		if _, err := io.ReadFull(r, sum[:]); err != nil {
			return recs, off, nil
		}
		if binary.BigEndian.Uint32(sum[:]) != checksum(hdr[:], data) {
			return recs, off, nil
		}
		recs = append(recs, Record{Kind: Kind(hdr[0]), Data: data})
		off += 9 + int64(n)
	}
}

func checksum(hdr, data []byte) uint32 {
	c := crc32.Update(0, castagnoli, hdr)
	return crc32.Update(c, castagnoli, data)
}

func frame(w io.Writer, rec Record) error {
	if len(rec.Data) > MaxRecordSize {
		return fmt.Errorf("store: record of %d bytes is too large", len(rec.Data))
	}
	var hdr [5]byte
	hdr[0] = byte(rec.Kind)
	binary.BigEndian.PutUint32(hdr[1:], uint32(len(rec.Data)))
	var sum [4]byte
	binary.BigEndian.PutUint32(sum[:], checksum(hdr[:], rec.Data))
	for _, b := range [][]byte{hdr[:], rec.Data, sum[:]} {
		if _, err := w.Write(b); err != nil {
			return err
		}
	}
	return nil
}

// Append writes one record and, unless NoSync is set, waits until it is on
// disk.
func (l *Log) Append(rec Record) error {
	if err := frame(l.w, rec); err != nil {
		return err
	}
	if err := l.w.Flush(); err != nil {
		return err
	}
	l.records++
	return l.syncFile()
}

func (l *Log) syncFile() error {
	if !l.sync {
		return nil
	}
	return l.f.Sync()
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
	if err := l.w.Flush(); err != nil {
		return err
	}
	tmp := l.path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	w := bufio.NewWriter(f)
	err = func() error {
		if _, err := w.Write(magic); err != nil {
			return err
		}
		for _, r := range recs {
			if err := frame(w, r); err != nil {
				return err
			}
		}
		if err := w.Flush(); err != nil {
			return err
		}
		return f.Sync()
	}()
	if err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	// Close the old file first: some systems cannot rename over a file
	// that is still open.
	l.f.Close()
	if err := os.Rename(tmp, l.path); err != nil {
		f.Close()
		os.Remove(tmp)
		old, oerr := os.OpenFile(l.path, os.O_RDWR|os.O_APPEND, 0o600)
		if oerr != nil {
			return errors.Join(err, oerr)
		}
		l.f, l.w = old, bufio.NewWriter(old)
		return err
	}
	syncDir(filepath.Dir(l.path))
	if _, err := f.Seek(0, io.SeekEnd); err != nil {
		f.Close()
		return err
	}
	l.f = f
	l.w = bufio.NewWriter(f)
	l.records = len(recs)
	return nil
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
	serr := l.f.Sync()
	cerr := l.f.Close()
	return errors.Join(ferr, serr, cerr)
}
