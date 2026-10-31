# Source Round 7: consensus review (Black, Part D)

## Summary

Black's first review. No change in this round adds or alters a
validity, weight, selection or byte-form rule, and no team proposed a
consensus change, so **nothing is passed to the owner for a consensus
decision**. The removal of the size caps (C1, C2) and of the depth and
bare-run limits (G1 to G3) matches the owner's decisions.

One hard-fork case remains, and it was reproduced with a test.
**B1** is a node that already holds a heavier branch's entries but not
the layers that make that branch heavier. Such a node never gets those
layers through a chase, so it stays on the lighter ledger. The case is
on Grey's base and on BLUE-008 alike, and it is what makes Orange's
warning about dropped layers a real risk. It must be fixed in Round 8
Part A before anything that leans on the chase as the backstop.

Three more spots can, by reading, keep a heavier ledger out of reach in
narrow cases. None was reproduced. Each becomes a condition and a check
for Red: C-1 (the work margin), C-2 (losing a long chase's progress) and
C-3 (more than about two million layers on one entry).

Everything else is clear: Green's GREEN-032, Orange's ORANGE-014 to 016,
the layer floor (D118, with BLUE-008's rate) and the parts transfer.

| Field | Value |
| --- | --- |
| Round | Source Round 7 |
| Consensus suggestions | None this round. P5 (G4) was built as node policy; see Items for the owner |
| Base reviewed | `claude/m10-watch` @ `0fd59fa` (code `e99e3e4`; D115 to D119) |
| Previous base | `claude/m10-watch` @ `9972137` |
| Blue branches | `claude/project-thread-ggo8dt` @ `79386c1` (BLUE-008, D120, code `58e4e42`) |
| Proposals | GREEN-032 (`claude/source-green` @ `a384076`); ORANGE-014, 015, 016 (`claude/source-orange-r7` @ `ddf1dd1`) |
| Result | 13 clear, 4 clear with conditions, 1 blocked: hard fork (B1) |

## Blocked

### B1. A held branch without its layers is never topped up (blocked: hard fork)

- **Where.** `pkg/p2p/chase.go`: `findCommon`, and `runChase` calling
  `topUpLayers` when `complete && len(after) == 0`. The code is from D116
  on `0fd59fa` and is unchanged by BLUE-008.
- **What happens.**
  1. The sender's newest hashes are all held, because the node already
     has the branch's entries off its selected ledger.
  2. The chase takes this as "both hold the same ledger".
  3. `topUpLayers` then asks for layers on the last `EntryLimit` entries
     of the node's own selected ledger, which is the wrong branch. The
     sender answers that it does not hold them.
  4. The chase ends with "not heavier once checked", and the sender is
     backed off.
  5. Every retry takes the same path, so the node never learns the
     layers that make the held branch heavier.
- **How a node gets there.** The branch's entries arrive by push while
  its layers are dropped. Orange measured one way this happens: the
  per-peer layer budget starves a steady heavy sender, because dropped
  messages are counted (ORANGE-016, "For Red and Grey"). An earlier fetch
  followed by layers added later on that branch works the same way. It
  also covers new layers on old entries of a branch the node already
  fetched.
- **Reproduced.** Test `TestBlackHeldBranchWithoutLayers` (Appendix):
  - Setup: b holds its own ledger at weight 240, plus every entry of a's
    ledger, which weighs 368, without the layers.
  - b connects to a. After 30 s b is still at 240, and its log shows
    `chase ... result=nowhere why="not heavier once checked"`.
  - The test fails the same way on `0fd59fa` and on `79386c1`.
- **The rule it breaks.** D115 and the owner's rule. A reachable peer
  serves a valid, heavier ledger, and the node never selects it.
- **Fix shape (node policy, no consensus change).**
  - When the sender's tip is held but is not this node's tip, treat the
    held branch as the branch.
  - Find where it leaves the selected ledger, by walking back through
    entries held locally, which costs no requests.
  - Run step 4 on the sender's layers for those hashes, paged with no cap
    as `checkWork` already does, and take the layers in.
  - `topUpLayers` should ask for the sender's entries, not this node's.
  - Add the test to the P0 guard (`heavier_test.go`).
- **Separately, for Grey and Red.** Orange's point that dropped messages
  count against the layer budget should be fixed too. Count only what
  was taken, so a steady honest sender gets through at the budget rate.
  That is node policy. With B1 fixed, the chase is a working backstop,
  and the budget only delays.

## Clear with conditions

### C-1. D116, the recheck before each fetch can end a heavier chase (not reproduced)

- **What it does.** `checkWork` stops counting the sender's work as soon
  as the total beats the node's own weight after the common entry. The
  margin is at most one page of layers. `fetchBranch` then ends the
  chase as "a ledger heavier than the branch was selected meanwhile" if
  the node's own weight after that entry has passed this partial total.
- **The risk.** The node's own ledger only has to grow by that margin,
  which can be one layer, while it fetches. The branch may still be much
  heavier, since the rest of its work was never counted. The chase ends
  Nowhere, the sender is backed off, and the next try counts to a new
  bare margin. A node on a busy minority side that fetches a long branch
  could repeat this without end.
- **Not reproduced.** A 63-entry fetch on loopback finished before the
  node's own growth caught up. Longer branches over real links were not
  tried.
- **Condition for Round 8 Part A.** When the node's weight passes the
  partial total, go back to step 4 and count more of the sender's
  listing from the saved progress, rather than ending. End as "not
  heavier" only when the whole listing has been counted and still does
  not beat it. Red should test this in Round 8 Part B: a long branch, and
  the node's own side adding layers throughout.

### C-2. D116 and D120, progress for at most 256 yielded chases (not reproduced)

- **What it does.** `maxProgress` keeps the progress of the 256 most
  recent yielded chases and forgets the oldest past that. Forgotten
  progress starts again from the beginning.
- **The risk.** A long weightless run is paced at `WeightlessPages`
  (64) pages per slice. A large entry's parts and a long fetch also span
  many slices. If 256 other chases yield between two slices of an honest
  chase, it restarts each time and never finishes. Chase slots
  (`Chases` 4, `PerPool` 2, `Unlisted` 1) make this hard, but nothing
  rules it out.
- **Condition.** Forgetting must prefer chases with less shown work, and
  a chase whose work check passed (`worked`) must never be forgotten for
  one that has not. Red should check whether unlisted senders can fill
  the 256 places.

### C-3. D117, a layers reply holds at most about two million layers per entry (by reading)

- **What it does.** `replier.layers` sends only the strongest `maxNonces`
  layers (about 2.1 million) of an entry. Step 4 counts only those, and
  `LayersReq` has no way to ask for the rest.
- **The risk.** Suppose the branch is heavier only when all of an
  entry's layers are counted, for example 3 million minimum-strength
  layers on one entry. The work check then never passes, so the
  extension that would carry every layer is never asked for. This is
  unlikely today. With a layer floor and many grinders it is not
  impossible, and the owner's rule allows no size limit that every peer
  would hit.
- **Condition.** Let the work check fetch the rest of a truncated entry's
  layers, through parts or an offset in the layers request (a new wire
  type, as D76 allows). Add an entry with more than `maxNonces` layers to
  the P0 test.

### C-4. ORANGE-014, checking a set of witnesses at once

- **Clear on these conditions.**
  - The cheap checks (size, address, allowed type and algorithm) run on
    every witness every time, as the proposal says.
  - The result must equal the in-order loop: the same pass, or the same
    first failing index and error (`TestCheckAllMatchesInOrder`).
  - The set key covers `tid` and every witness's encoding. `tid`
    commits to the series, and so to its hash function. If a
    satisfaction function ever reads something `tid` does not bind, it
    must be added to the key.
- **Why.** On those conditions, what is accepted is unchanged and only
  the order and speed of signature checks change.

## Clear

| Item | Why it is clear |
| --- | --- |
| D115 (process) | It records the owner's rule and the Black gate |
| D117: no `MaxTxs`, `MaxInputs`, `MaxOutputs` or witness cap | Matches the owner's C1 and C2 decisions. `Reader.Count` keeps allocation in proportion to the bytes received. A count is still a 32-bit field of the frozen v1 byte form, which is a class (b) bound. Info: `int(r.U32())` would turn negative on a 32-bit build. No supported platform is 32-bit (`scripts/release.sh`), but a guard is cheap |
| D117: wire types 26, 50 and 51; store `KindPart` | New wire types are allowed by D76, and the store format is local. The entry and transaction byte forms are unchanged, and `pkg/vectors` passes |
| D117: an entry is put together in memory | Not a coded limit. It is noted for Round 8. Keep it on the list until an entry can be checked as it streams |
| D116: halving, no `DepthLimit`, no `MaxBareRun`, yield with `YieldWait` | Matches G2 and G3. Every slice makes one request, and a yield has no backoff |
| D116: `tracking.Spend` lets one large spend through in an empty window, and checked chases are not charged | Matches G1/P2. A budget now only delays |
| D118 with BLUE-008's RED-018 and ORANGE-016's `layer_rate` | They decide only which of its own layers a node sends, the strongest first. Nothing counted is dropped, so the D118 condition holds. What peers accept is unchanged |
| D119: BLUE-007's kept view, ORANGE-012 lanes, ORANGE-013, GREEN-020, GREEN-031 | The kept view is dropped on every switch, prune or change, and the audit checked it. The lane grinders' layers are checked like any other. The rest is the CLI and scheduling |
| BLUE-008: RED-015 (a chase keeps only hashes that carried work) | Unworked hashes are let go, but step 5 checks each entry by linking to its parent, and marked hashes by index. Weight is recomputed from the layers held. The weightless-page pacing yields and never ends a chase. This is subject to C-2 |
| BLUE-008: RED-016 (a node holding nothing first asks for work; parts on disk) | Any heavier ledger has some work, against the empty node's none. Parts are checked to start at the expected parent, stay within the stated size, and be non-empty, and no size is refused |
| BLUE-008: RED-017 and RED-019 | How an item is encoded, and lease renewal. Neither touches selection |
| GREEN-032 | Node policy saving and validation, `-layer-floor`, and `-h` exiting 0 |
| ORANGE-015 (`Engine.Tip`) | It reads the same weight without listing branches |

## Items for the owner

- **No consensus change is proposed this round.** Nothing needs your
  approval.
- **For information: G4/P5 was built (D116) while it was still waiting
  for your word.** It is node policy and makes the known-bad list
  narrower than before, so it is within your rules.
  - **What it does now.** A hash goes on the list only when that hash
    itself is shown false. Senders that send out-of-order or weak data
    are flagged instead.
  - **One gap remains.** When a sender lists hash W at some position and
    then sends a different entry there, W is listed. W can be a valid
    entry's hash that the sender copied, which is the G4 problem in a
    smaller form. It only delays, since chases still work and the entry
    expires in 24 hours.
  - **Black's suggestion for Round 8 Part A.** List nothing in that case
    and only flag the sender, so a valid entry is never listed.
- **The fix for B1 is required before Round 8 Part B.** It changes no
  rule.

## Appendix: the B1 reproduction

Run from `pkg/p2p` with the package's test helpers. It fails today and
should pass once B1 is fixed.

```go
// b holds every entry of a's heavier branch but none of its layers.
func TestBlackHeldBranchWithoutLayers(t *testing.T) {
	a := startNode(t, "a", nil)
	b := startNode(t, "b", nil)
	last, owner := grow(t, a, nil, "alice", 3, 32)
	copyLedger(t, a, b)
	growTagged(t, b, last, owner, "b", 2, 8)
	growTagged(t, a, last, owner, "a", 4, 64)
	var hs []hash.Hash
	a.n.Engine().Ledger(exKey, func(l *ledger.Pool) { hs = l.Chain(4, l.Height()) })
	for _, h := range hs {
		a.n.Engine().Ledger(exKey, func(l *ledger.Pool) {
			e, _ := l.Entry(h)
			_, err := b.n.HandleEntry(exKey, e, engine.Pushed)
			must(t, err)
		})
	}
	if a.state() == b.state() {
		t.Fatal("setup: b already on a's ledger")
	}
	b.connect(a)
	eventuallyFor(t, 30*time.Second, "b to reach a's ledger", func() bool { return a.state() == b.state() }, a, b)
}
```

How this was checked:
- Every change to the consensus packages between `9972137` and
  `0fd59fa` was read line by line, and so was BLUE-008's diff.
- The new chase in `pkg/p2p/chase.go` and `serve.go` was read whole.
- Decisions D114 to D120 and the three proposals were read.
- The B1 test was run on both commits. A test for C-1 was tried and did
  not reproduce it.
