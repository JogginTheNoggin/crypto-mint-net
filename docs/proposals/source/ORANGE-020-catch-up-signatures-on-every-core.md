# ORANGE-020: Catching up measured; check each reply's signatures on every core

| Field | Value |
| --- | --- |
| Status | proposed |
| Based on | `claude/m10-watch` @ `0a6cc94` |
| Area | `pkg/ledger` (`Pool.PageOwners`), `pkg/engine` (`Engine.PreverifyPage`), `pkg/p2p` (`fetch`) |
| Effort | small |
| Decided by | Grey, with Red for RED-009's cost bound (Black reviews in Part D) |

## Summary

ORANGE-019 made long bare runs fast. This round I measured catching up
an ordinary ledger, where every entry holds a spend and a layer. That is
what a new node does when it first syncs.

**It runs on one core.** A node takes the entries of a reply one at a
time, and checking each entry's signature is about half its cost. One
spend has one witness, so ORANGE-014's `CheckAll` never has enough
witnesses to spread them across cores. On 4 cores, a 20,000-entry
ledger is caught up at 94 to 129 µs an entry.

**The fix: check a reply's signatures first, on every core, then take
the entries in order.**
1. Under the engine lock, `Pool.PageOwners` finds each entry's owners.
   It checks each entry against the state after the entries before it
   in the reply, using a scratch overlay, exactly as `EntryOwners` would
   once they are held.
2. Outside the lock, `Engine.PreverifyPage` checks every witness on all
   cores. The passes fill the list of verified witnesses that
   `PreverifyEntry` already fills.
3. The entries are then taken in order as before, and every signature
   check is already remembered.

| 4 vCPU | Before | After |
| --- | --- | --- |
| Catching up 5,000 entries | 520 ms (104 µs an entry) | 229–288 ms (46–58 µs) |
| Catching up 20,000 entries | 1.9–2.6 s (94–129 µs) | 0.85–0.87 s (43–44 µs) |
| 5,000 entries, `GOMAXPROCS=1` | 444 ms | 453 ms |

It is 2.2× faster on 4 cores, and the same on one core. Entries with
more transactions spread better still.

No consensus change: what is checked and what it returns are the same.
Only when signatures are checked, and on how many cores, changes.

## Measurements

Same machine as ORANGE-012: 4 vCPU Xeon at 2.10 GHz, Go 1.27.3.

How they were taken:
- A test-only harness with two `startNode` nodes and default settings.
- a grows N entries, each with one spend (Ed25519) and one layer, as
  the tests' `grow` does.
- b holds nothing and connects. The timer runs until b holds a's tip
  and weight.
- CPU is the process's user and system time for both nodes.
- "Before" is the same harness on `0a6cc94`, two runs each.

| Run | Time | µs an entry | CPU | Requests |
| --- | --- | --- | --- | --- |
| Before, 5,000 | 521, 520 ms | 104 | 547 ms | 16 |
| After, 5,000 | 288, 229 ms | 58, 46 | 563, 509 ms | 16 |
| Before, 20,000 | 2.59, 1.89 s | 129, 94 | 2.73, 1.91 s | 46 |
| After, 20,000 | 854, 871 ms | 43, 44 | 1.83, 1.86 s | 46 |
| Before, 5,000, one core | 444 ms | 89 | 441 ms | 16 |
| After, 5,000, one core | 453 ms | 91 | 452 ms | 16 |

The CPU used is the same; it is now spread over the cores.

Where b's time went before (CPU profile, 5,000 entries):

| Where | Share |
| --- | --- |
| `PreverifyEntry` → `ed25519.Verify`, one entry at a time | 55% |
| `HandleEntryFrom` under the node's lock (state, store, events) | 17% |
| Layers (`HandleLayerFrom`) | 10% |

After the fix, the signature checks run in `PreverifyPage` across the
cores, and the in-order intake is about a quarter of b's CPU.

## Idea

- **`ledger.Pool.PageOwners(es)`:** for a run of entries, each the
  parent of the next.
  - It returns the items and owners of each entry it gets through.
  - It finds each against an overlay of the state after the first
    entry's parent, with the entries before it applied.
  - It stops before an entry that fails a check short of the witnesses,
    does not link to the one before, or is already held.
  - It previews nothing if the first parent is unknown or deeper than
    `MaxPreviewCost` (RED-011).
  - It changes nothing.
- **`engine.Engine.PreverifyPage(k, es)`:** calls `PageOwners` under
  the lock. Outside it, it runs `tx.Preverify` for every transaction on
  `GOMAXPROCS` workers, which stop when one fails.
- **`p2p.fetch`:** for each chunk of more than one entry that starts on
  the entry taken last, it calls `PreverifyPage` before taking them in.
- **Tests:**
  - `TestPageOwnersMatchesEntryOwners` checks that the owners are equal
    to `EntryOwners` as each entry is added. It also covers where the
    preview stops: an unknown first parent, a gap, a held entry, and a
    spent input.
  - `TestPreverifyPage` counts satisfaction calls. A 12-entry run costs
    12 ahead and none when taken in. A bad fourth witness costs at most
    4, and `HandleEntry` still refuses that entry. An orphan run costs
    none.

## For Red: RED-009's bound

RED-009 (D107) says only items that passed every cheaper rule cost
signature checks, and a bad one costs one.
- The first half holds. Owners come from the same checks, and each
  witness's address and allowed checks run before its signature, as in
  `CheckCached`.
- The second half becomes "about one per core", as for `CheckAll` in
  ORANGE-014. The workers stop at the first failure, but the ones
  already running finish their check.
- A reply's entries after a bad one may have been verified too. Each
  was valid against the state, so the work is no more than taking a
  valid reply costs.

The lock is held for `PageOwners`: the shape and state checks of up to
one chunk of entries (512 by default), which `PreverifyEntry` already
did one entry at a time. They are cheap next to the signatures; in the
profile they do not show.

## Cost and risk

- About 110 lines with tests.
- Each entry's owners are found twice: once for the reply, once in
  `PreverifyEntry`. That is map lookups, not signatures.
- A chunk's witnesses go into the list of verified witnesses (16,384).
  A chunk holds far fewer.
- The full suite passes: `go test -race ./...`,
  `TestHeavierLedgerHuge`, `TestHeavierLedgerLongBareRun`, and
  `GOEXPERIMENT=simd` for the changed packages.

## Next

Checking the next reply's signatures while the node takes in the
current one would overlap the two. That needs a request in flight while
the chase takes entries, which is a larger change.

## Prototype

On `claude/source-orange-r10`; see the commit "ORANGE-020".
