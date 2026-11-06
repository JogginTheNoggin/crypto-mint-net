# Source Round 28: consensus review (Black, Part D)

## Summary

This round moves entry bodies to disk behind a bounded cache (D172). It
also handles a body that can't be read back as missing information
(D173, RED-084), strips witnesses from the in-memory items (ORANGE-038),
and sizes the cache in real bytes with a 256 MiB default, which the owner
approved.

**Result: 9 clear, 1 clear with a condition, 0 blocked.**

- **No consensus rule or depth limit is added.** No rule, byte form or
  wire message changes. Every entry is still held and checked as before.
  Only where its bytes live changes.
- **Selection doesn't depend on the cache, or even on the disk.** Black
  ran the random-history differential with a twin pool whose every body
  read fails, and with caches of 0 bytes, 1 byte, 1 KiB and 1 GiB. That
  was 150 seeds of 300 steps, with and without pruning. 691 reorganisations
  were checked against the model.
  - The twin selected the same tip with the same height and weight.
  - It held the same entries and had the same tip state at every step.
- **A deep switch to a heavier ledger never needs a body from disk.**
  - Switching reverts with the skeleton's undo and applies with the
    skeleton's items.
  - The chase walks parents from the skeleton.
  - A new entry is checked while its body is in memory.
  - The one body read in a switch only fills in the witnesses of
    displaced transactions for the pending pool and the report.
- **The condition is about robustness, not a rule.** D173's panic guard
  can now let a node run on after a panic part-way through changing a
  ledger. Black asks that it be narrowed. Details are below.
- **Fixed series, transactions and entries are unchanged.** The Round 24
  and Round 25 cross-version tests are byte for byte the same on `main`
  and on PR #202.

| Field | Value |
| --- | --- |
| Round | Source Round 28 |
| Consensus | None (D172, D173) |
| Other consensus suggestions | None |
| Base reviewed | `main` @ `705639a` (PR #197: D172, with Round 27's PRs merged) |
| Previous base | `main` @ `1a1c729` |
| Blue branches | PR #202 @ `c96b5bd` (BLUE-029, D173: RED-084; carries ORANGE-038) |
| Red | PR #199 @ `c908163` (RED-084) |
| Proposals | ORANGE-038 (PR #201 @ `abc3399`); GREEN-053 (PR #200 @ `255a9a4`) |
| Result | 9 clear, 1 clear with a condition, 0 blocked |

## No verdict reads a body that is not in memory (D172, D173)

Black read every place a pool reads an entry's body (`Pool.body`). On
PR #202 there are only two:

1. **`Pool.Entry`**, for callers outside the ledger:
   - serving a peer;
   - queries and the watch pages;
   - `Engine.Payment`;
   - the chase's old parent walk, which ORANGE-038 now replaces with
     `ParentOf`;
   - compaction.

   None of these decides whether an entry, layer or ledger is valid or
   selected.
2. **`switchTo`**, which reads back the body of a displaced entry only to
   give its transactions their witnesses.
   - Those transactions go to the pending pool (carried) and to the
     report.
   - If the read fails, they are reported without witnesses and none is
     carried. A peer can send them again.
   - The new tip's state, weight and selection are already decided at
     that point, from the skeleton.

Nothing else reads a body:
- `AddEntry`, `AddLayer`, `better`, the revert and apply in `switchTo`,
  and `prune` read only the skeleton (hash, parent, height, weights,
  layers, items, undo).
- `Rivals` now reads the skeleton's items too, so it can't miss a rival
  whose body can't be read back.

### Sorting displaced transactions without witnesses (ORANGE-038)

An entry whose body was released has items without witnesses. `switchTo`
sorts its transactions with `tx.Owners` instead of `tx.CheckAgainst`.
Black agrees this gives the same answer:

- **The witnesses have already passed.** They passed `CheckAgainst` when
  the entry was taken: the address hash, the allowed types and
  algorithms, the ceiling and satisfy. None of that depends on the
  branch. This is the same argument Round 23 accepted for
  `CheckPendingOn`.
- **The owners are fixed.** An input's owner is the address of the output
  it spends. That output is either the same on every branch or missing,
  and a missing output fails `Owners`.
- **The twin agrees.** `sameSwitch` and Black's runs above sort
  Unchanged and Void the same way in both pools.
- **It isn't a validity verdict anyway.** Displaced sorting feeds the
  pending pool and the report. It never decides what a node selects.

A stripped transaction can't be mistaken for a whole one. Every valid
transaction has at least one witness (one per input, or one per output
for a claim), and a stripped one has none.

## Cache size, cold or warm, compactions and restarts

- **Any cache size.** The cache bounds memory, not information. A cache of
  0 bytes, or a source that always fails, selects exactly as a node with
  every body in memory (Black's runs above, and D172's own 1 KiB twin).
- **Compaction.**
  - It copies entry records by location (`RewriteFunc`) and checks each
    frame's checksum.
  - An error leaves the old log exactly as it was. The temporary file is
    removed, and nothing replaces the old file.
  - `ErrStaleLoc` stops a location from an older generation being read as
    another record.
  - Copying keeps the same records. It drops none, and it keeps the replay
    order of the selected ledger.
- **Restarts.**
  - Start-up replays the log in windows of 1,024 records, through
    `AddEntry` as before, so the result is the same as a whole-log replay
    (`TestBodiesFromDisk`: no cache, through a compaction and a restart).
  - A damaged log is cut at its first bad record, as before, and the rest
    is fetched from peers. That is less information until it is fetched,
    not a limit on depth or a verdict.
- **An unreadable body (RED-084).** Since D173 `Pool.Entry` returns
  `ErrUnreadable`, and every caller treats it as "not held right now":
  - serving says `not held` and the asker gets it elsewhere;
  - queries say `unreadable`.

  That leaves this node's peers short of information, never in
  disagreement.

## The condition: narrow the panic guard (D173)

`handleSafe` recovers any panic while one peer message is handled. It
closes that peer, logs, and lets the process run on. On the serving paths
this is fine, because they only read.

But the work loop also handles pushed entries and layers
(`forward.go` → `Node.HandleEntryFrom` → `Engine.HandleEntry` →
`Pool.AddEntry` → `switchTo`). `Engine.HandleEntry` unlocks with `defer`.
So a panic part-way through a switch would now be recovered with the
engine unlocked and the pool possibly half-updated: some entries reverted
and others not applied, or the state out of step with the chain.

- **Before D173,** such a panic ended the process. A restart rebuilt the
  pool from the log, which is safe.
- **After D173,** the node could keep running on a wrong state and refuse
  a valid heavier ledger that its peers take, with the same information.
  That is a node bug, not a rule. But it is exactly the outcome the
  owner's hard-fork definition is about, and the old crash did not have
  it.

Black knows of no panic that can be reached on these paths today, so this
is not blocking, and PR #202 can merge. **Condition, for the next Part A
(Grey or Blue):** keep the recovery to paths that don't change a ledger.
Either of these is enough:

- re-panic when the message is one that reaches `HandleEntry` or
  `HandleLayer` (entries, layers, pushes);
- have the engine mark itself broken when a panic passes through its lock,
  and stop the node so the next start rebuilds from the log.

Add a test that a panic injected inside `AddEntry` doesn't leave a
running node with a changed pool.

## Clear

| Item | Why it is clear |
| --- | --- |
| D172: store locations, `ReadAt`, `OpenFunc`, `RewriteFunc`, copying by location | Storage. A read is checksummed, and a stale location is refused. A failed compaction leaves the old log |
| D172: entry skeleton, `BodySource`, `BodyCache`, `entry_cache_mib` | No verdict reads a body outside memory. Selection is the same at any cache size, and with every read failing (tested) |
| D172: windowed replay at start | The same `AddEntry` path, a window at a time. It ends where a whole-log replay ends (tested) |
| D172: open-file warning | A warning |
| D173 (RED-084): `ErrUnreadable`; serving says `not held`; queries say `unreadable`; `poon doctor` warns | Missing information for this node's peers and callers, never a verdict |
| D173: `Rivals` from the skeleton | Reads every input from the items. It can't miss a rival, and decides nothing |
| ORANGE-038 (carried in #202): witnesses stripped from released items; chase walks parents from the skeleton | Displaced sorting matches `CheckAgainst` (above). The chase needs only parents |
| D173: cache in real bytes, 256 MiB default (owner-approved) | Memory only |
| GREEN-053: release a body as soon as it is written; doctor columns | Memory and tooling |
| Red (PR #199): RED-084 | A finding, fixed in PR #202. It touches no rule |

## Clear with a condition

| Item | Condition |
| --- | --- |
| D173: `handleSafe` recovers panics in the p2p work loop | Recover only on paths that don't change a ledger, or stop the node after a panic under the engine's lock. Test it. Not blocking |

## Items for the owner

- **Nothing needs your approval this round.**
- **Moving bodies to disk changes cost, not decisions.** A node with a tiny
  cache, or even a failing disk, picks the same ledger as one that keeps
  everything in memory. Black tested this directly.
- **No depth limit was added.** A deep switch to a heavier ledger never
  needs an old body from disk, so it can't be refused for that reason.
- **One robustness fix is asked for next round.** The new panic guard
  should stop the node, not run on, when a panic happens while a ledger is
  being changed. Nothing is known to trigger it.

How this was checked:
- Read D172 and D173, `ledger/bodies.go`, `ledger/pool.go` (`Entry`,
  `switchTo`, `Rivals`), `node/bodies.go`, `node/node.go` (`liveRecords`,
  `compact`), `store/store.go` (`ReadAt`, `RewriteFunc`), `p2p/inbox.go`,
  `p2p/chase.go`, `engine.go` (`HandleEntry`, `Payment`), and the diffs
  of PRs #199 to #202.
- Black's runs of the random-history differential on `c96b5bd`, with a
  twin whose every body read fails, and with caches of 0 bytes, 1 byte,
  1 KiB and 1 GiB: 150 seeds × 300 steps, with and without pruning.
- The Round 24 and Round 25 cross-version tests: identical on `705639a`
  and `c96b5bd`.
- `go test ./...` passes on `705639a`, #200, #201 and #202. The ledger,
  node, store and p2p packages pass under `-race` on `c96b5bd`.
