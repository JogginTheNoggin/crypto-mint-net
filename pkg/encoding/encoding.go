// Package encoding implements the canonical byte form used by every hashed
// object in the protocol: policies, transactions, entries, layers and series.
//
// The rules are deliberately small so that independent implementations hash
// exactly the same bytes:
//
//   - Fixed-width unsigned integers are big-endian (U8, U16, U32, U64).
//   - Variable-length byte strings are prefixed with their length as a U32.
//   - Lists are prefixed with their element count as a U32.
//   - Hashes are written as their raw 32 bytes, with no prefix.
//   - Booleans are one byte, 0 or 1. Any other value is a decoding error.
//
// A Reader refuses trailing bytes when Finish is called, refuses lengths that
// exceed the remaining input, and refuses booleans that are not 0 or 1, so a
// value has exactly one valid encoding.
package encoding

import (
	"encoding/binary"
	"errors"
	"fmt"
)

// ErrShort is returned when the input ends before a value is complete.
var ErrShort = errors.New("encoding: input too short")

// ErrTrailing is returned by Finish when bytes remain after the last value.
var ErrTrailing = errors.New("encoding: trailing bytes")

// Writer appends canonical encodings to a byte slice.
type Writer struct {
	buf []byte
	// counting, set by NewCounter, adds lengths to n and keeps no bytes.
	counting bool
	n        int
}

// NewWriter returns a Writer with an initial capacity hint.
func NewWriter(capacity int) *Writer {
	return &Writer{buf: make([]byte, 0, capacity)}
}

// NewCounter returns a Writer that keeps no bytes, only their number:
// Len after encoding into it is the encoding's length, without building
// it (ORANGE-017).
func NewCounter() *Writer { return &Writer{counting: true} }

// Bytes returns the encoded bytes. The slice aliases the Writer's buffer.
func (w *Writer) Bytes() []byte { return w.buf }

// Reset empties the Writer, keeping its buffer.
func (w *Writer) Reset() { w.buf, w.n = w.buf[:0], 0 }

// Len returns the number of bytes written so far.
func (w *Writer) Len() int {
	if w.counting {
		return w.n
	}
	return len(w.buf)
}

// Raw appends b with no length prefix. Use it only for fixed-width values
// such as hashes and tags.
func (w *Writer) Raw(b []byte) *Writer {
	if w.counting {
		w.n += len(b)
		return w
	}
	w.buf = append(w.buf, b...)
	return w
}

// U8 appends one byte.
func (w *Writer) U8(v uint8) *Writer {
	if w.counting {
		w.n += 1
		return w
	}
	w.buf = append(w.buf, v)
	return w
}

// U16 appends a big-endian uint16.
func (w *Writer) U16(v uint16) *Writer {
	if w.counting {
		w.n += 2
		return w
	}
	w.buf = binary.BigEndian.AppendUint16(w.buf, v)
	return w
}

// U32 appends a big-endian uint32.
func (w *Writer) U32(v uint32) *Writer {
	if w.counting {
		w.n += 4
		return w
	}
	w.buf = binary.BigEndian.AppendUint32(w.buf, v)
	return w
}

// U64 appends a big-endian uint64.
func (w *Writer) U64(v uint64) *Writer {
	if w.counting {
		w.n += 8
		return w
	}
	w.buf = binary.BigEndian.AppendUint64(w.buf, v)
	return w
}

// Bool appends 1 for true and 0 for false.
func (w *Writer) Bool(v bool) *Writer {
	if v {
		return w.U8(1)
	}
	return w.U8(0)
}

// VarBytes appends a U32 length followed by b.
func (w *Writer) VarBytes(b []byte) *Writer {
	w.U32(uint32(len(b)))
	return w.Raw(b)
}

// Count appends a U32 list length.
func (w *Writer) Count(n int) *Writer {
	return w.U32(uint32(n))
}

// Reader decodes canonical encodings from a byte slice.
type Reader struct {
	buf []byte
	off int
	err error
}

// NewReader returns a Reader over b.
func NewReader(b []byte) *Reader {
	return &Reader{buf: b}
}

// Err returns the first error the Reader met, if any.
func (r *Reader) Err() error { return r.err }

// Remaining returns the number of unread bytes.
func (r *Reader) Remaining() int { return len(r.buf) - r.off }

// Finish returns the first error met, or ErrTrailing if unread bytes remain.
func (r *Reader) Finish() error {
	if r.err != nil {
		return r.err
	}
	if r.off != len(r.buf) {
		return ErrTrailing
	}
	return nil
}

func (r *Reader) take(n int) []byte {
	if r.err != nil {
		return nil
	}
	if n < 0 || r.Remaining() < n {
		r.err = ErrShort
		return nil
	}
	b := r.buf[r.off : r.off+n]
	r.off += n
	return b
}

// Raw reads exactly n bytes. The returned slice is a copy.
func (r *Reader) Raw(n int) []byte {
	b := r.take(n)
	if b == nil {
		return nil
	}
	out := make([]byte, n)
	copy(out, b)
	return out
}

// Into reads len(dst) bytes into dst.
func (r *Reader) Into(dst []byte) {
	b := r.take(len(dst))
	if b != nil {
		copy(dst, b)
	}
}

// U8 reads one byte.
func (r *Reader) U8() uint8 {
	b := r.take(1)
	if b == nil {
		return 0
	}
	return b[0]
}

// U16 reads a big-endian uint16.
func (r *Reader) U16() uint16 {
	b := r.take(2)
	if b == nil {
		return 0
	}
	return binary.BigEndian.Uint16(b)
}

// U32 reads a big-endian uint32.
func (r *Reader) U32() uint32 {
	b := r.take(4)
	if b == nil {
		return 0
	}
	return binary.BigEndian.Uint32(b)
}

// U64 reads a big-endian uint64.
func (r *Reader) U64() uint64 {
	b := r.take(8)
	if b == nil {
		return 0
	}
	return binary.BigEndian.Uint64(b)
}

// Bool reads one byte that must be 0 or 1.
func (r *Reader) Bool() bool {
	v := r.U8()
	if r.err == nil && v > 1 {
		r.err = fmt.Errorf("encoding: invalid boolean %d", v)
	}
	return v == 1
}

// VarBytes reads a U32 length and that many bytes, refusing lengths above
// max (use 0 for no limit beyond the remaining input).
func (r *Reader) VarBytes(max int) []byte {
	n := int(r.U32())
	if r.err != nil {
		return nil
	}
	if max > 0 && n > max {
		r.err = fmt.Errorf("encoding: length %d above limit %d", n, max)
		return nil
	}
	return r.Raw(n)
}

// Count reads a U32 list length, refusing counts above max or counts that
// could not fit in the remaining input given minItemSize bytes per item.
func (r *Reader) Count(max, minItemSize int) int {
	u := r.U32()
	if r.err != nil {
		return 0
	}
	n := int(u)
	if n < 0 { // a count past int on a 32-bit build
		r.err = ErrShort
		return 0
	}
	if max > 0 && n > max {
		r.err = fmt.Errorf("encoding: count %d above limit %d", n, max)
		return 0
	}
	if minItemSize > 0 && n > r.Remaining()/minItemSize {
		r.err = ErrShort
		return 0
	}
	return n
}

// Fail records err as the Reader's error if it has none yet. Decoders use
// it to report semantic errors found while reading.
func (r *Reader) Fail(err error) {
	if r.err == nil {
		r.err = err
	}
}
