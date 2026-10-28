# GREEN-014: Address history pages that skip nothing

| Field | Value |
| --- | --- |
| Status | proposed |
| Based on | `claude/m10-watch` @ `7e68fe0` |
| Area | `pkg/watch` (`AddressPage`), so `GET /v1/address` and `poo address` |
| Effort | small |
| Decided by | |

## Summary

When an address has history in more than one pool, paging through it by
following `Next` silently skips items. A test reproduces it: 4 of 6 items
are reached. The fix keeps whole entries on a page and moves each pool's
`Next` up to the newest entry the page left out. Prototyped, with the
reproduction test and a unit test. Source/Red spotted this pattern in
review and passed it to me as a note. It is a correctness problem, not a
security one.

## Problem

`AddressPage` walks each pool in scope from `below` (or its tip), taking
up to `limit` items per pool and recording where each pool stopped in
`Next`. It then merges the pools' items by weight and truncates to
`limit`. Items cut by that truncation sit above their pool's `Next`
height, so a client that follows `Next` (narrowed to that pool, as the
field's comment says) never sees them.

`TestAddressPagingAcrossPools` reproduces it. Alice has 3 history items in
pool 0 and 3 in pool 1. The first page, with `limit 2`, holds pool 0
height 3 and pool 1 height 3. Following `Next` then reaches heights 1 in
both pools. The two items at height 2 are never returned. A wallet or the
GUI's address view would show an incomplete history, and its totals
(received, spends) would be wrong.

## Idea

After the merge, `truncateHistory`:
- **Keeps whole entries.** Items of one pool and height stay together, so
  a page never ends in the middle of an entry. If the first entry alone
  holds more than `limit` items, it is still kept whole, so a page always
  makes progress. That matches how the per-pool walk already counts.
- **Moves `Next` up.** For each pool with items left off the page, its
  `Next.Below` becomes the newest height it left off. Within one pool,
  items are newest first, so what a pool keeps is always a run of its
  newest entries, and nothing is returned twice.
- **Leaves other pools alone.** Pools with nothing left off keep the
  `Next` from their walk.

Nothing changes for a single pool, or when everything fits on one page.
The bounded walk from D103 (50×limit entries) is untouched.

## Cost and risk

About 45 lines in one function. A page can now hold fewer than `limit`
items. That was already true near the end of a history, and clients page
by `Next`, not by counting. `Next` is sorted by pool, as `FollowedPools`
gave it before.

## Prototype

On `claude/source-green`:
- `truncateHistory` in `pkg/watch/query.go`
- `pkg/watch/paging_test.go`: two pools, following `Next` must reach
  all 6 items exactly once. It fails on the base with 4 of 6.
- `pkg/watch/truncate_internal_test.go`: whole entries, `Next` moved up,
  an entry larger than `limit`.
- the `AddressPage` row in `docs/modules/watch.md`
