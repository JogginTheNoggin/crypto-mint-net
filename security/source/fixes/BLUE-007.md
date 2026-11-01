# BLUE-007: A branch costs what its entries hold, once; node.json and job starts are serialised

| Field | Value |
| --- | --- |
| Fixes | RED-012 (medium), RED-013 (low), RED-014 (low) |
| Branch | `claude/project-thread-ggo8dt` |
| Base | `claude/m10-watch` @ `9972137` (Source Round 6 Part C) |
| Decisions | D114 |

## Summary

These are three fixes from Source Round 6.

- **RED-012.** Taking in a long branch with no layers cost time quadratic in its length, and a chase took it in without charging the sender. It now costs the same per entry at any length: 1.5 us at both 16,000 and 64,000 entries, where it was 221 us and 1.25 ms. Entries a chase takes in pay the sender's revert budget, and that budget now counts transactions as well as entries. A fetch ends after 4,096 entries in a row without layers.
- **RED-013.** Concurrent writes of `node.json` could tear the file, and concurrent edits could lose an update. Neither can happen now.
- **RED-014.** Concurrent job starts could exceed `MaxJobs`. They no longer can.

There is no change to byte forms, the wire or consensus.

## What changed

- `pkg/ledger`:
  - `Pool.view`/`viewOf` hold the state kept after the last entry added. `viewAfter` returns it for that entry, and a switch or a prune of that entry drops it.
  - `node.anchor` and `Pool.leaves` replace the anchor walk in `weightOf` and the scan of every off-chain node in `offChainLeaves`.
  - `Pool.units` is a prefix sum of view cost along the selected chain. `unitsOf(n)` is 1 plus the number of transactions in n.
  - `MaxPreviewCost` is now 256 units.
  - `CheckOff` (in the test export) also checks the leaf set and every anchor.
- `pkg/p2p`:
  - `spendRevert` is shared by `handlePushed` and `takeItem` (requested entries). An empty budget ends the fetch with `errRevertBudget`.
  - `fetch` takes a bare-run counter. `fetchAll` and `fetchBranch` end at more than `Config.MaxBareRun` (default 4,096) entries in a row without layers when the series has no entry hash credit.
- `pkg/nodepolicy`: `Save` writes to its own temporary file, syncs it, renames it, then syncs the directory.
- `pkg/rpc`: `Handler.policyMu` is held around the `node.json` edits in follow, unfollow and the schedule. The schedule is also applied under it.
- `pkg/grind`: `Manager.reserved` holds a job's slot across `checkPlans`, and `Manager.sched` serialises `Schedule`.
- Docs: D114, plus `docs/modules/ledger.md`, `engine.md`, `tracking.md`, `p2p.md`, `grind.md` and `nodepolicy.md`.

## Tests added

- `pkg/ledger`:
  - `TestEntryOwnersRefusesDeepParent` is updated for view cost units and the kept state.
  - `TestKeptViewIsTheParentsState`: siblings, an older parent and a refused entry are each checked against their own parent's state.
  - `TestLongBranchIsLinear`.
  - `BenchmarkOffChainBranch`.
  - I ran the property tests (`TestRandom*`) once with 400 seeds instead of 12, and they passed.
- `pkg/p2p`:
  - `TestBareRunEndsFetch`: a run of 6 bare entries is taken in and selected. A run of 10 stops at 6 held, with nothing selected.
  - `TestChaseInOrderCostsLittleRevert`: an honest switch passes on a revert budget of 20. I checked by hand that a budget of 5 stops it.
  - `TestDeepForkSpendsRevertBudget` is updated to the new units.
- `pkg/grind`: `TestJobLimitUnderConcurrency` and `TestScheduleIsSerialised`. Both fail on the old code.
- `pkg/nodepolicy`: `TestConcurrentSavesNeverTear`.
- `pkg/rpc`: `TestGrindEndpoints` now also sets two schedules concurrently and checks that `node.json` matches the running policy jobs.

## How Grey can check it

```sh
go test -race -run 'DeepParent|KeptView|LongBranch|Random' -v ./pkg/ledger
go test -run XXX -bench OffChainBranch -benchtime 16000x ./pkg/ledger
go test -run XXX -bench OffChainBranch -benchtime 64000x ./pkg/ledger
go test -race -run 'BareRun|InOrder|DeepFork|CatchesUp|ForkConverges' -v ./pkg/p2p
go test -race -run 'JobLimit|Schedule' -v ./pkg/grind
go test -race -run 'ConcurrentSaves' -v ./pkg/nodepolicy
go test -race ./...
```

staticcheck and govulncheck are not installed here, so please run them. The kept state is the riskiest part of this change. Its invariant is that the kept overlay is the state after `viewOf` over the current `p.st`, and `p.st` only changes in `switchTo`, which drops the overlay. The property tests cover it, but Red may want to aim at it. I left one finding open for Red: `AddLayer` on an off-chain entry still walks its subtree, as noted in RED-012.
