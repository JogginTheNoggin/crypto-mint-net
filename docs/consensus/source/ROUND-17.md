# Source Round 17: consensus review (Black, Part D)

## Summary

No change in this round adds or alters a validity, weight, selection or
byte-form rule, and no team proposed a consensus change, so **nothing is
passed to the owner for approval**. **Nothing is blocked.**

The round is node policy export and import (D149) and Blue's narrowing
of it (D150), plus the wallet, the CLI and file writes. A bundle holds
only node policy. It is checked the way node.json is, and node.json has
no depth limit and no rule a ledger is checked against.

Red asked Black to confirm that `k`, `entry_limit`, `hash_limit`,
`layer_limit` and `max_pending`, arriving through a bundle, cannot make
a heavier ledger unacceptable. **Confirmed, by reading and by test.** A
node with those settings at both extremes still reaches a heavier
ledger (see "Red's question" and the appendix).

| Field | Value |
| --- | --- |
| Round | Source Round 17 |
| Consensus suggestions | None |
| Base reviewed | `main` @ `b17e892` (PR #80: D148, D149) |
| Previous base | `main` @ `b502960` |
| Blue branches | PR #85 @ `fe95082` (BLUE-018, D150: RED-043, RED-044, RED-045) |
| Red findings | PR #83 @ `5740f7f` (RED-043 to RED-045) |
| Proposals | GREEN-042 (PR #84 @ `eeb7247`); ORANGE-027 (PR #82 @ `98abc08`, measurements only) |
| Result | 9 clear, 0 with conditions, 0 blocked |

## Round 16's items

| Item | Now |
| --- | --- |
| BLUE-017, GREEN-041, ORANGE-026 | Merged as cleared (D149) |
| The entry body has no size bound | Still true. Nothing this round touches `POST /v1/entry` |

## Red's question: node settings through a bundle (RED-043)

Under D150 these come in only with `-settings` or `-full`. Under D149
they came in by default. Either way they are the node's own policy, the
same values an operator can already type into node.json. None is read by
any validity check.

| Setting | What it does | Why it cannot refuse a heavier ledger |
| --- | --- | --- |
| `k` (and `ties`) | How many inactive branches the pool keeps (`ledger.Pool.prune`) | Pruning only forgets entries. A pruned ledger is fetched again through a chase when a peer states it is heavier (audit, "pruning to the top K stays allowed"; D12, D74, D79). The newest entry is kept while it is a leaf, so a branch being fetched is not cut from under the chase. Validation refuses `k` below -1 |
| `entry_limit`, `hash_limit`, `layer_limit`, `chunk_size` | How many items the node asks for per reply (`wire.Limits`); 0 means the default | They change only page size, so the number of requests. The server clamps to its own limits. The chase finds the common entry at any depth by halving (D116), and large entries come in parts. `TestHeavierLedgerAlwaysReached` already runs all four at 1 |
| `max_pending` (and `seen_size`) | The pending-transaction pool and the seen list | Neither is consulted when an entry is checked. The seen list drops only repeated pushes, and requested data gets past it (D15, D27) |

Black's test (appendix) loads node.json with `k` 0, `ties` 0,
`max_pending` 1, `seen_size` 1 and all four limits at 1, then again with
the limits at 2^32-1. That is the shape a hostile `-settings` bundle could
give. In both, b reaches a's heavier ledger: a fork 8 entries deep on
b's side, entries in parts, and a 600-entry run without layers. It
passes 5 times out of 5 under `-race`, on `fe95082`, and on `eeb7247`.

The other settings a `-settings` bundle carries (peers, slots,
`no_confirm`, `check_seconds`, the punishment switches) steer which
peers a node talks to. That is the eclipse question, which D96 and Red
own. It is not a validity rule, and D150 now lists those lines first as
`defences`. No setting in node.json, imported or typed, adds a depth
limit. `depth_limit` and `fast_catch_up` are ignored on load and never
exported.

## Clear

| Item | Why it is clear |
| --- | --- |
| D148 (BLUE-017), merged | Cleared in Round 16. Merged unchanged |
| D149: bundles, export and import | Node policy only, checked as node.json is, and `rpc` is never taken by a running node. Series identity is the hash of its parameters, so an import cannot change a known series, only add one |
| D149: GUI-022, a stale seal refused | The wallet only |
| D150 (RED-043): a default import takes only `Followed` | Fewer settings come in from a file than before. Which series a node follows and accepts is its owner's choice, not a rule within a pool |
| D150 (RED-043): the registry fetch refuses non-public addresses | Peer discovery only, as D96 already does for book candidates. Configured peers are always dialled, and `allow_local_peers` lifts it |
| D150 (RED-044): the follow merge in one pass | The same result as `AddFollow` one by one |
| D150 (RED-045): all-or-nothing import, the import body cap, new-name checks | CLI and local RPC only. The cap is on the policy import route, not the entry route |
| GREEN-042: `grind -series` help; import plans show values | Help text and plan output |
| ORANGE-027: the modular seams measured | Measurements only, no code change |

## Items for the owner

- **No consensus change is proposed this round.** Nothing needs your
  approval.
- **No hard-fork case is open.** No node setting, imported or typed,
  can make a node refuse a heavier ledger. Black's test checks the
  settings Red named at their extremes.

How this was checked:
- Read the code changes from `b502960` to `b17e892` (PR #80), the diffs
  of PRs #82 to #85 against `main`, and decisions D148 to D150.
- `go test ./pkg/nodepolicy ./pkg/cli ./pkg/rpc ./pkg/registry
  ./pkg/vectors ./pkg/p2p` passes on `eeb7247` and `fe95082`, with
  Black's test added.

## Appendix: Black's test

Not in the repository. Grey may take it into `pkg/p2p` if wanted. It
uses the helpers of `pkg/p2p`'s tests (`startNode`, `grow`,
`growTagged`, `copyLedger`, `bareRun`, `eventuallyFor`).

```go
package p2p_test

import (
	"context"
	"math"
	"path/filepath"
	"testing"
	"time"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/engine"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/node"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/nodepolicy"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/p2p"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/tracking"
)

// startNodePol is startNode with b's node.json edited first, as a bundle
// imported with -settings or -full would set it.
func startNodePol(t *testing.T, name string, edit func(*nodepolicy.Policy)) *tnode {
	t.Helper()
	s := exSeries
	id := s.ID()
	pol := nodepolicy.Default()
	pol.Series = []nodepolicy.SeriesInfo{{Params: s.Params()}}
	pol.Follow = []nodepolicy.Follow{{Series: &id, Pools: []uint64{1}}}
	pol.RPC.Listen = "off"
	pol.NoSync = true
	edit(&pol)
	dir := t.TempDir()
	must(t, nodepolicy.Save(filepath.Join(dir, nodepolicy.FileName), pol))
	n, err := node.Open(dir, node.Options{})
	must(t, err)
	tn := &tnode{t: t, name: name, n: n, done: make(chan struct{}), log: &syncBuf{}, key: engine.PoolKey{Series: id, Index: 1}}
	addr := freeAddr(t)
	cfg := p2p.FromPolicy(pol)
	cfg.Listen, cfg.Advertise, cfg.AllowLocalPeers = addr, addr, true
	cfg.CheckEvery, cfg.ManageEvery, cfg.PingEvery = -1, -1, -1
	cfg.RequestTimeout, cfg.FollowTimeout, cfg.Log = 5*time.Second, 500*time.Millisecond, tn.log
	cfg.ChaseSlice = 20 * time.Millisecond
	cfg.Chase.YieldWait = time.Millisecond
	cfg.RequestPace = -1
	tr, err := tracking.New(tracking.Config{LoopbackAsOne: true, Budgets: map[string]int{tracking.KindRevert: 1}}, "")
	must(t, err)
	cfg.Tracker = tr
	h, err := p2p.New(n, cfg)
	must(t, err)
	tn.h = h
	ctx, cancel := context.WithCancel(context.Background())
	tn.cancel = cancel
	ready := make(chan struct{})
	go func() {
		defer close(tn.done)
		if err := h.Run(ctx, func() { close(ready) }); err != nil {
			t.Error(err)
			close(ready)
		}
	}()
	<-ready
	t.Cleanup(func() {
		tn.stop()
		n.Close()
		if t.Failed() {
			t.Logf("%s log:\n%s", name, tn.log)
		}
	})
	return tn
}

// Black R17 (RED-043): every node setting a -settings bundle can carry,
// at its extremes, still lets b reach a's heavier ledger.
func TestBlackBundleSettingsCannotRefuseHeavier(t *testing.T) {
	zero := 0
	for _, c := range []struct {
		name string
		edit func(*nodepolicy.Policy)
	}{
		{"k0-tightest", func(p *nodepolicy.Policy) {
			p.K, p.Ties, p.MaxPending, p.SeenSize = &zero, 0, 1, 1
			p.P2P.ChunkSize, p.P2P.EntryLimit, p.P2P.HashLimit, p.P2P.LayerLimit = 1, 1, 1, 1
		}},
		{"k0-largest", func(p *nodepolicy.Policy) {
			p.K, p.Ties, p.MaxPending, p.SeenSize = &zero, 0, 1, 1
			p.P2P.ChunkSize, p.P2P.EntryLimit, p.P2P.HashLimit, p.P2P.LayerLimit = math.MaxUint32, math.MaxUint32, math.MaxUint32, math.MaxUint32
		}},
	} {
		t.Run(c.name, func(t *testing.T) {
			served, err := tracking.New(tracking.Config{LoopbackAsOne: true, Budgets: map[string]int{tracking.KindRequest: 1 << 30}}, "")
			must(t, err)
			a := startNode(t, "a", func(c *p2p.Config) { c.PartsOver = 128; c.Tracker = served })
			b := startNodePol(t, "b", c.edit)
			last, owner := grow(t, a, nil, "alice", 3, 32)
			copyLedger(t, a, b)
			growTagged(t, b, last, owner, "b", 8, 8)
			growTagged(t, a, last, owner, "a", 4, 32)
			bareRun(t, a, 600)
			if a.state() == b.state() {
				t.Fatal("setup: same ledger")
			}
			b.connect(a)
			eventuallyFor(t, 60*time.Second, "b to reach a's ledger", func() bool { return a.state() == b.state() }, a, b)
		})
	}
}
```
