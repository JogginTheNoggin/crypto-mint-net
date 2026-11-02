# BLUE-019: Huge whole-series follows cost what is in use; the digest check cannot be emptied; every node setting is named

| Field | Value |
| --- | --- |
| Fixes | RED-046 (medium, in part), RED-047 (low), RED-048 (low) |
| Branch | `claude/project-thread-ggo8dt` (PR against `main`) |
| Base | `main` @ `533dbcd` (Source Round 18 Part C) |
| Code | `a133161` |
| Decisions | D152 |
| Review | No consensus change and no limit. One protocol question for Black. |

## Summary

- **RED-046.**
  - A node following a series of 2^40 pools whole starts in about 20 ms.
    It reads the stores on disk, not every followed pool.
  - Status, metrics, queries and grind targets list only the pools in use
    for a whole series of 65,536 pools or more.
  - The p2p host's per-pool index costs a third of what it did. It is
    still linear, which is what is left.
  - The plan states pool counts, and names a large whole-series follow
    first.
- **RED-047.** An empty or malformed `-if-digest` is refused, and so is
  `-if-digest` without `-apply`.
- **RED-048.** Under `-settings`, every node-setting line is named first,
  except cosmetic ones (`log.format`).

## Measured (1M pools, PoolSize 1, followed whole)

| | before | after |
| --- | --- | --- |
| `node.Open` | 2.46 s, 901 MiB allocated | 5 ms, 8 MiB |
| `p2p.New` | 7.1 s, 1.7 GiB allocated | 1.1 s, 0.49 GiB |
| status, metrics scrape | made state for every pool | lists only pools in use |

## What changed

- `pkg/node/node.go`: `stored()` reads `pools/<series>/<k>.log`; `load` uses it.
- `pkg/engine/engine.go`:
  - `ListedPools`, `ListAll`
  - `EachFollowedPool`
  - `FollowedCount`, `FollowedSeries`
- Callers moved to the new engine calls:
  - `pkg/rpc/rpc.go`, `metrics.go` and `view.go`
  - `pkg/watch/query.go`
  - `pkg/grind/jobs.go`
  - `pkg/daemon/daemon.go`
- `pkg/p2p/host.go`:
  - The pool index is built with `EachFollowedPool`, and its maps are
    sized once.
  - The `byName` map is gone; `poolByName` parses the section text
    instead (`chase.go`).
- `pkg/nodepolicy/bundle.go`:
  - `Cosmetic` replaces `Defences`; `isDefence` is now the inverse of
    `Followed` and `Cosmetic`.
  - `Plan.Follows` (`FollowCount`) and `LargeFollow`.
- `pkg/cli/policyport.go`:
  - `-if-digest` is checked with `fs.Visit`.
  - Pool counts are printed.
- Docs:
  - `docs/modules/node.md`, `engine.md`, `nodepolicy.md`
  - `docs/decisions.md` (D152)

## Tests

- `TestHugeSeriesFollowedWholeStartsAtOnce` (pkg/node; it does not finish
  on 533dbcd)
- `TestListedPoolsOfAHugeSeries` (pkg/engine)
- `TestSettingsNameEveryNodeSetting` and `TestPlanStatesPoolCounts`
  (pkg/nodepolicy)
- `TestIfDigestEmptyIsRefused` (pkg/cli)

## For Black

There is no change to what is valid. The engine still follows every pool
and still makes a pool's state when its first item or request arrives.
Only listing and reporting skip unused pools of a huge whole series.

There is one protocol question, not a consensus one. The wire names a pool
only by `PoolID`, a hash of series and index. A node therefore has to
compute every pool id of a whole-series follow to recognise one, so a
2^32-pool series followed whole still cannot run a p2p host. Carrying the
pool index next to the pool id in pool sections, checked against
`PoolID`, would let the host index a pool when it first sees it. Is that
yours or jerry's to approve? Nothing is built for it.

Red's Part D questions:
- Can a node with `check_seconds` -1, or with small `hash_limit`,
  `layer_limit` or `chunk_size`, still find and accept a heavier ledger?
- RED-046 point 1.
