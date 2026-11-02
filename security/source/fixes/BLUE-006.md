# BLUE-006: Check a deep parent once, and charge its revert to the sender

| Field | Value |
| --- | --- |
| Fixes | RED-011 (medium), and Red's note on grind job sources |
| Branch | `claude/project-thread-ggo8dt` |
| Base | `claude/m10-watch` @ `29b08ce` (code of `a92c2ca`, Source Round 5 Part C) |
| Decisions | D111 |

## Summary

An entry built on the pool id, or deep in the selected chain, makes the
node revert the selected chain down to that parent to check it. BLUE-005's
preverify did that a second time under the engine lock. Now the preverify
skips any parent more than 64 entries of revert away, so the cost is paid
once, and each pushed entry's revert count is drawn from a per-address
budget, so one peer cannot keep the lock busy with forks on old parents. A
grind job started over the RPC can no longer carry a label that escapes
`DELETE /v1/grind?source=user`. No byte form, wire or consensus change.

## What changed

- `pkg/ledger`: `MaxPreviewCost` (64), `ErrDeepParent`,
  `Pool.ViewCost(e, limit)` and `viewCost` (the entries `viewAfter`
  reverts and applies, counted without building anything). `EntryOwners`
  returns `ErrDeepParent` past the bound.
- `pkg/engine`: `EntryCost(key, e)` (capped at `MaxCountedCost`, 2^20).
  `PreverifyEntry` needs no change: it already stops on any error.
- `pkg/tracking`: `Spend(addr, kind, n, now)` (`Budget` is `Spend` of
  1), `KindRevert` with a default of 200,000 a minute.
- `pkg/p2p`: `handlePushed` spends a pushed entry's `EntryCost` from
  the `revert` budget before handling it, and drops it when over.
- `pkg/rpc`: `POST /v1/grind` refuses any source but `user` or none.
- Docs: D111, `docs/modules/ledger.md`, `engine.md`, `tracking.md`,
  `p2p.md`, `rpc.md`.

## Tests added

- `pkg/ledger`: `TestEntryOwnersRefusesDeepParent`: on a 70-entry
  chain, `ViewCost` is 0 on the tip, 10 ten below it, 70 at the pool id,
  71 past an off-chain entry, 0 for an unknown parent; `EntryOwners`
  takes the near parent, refuses the pool id with `ErrDeepParent`, and
  `AddEntry` still takes that entry.
- `pkg/engine`: `TestPreverifySkipsDeepParent`: one signature verified
  ahead for an entry on the tip, none for one on the pool id;
  `EntryCost` 0 and 66.
- `pkg/tracking`: `TestSpend`.
- `pkg/p2p`: `TestDeepForkSpendsRevertBudget`: of three forks on the
  pool id against a budget of two, the first two are held, the third is
  dropped and flagged once.
- `pkg/rpc`: the grind test refuses a custom source.

## How Grey can check it

```sh
go test -race -run 'DeepParent' -v ./pkg/ledger ./pkg/engine
go test -race -run 'Spend|Budget' -v ./pkg/tracking
go test -race -run 'DeepFork|InvalidEntries|BadWitnessTx' -v ./pkg/p2p
go test -race -run 'Grind' -v ./pkg/rpc
go test -race ./...
```

staticcheck and govulncheck are not installed here, so please run them.
Orange may want Red's 40,000-entry measurement again: `AddEntry` still
pays one revert for a deep fork, now bounded per address by the budget.
