# RED-017: Every parts request re-encodes the whole entry item while the engine lock is held

| Field | Value |
| --- | --- |
| Severity | medium |
| Status | fixed |
| Found on | `claude/m10-watch` @ 0fd59fa (code e99e3e4, Source Round 7 Part B) |
| Component | pkg/p2p/serve.go (replier.parts, replier.build), pkg/wire/messages.go (EncodeItem) |
| Surface | peer |
| Fixed by | `claude/project-thread-ggo8dt` @ 58e4e42 (BLUE-008) |

## Summary

`replier.build` "reads what the reply needs under the engine's lock and
returns a function that sends it". `parts` does more than read:

```go
// pkg/p2p/serve.go, parts (runs inside build, so under the lock)
b := wire.EncodeItem(item(l, q.Entry, q.Variant == wire.EntriesWithLayers))
total := uint64(len(b))
...
return func() { ... slices of b ... }
```

It encodes the **whole** item (the entry and all its layers' nonces) to
answer a request that asks for a few kilobytes of it, and it does so for every
request, including each request of a fetch that goes on from an offset. A
peer that asks for the last bytes of a large selected entry over and over
costs the node one full encoding per request, with the global engine lock held,
for a reply of one chunk. The cost to the peer is a request of about 100 bytes;
the budget is 600 requests a minute per address (`KindRequest`).

## Reproduction

A temporary test in `pkg/wire` (not committed): one transaction with many
outputs in an entry, then `wire.EncodeItem`:

```
item  2.0 MB   EncodeItem 13.8 ms
item  8.0 MB   EncodeItem 62.7 ms
item 32.0 MB   EncodeItem 133.9 ms
```

Anything above `PartsOver` (16 MiB) is served in parts, so at least about 90 ms
per request; at 600 requests a minute that is a minute of lock a minute for
one address. The entry has to be in the *selected* ledger (`InSelected`); it
can be the attacker's own: an entry of that size needs a few valid
transactions with many outputs and one layer of the series' minimum strength.

## Impact

Medium. Remote and unauthenticated for the request, but it needs a large
entry in the node's selected ledger. While one exists, anyone can hold the
engine lock for most of every minute per address (several addresses add up),
which stalls every other peer, the RPC and the chase (the RED-006/008 class).

## Recommendation

- Do the encoding **outside the lock**: copy the entry pointer (entries are
  immutable) and the layers' nonces under the lock, and encode in the returned
  closure.
- Encode **only the requested range**, or cache the encoded item per
  (entry, variant) for a short time (a small LRU by bytes) so a fetch in many
  requests encodes once.
- Charge a parts request to the asker in proportion to the bytes it makes the
  node encode (`Cost`/`Request` budget), not as one request.

Tests: a benchmark of `parts` on a 32 MB item must not hold the engine lock
for more than a copy; two parts requests of one item encode it once.

## Resolution

Fixed by BLUE-008 (decision D120). `replier.parts` copies the item under
the engine lock: the entry pointer, since entries are immutable, and the
nonces. It encodes the item in the returned function, outside the lock,
through a cache keyed by entry and variant. An item stays cached for one
minute after its last use, up to 256 MiB in all, and the newest is always
kept. A fetch in many requests therefore encodes the item once, and sees
one total even if the entry gains layers meanwhile. Test:
`TestPartsCacheEncodesOnce` (`pkg/p2p`). Charging parts requests by
bytes encoded was not needed once the encoding was cached.
