# BLUE-020: Pool state only for kept items; p2p indexes a huge whole series by use; "all but these" follows; the digest binds options and plan

| Field | Value |
| --- | --- |
| Fixes | RED-049 (medium), RED-050 (medium), RED-051 (low) |
| Branch | `claude/project-thread-ggo8dt` (PR against `main`) |
| Base | `main` @ `4240d32` (Source Round 19 Part C) |
| Code | `a38ca97` |
| Decisions | D154 |
| Review | No consensus change and no limit on what can be followed. For Black: the engine change (RED-049 point 1) and the 4096-section check (RED-050). |

## Summary

- **RED-049.**
  - The engine keeps a pool's state only when an item leaves something in
    it. Queries and checks read a fresh state that is not kept.
  - In p2p, a series of 65,536 pools or more followed whole is announced
    as one section, and its pools are indexed as they come into use. A
    pushed transaction or entry finds its pool by its mint serial.
  - The sorted pool list is cached.
- **RED-050.**
  - `Follow.Except` lets a whole series leave some pools out, so
    unfollowing one pool of a big series stays one entry.
  - The engine follows such an entry with `FollowSeriesExcept`, and p2p
    announces it as one section.
  - A follow change that would need more than 4096 sections is refused
    when it is made.
- **RED-051.** The import digest covers the file, the options and the
  plan.

## Measured

| | before (4240d32) | after |
| --- | --- | --- |
| `p2p.New`, 1M pools whole | 2.3 s, 376 MiB kept | 0.12 ms, 6 KiB |
| `p2p.New`, 2^40 pools whole | never returns | 0.13 ms |
| 100,000 refused entries to distinct pools | 100,000 pool states, +137 MiB | none |
| 5000 pools, pool 7 unfollowed | `p2p.New` refuses to start | starts, one series section |

## What changed

- `pkg/engine/engine.go`:
  - `poolFor`, `peek`, `handling` (in `HandleTx`, `HandleEntry` and
    `HandleLayer`).
  - Read-only calls use `peek`.
  - `follow.except` and `follow.has`.
  - New calls: `FollowSeriesExcept`, `FollowsWhole`, `EachFollowedPoolOf`
    (it replaces `EachFollowedPool`).
- `pkg/node/node.go`: applies `except`.
- `pkg/nodepolicy`:
  - `Follow.Except` and its `Validate` rules.
  - `joinFollow`, `normalFollow`.
  - `RemoveFollow` writes `except`.
  - `MaxSections` and `setFollow`.
  - The plan's `follows` counts `except`.
- `pkg/p2p`:
  - `host.go`: the index is behind `idxMu`, the `big` series are indexed
    by use, the sorted list is cached. New: `makePool`, `index`,
    `poolByKey`, `poolByID`, `resolveBig`.
  - `forward.go`: `onKept` indexes a pool in use; `handlePushed` resolves
    a pool through `resolveBig`.
  - `chase.go`: `offer` indexes the pool it chases in.
- `pkg/cli/policyport.go`: `importDigest`; the preview prints an "import
  digest".
- Docs:
  - `docs/modules/engine.md`, `nodepolicy.md`, `p2p.md`
  - `docs/decisions.md` (D154)

## Tests

- `pkg/engine`:
  - `TestRefusedItemsLeaveNoState` (fails on 4240d32)
  - `TestFollowSeriesExcept`
  - `TestListedPoolsOfAHugeSeries`, updated: a status query makes no
    state, a kept claim does
- `pkg/nodepolicy`:
  - `TestUnfollowPoolsOfAWholeSeries`
  - `TestUnfollowMostPoolsListsTheRest`
- `pkg/p2p`:
  - `TestHostOfAHugeWholeSeriesStartsAtOnce`
  - `TestAllButOnePoolStarts`
  - `TestHugeWholeSeriesForwardsATransaction`
- `pkg/cli`:
  - `TestIfDigestBindsOptionsAndPlan`
  - `TestPolicyImportIfDigest` and `TestPlanNotesReadLikeItsLines`,
    updated for the new message and the "import digest" line

## With ORANGE-029

ORANGE-029 (PR #104) paces how weight requests go out. This PR changes
only which pools a new peer and a check go over: the indexed ones, from
the cached `sortedPools`. The two touch the same loops in
`verifyPeer` and `checkLoop`, but they compose: ORANGE-029 collects
`sortedPools()` per peer and sweeps it. Whichever merges second needs a
small merge there.

## For Black

1. **RED-049 point 1:** a pool's state is made for an item and dropped
   again if the item left nothing. A fresh state is the state from the
   series and index alone, so every item meets the state it met before.
2. **RED-050:** `MaxSections` refuses a follow change that the wire could
   not announce. It limits how the list is written, never what can be
   followed (a whole series with `except` is one section).
3. A whole series with `except` is announced as the whole series. A peer
   may send items for an excepted pool; they are dropped as not followed,
   as before for any pool.
