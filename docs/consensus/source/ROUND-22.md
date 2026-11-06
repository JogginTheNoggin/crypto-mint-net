# Source Round 22: consensus review (Black, Part D)

## Summary

No change in this round adds or alters a validity, weight, selection or
byte-form rule, and no team proposed a consensus change, so **nothing is
passed to the owner for approval**. **Nothing is blocked.**

The round makes more of the node's limits settable (D159, D160) and
changes which peer-book records give way when the book is full. The
coordinator asked Black to check above all that **no settable limit can
make a heavier ledger unacceptable**. None can:

- The peer book, tracker and discovery limits (D159, and
  `p2p.candidate_seconds` in D160) decide which peers a node knows and
  how long it remembers them. At worst a node knows fewer peers, which is
  a lack of information, not a hard fork under the owner's definition.
- `seen_size` and `max_pending` bound memory. A dropped item is checked
  again when it is sent again.
- `ties` was already a cap on the off-chain entries kept that tie with
  the selected weight. D160 gives it a ceiling (4096) and keeps its floor
  (1). A forgotten tie can be given again and is then taken when it is
  heavier; Black's test below shows this at the floor. A node that wants
  to keep every tie still can, with `k: -1`, which skips pruning
  altogether.

No consensus package changed. Between Round 21's base and this one, and
in each of this round's PRs, the Go packages touched are `cli`, `daemon`,
`engine` (only `seen.go`, the seen list's memory), `node` (a test),
`nodepolicy`, `p2p` (`policy.go`, which reads `registry_seconds`, and a
test), `peerport`, `peers`, `plain`, `rpc` and `tracking`. `chain`,
`entry`, `ledger`, `layer`, `weight`, `state` and `wire` are untouched.

| Field | Value |
| --- | --- |
| Round | Source Round 22 |
| Consensus suggestions | None |
| Base reviewed | `main` @ `b997eb0` (PR #130: D159, with #124 to #128 merged) |
| Previous base | `main` @ `8662766` |
| Blue branches | PR #138 @ `1ff3d2e` (BLUE-023, D160: RED-063, RED-064, RED-065; carries ORANGE-032) |
| Red | PR #134 @ `8570bdc` (RED-063 to RED-065; each says no consensus rule is touched) |
| Proposals | GREEN-047 (PR #133 @ `bcca1c2`); ORANGE-032 (PR #136 @ `2388f58`) |
| Result | 9 clear, 0 with conditions, 0 blocked |

## The settable limits against the heavier-ledger rule

| Limit | What it bounds | Can it refuse a heavier ledger? |
| --- | --- | --- |
| `p2p.max_book`, `book_per_range`, `verified_per_range` (D159) | How many peers the book holds | No. Fewer known peers is less information. The floors (256, 1, 1) keep a node able to find and keep peers |
| `p2p.retry_seconds`, `registry_seconds` (D159) | How often a failed peer is tried again and registries are read | No. Timing of whom a node talks to; the weight check is clamped by D153 and is not settable here |
| `p2p.candidate_seconds` (D160) | How long a learned, never-reached address keeps its place | No. Reached records and records the owner gave never give way |
| `tracking.*` (D159) | How long offences and logs are remembered | No. A tracker never bans a ledger, only notes peers |
| `seen_size` (D160: 1024 to 4,194,304) | The list of valid items already handled | No. Only valid items are listed, and one that drops out is checked again when sent again |
| `max_pending` (D160: 16 to 1,048,576) | Transactions not yet in an entry | No. The pending pool holds no entries and decides no weight |
| `ties` (D160: 1 to 4096) | Off-chain entries kept that tie with the selected weight | No. See below |

The chase queue, its pacing and the weight check stay fixed on purpose
(D159's "Left fixed"), which is the right line: those could slow or
starve the fetch of a heavier ledger.

### `ties`

`Pool.prune` keeps up to `Ties` off-chain leaves whose weight ties with
the selected ledger, earliest seen first, plus the newest entry. The rule
is unchanged; only the setting's range is new.

- Before D160 any value of 1 or more was allowed (0 meaning 64), so the
  floor of 1 is not new. The ceiling only removes values above 4096.
- A tie is by definition not heavier. It becomes heavier only when layers
  or a child arrive for it. If it was forgotten, the layer or child has
  an unknown parent and is chased (D60), or it is sent again; the entry
  is then held and counted like any other.
- `k: -1` skips pruning, so no tie is ever dropped.

Black's test (appendix): `k` 0 and `ties` at its floor, three zero-credit
entries. The second is forgotten. A layer on it is refused while it is
unknown; once the entry is given again, one layer makes it the selected
ledger. It passes 3 of 3 under `-race`.

Blue also changed `TestBlackBundleSettingsCannotRefuseHeavier` to run
with `ties`, `max_pending` and `seen_size` at their floors and at their
ceilings (it used 0, 1, 1 before). It still reaches the heavier ledger.
That matches Black's Round 17 test and keeps what it checked.

One note for operators, not a fork: a `node.json` with `ties`,
`seen_size` or `max_pending` above the new ceilings is now refused by
`Validate`, so such a node does not start until the value is lowered.
The node says which setting and its range. Nothing it already holds is
lost.

## The peer book's give-way order (RED-064, RED-065, D160)

When the book or a range is full, a new address takes the place of a
learned candidate the node has never reached: one whose last try failed
at once, an untried one after `candidate_seconds`. The owner's own add
takes any learned place. Records the owner gave or labelled, and records
the node reached, never give way. `Book.Fit` trims a book to a lowered
`max_book` at start. RED-065's fix decides what gives way before it
deletes, so a refused add changes nothing.

This is node policy. A flood of addresses can at most replace candidates
that have never answered; the peers a node has actually reached stay. It
decides whom a node knows, never which ledger it takes.

## Clear

| Item | Why it is clear |
| --- | --- |
| D159: settable book, tracker and discovery limits | Which peers a node knows and how long it remembers them (see the table) |
| D159: `registry_seconds` read by `p2p.FromPolicy` | Registry timing only |
| BLUE-023 (RED-063): bounds on `seen_size`, `max_pending`, `ties`; the seen list grows with its items | Memory only; the seen list keeps the same items and the same drop order (see above) |
| BLUE-023 (RED-064): learned candidates give way; `candidate_seconds`; `Book.Fit` | Peer-book policy. Reached and owner-given records stay |
| BLUE-023 (RED-065): a refused add changes nothing | Peer-book bookkeeping |
| ORANGE-032: per-range counts and the imported set kept up to date; the tracker's log trimmed without copying | Speed only. `TestBookCountsKept` checks the counts against a scan |
| GREEN-047 part 1: `poon policy limits` | Read-only listing from the same list `Validate` checks |
| GREEN-047 part 2: the survey of other limits | A list, nothing built. See below |
| Red (PR #134): RED-063 to RED-065 | Findings only; each is node policy, and each is fixed in PR #138 |

## GREEN-047 part 2: Black's view of the survey

Nothing here is built, and each would come back to Black when it is.

- **Could be settable:** `partsCacheBytes`, `ownListLimit`,
  `ownPeerListLimit`, `CompactMin` and `watch.MaxLimit` are local cost or
  what this node asks for. A small parts cache means parts are encoded
  again for each request, which is slower serving, not refusing. Any of
  them may be made settable as node policy.
- **Should stay fixed:** Black agrees with Green's list. In particular
  the chase queue and pacing, the wire and file size caps, the `policy`
  sizes (series validation), `engine.ListAll` and `MaxCountedCost`
  (how weight is counted). Making a size cap or weight-count limit
  settable would let nodes with the same information disagree on what is
  valid or how heavy it is, which is a hard fork. `MaxKeptLayers` and
  `KeptPerSecond` concern being flagged by peers, not acceptance.

## Items for the owner

- **No consensus change is proposed this round.** Nothing needs your
  approval.
- **No hard-fork case is open.** Every limit made settable this round, at
  its floor or its ceiling, only changes what a node knows or how much
  memory it spends. A heavier ledger a node is given is still checked and
  taken.

How this was checked:
- Read the code changes from `8662766` to `b997eb0`, the diffs of PRs
  #133, #134, #136 and #138 against `main`, decisions D159 and D160, and
  `Pool.prune` in `pkg/ledger/pool.go`.
- `go test ./...` passes on `b997eb0`, `bcca1c2`, `2388f58` and
  `1ff3d2e`.
- Black's ties test below passes on `1ff3d2e`.

## Appendix: the ties test

Put in `pkg/ledger` beside `pool_test.go` (it uses that file's fixture).

```go
package ledger_test

import (
	"testing"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/ledger"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/nodepolicy"
)

// Black, Round 22: ties at its floor with k at 0. Three zero-credit entries
// tie with the empty ledger; only one tie and the newest are kept, so one is
// forgotten. Given again, with its layers, it is taken as the heavier ledger.
func TestBlackTiesFloorForgottenTieTakenWhenHeavier(t *testing.T) {
	f := newFixture(t, ledger.Config{K: 0, Ties: nodepolicy.BoundTies.Min})
	a := f.build(f.pool.ID(), f.k.Claim(4, P("alice", 100)))
	b := f.build(f.pool.ID(), f.k.Claim(4, P("bob", 100)))
	c := f.build(f.pool.ID(), f.k.Claim(4, P("carol", 100)))
	f.add(a)
	f.add(b)
	f.add(c)
	ha, hb, hc := a.Hash(f.s), b.Hash(f.s), c.Hash(f.s)
	if !f.pool.Has(ha) || !f.pool.Has(hc) || f.pool.Has(hb) {
		t.Fatalf("want a (first tie) and c (newest) kept, b forgotten: a %v b %v c %v",
			f.pool.Has(ha), f.pool.Has(hb), f.pool.Has(hc))
	}
	if _, err := f.pool.AddLayer(layerOf(t, f.s, hb, 3, 1<<40)); err == nil {
		t.Fatal("a layer on a forgotten entry should wait for the entry")
	}
	// b is given again (a chase or a resend) and its layers make it heavier.
	f.add(b)
	if sw := f.layers(hb, 3); sw == nil || sw.To != hb {
		t.Fatal("the forgotten tie, given again with a layer, must be selected")
	}
	if tip, _, _ := f.pool.Tip(); tip != hb {
		t.Fatal("tip is not b")
	}
}
```
