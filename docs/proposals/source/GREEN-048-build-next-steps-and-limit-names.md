# GREEN-048: Build and claim next steps that work, and -set names it suggests

| Field | Value |
| --- | --- |
| Status | proposed (prototyped) |
| Based on | `main` @ `215d428` |
| Area | `pkg/cli` (`buildOnParent`, `buildCmd`, `claimCmd`, `policyLimitsCmd`), `docs/modules/poon.md` |
| Effort | small |
| Consensus | none: command-line hints and output only. No rule, check or selection changes |
| Decided by | Grey (Black may skip it) |

## Summary

Green tried D161's rival claims and `poon policy limits -set` as a user,
following `running-a-node.md` 4d, and found five problems.

1. **A `-pool` build's next step named the wrong pool.** `poon build -pool 1 -parent root` printed "poon grind start HASH -serial 0 (or poon work -serial 0 -entry HASH)". Serial 0 is in pool 0, so the suggested command works on another pool. The hint used the `-serial` value, which is 0 when the person gave `-pool`.
2. **A plain `poon build` gave no next step.** Its entry has no weight until a layer lands on it, so it is not selected (`switched: false`). Then `poon work -serial 5` fails with "pool 1 has no selected entry; name one with -entry". The person has to find the hash in the reply and build the command themselves.
3. **`-bytes` was dropped with `-parent`.** `poon build -parent ... -bytes` took the flag and still blanked `entry`, unlike a build on the tip (GREEN-039).
4. **`-parent before-claim` was misleading on a claim that is not yet selected.** After `poon claim` and a plain `poon build`, the claim is pending or in an unweighted entry. `before-claim` then says only "the selected ledger holds no claim of this mint", although the person just claimed it.
5. **A `-set` name without its section was simply refused.** `poon policy limits -set max_book=300` said only "not a settable limit", and so did a typo such as `p2p.max_bok`.

All five are fixed on the branch.

## Idea

- `poolArgs(c, ref)` writes the pool as `-pool K`, with `-series ID` only when the person named a series. Every next step uses it, so it always names the pool that was built in.
- `buildCmd` adds `next` when its entry is not selected: "the entry is selected once a layer lands on it: poon work -pool K -entry HASH, or add -work N to the build". It is the same field `buildOnParent` already prints, `omitempty`, so it does not appear once a layer switched.
- `buildOnParent` takes `keep`, so `-bytes` keeps the entry and the layer bytes with `-parent` as well.
- On "holds no claim" with `before-claim`, the error adds: "a claim still pending, or in an entry with no layer yet, is not in it (poon pending -pool K shows the pending ones); -parent root builds on the pool's first entry".
- `limitHint(name)` names the limit the person most likely meant. It takes the limits that end in `.NAME` (`max_book` gives `p2p.max_book`), else the nearest by spelling (`suggest`, as `poon help` uses). It is checked before anything is sent or saved.
- `poon.md`: the `build` row names `next`.

## Testing

- `TestBuildNextSteps`:
  - A plain build's `next` names `poon work -pool 1 -entry HASH`.
  - `before-claim` on an unselected claim names `poon pending -pool 1` and `-parent root`.
  - A `claim -parent root` names `-pool 1` and no `-serial`.
  - `build -pool 1 -parent root -bytes` keeps the entry.
  - It fails on `main`: the plain build has no `next`.
- `TestLimitsSetSuggests`: `max_book`, `p2p.max_bok` and `log_size` each get their limit. A real name and a far one get no hint.
- `TestRivalClaimCLI` still passes. Its `next` check reads "poon grind start HASH", which is unchanged.
- gofmt, vet, staticcheck, and `go test -race ./...` pass locally. GitHub Actions is not running this round.

## Risks

A script that read `-serial N` out of `next` now reads `-pool K`. `next`
is a hint for people, so JSON tools should not parse it.

## Ideas for later (not built)

- **`poon claim -parent before-claim -until-heavier`.** After building, start a grind job on the rival entry that ends when it is selected (`grind start ENTRY` with a "until switched" goal). Today the person loops `poon work` or watches `grind jobs`. D161 left a rival-claim grind target out because `grind start ENTRY` already grinds it. A stop-when-selected goal is the missing piece, and would be a new job goal, not a new target.
- **A weight gap in the reply.** `next` could say how far the branch is behind: "behind by W (selected W1, this branch W2)". Then the person can size `-work` or `-for` instead of guessing. The engine has both weights.
- Still open from GREEN-020: errors name `until_from` where the person typed `-until-from`, and no command renews a lease.
