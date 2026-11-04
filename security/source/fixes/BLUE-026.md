# BLUE-026: Every reader of a listed series sees its kept release; a bad release is left out, never the series; add-release checks every release; the chain is not hashed again on each load; the series_id text is quoted

| Field | Value |
| --- | --- |
| Fixes | RED-072 (medium), RED-073 (low), RED-074 (low), RED-075 (low), RED-076 (low) |
| Branch | `claude/project-thread-ggo8dt` (PR against `main`) |
| Base | `main` @ `e93eb29` (Source Round 25 Part C), with ORANGE-035 (#167) merged in |
| Code | `15e8f34` |
| Decisions | D167 |
| With | ORANGE-035 (PR #167): its part ids shared within a process are half of RED-075, so the PR carries its commit. Green's GREEN-050 (#168) touches other code and stays separate. |
| Review | No consensus change. For Black: the `releases-checked.json` cache seeds one id the node itself checked, and is used only when it matches the kept release exactly; a bad kept release now leaves the node knowing fewer parts (missing information) instead of refusing the series. |

## Summary

- **RED-072.**
  - `SeriesInfo.Build` builds a listed series with its kept release.
    `follow`, `unfollow`, `wallet follow`, `ExportSeries`, the import
    merge and its plan counts, and the CLI's `policySeries` all use it.
  - A pool or mint of a part not yet released at this node is refused
    with "waiting for a release" and the part named.
  - `unfollow` keeps a variable series whole with exceptions; the list of
    pools left is written for fixed series only.
- **RED-073.**
  - A kept release that does not check is left out (`Accepted.ReleaseErr`):
    the series is followed at the parts proven without it, the node logs
    it, and `poo doctor` warns under `releases`.
  - `add-release` of a valid release replaces it.
  - An import drops it and names it (`dropped_releases`).
- **RED-074.**
  - `Policy.AddRelease` checks every release in full, also one of a known
    part.
  - Release JSON refuses unknown keys. The keys that `poo series release`
    adds for people are read and ignored.
- **RED-075.**
  - ORANGE-035 shares part ids within a process.
  - A node keeps `releases-checked.json` beside node.json. `Load` seeds
    `part_id(k − 1)` from it when the entry matches the kept release
    exactly, and the witness is still checked.
  - The RPC checks a release before taking the policy lock.
  - By hand, at part 20,000,000: `poo doctor` takes 10 ms with the cache
    and 8.6 s without it.
- **RED-076.** The `series_id` text is printed with `%q`.

## Consensus

None.

- What is valid is unchanged, and still nothing caps part numbers (D165).
- The cache holds only ids this node worked out and checked. It is read
  only from the node's own directory, never exported, and ignored when it
  does not match. It can make a node slower, never change a result.
- Leaving out a bad kept release means the node knows fewer parts:
  missing information, which is never a hard fork.

## Tests

- `pkg/nodepolicy/blue026_test.go`:
  - `TestFollowSeesReleasedParts`
  - `TestUnfollowKeepsAVariableSeriesWhole`
  - `TestBadKeptReleaseIsReplaced`
  - `TestAddReleaseChecksAKnownPart`
  - `TestCheckedReleasesCache`
- `pkg/nodepolicy`: `TestVariableFollowed` (updated: a bad release and a
  fixed series' release are left out, not the series)
- `pkg/series/blue025_internal_test.go`: `TestTrustReleaseSeedsOnlyThatPart`
- `pkg/cli/blue026_test.go`:
  - `TestSeriesIDTextIsQuoted`
  - `TestReleaseFileKeys`
