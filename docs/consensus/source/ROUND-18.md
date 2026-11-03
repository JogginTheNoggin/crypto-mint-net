# Source Round 18: consensus review (Black, Part D)

## Summary

No change in this round adds or alters a validity, weight, selection or
byte-form rule, and no team proposed a consensus change. **This round's
changes are all clear.**

Answering Red's RED-048 question, Black found and reproduced one
hard-fork case **already on `main`**. It was not added this round. It is
**B7, blocked: hard fork**:
- A node with `check_seconds: -1` never asks its peers for their weight
  again after it connects to them.
- If its peers run `no_notices` and the pool is quiet, it stays on a
  lighter ledger for good. It does so while it is connected to a peer
  that holds the heavier one.

The fix is node policy only and needs no consensus change, so it goes to
Blue or Grey, not to the owner for approval.

Blue's question on carrying the pool index on the wire is **a protocol
change, not a consensus rule**. Black neither pre-approves nor filters
it. If it is built, it must follow D76: a new wire version or a new
message type, never a change to wire version 1. The conditions are below.

| Field | Value |
| --- | --- |
| Round | Source Round 18 |
| Consensus suggestions | None |
| Base reviewed | `main` @ `533dbcd` (PR #91: D150, D151) |
| Previous base | `main` @ `b17e892` |
| Blue branches | PR #95 @ `057badb` (BLUE-019, D152: RED-046 in part, RED-047, RED-048) |
| Red findings | PR #94 @ `e66a00f` (RED-046 to RED-048) |
| Proposals | GREEN-043 (PR #93 @ `3e8beea`); ORANGE-028 (PR #92 @ `cd4ab09`, measurements only) |
| Result | 8 clear, 0 with conditions, 1 blocked: hard fork (B7, on `main` since D59, reproduced) |

## Round 17's items

| Item | Now |
| --- | --- |
| Black's extreme-settings test | Adopted as `pkg/p2p/settings_extremes_test.go` (D151) |
| BLUE-018, GREEN-042, ORANGE-027 | Merged as cleared (D151) |

## B7: `check_seconds: -1` with peers that send no notices (blocked: hard fork)

**How a node learns of a heavier ledger.** It learns of one in four ways:
1. Its weight request to each peer when the peer connects
   (`discovery.go`).
2. The regular check (`checkLoop`, D59), which asks each priority peer
   every `check_seconds`.
3. A pushed entry or layer whose parent it lacks (D60).
4. A notice, which a peer sends after it switches to a ledger by a chase.
   Forwarding does not reach peers in that case.

**The failing case.**
1. b runs `check_seconds: -1` ("never"). It is connected to a and holds
   the same ledger.
2. a runs `no_notices`. a then takes a heavier ledger by a chase from c.
3. b is not asked about it, since it is already connected. Its check
   never runs, and a sends no notice.
4. The chased entries are not forwarded (`forward.go`: "data the node asked for
   is never forwarded").
5. In a quiet pool nothing new is pushed. b stays on the lighter ledger
   for good, connected to a peer that serves the heavier one.

That breaks D115 and the owner's rule that no node policy may ever make
a heavier ledger unacceptable. It matches the AGENTS.md meaning of
blocked: hard fork: "some setting could leave a node unable to take in
and select a heavier valid ledger for good".

**Reproduced.** `TestBlackNoCheckStillReachesLaterHeavier/a-no-notices`
(appendix) times out on `main` @ `533dbcd` (2 of 2) and on `057badb`
(1 of 1). Three controls pass:
- The same case with a sending notices.
- b checking every second (`TestBlackCheckFindsLaterHeavier`).
- One new entry on a's ledger after the switch
  (`TestBlackNoCheckCaughtByNextEntry`). This shows what clears it, and
  that it is not an acceptance fault.

**Why it is b's to fix.** b cannot control its peers' policy. A peer that
holds a heavier ledger may be honest and quiet, so b's own guarantee has
to come from b. `no_notices` on its own is fine, since a node that checks
hears of a heavier ledger within `check_seconds`.

**A node-policy shape that keeps the rule** (for Blue or Grey; no
consensus change):
- `check_seconds` may not turn the check off. A negative value, or one
  past a ceiling, is treated as the longest interval allowed, for example
  one hour. Load can note the change in `Lowered`, as `grind.layer_rate`
  does.
- Any finite interval satisfies D115. The ceiling only bounds how long
  "in the end" can take. It is not a depth or size limit.
- `docs/modules/nodepolicy.md` line 98 ("`check_seconds: -1` turns off
  regular checks") changes with it.
- `ping_seconds: -1` can stay, since a ping carries no weight.
- B7's test, with `-1` in b's policy, then passes. That is the
  acceptance test.

**Since when.** D59, which made the regular check a weight request with
`-1` meaning never. No change in this round makes it worse. RED-048 asked
the question that found it.

## Red's other question: lazy pool state (RED-046 point 1, D152)

**Confirmed: it cannot change which entries or ledgers a node accepts.**
- `Node.load` now finds stores by reading `pools/`. It keeps a store only
  where `Engine.Follows` says the pool is followed. That is the same set
  the old loop over `FollowedPools` opened, since names are checked as
  canonical: the id's `String()` and a plain decimal index.
- `Engine.ListedPools` feeds status, metrics, queries and grind targets
  only. Nothing that takes in or selects a ledger reads it.
- The p2p host still indexes every followed pool (`EachFollowedPool`).
  It still asks a new peer about each of them, and a pool's state is
  still made by its first item or request.
- `poolByName` parses the section text and requires the parsed section
  to equal the pool's own. It finds the same pool the old map did.
- No limit is added, no series parameter is capped and no follow is
  refused.

## Red's question on small limits (RED-048)

Yes. With `hash_limit`, `layer_limit`, `entry_limit` and `chunk_size` at
1, a node still finds and takes a heavier ledger. This holds whether the
heavier ledger is there at connection (`settings_extremes_test.go`, on
`main`) or arrives later from a third node with notices on (B7's
`a-notices` control). `check_seconds: -1` is B7 above.

## Blue's question: the pool index on the wire

- **Not a consensus rule.** A pool section's extra index would be checked
  against `PoolID`, and a mismatch would be a malformed message. It
  changes no rule of validity, weight or selection, and no byte form of
  the ledger (`encoding.md`). `pool_id` stays the pool's identity.
  Black's gate is for consensus suggestions, so it neither pre-approves
  nor filters this.
- **It is a wire protocol change, so D76 governs it.** Wire version 1 is
  frozen: the section's bytes and the request id derived from them may
  not change. It could only come as a new wire version offered in the
  hello, or as new message types.
- **Conditions if it is built:**
  1. A node keeps speaking wire version 1 to peers that offer only it.
     A peer serving a heavier ledger must never become unreachable over
     the version.
  2. The index must match `PoolID(series, index)`, or the message is
     malformed.
  3. A section without the index keeps its meaning.
- **Who decides.** It changes the frozen wire, and no series of 2^32
  pools exists yet, so whether to build it is the owner's call as a
  protocol feature. A team that wants it should put it to the owner
  through the coordinator, with these conditions. It does not go through
  Black's consensus gate.

## Clear

| Item | Why it is clear |
| --- | --- |
| D150 (BLUE-018), merged | Cleared in Round 17. Merged unchanged |
| D151: GUI-022, `Unlock` refuses a stale seal | The wallet only |
| D151: `-if-digest` | The CLI's import only |
| D151: Black's extreme-settings test adopted | A test |
| D152 (RED-046): lazy pool listing and start-up | Nothing a node accepts changes (see above) |
| D152 (RED-047, RED-048): `-if-digest` checks; every setting named under `-settings` | The CLI and node policy import only |
| GREEN-043: import plan wording, the digest only on a preview | Output only |
| ORANGE-028: imports and `Unlock` measured | Measurements only, no code change |

## Items for the owner

- **No consensus change is proposed this round.** Nothing needs your
  approval.
- **B7 is a hard-fork case on `main`. It is a node-policy bug, not a
  rule change.** A node set never to check its peers' weight can stay
  on a lighter ledger if its peers send no notices. The fix keeps the
  check from being turned off, which needs no approval. Black will clear
  B7 once a fix makes its test pass.
- **The pool index on the wire** is a protocol feature, not consensus.
  If a team wants it, it is yours to approve, with the conditions above.

How this was checked:
- Read the code changes from `b17e892` to `533dbcd` (PR #91), the diffs
  of PRs #92 to #95 against `main`, and decisions D150 to D152, D59, D60
  and D76.
- `go test ./pkg/engine ./pkg/node ./pkg/nodepolicy ./pkg/cli ./pkg/rpc
  ./pkg/watch ./pkg/grind ./pkg/daemon ./pkg/vectors ./pkg/p2p` passes on
  `3e8beea` and `057badb`.
- B7's test and its controls were run on `533dbcd` and `057badb`.

## Appendix: B7's test

Not in the repository. It uses `pkg/p2p`'s test helpers and
`startNodePol` from `settings_extremes_test.go`. `startNodePolKeep` is
`startNodePol` without forcing `check_seconds` and `ping_seconds` off.
`TestBlackNoCheckStillReachesLaterHeavier/a-no-notices` fails on `main`
and is the acceptance test for a fix.

```go
package p2p_test

import (
	"context"
	"path/filepath"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/engine"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/node"
	"testing"
	"time"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/nodepolicy"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/p2p"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/tracking"
)

// Black R18 (RED-048): b never asks for stated weights (check_seconds -1,
// ping -1) and uses the smallest reply limits. It is already connected
// to a when a switches, by a chase from c, to a heavier ledger b has never
// seen. b must still reach it.
func TestBlackNoCheckStillReachesLaterHeavier(t *testing.T) {
	zero := 0
	for _, quiet := range []bool{false, true} {
		name := "a-notices"
		if quiet {
			name = "a-no-notices"
		}
		t.Run(name, func(t *testing.T) {
			served := func() *tracking.Tracker {
				tr, err := tracking.New(tracking.Config{LoopbackAsOne: true, Budgets: map[string]int{tracking.KindRequest: 1 << 30}}, "")
				must(t, err)
				return tr
			}
			c := startNode(t, "c", func(cf *p2p.Config) { cf.PartsOver = 128; cf.Tracker = served() })
			a := startNode(t, "a", func(cf *p2p.Config) { cf.PartsOver = 128; cf.Tracker = served(); cf.NoNotices = quiet })
			b := startNodePol(t, "b", func(p *nodepolicy.Policy) {
				p.K, p.Ties, p.MaxPending, p.SeenSize = &zero, 0, 1, 1
				p.P2P.CheckSeconds, p.P2P.PingSeconds = -1, -1
				p.P2P.ChunkSize, p.P2P.EntryLimit, p.P2P.HashLimit, p.P2P.LayerLimit = 1, 1, 1, 1
			})
			last, owner := grow(t, c, nil, "alice", 3, 32)
			copyLedger(t, c, a)
			copyLedger(t, c, b)
			b.connect(a)
			time.Sleep(200 * time.Millisecond)
			growTagged(t, c, last, owner, "c", 4, 32)
			bareRun(t, c, 300)
			a.connect(c)
			eventuallyFor(t, 60*time.Second, "a to reach c's ledger", func() bool { return a.state() == c.state() }, a, c)
			eventuallyFor(t, 60*time.Second, "b to reach a's ledger", func() bool { return a.state() == b.state() }, a, b)
		})
	}
}

func startNodePolKeep(t *testing.T, name string, edit func(*nodepolicy.Policy)) *tnode {
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
	cfg.ManageEvery = -1
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


// The same with b checking every second: the check is what finds it.
func TestBlackCheckFindsLaterHeavier(t *testing.T) {
	served := func() *tracking.Tracker {
		tr, err := tracking.New(tracking.Config{LoopbackAsOne: true, Budgets: map[string]int{tracking.KindRequest: 1 << 30}}, "")
		must(t, err)
		return tr
	}
	c := startNode(t, "c", func(cf *p2p.Config) { cf.PartsOver = 128; cf.Tracker = served() })
	a := startNode(t, "a", func(cf *p2p.Config) { cf.PartsOver = 128; cf.Tracker = served(); cf.NoNotices = true })
	b := startNodePolKeep(t, "b", func(p *nodepolicy.Policy) {
		p.P2P.CheckSeconds, p.P2P.PingSeconds = 1, -1
		p.P2P.ChunkSize, p.P2P.EntryLimit, p.P2P.HashLimit, p.P2P.LayerLimit = 1, 1, 1, 1
	})
	last, owner := grow(t, c, nil, "alice", 3, 32)
	copyLedger(t, c, a)
	copyLedger(t, c, b)
	b.connect(a)
	time.Sleep(200 * time.Millisecond)
	growTagged(t, c, last, owner, "c", 4, 32)
	bareRun(t, c, 300)
	a.connect(c)
	eventuallyFor(t, 60*time.Second, "a to reach c's ledger", func() bool { return a.state() == c.state() }, a, c)
	eventuallyFor(t, 60*time.Second, "b to reach a's ledger", func() bool { return a.state() == b.state() }, a, b)
}

// With check -1, one new entry on a's ledger is enough: b chases it.
func TestBlackNoCheckCaughtByNextEntry(t *testing.T) {
	zero := 0
	served := func() *tracking.Tracker {
		tr, err := tracking.New(tracking.Config{LoopbackAsOne: true, Budgets: map[string]int{tracking.KindRequest: 1 << 30}}, "")
		must(t, err)
		return tr
	}
	c := startNode(t, "c", func(cf *p2p.Config) { cf.PartsOver = 128; cf.Tracker = served() })
	a := startNode(t, "a", func(cf *p2p.Config) { cf.PartsOver = 128; cf.Tracker = served(); cf.NoNotices = true })
	b := startNodePol(t, "b", func(p *nodepolicy.Policy) {
		p.K, p.Ties = &zero, 0
		p.P2P.CheckSeconds, p.P2P.PingSeconds = -1, -1
	})
	last, owner := grow(t, c, nil, "alice", 3, 32)
	copyLedger(t, c, a)
	copyLedger(t, c, b)
	b.connect(a)
	time.Sleep(200 * time.Millisecond)
	last, owner = growTagged(t, c, last, owner, "c", 4, 32)
	a.connect(c)
	eventuallyFor(t, 60*time.Second, "a to reach c's ledger", func() bool { return a.state() == c.state() }, a, c)
	time.Sleep(2 * time.Second)
	if a.state() == b.state() {
		t.Fatal("b reached it without a new entry")
	}
	growTagged(t, a, last, owner, "a", 1, 32)
	eventuallyFor(t, 60*time.Second, "b to reach a's ledger", func() bool { return a.state() == b.state() }, a, b)
}
```
