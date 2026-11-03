# RED-049: D152 made `node.Open` cheap for a series of very many pools, but a rejected item still makes a pool's state, and `p2p.New` still keeps and walks every pool

| Field | Value |
| --- | --- |
| Severity | medium |
| Status | open |
| Found on | `main` @ 4240d32 (Source Round 19 Part B) |
| Component | pkg/engine/engine.go (state made on first item), pkg/p2p/host.go (`New`, `sortedPools`), pkg/p2p/discovery.go, pkg/p2p/peer.go |
| Surface | a series from a bundle (RED-046), a peer's items |
| Fixed by | - |

## Summary

RED-046 was that a series of very many pools, followed whole, costs the node
time and memory for every pool, and a default import carries one. D152 fixed
`node.Open` (it lists the stores on disk), the listings and the plan. Two
things are left, and both keep the cost tied to the pool count:

1. **p2p.** `p2p.New` calls `EachFollowedPool` and makes a `poolInfo` (with
   its section) and two map entries for every pool of a whole follow. Each
   new peer then runs `sortedPools()` (build and sort all of them) and spawns
   one `askWeight` per covered pool, and so does every check tick. The cost is
   in the pools, as before, now in a different place.
2. **State on a rejected item.** D152 says a pool's state "is made when its
   first item or request arrives". It is made by an item that is then
   rejected: `Engine.HandleEntry` makes the pool before the entry is
   checked. So anything that can hand the node an item for a followed pool
   makes that pool's state, kept for good, and a pool's state is about 1.4 KiB.

## Reproduction

`TestRedHugeSeriesP2PNew` (scratchpad `red_r19b_p2p_test.go.txt`, goes in
`pkg/p2p`): a series of n pools followed whole, `node.Open` and then
`p2p.New`:

```
65536 pools:   node.Open 5ms, p2p.New 57ms,   heap after 31 MiB
262144 pools:  node.Open 8ms, p2p.New 541ms,  heap after 100 MiB
1048576 pools: node.Open 3ms, p2p.New 2.29s,  heap after 376 MiB   (about 360 bytes a pool)
```

An internal test of `sortedPools` over 1,048,576 pools took about 0.5 s per call.
At 4M pools `p2p.New` would keep about 1.5 GiB and at 2^32 pools it never
returns, the same wall as RED-046 with p2p switched on (the usual way to run
a node).

`TestRedStatePerPoolOfRejectedItems` (scratchpad `red_r19b_node_test.go.txt`,
goes in `pkg/node`): a series of 2^40 pools followed whole, 100,000 items
(an entry with an unknown parent) to 100,000 different pools through
`n.HandleEntry`:

```
100000 rejected items to 100000 distinct pools: 737ms, heap +137 MiB,
pools with state 100000, last err: ledger: parent not held
```

Every one was rejected, and every pool now has state, listed by `status`.
I called `HandleEntry` directly; I did not send the items over a socket.
`HandleEntryFrom` is what the peer path calls, and it goes through the same
engine call.

## Impact

For series of ordinary size nothing changes (a thousand pools is 1.4 MB).
For a series with huge parameters (RED-046's bundle case, or a series
published for that purpose) the node still cannot run with p2p on, and
once it runs, whoever can send it items for many pools grows its memory
without being accepted. Each such item may also count against the sender
(unknown parent, not held), so a single peer is limited; many are not.

## Recommendation

1. Make the state when an item is **kept**: look up the pool without making
   it, run the checks against a scratch pool or the genesis, and insert the
   pool on success (or on a stated, bounded number of failures per sender).
2. In p2p, keep `poolInfo` only for pools in use, as the engine now does:
   derive a pool's section and id from the series and index on demand
   (`poolByName` already parses), and let `sortedPools` and the check loop
   walk the pools that hold state or that a priority peer has been asked
   about, not every pool of the series. A pool nobody has used has nothing to
   check.
3. As for RED-046, no cap on the series: it is consensus data, and (1) and
   (2) need none.

For Black: nothing here changes what is valid. (1) must not change which
entries a node keeps: an entry that is accepted today still is, only the
pool's state is made later.

## Also read, not run

Each check tick starts one goroutine and one request per pool and priority
peer, with no guard for a request still outstanding. At the default (60 s
check, 30 s request timeout) they do not overlap; with a small
`check_seconds` and a peer that never answers, up to
`RequestTimeout / check_seconds` rounds are outstanding at once. D153 raised
the longest interval but did not set a shortest.

## Resolution

Open.
