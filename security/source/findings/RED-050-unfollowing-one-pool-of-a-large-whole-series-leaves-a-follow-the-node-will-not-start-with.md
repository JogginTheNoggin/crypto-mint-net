# RED-050: taking one pool off a whole-series follow of more than 4096 pools writes every other pool out, and `p2p.New` then refuses to start

| Field | Value |
| --- | --- |
| Severity | medium |
| Status | open |
| Found on | `main` @ 4240d32 (Source Round 19 Part B) |
| Component | pkg/nodepolicy/follow.go (`RemoveFollow`), pkg/p2p/host.go (`New`, the 4096 coverage check) |
| Surface | local operator (`poo unfollow -pool`, the GUI, `POST /v1/follow`) |
| Fixed by | - |

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

Open.
