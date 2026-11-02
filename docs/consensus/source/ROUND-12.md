# Source Round 12: consensus review (Black, Part D)

## Summary

No change in this round adds or alters a validity, weight, selection or
byte-form rule, and no team proposed a consensus change, so **nothing is
passed to the owner for approval**.

Round 11's B5 is fixed on main (D137). Black's test passes there and on
BLUE-013.

**BLUE-013's paid cuts (D139) are blocked: hard fork (B6, reproduced).**
They fix ORANGE-022's case and Red's three findings. But they compare a
waiting fetch with a holder by "time to finish", and they judge the
cutting fetch by the rate of the part that has just arrived. So two
colluding senders can fill the parts disk once and then keep it for as
long as they like at a trickle:
- Each claims only a little more than it holds, so each always looks
  near its end.
- Each sends one burst of parts a chase, which cuts the other's file, so
  neither ever completes.
- An honest fetch many times faster is never allowed to cut either.

In Black's test the honest fetch held at most 1 part in 20 seconds in 8
runs out of 9. On main it took the room in every run.

ORANGE-022's finding is upheld. On main (D137), two slow honest fetches
that share room for one throw each other's files away for good. That
breaks D115 on main today, so Round 13 Part A must fix both cases.
BLUE-013 is the right base for that fix, with B6 closed. ORANGE-022's
own branch is superseded and should not be merged.

RED-034, GUI-018, GREEN-037 and the Round 11 adoptions (D138) are clear.

| Field | Value |
| --- | --- |
| Round | Source Round 12 |
| Consensus suggestions | None |
| Base reviewed | `claude/m10-watch` @ `bc2e4bc` (code `7af8744`; D137, D138) |
| Previous base | `claude/m10-watch` @ `1ae02ca` |
| Blue branches | `claude/project-thread-ggo8dt` @ `86e6049` (BLUE-013, D139: RED-031 to RED-034, GUI-018) |
| Proposals | ORANGE-022 (`claude/source-orange-r12` @ `4f0ab30`); GREEN-037 (`claude/source-green` @ `557534b`) |
| Result | 6 clear, 0 with conditions, 1 blocked: hard fork (B6, reproduced); ORANGE-022's finding upheld and superseded |

## Round 11's item

| Item | Now | Checked how |
| --- | --- | --- |
| B5, a fetch marked slow never takes the reserve back | Fixed on main (D137) and on BLUE-013 | `TestBlackSlowHonestCannotReclaimFromTrickler` (now `pkg/p2p/b5_test.go`) passes on both, three runs under `-race` on `86e6049`. D137 brought its own D115 case (ORANGE-022, below) |

## Blocked

### B6. Two colluding senders hold the reserve with paid cuts (blocked: hard fork, reproduced)

- **Where.** `takeRoom`, `takeFrom`, `mayTake` and `fetchParts` in
  `pkg/p2p/chase.go` (BLUE-013, D139).
- **The rule as written.** A fetch whose part does not fit may cut a
  kept file:
  1. first one whose sender left;
  2. then one with no measure, or a stale one;
  3. then one it would finish before.

  "Finish" is bytes still to come over a rate. For a kept file that is
  `progress.partsRate`, from its last chase. For the cutting fetch it is
  the rate of this chase so far, so for its first part, that part's own
  arrival rate.
- **What goes wrong.**
  - Bytes still to come is the sender's claim less what it has sent. A
    sender that has already sent a lot only has to claim a little more,
    so it always looks near its end.
  - A fetch whose part arrives at once has a near-infinite rate at that
    moment. So it may cut any file that is not finishing at once,
    whatever it does over the rest of the chase.
  - With even one byte free above the reserve, a chase asks without
    `mayTake`. The part that then does not fit goes to `takeRoom`, and
    each cut frees exactly the room of the part it is paying for. So the
    free byte stays free.
- **The case.**
  1. Two senders X and Y each list the heavier ledger's hashes and
     layers, so their chases pass the work check. Together they fill the
     parts disk once, leaving one byte above the reserve. Each claims a
     few chases' worth more than it holds.
  2. Each chase, X sends a few parts at once, then lets a request time
     out. Its burst cuts Y's file by the same bytes. Then Y does the same
     to X. What each still has to come goes up and down, and neither
     ever completes, so the hash check never runs.
  3. Each saves a fresh measure every chase, so neither goes stale. Its
     finish (a few chases' worth over parts per time-out) stays below the
     honest fetch's.
  4. An honest sender of the heavier ledger's large entry E, on a
     normal link, has `finish = size(E) / its rate`. Once that is longer
     than the tricklers' finish, it may cut neither. Its part is dropped
     and its next chase does not ask (`mayTake`). It never holds a byte.
  5. With the defaults, the tricklers' finish is about 4 × 30 s, so
     this holds against any large entry that takes an honest sender more
     than about 2 minutes, such as 1.2 GB at 10 MB/s.
  6. Each trickler costs about 4 KB/s (four 64 KiB parts a minute). The
     cost of filling the disk is paid once, as in B4.
- **Reproduced.** The test is in the appendix. Two tricklers each hold
  32 MiB and claim 2 MiB more, with one byte free above the reserve.
  Each sends four parts at once a chase, then a time-out. The honest
  sender sends a 64 MiB entry at about 13 MB/s, which fits the room
  exactly.
  - On `86e6049` the honest fetch held at most 1 part in 20 s in 8 runs
    out of 9. The tricklers were asked about 650 times each and never
    completed.
  - On `bc2e4bc` (D137) the honest fetch took the room (427 to 1,023
    parts). There the tricklers rank slow and the honest fetch moving.
- **Fix shape, for Round 13 Part A (node policy, no rule changes).**
  Keep BLUE-013's paid cuts: room only for bytes that arrive, cut from
  the end, so a cut file resumes instead of starting over. But change
  what decides a cut:
  - **Compare rates, not times to finish.** A fetch may cut a file whose
    sender delivers more slowly than it does. A claimed size, and how
    much is still to come, never decide. A holder then keeps the room
    only by sending at least as fast as the honest sender, for as long
    as it holds it. That is the bandwidth contest D139 already names, so
    the "nearer its end" order only breaks ties.
  - **Measure over a whole chase, never a burst.** The cutting fetch is
    judged by its last complete chase (bytes over request time, lost
    requests included). A fresh fetch is judged by its current chase only
    after at least a full parts reply or one time slice.
  - **Add a margin** (for example, the cutter must be at least 1.5 times
    as fast), so two honest fetches at about the same rate do not trade
    bytes back and forth. With cuts by bytes, neither starts over, and
    the faster one finishes first (ORANGE-022's aim).
  - **One byte of room is not room.** A chase asks without `mayTake`
    only when there is room for a full part.
- **Test to add to P0.** The appendix test, unchanged. The honest fetch
  must hold at least 64 parts within 20 s. Keep ORANGE-022's test and
  the B4 and B5 tests too.
- **For Red, Round 13 Part B.** Try three or more colluders, and a fresh
  sender at every chase.

## ORANGE-022: upheld, superseded

- **The finding.** On main (D137), two slow honest fetches with room
  for one take turns deleting each other's whole file, so neither ever
  reaches its end (Orange's measurements). That breaks D115 on main
  today. It is not an attack: it happens to honest senders on slow links
  whenever the parts disk is short.
- **Its branch.** `finishesFirst` fixes it within D137's ranks. BLUE-013
  replaces those ranks, and with them this fix, with cuts by bytes that
  never send a file back to offset 0. That is the better base. Do not
  merge `claude/source-orange-r12` as well.
- **Condition for Round 13.** Whatever closes B6 must keep
  `TestTwoSlowFetchesBothFinish` (ORANGE-022's test, now in
  `orange022_test.go` on BLUE-013) passing.

## Clear

| Item | Why it is clear |
| --- | --- |
| D137, B5's turns | It closes B5 (the test passes). It is replaced by D139, and its own D115 case is ORANGE-022, above |
| D138: ORANGE-021 adopted | Cleared in Round 11. Witnesses of the node's own store are warmed with the same check, and every check still runs on replay |
| D138: GREEN-036 adopted | `poo doctor` advice, and the grinder reads the floor and rate at each search. Nothing a node accepts |
| BLUE-013: RED-034, the probe halves | Only how fast this node asks one peer. It never goes below where the probe began, which is at least one request a window, and the learned pace still lapses. Nothing is refused |
| BLUE-013: GUI-018, one follow replace | Which pools a node follows is its own choice, as before. The replace is one change under `policyMu` instead of a remove and an add |
| GREEN-037, `policy_changed` | Status and a `poo doctor` warning only |

## Items for the owner

- **No consensus change is proposed this round.** Nothing needs your
  approval.
- **Main has a D115 case today** (ORANGE-022). Two honest fetches of
  large entries that share too little disk can keep throwing each other's
  work away, and neither finishes. BLUE-013 fixes that but opens B6. Two
  colluding senders that fill the disk once can then hold it with a
  trickle against an honest fetch. Both need a node-policy fix in Round
  13. Neither changes a rule.

How this was checked:
- Read the code changes from `1ae02ca` to `bc2e4bc`, BLUE-013's,
  ORANGE-022's and GREEN-037's diffs, and decisions D137 to D139.
- On `86e6049`, these pass: `go test ./pkg/p2p ./pkg/chase ./pkg/vectors
  ./pkg/cli ./pkg/rpc ./pkg/nodepolicy`. So does `go test -race -count=3`
  on the slow-sender, B5, trickler, `TakeFrom`, ORANGE-022 and probing
  tests.
- B6 was reproduced with the appendix test: it failed 8 of 9 runs on
  `86e6049` and passed 2 of 3 runs on `bc2e4bc`. The third run on
  `bc2e4bc` failed for a reason Black did not track down, after the
  honest fetch had already taken 984 parts. That is past the 64 the test
  asks for, so the failure is not the cut order. B6 was also confirmed
  by logging `takeFrom`'s numbers locally: the honest fetch's finish
  came out as 5.7 to 6.1 s, against the tricklers' 2.2 to 2.9 s.

## Appendix: B6 test

Put it in `pkg/p2p` as a `_test.go` file. It fails today on BLUE-013,
and it should pass once B6 is fixed. `serveDelayed` is
`largeSender.serve` with each part sent a delay after its request, so
the honest sender's first part does not arrive at once.

```go
package p2p_test

import (
	"crypto/rand"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/p2p"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/weight"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/wire"
)

// Black R12 (B6): two colluding senders fill the parts disk once, leave
// one byte above the reserve, and then each sends one part a chase and
// lets the next request time out. With a byte of room each may ask, and
// its part, arriving at once, finishes "first" against the other's file,
// so each cuts the other in turn and neither ever completes. Both keep a
// fresh measure, and each claims only a little more than it holds, so
// both are expected to finish before the honest fetch and it may cut
// neither. The honest fetch, delivering many times faster, never holds a
// byte.
func TestBlackColludingTricklersHoldTheReserve(t *testing.T) {
	const part = 64 << 10
	const hold, extra = 512, 32 // parts each trickler holds, and claims beyond
	const capParts = 2 * hold  // the parts disk above the reserve
	const disk = 1 << 50
	reserve := uint64(disk / 50)
	restore := p2p.SetDiskSpace(func(dir string) (uint64, uint64, error) {
		es, err := os.ReadDir(dir)
		if err != nil {
			return 0, 0, err
		}
		var used uint64
		for _, e := range es {
			if fi, err := e.Info(); err == nil {
				used += uint64(fi.Size())
			}
		}
		room := uint64(capParts*part + 1)
		if used >= room {
			return reserve, disk, nil
		}
		return reserve + room - used, disk, nil
	})
	t.Cleanup(restore)
	dir := t.TempDir()
	a := startNode(t, "a", func(c *p2p.Config) {
		c.RequestTimeout = 300 * time.Millisecond
		c.Chase.Backoff = 500 * time.Millisecond
		c.Chase.YieldWait = 50 * time.Millisecond
		c.PartsDir = dir
		c.RequestPace = 1 << 20
	})
	var done atomic.Bool
	var wg sync.WaitGroup
	t.Cleanup(func() { done.Store(true); wg.Wait() })
	trickler := func() *atomic.Int64 {
		tr := startLarge(t, a)
		rand.Read(tr.eh[:])
		tr.size = (hold + extra) * part
		// Fill hold parts, time out, then four parts at once and a time-out each chase.
		tr.skip = func(n int) bool { return n == hold || n > hold && (n-hold)%5 == 0 }
		var asked atomic.Int64
		wg.Add(1)
		go func() {
			defer wg.Done()
			tr.serve(t, time.Now().Add(time.Hour), func() bool {
				asked.Store(int64(len(tr.offsets)))
				return done.Load()
			})
		}()
		return &asked
	}
	wait := func(n *atomic.Int64, want int64) {
		until := time.Now().Add(30 * time.Second)
		for n.Load() < want {
			if time.Now().After(until) {
				t.Fatal("setup: a trickler did not fill")
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
	x := trickler()
	wait(x, hold+1)
	y := trickler()
	wait(y, hold+1)
	time.Sleep(2 * time.Second) // a few rounds of cutting each other

	hs := startLarge(t, a)
	rand.Read(hs.eh[:])
	hs.size = capParts * part // fits the parts disk exactly
	var most atomic.Uint64
	wg.Add(1)
	go func() {
		defer wg.Done()
		serveDelayed(t, hs, 5*time.Millisecond, func() bool {
			for _, o := range hs.offsets {
				if o > most.Load() {
					most.Store(o)
				}
			}
			return done.Load()
		})
	}()
	time.Sleep(20 * time.Second)
	if most.Load() < 64*part {
		t.Fatalf("the honest fetch held %d parts after 20 s (the tricklers were asked %d and %d times)", most.Load()/part, x.Load(), y.Load())
	}
}

// serveDelayed is largeSender.serve with each part sent delay after its
// request, as over a link of 64 KiB a delay.
func serveDelayed(t *testing.T, ls *largeSender, delay time.Duration, stop func() bool) {
	huge := weight.FromUint64(1 << 62)
	eh := ls.eh
	for !stop() {
		m, _, err := ls.r.readUntil(time.Now().Add(20 * time.Millisecond))
		if err != nil {
			continue
		}
		switch m.Type {
		case wire.TypeHashChainReq:
			ls.r.reply(m, wire.TypeHashChain, wire.HashChainChunk{FromStart: true, Hashes: []hash.Hash{eh}}.Encode(), huge)
		case wire.TypeLayersReq:
			ls.r.reply(m, wire.TypeLayers, wire.LayersChunk{Items: []wire.HeldLayers{{Entry: eh, Held: true, Nonces: []uint64{1}}}}.Encode(), huge)
		case wire.TypeWeightReq:
			ls.r.reply(m, wire.TypeWeight, nil, huge)
		case wire.TypeFullLedgerReq, wire.TypeExtensionReq:
			ls.r.reply(m, wire.TypeLarge, wire.Large{Entry: eh, Size: ls.size}.Encode(), huge)
		case wire.TypePartsReq:
			req, err := wire.DecodeRequest(m.Type, m.Body)
			must(t, err)
			q := req.(wire.PartsReq)
			ls.offsets = append(ls.offsets, q.Offset)
			data := make([]byte, 64<<10)
			if q.Offset == 0 {
				copy(data, ls.first)
			}
			time.Sleep(delay)
			ls.r.write(wire.Message{Header: wire.Header{Type: wire.TypePart, Section: m.Section, RequestID: m.RequestID,
				Chunk: 1, Flags: wire.FlagLast | wire.FlagMore, Stated: huge}, Body: wire.Part{Total: ls.size, Offset: q.Offset, Data: data}.Encode()})
		}
	}
}
```
