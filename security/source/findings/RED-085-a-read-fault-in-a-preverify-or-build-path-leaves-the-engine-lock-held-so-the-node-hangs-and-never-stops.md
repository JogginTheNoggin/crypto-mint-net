# RED-085: A read fault on a cold entry in `PreverifyEntry`, `PreverifyPage` or `BuildEntryOn` leaves the engine's lock held: the node hangs, `Failed` never closes, and shutdown hangs too

| Field | Value |
| --- | --- |
| Severity | Medium (the trigger is a damaged or changed log plus an entry or build on a parent past `recent_window`; a peer reaches it only when `recent_window` is set below about 128, the operator at any setting through `BuildEntryOn`; the result is a frozen node that looks alive to its peers and cannot be stopped by a signal) |
| Status | fixed |
| Found | Source Round 29 Part B, on `main` @ `d1a9128` (D174, D175) |
| Area | `pkg/engine` (`engine.go`: `PreverifyEntry` :621, `PreverifyPage` :655, `BuildEntryOn` :984; `Err` :192), `pkg/ledger` (`viewAfter`, `warmed`, `fault`), `pkg/p2p` (`handleSafe`), `pkg/node` (`Close`), `pkg/daemon` |
| For Black | No consensus rule is touched. D175's own rule holds on the change paths: the node stops and selects nothing. This is about the paths that read the cold entries and do not change the pool. |
| Fixed by | `claude/project-thread-ggo8dt` @ `255f269` (BLUE-030) |

## What is wrong

D175 makes a cold entry's items and undo come back from the log, and a read
that fails there panics with `ledger.ReadFault`. D174 says a panic part way
through a change fails the engine (`handling`'s deferred func sets
`e.failed`, closes `e.stopped`) so the daemon stops the node. That is done
only in `handling`, which `HandleEntry`, `HandleLayer`, `HandleTx` and the
others use.

Three engine entry points take `e.mu` with a bare `Lock()`/`Unlock()` pair
and no `defer`, and call a ledger method that now reads cold entries:

| Entry point | Ledger call | Reaches `viewAfter` |
| --- | --- | --- |
| `PreverifyEntry` (every pushed entry) | `EntryOwners` | yes, if the parent is within `MaxPreviewCost` (256 units) of the tip |
| `PreverifyPage` (every chase page) | `PageOwners` | yes, same bound |
| `BuildEntryOn` (RPC and grind with a parent or transactions) | `CheckOn`, `CheckPendingOn` | yes, **any depth** |

With `recent_window` low (its floor is 16, and the default 10,000 is
what keeps this out of a peer's reach) a parent a few dozen entries below the
tip is already past the window, so its revert needs cold entries. At the
default, `PreverifyEntry` and `PreverifyPage` cannot reach a cold entry
(their bound is 256 units, about 128 entries) and only `BuildEntryOn`, which
has no depth bound, can. When one of those reads fails, `ReadFault`
panics up through the bare `Unlock()`, which is never reached. The panic is
then caught where it is caught:

- `p2p.handleSafe` recovers it and closes that peer, as D173 intends. But
  `e.mu` stays locked. `e.failed` is not set and `Failed()` is not closed,
  because that code is in `handling`, not here.
- `net/http` recovers it in an RPC handler for `BuildEntryOn`, the same way.

From then on every call that takes `e.mu` waits for ever: `Status`, `Ledger`
(so every p2p reply and every RPC query), all `Handle*` calls, and `Err()`.
`Node.Close` calls `n.eng.Err()` (`node.go:979`) to decide whether to compact,
so **shutdown hangs on the same mutex**. The daemon's `<-Failed()` case never
fires, so it does not even try. The TCP listeners are up and the sockets
accept, so peers see a node that is slow, not one that is gone; only `kill -9`
ends it. This is worse than the crash RED-084 had: that one restarted and
rebuilt, this one needs someone to notice.

## How I checked it

`pkg/engine` test (throwaway, removed). An engine with `RecentWindow: 16`, a body
cache set to 0 after the build, and a source that returns an error from
`Load` once `fail` is set (what `ReadAt` gives for a truncated file or an
EIO). 40 entries of single-input spends of mint 4, each selected by a
layer; 24 are cold. Then `fail = true`, and a valid rival entry whose parent
is entry 10 (29 entries reverted, view cost 58, well inside the 256):

```
PreverifyEntry:  recovered: ledger: could not read back entry 7610b182 from the store:
                 ledger: entry body cannot be read back
e.Status(key):   no answer after 3 s (engine mutex stuck)
e.Failed():      not closed
```

The same with `PreverifyPage(key, [r1, r2])` and with
`BuildEntryOn(ctx, key, 0, Grind{}, BuildOn{Parent: &par, Txs: ...})`
(`stuck=true` after 2 s). Nothing races: the lock is simply never released. With a parent inside the
window (entry 25 of 40) the same call succeeds with the source failing, so
the trigger is exactly the cold entries.

`store.Log.ReadAt` returns an error for a truncated file (the cached
`l.size` is larger than the file, so `f.ReadAt` gives EOF), for a bad
checksum and for a stale location, so each of these reaches `Load` as an
error and then `ReadFault`; I did not run a whole node with a file edited
under it, only the source returning that error.

## What could happen

- A node with a damaged log (bit rot, a failing disk, a restore, a
  `copytruncate` rotation over a running node) and a small `recent_window`
  (a low-memory node, which is what the option is for) hangs the first time any
  peer sends one entry on a parent between `recent_window` and about 128 entries
  below the tip. The entry needs no work and no valid signature (the check is
  before signatures), so any peer can send it at the moment it likes.
- The same node cannot be stopped with SIGTERM, so a service manager waits
  for its timeout; a supervisor that watches `Failed` sees nothing.
- An operator running `poo build -parent` in a part older than `recent_window` (or a grind job on a chosen
  parent) hits it with no peer involved, at any window.

## Suggested fix (Blue's call)

- Use `defer e.mu.Unlock()` in the three functions (a closure where the lock
  must be dropped before signatures are checked), so a panic never leaves
  the lock held.
- Make the fail-the-engine step shared: a helper that recovers a
  `ledger.ReadFault` from any call made on a pool, sets `e.failed`, closes
  `e.stopped`, and then re-panics, used by `handling` and by these three. A
  read fault on a path that changes nothing still means the log cannot be
  trusted to serve, so stopping is right and matches "the node must stop,
  not select".
- Add `Failed` to the engine test list: for each entry point that takes the
  lock, inject a `ReadFault` and check that `Status` answers and `Failed` is
  closed. Every `e.mu.Lock()` in `engine.go` that is not followed by `defer
  e.mu.Unlock()` and calls into `ledger` is the list to check; I found these
  three.

## Not tested

A whole node with the log file truncated or edited under it (the source
returned the error); `Close` on a stuck engine (read from the code: `Err()`
takes `e.mu`); `ClaimParent` and `Pool.Tx` callers, which use `defer` and
handle the error.

## Resolution

Fixed in BLUE-030 (D177). `PreverifyTx`, `PreverifyEntry`,
`PreverifyPage` and `BuildEntryOn` now take the lock through
`Engine.under`. It always releases the lock, and on a panic it fails the
engine as `handling` does (`e.failed` is set and `Failed()` is closed)
before the panic goes on. Once the engine has failed they do nothing,
and `BuildEntryOn` returns `ErrFailed`. A grep of every `e.mu.Lock()` in
`engine.go` without a deferred unlock found no other call into the
ledger beyond skeleton reads (`Has`). Test:
`TestColdReadFaultReleasesTheLock` covers all three entry points with an
injected cold read failure. Without the fix each case hangs on the lock
for 5 s.
