# Source Round 29: consensus review (Black, Part D)

## Summary

This round takes the second step of disk-backed state. Items and undo
records of selected entries more than `recent_window` below the tip leave
memory (D175). Undo is recomputed from the tids index, and a switch reads
back everything it reverts. The round also includes:
- Black's Round 28 condition, met by D174 and widened by D177 (RED-085);
- bodies served outside the engine's lock (D174);
- spends now costing three view units (RED-086).

**Result: 12 clear, 0 with conditions, 0 blocked.**

- **No consensus rule or depth limit is added.**
  - No rule, byte form or wire message changes.
  - `recent_window` decides only what stays in memory.
  - The view cost decides only when a peer's entry is checked, never
    whether it is valid.
- **The window doesn't change what a node selects.** Black ran the
  random-history differential at windows of 1, 2, 3, 5 and 16, with caches
  of 0 bytes, 1 KiB and 1 GiB. That was 150 seeds × 300 steps, with and
  without pruning, and 691 reorganisations through cold entries.
  - The twin selected the same tip, height and weight as the in-memory
    pool after every step.
  - It held the same entries and reached the same state.
  - That state is reached through reverts with recomputed undo, so the
    recomputed undo matches the original.
- **A read failure can't make a node accept a different ledger.** Black
  modelled a node that stops on every `ReadFault` and restarts from its
  store. A third of all body reads failed at random, and there were 9,125
  faults at a window of 1. After each restart the node selected exactly
  what the in-memory pool did.
- **A deep switch is never refused.** It reads the disk and is slower.
- **Round 28's condition is met.** A panic while the engine changes a pool
  now fails the engine, and the node stops without rewriting its stores.
- **Fixed series, transactions and entries are unchanged.** The Round 24
  and Round 25 cross-version tests are byte for byte the same on `main`
  and on PR #213.

| Field | Value |
| --- | --- |
| Round | Source Round 29 |
| Consensus | None (D174 to D177) |
| Other consensus suggestions | None |
| Base reviewed | `main` @ `d1a9128` (PR #207: D174 to D176, with Round 28's PRs merged) |
| Previous base | `main` @ `705639a` |
| Blue branches | PR #213 @ `ff4c3e4` (BLUE-030, D177: RED-085, RED-086) |
| Red | PR #210 @ `b798247` (RED-085, RED-086) |
| Proposals | ORANGE-039 (PR #212 @ `030bb56`); GREEN-054 (PR #209 @ `c8ca330`) |
| Result | 12 clear, 0 with conditions, 0 blocked |

## Cold entries (D175)

### What a cold entry keeps, and what reads it back

A cold entry keeps its skeleton: hash, parent, height, weights, layers,
its tids in the index, and the counts of its transactions and makers.

- **What drops out of memory.** Only its items and undo record.
- **Which entries go cold.** Only selected-chain entries. Off-chain
  entries keep their items, so a view of a branch applies the branch from
  memory.
- **Where cold entries are read back.** Only `viewAfter` (on an overlay)
  and `switchTo` (through `warm`). Both bring back items and undo before
  they use them.
- **What `viewCost` reads.** Only the skeleton's counts.

### The recomputed undo is the record `Apply` made

`undoOf` rebuilds `state.Undo`. That record is only `Spent`: for each
input, in item and input order, the portion it spent.

- **Every portion has one maker.** Each portion is an output of the
  transaction that made it. That transaction is on the selected ledger
  below the entry, because an entry never spends a bundle-mate. It is
  found through the tids index.
- **The index can't name the wrong maker.** It is read while the entry is
  still selected (`warm` runs before the revert), and a tid appears at
  most once on one ledger.
- **The order matches `Apply`.** It is the order in which `ApplyTx`
  appends, so `Revert` reads the same slices.
- **The tests agree.** Black's runs above show it: reverts through cold
  entries reach the same state as the in-memory pool at every step.

### A deep switch reads and is slower, never refused

- **No limit.** Nothing in `warm`, `viewAfter` or `switchTo` limits depth.
- **The view cost paces.** RED-086 makes it count each maker that a cold
  revert reads, and that cost goes to the sender's revert budget. As D116
  says, that budget yields and resumes a chase: a pushed entry over budget
  is offered to the chase queue, and the entry still comes.
- **Pacing only.** Counting three units per spend, not two, paces sooner.
  It refuses nothing.

## A read failure stops the node (D174, D175, D177)

A failed read that the pool needs (`ReadFault`) panics. Black checked each
path where that can happen.

- **Only one path changes a pool.** `AddEntry` and `AddLayer` are called
  only from `Engine.HandleEntry` and `HandleLayer`, through `handling`.
  `cool` runs inside `AddEntry`.
- **A panic there fails the engine.** From then on the engine takes
  nothing more, and the daemon stops the node.
- **The store stays as it was.** `Close` and `Compact` don't rewrite the
  store, and `HandleEntryFrom` and `HandleLayerFrom` write a record only
  after the engine took it.
- **The read paths.** `PreverifyTx`, `PreverifyEntry`, `PreverifyPage`
  and `BuildEntryOn` reach views under `Engine.under` (D177), which fails
  the engine the same way.
- **The other locked paths only read.** `EntryCost`, `Ledger` callbacks
  and the watch hub read only the skeleton, or return `ErrUnreadable`
  without panicking.

**One correction to D175's wording, for Grey.** D175 says a fault comes
"before the pool changes". That is true of the switch itself: `warm` reads
everything before anything is reverted. But `AddLayer` has already counted
the layer and its weight, and `AddEntry` has already inserted the entry,
when the switch is reached. Black's fault runs show this directly: retrying
the same layer in the same pool gave `ErrKnownLayer`.

So the safety rests on D174, not on there being no change. The engine
fails, the node stops, and the restart replays the store, which never got
the record. Black's restart model above checks exactly that. Selection
was identical after every one of the 9,125 faults. The decision text
should say "the engine fails and the node rebuilds", and not suggest the
pool is untouched.

### After a restart

- **The same records.** A restart replays the store's records in their
  order through `AddEntry` and `AddLayer`. The node rebuilds what any node
  with those records rebuilds.
- **Missing records.** A record lost to the fault, or cut at a damaged
  frame, is missing information. Peers send it again.
- **Different information, not a different verdict.** Nothing on the
  restart path depends on the window, the cache, or what was cold.

## Bodies served outside the lock (D174)

A reply takes `BodyRef`s and layer nonces under the lock, and reads the
bodies after it is released. A reply holds the same entries as before.
Only the time the bodies are read changes.

## Clear

| Item | Why it is clear |
| --- | --- |
| D174: a panic mid-change fails the engine and stops the node | Black's Round 28 condition, met. The restart rebuilds from the stores |
| D174: bodies served outside the engine's lock | The same reply. Only when bodies are read changes |
| D175: items and undo outside `recent_window`; undo recomputed from the tids index | Selection and state identical at windows of 1 to 16 and any cache (Black's runs). The recomputed undo is `Apply`'s record |
| D175: a switch reads back everything it reverts; a view reads on an overlay | No depth limit. Deeper only means slower |
| D175: `ReadFault` stops the node | Missing information until the restart. The same selection after it (Black's fault and restart runs). Wording note above |
| D175: `recent_window` (16 to 2^30, on reload); `bodies.cold` | Memory and status |
| D176: replace imports honour `series_ceiling` | Which series a node adds is information |
| D177 (RED-085): read paths fail the engine through `under` | No lock left held. Stopping decides nothing |
| D177 (RED-086): view cost counts makers (3 units a spend) | Pacing through the revert budget (D111, D116). Nothing is refused |
| D177: `poo status -counts` | Tooling |
| ORANGE-039: the pending pool kept in insertion order; measurements | Which pending transactions this node builds with, oldest first as before. Not selection |
| GREEN-054: flags after the word; Red (PR #210): RED-085, RED-086 | Tooling, and findings fixed in PR #213 |

## Items for the owner

- **Nothing needs your approval this round.**
- **Old entries now live on disk, and the node still picks the same
  ledger.** Black tested windows from 1 to 16 entries, and random disk
  failures followed by restarts. Selection never changed.
- **No depth limit.** A deep switch to a heavier ledger reads old entries
  back from disk. It is slower, never refused.

How this was checked:
- Read D174 to D177, `ledger/cold.go`, `ledger/pool.go` (`viewAfter`,
  `viewCost`, `unitsOf`, `AddLayerHashed`, `switchTo`), `state.Undo`,
  `engine.go` (`handling`, `under`, every `e.mu.Lock`), `node.go`
  (`HandleEntryFrom`, `HandleLayerFrom`), and the diffs of PRs #209,
  #210, #212 and #213.
- Black's random-history runs on `ff4c3e4`:
  - windows 1, 2, 3, 5 and 16;
  - caches of 0 bytes, 1 KiB and 1 GiB;
  - 150 seeds × 300 steps, with and without pruning;
  - a model of stop and restart in which a third of body reads failed at
    random, with 9,125 faults at a window of 1 and 6,272 at a window of 2.

  Runs at windows above 2 also trip the test's guard that every seed must
  go cold, because short histories don't reach the window. That is the
  guard, not a difference.
- The Round 24 and Round 25 cross-version tests: identical on `d1a9128`
  and `ff4c3e4`.
- `go test ./...` passes on `d1a9128`, #209, #212 and #213. The ledger,
  node, engine and p2p packages pass under `-race` on `ff4c3e4`.
