# RED-050: taking one pool off a whole-series follow of more than 4096 pools writes every other pool out, and `p2p.New` then refuses to start

| Field | Value |
| --- | --- |
| Severity | medium |
| Status | fixed |
| Found on | `main` @ 4240d32 (Source Round 19 Part B) |
| Component | pkg/nodepolicy/follow.go (`RemoveFollow`), pkg/p2p/host.go (`New`, the 4096 coverage check) |
| Surface | local operator (`poo unfollow -pool`, the GUI, `POST /v1/follow`) |
| Fixed by | `claude/project-thread-ggo8dt` @ a38ca97 (BLUE-020) |

## Summary

A series followed whole is one follow entry with no pools. `RemoveFollow`
of a single pool "first writes it out pool by pool": it lists every other pool
into `Follow.Pools`. For a series of 5000 pools that is 4999 pool numbers.
`p2p.New` then builds the node's coverage: a whole series is one section, but a
series followed by listed pools gets one section per pool, and above 4096 it
refuses: `p2p: following 4999 separate pools; follow whole series instead`.
So a routine unfollow of one pool makes the node unable to start (or reload)
with p2p on; the only way back is to follow the whole series again or edit
node.json by hand.

## Reproduction

`TestRedUnfollowOnePoolOfBigSeries` (scratchpad `red_r19b_p2p_test.go.txt`):

```
4000 pools followed whole: p2p.New err=<nil>
4000 pools, pool 7 unfollowed (3999 pools listed): p2p.New err=<nil>
5000 pools followed whole: p2p.New err=<nil>
5000 pools, pool 7 unfollowed (4999 pools listed): p2p.New err=p2p: following 4999 separate pools; follow whole series instead
```

Time and memory of the write-out itself are linear and small (4M pools: 580 ms,
383 MiB), so this is about the 4096 limit, not the speed.

## Impact

"All but one pool" is a normal thing to want, and the node cannot hold it
beyond 4096 pools. The failure is at the next start or reload, not at the
unfollow, so the person who made the change sees success and the node later
fails to come up. It also makes a follow that is a node policy choice into a
node that cannot run at all.

## Recommendation

Say "all but these" in the follow itself (a whole follow with an `except`
list), or build the coverage section as a series section with an exclusion;
and check the 4096 coverage limit when the change is made (the
`RemoveFollow`, the RPC and the import all call `Validate`), so the unfollow
is refused with the reason instead of written. The limit is a node-policy
limit on how many separate sections the node announces; it touches no
validity rule, but please read it against the no-caps rule and have Black
confirm.

## Resolution

Fixed (D154), both recommendations.

- A follow can say "all but these": `Follow.Except`, with a whole series.
  `RemoveFollow` of pools from a series followed whole puts them on it
  instead of writing out every other pool. When fewer pools are left than
  are taken off, the pools left are listed instead.
- `AddFollow` of a pool, or of a mint of an excepted pool, takes it off
  the list. Two follows join into what either follows (`joinFollow`), in
  both `AddFollow` and the import's `addFollows`.
- The engine follows it with `FollowSeriesExcept`, and p2p announces a
  whole series, with or without `except`, as one series section.
- A follow change that would need more than `MaxSections` (4096) separate
  sections, the wire's coverage limit, is refused when it is made, with
  the way out ("follow the whole series and take the pools you do not
  want off it"). Before, the node failed at its next start.
- This limits only how a follow list is written. Any set of pools can
  still be followed, and nothing about ledgers changes; Black to confirm.

Tests: `TestUnfollowPoolsOfAWholeSeries`,
`TestUnfollowMostPoolsListsTheRest` (pkg/nodepolicy),
`TestFollowSeriesExcept` (pkg/engine), `TestAllButOnePoolStarts` (pkg/p2p,
Red's 5000-1 case).
