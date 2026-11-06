# GREEN-050: Release tooling notes, a readable followed table, and a reload that waits for its lock

| Field | Value |
| --- | --- |
| Status | proposed (prototyped) |
| Based on | `main` @ `e93eb29` |
| Area | `pkg/cli` (`seriesFollowed`, `seriesAddRelease`, `claimCmd`, `poolFlags.resolve`, `reloadretry.go`), `pkg/daemon` (`Run`), `docs/modules/poon.md` |
| Effort | small |
| Consensus | none. What is valid, waiting, released or selected is unchanged. The changes are output, a client retry, and the daemon waiting for its own lock after a reload. `series.ErrWaiting`, `AddRelease`, the byte forms and the 409 `waiting` reply are untouched |
| Decided by | Grey. Black may skip it: no rule changes, and a node that waits for its lock decides nothing differently |

## Summary

Green walked through D166 as an issuer and as a node runner:

1. Made a capped variable series and signed it.
2. Ran `poon init -series` with the signed file and started the node.
3. Claimed in part 1, and tried a claim in part 2 before its release.
4. Released parts 2 and 3 and added them out of order.
5. Claimed in part 3.

Everything did what `docs/variable-supply.md` says:

- A part 2 claim waits.
- A newer release moves the count, and an older one changes nothing.
- The claim goes through once its part is released.

Green found three rough edges and one real fault.

1. **`poon series followed -o table` printed `{14}` and `{4}`.** The generic table counts a nested object, so the params became `{14}` and the variable block became `{4}` once it held a release. The table told a reader nothing about what is released.
2. **`add-release` of a release the node already has said only `"added": false`.** That reads like a refusal. It is fine, because a newer kept release proves every part before it, but the reply did not say so.
3. **A mint that waits said what is wrong but not what to do.** `serial 9: series: waiting for a release ... (part 2; released through part 1)` did not name `poon series add-release`, the command that releases it.
4. **A reload could stop the node for good (fault).**
   - The cause: a reload closes the node, which lets go of the data directory's `LOCK`, then opens it again. A command that takes the lock in that gap makes the open fail with "data directory in use by another node", and `daemon.Run` returns.
   - Who takes the lock: an offline `poon policy limits -set` or `poon series add-release` does, because it found no node answering.
   - Reproduced with the `poon` binary: a process took and dropped `LOCK` during one `limits -set` reload. The node logged `reloading its node policy`, then `poon: nodepolicy: data directory in use by another node`, and exited.
   - In tests, a second `add-release` sent just after the first's reload hit the same gap. It failed with "a node or another command holds ...", and in other timings it would win the lock and stop the node.

All four are fixed on the branch.

## Idea

- **Followed table.** With `-o table`, `poon series followed` prints a `followedRow` for each series: `id, supply, pools, mints, units, released_through, next_part`.
  - A fixed series' units are mints × supply, computed with big.Int.
  - Its part columns are `-`.
  - The JSON, which the GUI reads, is unchanged.
- **Release note.** `add-release` prints the reply with a `note` when nothing was added:
  - an older part: "part 2 is already released here: the kept release of part 3 proves every part before it; nothing changed";
  - the same part: "part 3 is the release this node already keeps; nothing changed".

  The RPC reply is unchanged.
- **Waiting hint.** `claim -serial N` and every `-serial` pool pick, when the mint waits, now end with "; once the issuer publishes that part's release, give it to the node with poon series add-release FILE (poon series followed shows what is released here)".
- **Node waits for its lock.** `daemon.Run`: after a reload, never on the first start, an open that fails with `nodepolicy.ErrLocked` is tried again every 100 ms for up to `reloadLockWait` (10 s). The first start still refuses a directory that is in use at once. Whatever the command wrote to node.json while it held the lock is what the node reloads, so nothing is lost.
- **Client dials through the gap.** `dialThroughReload` replaces the first `dialNode` in `limits -set` and `add-release`:
  - When no node answers but the lock is held, it dials again for up to `reloadWait` instead of falling to the offline path.
  - When no node holds the lock, it returns at once and the offline path runs as before.
  - Its probe holds the lock for a moment only, and the daemon change above makes even that harmless.

## How it was tested

- **`TestReloadWaitsForTheLock`** (`pkg/daemon`) uses a test-only hook, `beforeReopen`, that runs in the reload's gap. The hook takes the lock and holds it for 500 ms, and the test checks that the node comes back up. With the wait set to 0, as on `main`, the node stops with "data directory in use by another node". On the branch it passes five times in a row under `-race`. A first version raced for the lock instead of using the hook, and missed the gap under `-race`, so it was replaced.
- **`TestVariableSeriesOnANode`** (`pkg/cli`) now also checks:
  - the waiting hint;
  - both release notes, including part 2 added after part 3;
  - that `series followed -o table` has `SUPPLY`, `variable`, `RELEASED_THROUGH` and no `{`.

  On `main` it fails on the hint, and with only the output changes it also failed on the back-to-back `add-release`. It passes five runs in a row on the branch.
- **Checks run locally:** gofmt, go vet, staticcheck, `go test -race ./...`, and the nine GUI e2e scripts.

## Ideas for later

- `poon series add-release -h` lists only flags. A one-line description, as `poon series release` has, would help.
- An issuer releasing several parts in a row only needs the newest. `poon series release -part K` could say "this release also releases parts 2 to K−1" in its output, as `-check` already does.
- Read commands made just after a reload (`poon series followed` right after `add-release`) can fail with "rpc: is the node running? open .../rpc.addr: no such file", because the reload removes `rpc.addr` for a moment. Reads could use `dialThroughReload` too, which would wait only while a node holds the lock.
