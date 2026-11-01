# RED-020: A layers-from or parts request copies, sorts and hashes all of an entry's layers under the engine lock, for a reply of a few bytes

| Field | Value |
| --- | --- |
| Severity | medium |
| Status | fixed |
| Found on | `claude/m10-watch` @ ae8a103 (Source Round 8 Part B) |
| Component | pkg/p2p/serve.go (`layersFrom`, `parts`, `item`, `strongestFirst`), pkg/ledger/pool.go (`Layers`) |
| Surface | peers (any peer that can send a request) |
| Fixed by | `claude/project-thread-ggo8dt` @ 69aaf8a (BLUE-009) |

## Summary

`replier.build` runs under the engine's lock (`Engine.Ledger`). For a
layers-from request (wire type 27, D121) `layersFrom` calls `l.Layers(entry)`
(copy of every layer and a sort by nonce) and then `strongestFirst`, which
hashes every layer once to get its strength and sorts them again. All of that
happens before the reply function is returned, so under the lock, and it is
done again for every page. The cost depends on how many layers the entry
holds, not on the page asked for: a request for the last index (`From` =
total - 1) is answered with 28 bytes and costs the same as one for the first
page. A parts request does the same: `parts` calls `item()`, which copies and
sorts all the layers of the entry, before it looks at `Offset` (RED-017 moved
the encoding out of the lock, not this copy), and an offset past the end is
refused with an empty reply.

An entry with many layers is the case D121 added the paging for, and it is
cheap to make: a series may set `min_layer_strength` to 0 (the default of
`poo series create`), where every nonce is a layer. Any grinder's entry gets
there as well.

## Reproduction

A temporary test in `pkg/p2p` (not committed): a node `a` with two entries,
150,000 layers added to the tip, and a raw peer (`dialRaw`) that asks:

```
layersfrom From=149999 : 208-245 ms, 1 chunk,  28 bytes
layersfrom From=0      : 225-243 ms, 1 chunk, 1,200,020 bytes
parts-request, Offset=1<<40 : 83 ms and 97 ms, refused, 0 bytes
hash-chain-request          : 0.18 ms
```

Eight seconds of tiny layers-from requests from the one connection, with an
honest `Engine.Ledger()` call made every millisecond next to it:

```
answered 41 requests in 8 s (5 a second, under the 600 a minute request budget)
honest Ledger() calls: 42, mean wait 190 ms, worst 247 ms
```

So one connection holds the lock for nearly all of the time, and what an
honest caller (the chase, a push, a transaction, the RPC) waits for is the
cost of the sort. The cost grows with the layers: 1.5 million would be about
2 s a request, and the request budget allows ten a second from one address
and the same again from every other.

## Impact

Medium. Every peer can take the engine lock for as long as it likes at the
price of an ordinary request, with no work shown, which blocks the node's
validation, forwarding and RPC the way RED-011 and RED-017 did. It needs an
entry with many layers on the victim's selected ledger, which an attacker
can make at the cost of the layers (2^min_layer_strength hashes each, nothing
at strength 0) and which a busy honest ledger can also have.

## Recommendation

Do not do work that depends on all of an entry's layers while the lock is
held, and do not make a request's cost depend on a number the reply does not
show. No limit and no refusal is needed:

- `layersFrom`: under the lock copy only the nonces (a map walk), and do
  the hashing and the sort in the returned function. Better, keep per entry a
  strongest-first order built once and reused by every page of a fetch, keyed
  by the entry and its layer count, as `partsCache` does for encoded items.
  The order only has to be the one a paging asker sees; a page that moves
  because layers arrived is already allowed by the wire description.
- `parts`: look the cache up before `item()`, so a request that the cache
  can answer (or an offset past its total) does not touch the pool; build the
  copy outside the lock from a copy of the nonces.
- Charge the request to the sender's request budget by the number of layers
  it examined (a cost, as `EntryCost` is for entries), not as one request.

Test: 150,000 layers on one entry; a second goroutine takes `Engine.Ledger`
every millisecond while a raw peer sends 20 layers-from requests for the last
index. The worst wait is a few milliseconds, not 200.

## Resolution

Fixed by BLUE-009 (decision D123). Under the engine's lock a
layers-from reply now only counts the entry's layers (`Pool.LayerCount`)
and, unless the order for that count is cached, copies the nonces
unsorted (`Pool.LayerNonces`, a map walk). The hashing and the
strongest-first sort (`strongestNonces`) run in the reply closure, outside
the lock. The order is cached in the host's `partsCache` for a minute
under the entry and its layer count, so the pages of one fetch sort once
and a new layer makes a new order. A parts reply copies the entry and its
unsorted nonces under the lock (or nothing, when the item is cached) and
sorts and encodes outside it. A layers reply counts the layers before it
copies any. Test: `TestLayerOrderIsCachedByCount` (`pkg/p2p`, internal);
`TestServeManyLayers` and `TestManyLayersOnOneEntryAreCounted` still pass.
