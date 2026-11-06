# Source Round 11: consensus review (Black, Part D)

## Summary

No change in this round adds or alters a validity, weight, selection or
byte-form rule, and no team proposed a consensus change, so **nothing is
passed to the owner for approval**.

Round 10's flaky guard test is fixed at its cause (D133).
`TestSlowSenderIsReachedOverManyChases` passed 10 runs out of 10 under
`-race` on BLUE-012.

**B4 is only partly closed. One hard-fork case remains, B5, and Black
reproduced it.** D132 and BLUE-012's RED-029 let a moving honest fetch
take the disk reserve from a trickler. But a waiting fetch is ranked by
its own last chase, and it cannot ask for parts while it waits. So an
honest fetch whose last parts request timed out stays "slow" for as long
as it waits. A trickler that is also slow, with less left to come, then
holds the reserve against it for good. One lost request is enough. So is
an honest link under about 2 MB/s, because every 64 MiB parts reply on
such a link times out. The fix is node policy only, and goes in Round 12
Part A.

BLUE-012 should still be merged. It is a strict improvement on B4, and
its RED-028 change to D133 is clear. ORANGE-021, GREEN-036 and the rest
of Part A's changes are clear.

| Field | Value |
| --- | --- |
| Round | Source Round 11 |
| Consensus suggestions | None |
| Base reviewed | `claude/m10-watch` @ `1ae02ca` (code `5114e0b`; D132 to D135) |
| Previous base | `claude/m10-watch` @ `0a6cc94` |
| Blue branches | `claude/project-thread-ggo8dt` @ `840671c` (BLUE-012, D136) |
| Proposals | ORANGE-021 (`claude/source-orange-r11` @ `cf077ca`); GREEN-036 (`claude/source-green` @ `590784a`) |
| Result | 10 clear, 0 with conditions, 1 blocked: hard fork (B5, reproduced) |

## Round 10's items

| Item | Now | Checked how |
| --- | --- | --- |
| B4, a trickler holds the disk reserve | Closed for a moving honest fetch (D132, RED-029). Still open for a fetch marked slow: see B5 | `TestTricklerCannotHoldTheReserve` and `TestTricklerJustUnderCannotHoldTheReserve` pass. Black's B5 test fails on both `1ae02ca` and `840671c` |
| The flaky C-6 guard test | Met (D133) | Fixed at its cause: the asker learns a sender's request budget, so it is no longer timed out and refused. The test is not skipped or retried. It passed 10 of 10 runs under `-race` on `840671c` |

## Blocked

### B5. An honest fetch marked slow can never take the reserve back (blocked: hard fork, reproduced)

- **Where.** `reclaimParts`, `partsRank` and `saveProgress` (D132, and
  RED-029 in D136). The case is the same on `1ae02ca`, where it is
  RED-029's case.
- **The rule.** A fetch waiting for disk takes room only from files that
  rank strictly below it. "Slow" ranks below "moving". A waiting chase
  is ranked by `partsSlow` from its own last chase that asked for parts.
- **What goes wrong.**
  - `partsSlow` is set only when a chase asked for parts (`pt.tried`).
    A chase that waits for room never asks, so its flag never changes
    while it waits.
  - The flag is set when the last chase took in under 1 MiB **or** had
    a parts request time out, even one that took in a lot first.
- **The case.**
  1. The honest sender's parts request times out once. That can be one
     lost request, or a link under about 2 MB/s, where a 64 MiB parts
     reply cannot arrive within the 30 s `RequestTimeout`. The chase
     stalls, its parts are kept, and it is now slow.
  2. A trickler that copied the heavier ledger's listing arrives while
     the honest chase is backed off. It is fresh, so it is moving, and it
     reclaims the honest file. Or it reaches the reserve first with more
     bytes held.
  3. It then sends a little each chase and is slow too. It holds more
     bytes of the same entry, so it has less still to come, and D132
     ranks it above the honest fetch.
  4. The honest chase waits for room. It is slow and ranks below, so it
     cannot reclaim the trickler's file. It cannot ask for parts, so its
     flag never clears. The trickler's own chase reclaims the honest file
     whenever it is recreated.
  5. The trickler can send one byte a chase. The entry is never
     finished, and D115 is broken.
- **Reproduced.** The test is in the appendix. The honest sender drops
  only its first parts request. Over 25 seconds it is asked for one part
  and the trickler for 22, with about 450 waits for disk space. It fails
  on `840671c` and on `1ae02ca`.
- **Cost to the attacker.** As for B4: fill the disk to its reserve,
  then trickle. It needs an honest fetch that was marked slow once. With
  the timeout rule, that is every sender on a link under about 2 MB/s.
- **Fix shape, for Round 12 Part A (node policy, no rule changes).** Pick
  one, or combine:
  - **Let a waiting chase's flag expire.** A chase that has waited for
    room for a while (for example two backoff steps) ranks as moving
    again, takes room and asks. If it really is slow, its next save marks
    it slow again and it gives the room back. Two slow chases then take
    turns, and each makes progress on its turn.
  - **Take turns among slow chases.** Between two slow files, do not use
    D132's tie-break. The one that waited longer gets the room next.
  - **A timeout that took in at least `partsMoving` is moving.** The
    bytes arrived, so the link is moving. This helps the slow-link case,
    but not a single request lost at 0 bytes, so on its own it is not
    enough.
- **Test to add to P0.** The appendix test, unchanged: an honest sender
  whose first parts request is lost, and a trickler of the same entry,
  with the parts disk at its reserve. The honest fetch must reach the
  entry's end.
- **For Red, Round 12 Part B.** Try the slow-link case: an honest sender
  whose every parts reply times out after more than 1 MiB.

## Clear

| Item | Why it is clear |
| --- | --- |
| D132: "more still to come ranks below" | Ranking only. Between moving fetches it gives the room to the one nearer its end, and that one only gets nearer, so they never take turns. It leaves B5 open, which is about the slow key, not this tie-break |
| D133: the learned pace | It only spaces this node's requests to one peer, never below one a window (`max(1, n*3/4)`), and it lapses after 30 windows. A peer can only slow requests to itself. Nothing is refused |
| BLUE-012: RED-028, the probed pace | It changes D133's "never rises while it lasts" for a first learning only. A probe that rises too high is learned again on the next drops and then holds. Either way it only paces |
| BLUE-012: RED-029, the slow key | Ranking only, and better than D132 alone: a moving honest fetch now beats a trickler whatever it claims. It does not cover a fetch that is already slow (B5) |
| BLUE-012: RED-030, and D134 (GUI-017) | `node.json` writes from the CLI take the data directory's lock. Local files only |
| D135: ORANGE-020 adopted | Cleared in Round 10. `PreverifyPage` only fills the verified-witness list. Every entry is still checked in full |
| D135: GREEN-035 adopted | Grinding job slots, schedule restarts and `Policy.Lowered`. Nothing a node accepts |
| ORANGE-021, warming the store's witnesses on restart | `Policy.Warm` runs the same satisfaction check as `CheckCached`. It remembers only a pass, under the same key (the whole witness and the TID). The later check still runs the size, address and allowed checks before it consults the list. A witness that fails is not remembered and fails again on replay, so a tampered store is refused as before. Decoding is unchanged, and a record that fails to decode is still not kept. ORANGE-014's C-4 conditions apply as before |
| GREEN-036, doctor `fix_args` name the checked policy | Advice in `poon doctor` |
| GREEN-036, floor and rate changes keep running jobs | The grinder reads them at each search. It is the node's own grinding only |

## Items for the owner

- **No consensus change is proposed this round.** Nothing needs your
  approval.
- **B5 is a node-policy fix needed in Round 12.** It changes no rule.
  An attacker who fills a node's spare disk can still keep it full with a
  trickle, if the honest sender of the large entry ever had one request
  time out, or is on a link under about 2 MB/s. The node then waits for
  good for that entry of the heavier ledger.

How this was checked:
- The code changes from `0a6cc94` to `1ae02ca` were read, along with
  BLUE-012's, ORANGE-021's and GREEN-036's diffs and decisions D132 to
  D136.
- On `840671c`, these all pass: `go test ./pkg/p2p ./pkg/chase
  ./pkg/vectors ./pkg/cli ./pkg/engine ./pkg/ledger ./pkg/grind
  ./pkg/nodepolicy`, and `go test -race -count=10 -run
  TestSlowSenderIsReachedOverManyChases ./pkg/p2p`.
- B5 was reproduced with the appendix test on `840671c` and `1ae02ca`.

## Appendix: B5 test

Put it in `pkg/p2p` as a `_test.go` file. It fails today, and it should
pass once B5 is fixed.

```go
package p2p_test

import (
	"crypto/rand"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/p2p"
)

// Black R11 (B5): an honest sender whose one parts request timed out is
// marked slow and, while waiting for room, never asks again, so the flag
// never clears. A trickler (also slow) with less still to come then holds
// the reserve against it for as long as it trickles.
func TestBlackSlowHonestCannotReclaimFromTrickler(t *testing.T) {
	const disk = 1 << 50
	restore := p2p.SetDiskSpace(func(dir string) (uint64, uint64, error) {
		es, err := os.ReadDir(dir)
		if err != nil {
			return 0, 0, err
		}
		if len(es) >= 2 {
			return disk / 100, disk, nil
		}
		return disk, disk, nil
	})
	t.Cleanup(restore)
	dir := t.TempDir()
	a := startNode(t, "a", func(c *p2p.Config) {
		c.RequestTimeout = 300 * time.Millisecond
		c.Chase.Backoff = 2 * time.Second
		c.Chase.YieldWait = 50 * time.Millisecond
		c.PartsDir = dir
	})
	const parts = 64
	hs := startLarge(t, a)
	rand.Read(hs.eh[:])
	hs.size = parts * 64 << 10
	hs.skip = func(n int) bool { return n == 0 } // one lost request
	var done, reached atomic.Bool
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		hs.serve(t, time.Now().Add(time.Hour), func() bool {
			if len(hs.offsets) > 0 && hs.offsets[len(hs.offsets)-1] == hs.size-64<<10 {
				reached.Store(true)
			}
			return done.Load() || reached.Load()
		})
	}()
	t.Cleanup(func() { done.Store(true); wg.Wait() })
	time.Sleep(700 * time.Millisecond) // the honest request times out, the chase stalls
	tr := startLarge(t, a)
	tr.eh, tr.size = hs.eh, hs.size
	tr.skip = func(n int) bool { return n%2 == 1 } // one part a chase, then a time-out
	wg.Add(1)
	go func() {
		defer wg.Done()
		tr.serve(t, time.Now().Add(time.Hour), func() bool { return done.Load() })
	}()
	until := time.Now().Add(25 * time.Second)
	for !reached.Load() {
		if time.Now().After(until) {
			t.Fatalf("honest fetch never reached its end: honest asked %d parts, trickler %d", len(hs.offsets), len(tr.offsets))
		}
		time.Sleep(50 * time.Millisecond)
	}
}
```
