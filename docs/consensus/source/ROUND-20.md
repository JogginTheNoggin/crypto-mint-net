# Source Round 20: consensus review (Black, Part D)

## Summary

No change in this round adds or alters a validity, weight, selection or
byte-form rule, and no team proposed a consensus change, so **nothing is
passed to the owner for approval**. **Nothing is blocked** under the
owner's hard-fork definition (AGENTS.md): nodes that have the same
information cannot agree which ledger is valid or heavier.

The round is mostly node policy and the CLI: the owner's settings export
and import (D155), and Red's findings on it (D156). It also has two
changes to how unused pools are held:
- RED-057, chase-only records for a huge series;
- ORANGE-030, reading an unused pool without making its state.

Black tested RED-057's record end to end. An unused pool of a huge whole
series is still chased to a heavier ledger, and then indexed.

| Field | Value |
| --- | --- |
| Round | Source Round 20 |
| Consensus suggestions | None |
| Base reviewed | `main` @ `522e891` (PR #109: D155) |
| Previous base | `main` @ `4240d32` |
| Blue branches | PR #116 @ `b877c45` (BLUE-021, D156: RED-052 to RED-057) |
| Red findings | PR #114 @ `55ff558` (RED-052 to RED-057) |
| Proposals | GREEN-045 (PR #111 @ `fba4095`); ORANGE-030 (PR #112 @ `dd527df`) |
| Result | 8 clear, 0 with conditions, 0 blocked |

## Round 19's items

| Item | Now |
| --- | --- |
| BLUE-020 (D154), ORANGE-029, GREEN-044 | Merged as cleared (D155) |
| The pool index on the wire | Not built; the owner has not decided |

## D155: settings export and import

A settings import carries the same node settings that `-settings` did in
Round 17, so Black's verdict there still holds. `pkg/p2p/settings_extremes_test.go`
sets `k`, the reply limits, `max_pending` and `seen_size` to both
extremes, and a node still reaches a heavier ledger. Three more points:
- **The weight check stays on.** `Load` and `FromPolicy` both clamp
  `check_seconds` (D153), so a settings file cannot turn it off.
- **No depth limit can come in.** `depth_limit` and `fast_catch_up` are
  ignored on load and left out of every export.
- **`Plan.Fit` refuses nothing.** It only lists lines that may not fit
  the machine.

Clear.

## RED-057: chase-only records for a huge whole series (D156)

Blue's question: does holding an unindexed pool's record only for the
chase keep D115 safe?

**Yes, by reading and by test.**
- **The record lasts as long as the chase.** `offer` takes the record
  from `chaseIn`: the indexed one if there is one, else one held in
  `chasing` by pool id. `runJob` finds it through `poolByName`, which
  looks in both.
- **A running chase keeps its record.** `sweepChasing` drops a record
  only when `Queue.Pools()` holds no group for its pool, and a running
  group stays in the queue until `Finish`.
- **Kept data indexes the pool.** Once an entry is kept, `onKept`
  indexes the pool, and `indexLocked` removes it from `chasing`.
- **An offer after a drop gets a new record.** That is fair, since a
  stated weight is only the sender's claim.

**Black's test** (appendix, `TestBlackHugeSeriesUnusedPoolChasedToHeavier`):
1. a and b follow a series of 2^40 pools whole.
2. b holds a four-entry ledger in a pool a has never used. a is
   connected, and holds nothing there and indexes nothing.
3. b then forwards one new entry.
4. a holds the pool for the chase, reaches b's tip, and indexes the pool.

It passes 5 of 5 under `-race` on `b877c45`.

**What stays an information gap** (as in Round 19): a notice, a stated
weight on its own, or an entry with no transaction, in such a pool. The
wire names the pool only by its id hash, so these cannot be placed. Under
the owner's definition that is a lack of information, not a hard fork.
The pool index on the wire would close it.

## RED-053: the text check in node policy (D156)

`Validate` now refuses a control character, a bidirectional override or
bytes that are not UTF-8 in any string of a node policy. A node.json
holding such text no longer loads, so the node stops until the operator
fixes it. That is the node's own configuration, and it refuses no ledger.

Black checked the one place where policy text meets consensus data:
series parameters (`SeriesInfo.Params`). `series.Params` has no free
text, only numbers, booleans, type and algorithm ids, a hash name and an
issuer hash. So no valid series is refused by the check. Clear.

## ORANGE-030: reading an unused pool without making its state

`Status` and `Tip` answer a followed pool with no state as a fresh one
would: the pool id as tip, height 0, weight zero, no branches.
`TestUnstatedReadsAsFresh` compares the two. That is the same answer as
before, without the allocation. Clear.

## Clear

| Item | Why it is clear |
| --- | --- |
| D155: settings export and import | Node settings only, already shown unable to refuse a heavier ledger (see above) |
| D156 (RED-052): personal fields left out of a plain export | Export only |
| D156 (RED-053): the text check; `termSafe`; `Fit` never opens network paths | Node policy and CLI. No series parameter is text (see above) |
| D156 (RED-054, RED-055, RED-056): plans, `-map-path`, series now used | Import planning only |
| D156 (RED-057): chase-only records | The chase runs as before. Tested end to end (see above) |
| GREEN-045: `help` and `-h` for every command; `-map-path` notes | CLI only |
| ORANGE-030: unused pools read without state | The same answer as a fresh state |
| Red PR #114 | The findings. Their fix is BLUE-021 |

## Items for the owner

- **No consensus change is proposed this round.** Nothing needs your
  approval.
- **No hard-fork case is open.** Settings files, chase records and the
  text check change what a node knows or loads, never what it accepts.

How this was checked:
- Read the code changes from `4240d32` to `522e891` (PR #109), the diffs
  of PRs #111, #112, #114 and #116 against `main`, and decisions D155
  and D156.
- `go test ./pkg/engine ./pkg/node ./pkg/nodepolicy ./pkg/cli
  ./pkg/chase ./pkg/p2p ./pkg/vectors` passes on `fba4095`, `dd527df`
  and `b877c45`.
- Black's RED-057 test passes on `b877c45`.

## Appendix: Black's RED-057 test

Not in the repository. Grey may take it into `pkg/p2p` if wanted. It uses
`pkg/p2p`'s test helpers (`startNodeWith`, `eventually`,
`eventuallyFor`) and `IndexedPools` from BLUE-021's `export_test.go`.

```go
package p2p_test

import (
	"testing"
	"time"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/engine"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/nodepolicy"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/testutil"
)

// Black R20 (RED-057): a pool of a huge whole series that a is not using.
// b holds a ledger there. When b forwards one new entry on it, a holds the
// pool for the chase only, chases b's ledger, and ends on it (then indexed).
func TestBlackHugeSeriesUnusedPoolChasedToHeavier(t *testing.T) {
	prm := testutil.ExampleParams()
	prm.PoolSize, prm.Mints = 1, 1<<40
	s := testutil.MustSeries(prm)
	id := s.ID()
	keys := testutil.NewKeys(s)
	a := startNodeWith(t, "a", s, nodepolicy.Follow{Series: &id}, nil)
	b := startNodeWith(t, "b", s, nodepolicy.Follow{Series: &id}, nil)
	serial := uint64(1) << 39
	k := engine.PoolKey{Series: id, Index: s.PoolOf(serial)}
	owner := "alice"
	prev := keys.Claim(serial, testutil.P(owner, prm.Supply))
	step := func(next string) {
		_, err := b.n.HandleTx(prev, engine.Local)
		must(t, err)
		e, _, err := b.n.Build(k, 0)
		must(t, err)
		_, _, err = b.n.Work(k, e.Hash(s), 0, 16)
		must(t, err)
		nt := keys.Spend(serial, []testutil.In{keys.Out(prev, 0, owner)}, testutil.P(next, prm.Supply))
		prev, owner = nt, next
	}
	for _, o := range []string{"o1", "o2", "o3", "o4"} {
		step(o)
	}
	a.connect(b)
	eventually(t, "a and b to peer", func() bool { return len(a.h.Peers()) == 1 && len(b.h.Peers()) == 1 })
	time.Sleep(300 * time.Millisecond)
	if _, h, _, _ := a.n.Engine().Tip(k); h != 0 || a.h.IndexedPools() != 0 {
		t.Fatalf("setup: a already has height %d, %d pools indexed", h, a.h.IndexedPools())
	}
	step("o5")
	eventuallyFor(t, 30*time.Second, "a to reach b's ledger in the unused pool", func() bool {
		ta, _, _, err1 := a.n.Engine().Tip(k)
		tb, _, _, err2 := b.n.Engine().Tip(k)
		return err1 == nil && err2 == nil && ta == tb
	}, a, b)
	if a.h.IndexedPools() == 0 {
		t.Fatal("the pool was not indexed once kept")
	}
}
```
