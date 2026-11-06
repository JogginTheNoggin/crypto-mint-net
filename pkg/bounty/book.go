package bounty

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/brand"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/fsutil"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/keepjson"
)

// Config is the node policy of the market (nodepolicy's "bounty"
// section). The zero value posts nothing and takes nothing.
type Config struct {
	// Post lets this node post offers; Take lets it hold others' offers
	// and claim them.
	Post bool
	Take bool
	// MaxOpen bounds this node's open offers.
	MaxOpen int
	// MaxAmount bounds what one offer may pay in all (amount × count);
	// Budget bounds what all open offers and unpaid claims may owe
	// together. Both must be set before anything is posted.
	MaxAmount uint64
	Budget    uint64
	// MaxLeasesPerGrinder bounds the leases one grinder holds on this
	// node's offers at once; a grinder starts at one and earns one more
	// for each layer it hands in, up to this.
	MaxLeasesPerGrinder int
	// LeaseFloor and LeaseCeiling bound a lease. A lease is three times
	// the expected time to find the layer at the rate the grinder states.
	LeaseFloor   time.Duration
	LeaseCeiling time.Duration
	// Held bounds the offers from others this node keeps, the oldest
	// dropped first; PerPeer bounds those from one peer.
	Held    int
	PerPeer int
	// MinPrice: offers paying less per layer are not kept.
	MinPrice uint64
}

// Defaults for the zero fields of Config.
const (
	DefaultMaxOpen             = 16
	DefaultMaxLeasesPerGrinder = 4
	DefaultLeaseFloor          = time.Minute
	DefaultLeaseCeiling        = time.Hour
	DefaultHeld                = 256
	DefaultPerPeer             = MaxListed
)

func (c *Config) fill() {
	if c.MaxOpen <= 0 {
		c.MaxOpen = DefaultMaxOpen
	}
	if c.MaxLeasesPerGrinder <= 0 {
		c.MaxLeasesPerGrinder = DefaultMaxLeasesPerGrinder
	}
	if c.LeaseFloor <= 0 {
		c.LeaseFloor = DefaultLeaseFloor
	}
	if c.LeaseCeiling <= 0 {
		c.LeaseCeiling = DefaultLeaseCeiling
	}
	if c.LeaseCeiling < c.LeaseFloor {
		c.LeaseCeiling = c.LeaseFloor
	}
	if c.Held <= 0 {
		c.Held = DefaultHeld
	}
	if c.PerPeer <= 0 {
		c.PerPeer = DefaultPerPeer
	}
}

// Lease is an offer held by one grinder until Until.
type Lease struct {
	Peer string `json:"peer"`
	// Addr is the grinder's network address (its host, no port): leases
	// are limited per address as well as per node id, since a node id
	// costs nothing (RED-112).
	Addr     string    `json:"addr,omitempty"`
	PayTo    hash.Hash `json:"pay_to"`
	Rate     uint64    `json:"rate"`
	Until    time.Time `json:"until"`
	Extended bool      `json:"extended,omitempty"`
}

// ClaimRecord is a layer handed in and accepted.
type ClaimRecord struct {
	Peer     string      `json:"peer"`
	Nonce    uint64      `json:"nonce"`
	PayTo    hash.Hash   `json:"pay_to"`
	Strength int         `json:"strength"`
	At       time.Time   `json:"at"`
	Paid     []hash.Hash `json:"paid,omitempty"`
	// kept holds fields a later release wrote (W-28).
	kept keepjson.Kept
}

// UnmarshalJSON reads a claim, keeping fields a later release added.
func (c *ClaimRecord) UnmarshalJSON(b []byte) error {
	type plain ClaimRecord
	var p plain
	if err := json.Unmarshal(b, &p); err != nil {
		return err
	}
	kept, err := keepjson.SplitValid(b, p)
	if err != nil {
		return err
	}
	*c = ClaimRecord(p)
	c.kept = kept
	return nil
}

// MarshalJSON writes a claim with the fields it kept.
func (c ClaimRecord) MarshalJSON() ([]byte, error) {
	type plain ClaimRecord
	b, err := json.Marshal(plain(c))
	if err != nil {
		return nil, err
	}
	return keepjson.JoinValid(b, c.kept)
}

// Mine is an offer this node posted.
type Mine struct {
	ID     hash.Hash     `json:"id"`
	Offer  Offer         `json:"offer"`
	Posted time.Time     `json:"posted"`
	Closed bool          `json:"closed,omitempty"`
	Leases []Lease       `json:"leases,omitempty"`
	Claims []ClaimRecord `json:"claims,omitempty"`
	// State is open, filled (every layer handed in), closed (cancelled)
	// or expired (past its open seconds); Closes is when it closes, if it
	// has open seconds. Both are worked out when the offer is read
	// (GUI-GREEN-062).
	State  string     `json:"state,omitempty"`
	Closes *time.Time `json:"closes,omitempty"`
	kept   keepjson.Kept
}

// The states of an offer (GUI-GREEN-062).
const (
	StateOpen    = "open"
	StateFilled  = "filled"
	StateClosed  = "closed"
	StateExpired = "expired"
	// Of a peer's offer only: this node holds a lease on it; or the payer
	// said it is leased to another grinder or filled.
	StateLeased = "leased"
	StateTaken  = "taken"
)

// UnmarshalJSON reads an offer, keeping fields a later release added.
func (m *Mine) UnmarshalJSON(b []byte) error {
	type plain Mine
	var p plain
	if err := json.Unmarshal(b, &p); err != nil {
		return err
	}
	kept, err := keepjson.SplitValid(b, p)
	if err != nil {
		return err
	}
	*m = Mine(p)
	m.kept = kept
	return nil
}

// MarshalJSON writes an offer with the fields it kept.
func (m Mine) MarshalJSON() ([]byte, error) {
	type plain Mine
	b, err := json.Marshal(plain(m))
	if err != nil {
		return nil, err
	}
	return keepjson.JoinValid(b, m.kept)
}

// state works out the offer's state at now.
func (m *Mine) state(now time.Time) string {
	// Filled first: an offer with every layer handed in stays filled once
	// its time runs out (GUI-GREEN-063).
	switch {
	case len(m.Claims) >= int(m.Offer.Count):
		return StateFilled
	case m.Closed && m.expired(now):
		return StateExpired
	case m.Closed:
		return StateClosed
	}
	return StateOpen
}

// done says the offer needs nothing more: no more layers can come, none
// is unpaid and no lease is open. Such an offer moves to the done file
// (ORANGE-046 P1).
func (m *Mine) done() bool {
	if !m.Closed && len(m.Claims) < int(m.Offer.Count) || len(m.Leases) > 0 {
		return false
	}
	for _, c := range m.Claims {
		if len(c.Paid) == 0 {
			return false
		}
	}
	return true
}

func closes(from time.Time, open uint32) *time.Time {
	if open == 0 {
		return nil
	}
	t := from.Add(time.Duration(open) * time.Second)
	return &t
}

// Owed is what the offer may still have to pay: the unpaid accepted
// layers, and while it is open the layers not yet handed in.
func (m *Mine) Owed() uint64 {
	n := uint64(0)
	for _, c := range m.Claims {
		if len(c.Paid) == 0 {
			n++
		}
	}
	if !m.Closed {
		n += uint64(m.Offer.Count) - uint64(min(len(m.Claims), int(m.Offer.Count)))
	}
	// A sum too large to hold reads as the most there is, never as a
	// small one (RED-114).
	owed, over := mulCheck(n, m.Offer.Amount)
	if over {
		return math.MaxUint64
	}
	return owed
}

func (m *Mine) free() int { return int(m.Offer.Count) - len(m.Claims) - len(m.Leases) }

func (m *Mine) lease(peer string) int {
	return slices.IndexFunc(m.Leases, func(l Lease) bool { return l.Peer == peer })
}

func (m *Mine) expired(now time.Time) bool {
	return m.Offer.OpenSeconds > 0 && now.After(m.Posted.Add(time.Duration(m.Offer.OpenSeconds)*time.Second))
}

// Held is an offer from a peer this node keeps, with what it did with it.
type Held struct {
	ID    hash.Hash `json:"id"`
	Offer Offer     `json:"offer"`
	From  string    `json:"from"`
	// Addr is the sender's network address key (RED-116).
	Addr     string    `json:"addr,omitempty"`
	Received time.Time `json:"received"`
	// Lease is what the payer granted, if anything.
	Lease *HeldLease `json:"lease,omitempty"`
	// Acks are the payer's answers to this node's claims.
	Acks []Ack `json:"acks,omitempty"`
	// State is open, leased (this node holds a lease), taken (the payer
	// said it is leased to another or filled), filled or closed; Closes
	// is when it closes, counted from when it was received
	// (GUI-GREEN-062).
	State  string     `json:"state,omitempty"`
	Closes *time.Time `json:"closes,omitempty"`
	// ended is what the payer last said ended the offer for this node.
	ended string
}

// MaxAcks bounds the acks kept for one held offer, the newest kept
// (RED-113).
const MaxAcks = 16

// maxPeerRecords bounds the payers and grinders the book remembers
// (RED-113).
const maxPeerRecords = 4096

// HeldLease is a lease this node holds on a peer's offer.
type HeldLease struct {
	PayTo  hash.Hash `json:"pay_to"`
	Until  time.Time `json:"until"`
	Reason string    `json:"reason,omitempty"`
}

// PayerRecord is what this node saw of one payer: layers it accepted, and
// how many of those it paid.
type PayerRecord struct {
	Accepted int `json:"accepted"`
	Paid     int `json:"paid"`
}

// grinder is what this node saw of one grinder on its own offers.
type grinder struct {
	filled, expired int
}

// Book is a node's market: the offers it posted, with their leases and
// claims, and the offers peers sent it.
type Book struct {
	mu       sync.Mutex
	cfg      Config
	now      func() time.Time
	path     string
	mine     map[hash.Hash]*Mine
	order    []hash.Hash
	held     map[hash.Hash]*Held
	heldOrd  []hash.Hash
	grinders map[string]*grinder
	payers   map[string]*PayerRecord
	// heldPer counts the held offers from each peer (ORANGE-046 P3), and
	// heldAddr those from each network address (RED-116).
	heldPer  map[string]int
	heldAddr map[string]int
	// heldSwept is when held offers were last checked for their time.
	heldSwept time.Time
	// doneSize is the done file's size as this book last knew it.
	doneSize int64
	// pending says a save is waiting on its timer (ORANGE-046 P2).
	pending bool
	// kept holds the file's fields a later release wrote (W-28).
	kept keepjson.Kept
	// dropped says what Open left out of the file, and why (RED-114).
	dropped []string
}

// FileName is the file, in the node's data directory, that keeps the
// offers a node posted, so a restart keeps every accepted claim to pay.
const FileName = "bounties.json"

// FileKind and FileFormat name that file.
const (
	FileKind   = brand.KindPrefix + "bounties"
	FileFormat = 1
)

type file struct {
	Kind   string  `json:"kind"`
	Format int     `json:"format"`
	Offers []*Mine `json:"offers"`
	// DoneSize is the done file's size when this file was written: only
	// what lies past it can be an offer also still in Offers, after a
	// crash between the two writes, so a start reads only that much of
	// the done file (ORANGE-047 P1, D200).
	DoneSize int64 `json:"done_size,omitempty"`
	kept     keepjson.Kept
}

func (f *file) UnmarshalJSON(b []byte) error {
	type plain file
	var p plain
	if err := json.Unmarshal(b, &p); err != nil {
		return err
	}
	kept, err := keepjson.SplitValid(b, p)
	if err != nil {
		return err
	}
	*f = file(p)
	f.kept = kept
	return nil
}

func (f file) MarshalJSON() ([]byte, error) {
	type plain file
	b, err := json.Marshal(plain(f))
	if err != nil {
		return nil, err
	}
	return keepjson.JoinValid(b, f.kept)
}

// DoneFileName is the file, beside FileName, that offers which need
// nothing more move to, one JSON object a line after a first line naming
// the file, so bounties.json holds only what may still be owed
// (ORANGE-046 P1). It is appended to and synced before an offer leaves
// bounties.json; an offer found in both is left out of bounties.json.
const DoneFileName = "bounties-done.jsonl"

// DoneFileKind and DoneFileFormat name the done file. Frozen with format
// 1: the first line is {"kind": DoneFileKind, "format": 1}, and every
// later line is one offer whose JSON begins {"id":"<64 hex digits>",
// whatever fields follow. A reader takes the id from that prefix alone,
// so a later release may add fields after it; a layout that moves the id
// is a new format, which this release refuses (W-31, D200).
const (
	DoneFileKind   = brand.KindPrefix + "bounties-done"
	DoneFileFormat = 1
)

// saveDelay is how long a lease's save waits, so a burst of takes is
// written once (ORANGE-046 P2).
var saveDelay = 200 * time.Millisecond

// Open returns the book whose posted offers are kept in path ("" keeps
// them in memory only).
func Open(cfg Config, path string) (*Book, error) {
	cfg.fill()
	b := &Book{cfg: cfg, now: time.Now, path: path, mine: map[hash.Hash]*Mine{}, held: map[hash.Hash]*Held{},
		grinders: map[string]*grinder{}, payers: map[string]*PayerRecord{}, heldPer: map[string]int{}, heldAddr: map[string]int{}}
	if path == "" {
		return b, nil
	}
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return b, nil
	}
	if err != nil {
		return nil, err
	}
	var f file
	if err := json.Unmarshal(raw, &f); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	switch {
	case f.Kind != FileKind:
		return nil, fmt.Errorf("%s: not a %s file", path, FileKind)
	case f.Format > FileFormat:
		return nil, fmt.Errorf("%s is format %d, from a later release than this one (format %d): run that release, or move the file aside to start with no offers", path, f.Format, FileFormat)
	case f.Format < 1:
		return nil, fmt.Errorf("%s: format %d is not a format", path, f.Format)
	}
	b.kept = f.kept
	done, unread, size, err := doneIDs(b.donePath(), f.DoneSize)
	if err != nil {
		return nil, err
	}
	b.doneSize = size
	if unread > 0 {
		b.dropped = append(b.dropped, fmt.Sprintf("%s: %d lines with no offer id this release reads", DoneFileName, unread))
	}
	// The file is checked as an offer and a claim are when they are made:
	// what fails is left out and reported, never paid (RED-114).
	for i, m := range f.Offers {
		if m != nil && len(m.Offer.Raw) > 0 {
			// The bytes are the offer; the fields beside them are for
			// people (W-27).
			o, err := DecodeOffer(m.Offer.Raw)
			if err != nil {
				b.dropped = append(b.dropped, fmt.Sprintf("offer %d: %v", i, err))
				continue
			}
			m.Offer = o
		}
		if m != nil && done[m.ID] {
			// Moved to the done file before a crash. The done file's copy
			// is the later one (its payment recorded), so this one is left
			// out, and said, as every other drop is (RED-117).
			b.dropped = append(b.dropped, fmt.Sprintf("offer %d: %s is in %s already, finished", i, m.ID.Short(), DoneFileName))
			continue
		}
		if why := checkMine(m); why != "" {
			b.dropped = append(b.dropped, fmt.Sprintf("offer %d: %s", i, why))
			continue
		}
		if _, dup := b.mine[m.ID]; dup {
			b.dropped = append(b.dropped, fmt.Sprintf("offer %d: %s appears twice", i, m.ID.Short()))
			continue
		}
		b.mine[m.ID] = m
		b.order = append(b.order, m.ID)
	}
	// An offer that finished while the node was stopped (its time ran
	// out, its last lease ended) moves to the done file now, not only at
	// the next change (GUI-GREEN-063).
	b.expire()
	if slices.ContainsFunc(b.order, func(id hash.Hash) bool { return b.mine[id].done() }) {
		if err := b.save(); err != nil {
			return nil, err
		}
	}
	return b, nil
}

// checkMine says what is wrong with an offer read from the file, or "".
func checkMine(m *Mine) string {
	switch {
	case m == nil:
		return "empty"
	case m.ID != m.Offer.ID():
		return "its id is not its offer's"
	}
	if err := m.Offer.Check(); err != nil {
		return err.Error()
	}
	if _, over := mulCheck(m.Offer.Amount, uint64(m.Offer.Count)); over {
		return "it pays more than a sum can hold"
	}
	if len(m.Claims) > int(m.Offer.Count) || len(m.Leases) > int(m.Offer.Count) {
		return "more claims or leases than layers asked for"
	}
	seen := map[uint64]bool{}
	for _, c := range m.Claims {
		if c.Strength < int(m.Offer.Strength) {
			return fmt.Sprintf("a claim of strength %d on an offer of %d", c.Strength, m.Offer.Strength)
		}
		if seen[c.Nonce] {
			return fmt.Sprintf("two claims with nonce %d", c.Nonce)
		}
		seen[c.Nonce] = true
		if len(c.Paid) > MaxTIDs {
			return "a payment with too many legs"
		}
	}
	return ""
}

// Dropped says what Open left out of the file, and why: an offer or
// claim the posting or claiming checks would refuse (RED-114).
func (b *Book) Dropped() []string { return slices.Clone(b.dropped) }

// SetClock replaces the clock; tests use it.
func (b *Book) SetClock(now func() time.Time) { b.mu.Lock(); b.now = now; b.mu.Unlock() }

// Config returns the book's settings.
func (b *Book) Config() Config { return b.cfg }

func (b *Book) donePath() string {
	if b.path == "" {
		return ""
	}
	return filepath.Join(filepath.Dir(b.path), DoneFileName)
}

// doneIDs reads the ids of the offers in the done file. Only the start
// of each line is read, where the id is, and the rest is skipped as it
// streams, so a line of any length costs a few KiB (RED-117). It also
// says how many lines it could not read an id from.
func doneIDs(path string, from int64) (map[hash.Hash]bool, int, int64, error) {
	ids := map[hash.Hash]bool{}
	if path == "" {
		return ids, 0, 0, nil
	}
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return ids, 0, 0, nil
	}
	if err != nil {
		return nil, 0, 0, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return nil, 0, 0, err
	}
	size := st.Size()
	r := bufio.NewReader(f)
	first, _, err := linePrefix(r, maxDoneHead)
	if len(first) == 0 && err != nil {
		return ids, 0, size, nil // empty
	}
	var head struct {
		Kind   string `json:"kind"`
		Format int    `json:"format"`
	}
	if json.Unmarshal(first, &head) != nil || head.Kind != DoneFileKind {
		return nil, 0, 0, fmt.Errorf("%s: not a %s file", path, DoneFileKind)
	}
	switch {
	case head.Format > DoneFileFormat:
		return nil, 0, 0, fmt.Errorf("%s is format %d, from a later release than this one (format %d): run that release, or move the file aside", path, head.Format, DoneFileFormat)
	case head.Format < 1:
		return nil, 0, 0, fmt.Errorf("%s: format %d is not a format", path, head.Format)
	}
	// Only the part past what bounties.json last saw can hold an offer in
	// both files (ORANGE-047 P1). An offset the file does not reach, or
	// none, reads it all.
	if from > int64(len(first)) && from <= size {
		if _, err := f.Seek(from, io.SeekStart); err != nil {
			return nil, 0, 0, err
		}
		r.Reset(f)
	}
	bad := 0
	for {
		line, whole, err := linePrefix(r, len(doneIDPrefix)+2*hash.Size+1)
		if len(line) > 0 {
			rest, ok := bytes.CutPrefix(line, []byte(doneIDPrefix))
			var id hash.Hash
			var perr error = errors.New("no id")
			if ok && len(rest) > 2*hash.Size && rest[2*hash.Size] == '"' {
				id, perr = hash.Parse(string(rest[:2*hash.Size]))
			}
			switch {
			case perr == nil:
				ids[id] = true
			case whole:
				// Not a line this release writes; a line cut short by a
				// crash at the end of the file is not counted.
				bad++
			}
		}
		if err != nil {
			return ids, bad, size, nil
		}
	}
}

// doneIDPrefix is how each done line begins: the offer's id comes first.
const doneIDPrefix = `{"id":"`

// maxDoneHead bounds the done file's first line.
const maxDoneHead = 4 << 10

// linePrefix reads one line from r and returns its first max bytes,
// skipping the rest without holding it; whole says the line ended with a
// newline. err is io.EOF at the end.
func linePrefix(r *bufio.Reader, max int) (prefix []byte, whole bool, err error) {
	for {
		chunk, err := r.ReadSlice('\n')
		if room := max - len(prefix); room > 0 {
			prefix = append(prefix, chunk[:min(len(chunk), room)]...)
		}
		switch {
		case err == nil:
			return bytes.TrimRight(prefix, "\r\n"), true, nil
		case errors.Is(err, bufio.ErrBufferFull):
			continue
		default:
			return prefix, false, err
		}
	}
}

// archive moves the offers that need nothing more to the done file:
// appended and synced first, then left out of bounties.json by the save
// that follows (ORANGE-046 P1). Called with mu held.
func (b *Book) archive() error {
	if b.path == "" {
		return nil
	}
	var done []*Mine
	for _, id := range b.order {
		if m := b.mine[id]; m.done() {
			done = append(done, m)
		}
	}
	if len(done) == 0 {
		return nil
	}
	var buf bytes.Buffer
	f, err := os.OpenFile(b.donePath(), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	st, err := f.Stat()
	at := int64(0)
	if err == nil {
		at = st.Size()
	}
	if err == nil && st.Size() == 0 {
		head, _ := json.Marshal(map[string]any{"kind": DoneFileKind, "format": DoneFileFormat})
		buf.Write(append(head, '\n'))
	}
	for _, m := range done {
		c := *m
		c.State = m.state(b.now())
		line, merr := json.Marshal(c)
		if merr != nil {
			err = merr
			break
		}
		buf.Write(append(line, '\n'))
	}
	if err == nil {
		_, err = f.Write(buf.Bytes())
	}
	if err == nil {
		b.doneSize = at + int64(buf.Len())
	}
	if err == nil {
		err = f.Sync()
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return err
	}
	for _, m := range done {
		delete(b.mine, m.ID)
	}
	b.order = slices.DeleteFunc(b.order, func(id hash.Hash) bool { return b.mine[id] == nil })
	return nil
}

// saveLater saves on a timer, so a burst of changes is written once; a
// lease is only worth keeping across a restart, never a promise to pay
// (ORANGE-046 P2). Called with mu held.
func (b *Book) saveLater() {
	if b.path == "" || b.pending {
		return
	}
	b.pending = true
	time.AfterFunc(saveDelay, b.flush)
}

func (b *Book) flush() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.pending {
		_ = b.save()
	}
}

// Close writes a save still waiting on its timer.
func (b *Book) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.pending {
		return nil
	}
	return b.save()
}

// save writes the posted offers now, first moving those that need nothing
// more to the done file. Called with mu held.
func (b *Book) save() error {
	if b.path == "" {
		return nil
	}
	b.pending = false
	if err := b.archive(); err != nil {
		return err
	}
	f := file{Kind: FileKind, Format: FileFormat, DoneSize: b.doneSize, kept: b.kept}
	for _, id := range b.order {
		f.Offers = append(f.Offers, b.mine[id])
	}
	raw, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	return fsutil.WriteFile(b.path, append(raw, '\n'), 0o600)
}

// expire drops leases that ran out and peers' offers past their time.
// Called with mu held.
func (b *Book) expire() {
	now := b.now()
	finished := false
	for _, m := range b.mine {
		n := len(m.Leases)
		m.Leases = slices.DeleteFunc(m.Leases, func(l Lease) bool {
			if now.After(l.Until) {
				b.grinderOf(l.Peer).expired++
				return true
			}
			return false
		})
		if !m.Closed && m.expired(now) {
			m.Closed, m.Leases = true, nil
		}
		if len(m.Leases) != n || m.Closed {
			finished = finished || m.done()
		}
	}
	// An offer that finished by the clock moves to the done file too
	// (GUI-GREEN-063).
	if finished {
		b.saveLater()
	}
	// Peers' offers are checked at most once a second, so a list of 64
	// offers walks them once, not once an offer (ORANGE-046 P3).
	if now.Sub(b.heldSwept) < time.Second && !now.Before(b.heldSwept) {
		return
	}
	b.heldSwept = now
	b.heldOrd = slices.DeleteFunc(b.heldOrd, func(id hash.Hash) bool {
		h := b.held[id]
		if h.Offer.OpenSeconds > 0 && now.After(h.Received.Add(time.Duration(h.Offer.OpenSeconds)*time.Second)) {
			delete(b.held, id)
			b.uncount(h)
			return true
		}
		return false
	})
}

func (b *Book) grinderOf(peer string) *grinder {
	g := b.grinders[peer]
	if g == nil {
		if len(b.grinders) >= maxPeerRecords {
			b.forgetGrinder()
		}
		g = &grinder{}
		b.grinders[peer] = g
	}
	return g
}

// forgetGrinder drops one grinder record that holds no lease, so the
// records stay bounded (RED-113). Called with mu held.
func (b *Book) forgetGrinder() {
	leased := map[string]bool{}
	for _, m := range b.mine {
		for _, l := range m.Leases {
			leased[l.Peer] = true
		}
	}
	// The record that earned least goes, so a grinder that earned more
	// leases keeps them (RED-116).
	drop, least := "", -1
	for k, g := range b.grinders {
		if !leased[k] && (least < 0 || g.filled < least) {
			drop, least = k, g.filled
		}
	}
	if least >= 0 {
		delete(b.grinders, drop)
	}
}

// filled is how many layers peer handed in, without making a record.
func (b *Book) filled(peer string) int {
	if g := b.grinders[peer]; g != nil {
		return g.filled
	}
	return 0
}

// ErrOff is a market action this node's policy does not allow.
var ErrOff = errors.New("bounty: off in this node's policy")

// Post adds an offer this node will pay for. A zero Salt is filled with a
// random one, so the same request twice makes two offers.
func (b *Book) Post(o Offer) (Mine, error) {
	if err := o.Check(); err != nil {
		return Mine{}, err
	}
	if o.Salt == 0 {
		var s [8]byte
		_, _ = rand.Read(s[:])
		o.Salt = binary.BigEndian.Uint64(s[:]) | 1
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.cfg.Post {
		return Mine{}, fmt.Errorf("%w: poo policy limits -set bounty.post=1 posts offers", ErrOff)
	}
	if missing := unsetSpend(b.cfg); missing != "" {
		return Mine{}, fmt.Errorf("%w: %s not set, so this node posts no offer; poo policy limits -set bounty.max_amount=N -set bounty.budget=N", ErrOff, missing)
	}
	b.expire()
	total, over := mulCheck(o.Amount, uint64(o.Count))
	if over || total > b.cfg.MaxAmount {
		return Mine{}, fmt.Errorf("bounty: the offer pays %d × %d, over bounty.max_amount %d", o.Amount, o.Count, b.cfg.MaxAmount)
	}
	open, owed := 0, uint64(0)
	for _, m := range b.mine {
		if !m.Closed {
			open++
		}
		owed += m.Owed()
	}
	if open >= b.cfg.MaxOpen {
		return Mine{}, fmt.Errorf("bounty: %d offers open, the most bounty.max_open allows", open)
	}
	if owed+total > b.cfg.Budget || owed+total < owed {
		return Mine{}, fmt.Errorf("bounty: open offers and unpaid layers owe %d; this one would take it over bounty.budget %d", owed, b.cfg.Budget)
	}
	o.Raw = nil
	o.Raw = o.Encode()
	m := &Mine{ID: o.ID(), Offer: o, Posted: b.now()}
	if _, dup := b.mine[m.ID]; dup {
		return Mine{}, errors.New("bounty: that offer is posted already")
	}
	b.mine[m.ID] = m
	b.order = append(b.order, m.ID)
	if err := b.save(); err != nil {
		return Mine{}, err
	}
	return b.clone(m), nil
}

// unsetSpend names the spending limits posting needs that cfg leaves
// unset (GREEN-062): empty when both are set.
func unsetSpend(cfg Config) string {
	switch {
	case cfg.MaxAmount == 0 && cfg.Budget == 0:
		return "bounty.max_amount and bounty.budget are"
	case cfg.MaxAmount == 0:
		return "bounty.max_amount is"
	case cfg.Budget == 0:
		return "bounty.budget is"
	}
	return ""
}

func mulCheck(a, b uint64) (uint64, bool) {
	if a != 0 && b > math.MaxUint64/a {
		return 0, true
	}
	return a * b, false
}

// Cancel closes an offer: no new lease or claim. Layers it accepted are
// still owed. It returns the offer as closed; one that owes nothing moves
// to the done file at once.
func (b *Book) Cancel(id hash.Hash) (Mine, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	m := b.mine[id]
	if m == nil {
		return Mine{}, errors.New("bounty: no such offer of this node")
	}
	m.Closed, m.Leases = true, nil
	out := b.clone(m)
	return out, b.save()
}

// Mine returns this node's offers, oldest first.
func (b *Book) Mine() []Mine {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.expire()
	out := make([]Mine, 0, len(b.order))
	for _, id := range b.order {
		out = append(out, b.clone(b.mine[id]))
	}
	return out
}

// Get returns one of this node's offers.
func (b *Book) Get(id hash.Hash) (Mine, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.expire()
	m := b.mine[id]
	if m == nil {
		return Mine{}, false
	}
	return b.clone(m), true
}

// clone copies m with its state worked out. Called with mu held.
func (b *Book) clone(m *Mine) Mine {
	c := *m
	c.State, c.Closes = m.state(b.now()), closes(m.Posted, m.Offer.OpenSeconds)
	c.Leases = slices.Clone(m.Leases)
	c.Claims = slices.Clone(m.Claims)
	for i := range c.Claims {
		c.Claims[i].Paid = slices.Clone(c.Claims[i].Paid)
	}
	return c
}

// Listable returns the offers a peer may take now, in series (all when
// zero): open, with a layer neither handed in nor leased. A leased offer
// is not listed, so only its grinder works on it.
func (b *Book) Listable(series hash.Hash) []Offer {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.expire()
	var out []Offer
	for _, id := range b.order {
		m := b.mine[id]
		if m.Closed || m.free() <= 0 || (series != hash.Hash{} && m.Offer.Series != series) {
			continue
		}
		out = append(out, m.Offer)
		if len(out) == MaxListed {
			break
		}
	}
	return out
}

// ExpectedSeconds is the expected time to find a layer of strength bits
// at rate tries a second.
func ExpectedSeconds(strength uint8, rate uint64) float64 {
	if rate == 0 {
		return math.Inf(1)
	}
	return math.Ldexp(1, int(strength)) / float64(rate)
}

// Take answers a grinder's take: a new lease, the one extension of a
// lease it holds, or a refusal with its reason.
func (b *Book) Take(peer string, t Take) Grant { return b.TakeFrom(peer, "", t) }

// TakeFrom is Take from a grinder at network address addr (its host):
// the leases from one address are limited as one grinder's are, since a
// node id costs nothing (RED-112). An empty addr is not limited.
func (b *Book) TakeFrom(peer, addr string, t Take) Grant {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.expire()
	g := Grant{Offer: t.Offer}
	refuse := func(why string) Grant { g.Reason = why; return g }
	m := b.mine[t.Offer]
	switch {
	case m == nil || m.Closed:
		return refuse("no such open offer")
	case t.Rate == 0:
		return refuse("no rate stated")
	}
	exp := ExpectedSeconds(m.Offer.Strength, t.Rate)
	dur := time.Duration(math.Min(3*exp, b.cfg.LeaseCeiling.Seconds()) * float64(time.Second))
	dur = max(dur, b.cfg.LeaseFloor)
	if i := m.lease(peer); i >= 0 {
		l := &m.Leases[i]
		if l.Extended {
			return refuse("the lease was extended once already")
		}
		l.Until, l.Extended = l.Until.Add(dur), true
		g.Seconds = uint32(max(0, l.Until.Sub(b.now())).Seconds())
		b.saveLater()
		return g
	}
	if m.free() <= 0 {
		return refuse("leased or filled")
	}
	held, work, fromAddr := 0, exp, 0
	for _, o := range b.mine {
		if i := o.lease(peer); i >= 0 {
			held++
			work += ExpectedSeconds(o.Offer.Strength, o.Leases[i].Rate)
		}
		if addr != "" {
			for _, l := range o.Leases {
				if l.Addr == addr {
					fromAddr++
				}
			}
		}
	}
	allowed := min(b.cfg.MaxLeasesPerGrinder, 1+b.filled(peer))
	if held >= allowed {
		return refuse(fmt.Sprintf("lease limit %d reached; each layer handed in earns one more, up to %d", allowed, b.cfg.MaxLeasesPerGrinder))
	}
	if fromAddr >= b.cfg.MaxLeasesPerGrinder {
		return refuse(fmt.Sprintf("lease limit %d reached for this network address", b.cfg.MaxLeasesPerGrinder))
	}
	if work > b.cfg.LeaseCeiling.Seconds() {
		return refuse("the stated rate does not cover another lease")
	}
	m.Leases = append(m.Leases, Lease{Peer: peer, Addr: addr, PayTo: t.PayTo, Rate: t.Rate, Until: b.now().Add(dur)})
	b.saveLater()
	g.Seconds = uint32(dur.Seconds())
	return g
}

// Claim answers a grinder's claim. strength is the claimed layer's
// strength on the offer's entry, worked out by the caller with the
// series' hash, and known says the caller's node holds that layer
// already. A claim needs the grinder's lease, and a known layer is not
// owed (RED-110). An accepted claim ends the grinder's lease and is kept
// until it is paid.
func (b *Book) Claim(peer string, c Claim, strength int, known bool) Ack {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.expire()
	a := Ack{Offer: c.Offer, Nonce: c.Nonce}
	m := b.mine[c.Offer]
	if m == nil {
		a.Outcome = Closed
		return a
	}
	if slices.ContainsFunc(m.Claims, func(r ClaimRecord) bool { return r.Nonce == c.Nonce }) {
		a.Outcome = Repeat
		return a
	}
	i := m.lease(peer)
	switch {
	case len(m.Claims) >= int(m.Offer.Count):
		a.Outcome = Filled
	case m.Closed:
		a.Outcome = Closed
	case i >= 0 && m.Leases[i].PayTo != c.PayTo:
		a.Outcome = Mismatch
	case i < 0 && m.free() <= 0:
		a.Outcome = Reserved
	case i < 0:
		a.Outcome = NoLease
	case strength < int(m.Offer.Strength):
		a.Outcome = TooWeak
	case known:
		a.Outcome = Known
	default:
		a.Outcome = Accepted
		if i >= 0 {
			m.Leases = slices.Delete(m.Leases, i, i+1)
		}
		m.Claims = append(m.Claims, ClaimRecord{Peer: peer, Nonce: c.Nonce, PayTo: c.PayTo, Strength: strength, At: b.now()})
		b.grinderOf(peer).filled++
		// On disk before the ack says accepted: a promise to pay
		// (White's condition on ORANGE-046 P2).
		_ = b.save()
	}
	return a
}

// Paid records the payment of an accepted claim and returns the ack to
// send its grinder, and the grinder's peer.
func (b *Book) Paid(id hash.Hash, nonce uint64, tids []hash.Hash) (Ack, string, error) {
	if len(tids) == 0 || len(tids) > MaxTIDs {
		return Ack{}, "", fmt.Errorf("bounty: a payment has 1 to %d legs", MaxTIDs)
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	m := b.mine[id]
	if m == nil {
		return Ack{}, "", errors.New("bounty: no such offer of this node")
	}
	i := slices.IndexFunc(m.Claims, func(r ClaimRecord) bool { return r.Nonce == nonce })
	if i < 0 {
		return Ack{}, "", errors.New("bounty: no accepted claim with that nonce")
	}
	if len(m.Claims[i].Paid) > 0 {
		return Ack{}, "", errors.New("bounty: that claim is paid already")
	}
	m.Claims[i].Paid = slices.Clone(tids)
	if err := b.save(); err != nil {
		return Ack{}, "", err
	}
	return Ack{Offer: id, Nonce: nonce, Outcome: Paid, TIDs: slices.Clone(tids)}, m.Claims[i].Peer, nil
}

// Receive keeps an offer a peer sent, if this node takes offers and the
// offer pays at least MinPrice. It reports whether the offer was kept.
func (b *Book) Receive(peer string, o Offer) bool { return b.ReceiveFrom(peer, "", o) }

// ReceiveFrom is Receive from a peer at network address addr: the offers
// held from one address are bounded as one peer's are, since a node id
// costs nothing (RED-116). An empty addr is not bounded.
func (b *Book) ReceiveFrom(peer, addr string, o Offer) bool {
	if o.Check() != nil {
		return false
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.cfg.Take || o.Amount < b.cfg.MinPrice {
		return false
	}
	b.expire()
	id := o.ID()
	if _, ok := b.held[id]; ok {
		// Kept from the peer that sent it first: another peer sending it
		// again does not become its payer (RED-111).
		return true
	}
	if _, mine := b.mine[id]; mine {
		return false
	}
	// The count per peer is kept, so only a peer at its bound walks the
	// list (ORANGE-046 P3).
	if b.heldPer[peer] >= b.cfg.PerPeer {
		if i := slices.IndexFunc(b.heldOrd, func(hid hash.Hash) bool { return b.held[hid].From == peer }); i >= 0 {
			b.dropHeld(i)
		}
	}
	if addr != "" && b.heldAddr[addr] >= b.cfg.PerPeer {
		if i := slices.IndexFunc(b.heldOrd, func(hid hash.Hash) bool { return b.held[hid].Addr == addr }); i >= 0 {
			b.dropHeld(i)
		}
	}
	if len(b.heldOrd) >= b.cfg.Held {
		// The oldest goes, but not one this node holds a lease on
		// (RED-116).
		now := b.now()
		i := slices.IndexFunc(b.heldOrd, func(hid hash.Hash) bool {
			l := b.held[hid].Lease
			return l == nil || !l.Until.After(now)
		})
		b.dropHeld(max(i, 0))
	}
	b.held[id] = &Held{ID: id, Offer: o, From: peer, Addr: addr, Received: b.now()}
	b.heldOrd = append(b.heldOrd, id)
	b.heldPer[peer]++
	if addr != "" {
		b.heldAddr[addr]++
	}
	return true
}

func (b *Book) dropHeld(i int) {
	h := b.held[b.heldOrd[i]]
	delete(b.held, b.heldOrd[i])
	b.heldOrd = slices.Delete(b.heldOrd, i, i+1)
	b.uncount(h)
}

// uncount takes a dropped held offer out of the counts.
func (b *Book) uncount(h *Held) {
	if b.heldPer[h.From]--; b.heldPer[h.From] <= 0 {
		delete(b.heldPer, h.From)
	}
	if h.Addr != "" {
		if b.heldAddr[h.Addr]--; b.heldAddr[h.Addr] <= 0 {
			delete(b.heldAddr, h.Addr)
		}
	}
}

// Granted records the payer's answer to this node's take.
func (b *Book) Granted(peer string, g Grant, payTo hash.Hash) {
	b.mu.Lock()
	defer b.mu.Unlock()
	h := b.held[g.Offer]
	if h == nil || h.From != peer {
		return
	}
	if g.Seconds == 0 {
		h.Lease = &HeldLease{Reason: g.Reason}
		switch g.Reason {
		case "no such open offer":
			h.ended = StateClosed
		case "leased or filled":
			h.ended = StateTaken
		}
		return
	}
	h.ended = ""
	h.Lease = &HeldLease{PayTo: payTo, Until: b.now().Add(time.Duration(g.Seconds) * time.Second)}
}

// Acked records the payer's answer to one of this node's claims.
func (b *Book) Acked(peer string, a Ack) {
	b.mu.Lock()
	defer b.mu.Unlock()
	h := b.held[a.Offer]
	if h == nil || h.From != peer {
		return
	}
	p := b.payers[peer]
	if p == nil {
		if len(b.payers) >= maxPeerRecords {
			for k := range b.payers { // bounded (RED-113)
				delete(b.payers, k)
				break
			}
		}
		p = &PayerRecord{}
		b.payers[peer] = p
	}
	switch a.Outcome {
	case Accepted:
		p.Accepted++
	case Paid:
		p.Paid++
	}
	h.Acks = append(h.Acks, a)
	if len(h.Acks) > MaxAcks { // the newest kept (RED-113)
		h.Acks = slices.Delete(h.Acks, 0, len(h.Acks)-MaxAcks)
	}
	switch a.Outcome {
	case Accepted:
		h.Lease = nil
	case Filled:
		h.ended = StateFilled
	case Closed:
		h.ended = StateClosed
	case Reserved:
		h.ended = StateTaken
	}
}

// HeldOffers returns the offers peers sent, oldest first.
func (b *Book) HeldOffers() []Held {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.expire()
	out := make([]Held, 0, len(b.heldOrd))
	for _, id := range b.heldOrd {
		h := *b.held[id]
		if h.Lease != nil {
			l := *h.Lease
			h.Lease = &l
		}
		h.Acks = slices.Clone(h.Acks)
		h.State, h.Closes = b.heldState(&h), closes(h.Received, h.Offer.OpenSeconds)
		out = append(out, h)
	}
	return out
}

// heldState works out a held offer's state. Called with mu held.
func (b *Book) heldState(h *Held) string {
	switch {
	case h.ended != "":
		return h.ended
	case h.Lease != nil && h.Lease.Until.After(b.now()):
		return StateLeased
	}
	return StateOpen
}

// HeldFrom returns the peer an offer this node holds came from: the only
// peer whose grants and acks for it count (RED-111).
func (b *Book) HeldFrom(id hash.Hash) (string, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	h := b.held[id]
	if h == nil {
		return "", false
	}
	return h.From, true
}

// HeldOffer returns one offer a peer sent.
func (b *Book) HeldOffer(id hash.Hash) (Held, bool) {
	for _, h := range b.HeldOffers() {
		if h.ID == id {
			return h, true
		}
	}
	return Held{}, false
}

// Payers returns, by peer, what this node saw each payer accept and pay.
func (b *Book) Payers() map[string]PayerRecord {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := map[string]PayerRecord{}
	for k, v := range b.payers {
		out[k] = *v
	}
	return out
}
