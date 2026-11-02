# RED-008: `/v1/address` walks the whole ledger per request, under the engine lock, reachable with the read-only view token

| Field | Value |
| --- | --- |
| Severity | medium |
| Status | fixed |
| Found on | `claude/m10-watch` @ f18a76e |
| Component | pkg/watch/query.go (AddressOf), pkg/engine/engine.go (Ledger), pkg/rpc/view.go (address), pkg/rpc/rpc.go (view token) |
| Surface | local RPC (read-only view token) |
| Fixed by | `claude/project-thread-ggo8dt` @ 78cea9c (BLUE-004) |

## Summary

The address view (`GET /v1/address`) walks a pool's entire ledger, tip to
height 1, for **every** followed pool, with no per-request work bound. The
`limit` parameter only truncates the result slice *after* the whole walk:

```go
// pkg/watch/query.go, AddressOf
for _, k := range e.FollowedPools() {
    ...
    e.Ledger(k, func(p *ledger.Pool) {
        for h := p.Height(); h >= 1; h-- {        // whole chain, every call
            eh := p.Chain(h, h)[0]
            en, _ := p.Entry(eh)
            for _, t := range txsOf(s, en) {
                if !f.Match(t.Subject(s, "")) { continue }
                ... out.History = append(out.History, it)
            }
        }
    })
}
...
if len(out.History) > limit {                      // truncation is post-walk
    out.History = out.History[:limit]
}
```

The whole walk runs **while holding the engine lock**, which is a single
`sync.Mutex`, not an `RWMutex`:

```go
// pkg/engine/engine.go
func (e *Engine) Ledger(k PoolKey, fn func(*ledger.Pool)) error {
    e.mu.Lock()
    defer e.mu.Unlock()
    ...
    fn(p.ledger)      // fn is AddressOf's whole tip→1 loop
    return nil
}
```

So one `/v1/address` request does `O(total ledger height × followed pools)`
work holding `e.mu` for the whole time, with no `context` to cancel it.
Every other operation that needs the engine — peer-item handling,
forwarding, chasing, `/v1/status`, `/v1/tx` submits, the other views — blocks
behind it for the duration.

The sibling paged view does **not** do this. `LedgerPage` deliberately
bounds its walk, with a comment saying why:

```go
// pkg/watch/query.go, LedgerPage
// Look at a bounded number of entries per page, so a filter that
// matches nothing cannot make one request walk the whole ledger.
looked := 0
for ; h >= 1 && len(out.Entries) < limit && looked < 50*limit; h-- {
```

`AddressOf` is the same kind of query (walk the chain, keep the entries a
filter selects) with that bound left out, so the exact case `LedgerPage`
was hardened against is open on the address path.

Finally, `/v1/address` is a `GET`, so it is reachable with the **read-only
view token** (D100), not only the full control token. `ServeHTTP` admits a
GET/HEAD carrying the view token, and `viewRoutes` maps `GET /v1/address`
to this handler. The view token is the credential an operator hands to a
dashboard, the GUI, or a semi-trusted reader precisely so they cannot
*change* anything — but it still lets them wedge the engine.

## Reproduction

1. Run a node that follows one or more pools and let it build a tall ledger
   (any mature node; height in the thousands is enough to be noticeable,
   and it grows without bound over a node's life).
2. With only the **view token**, issue
   `GET /v1/address?series=<id>&id=<any 32-byte address>` — any address
   value works; a non-matching filter just means the walk appends nothing,
   so it still visits every height.
3. The handler calls `AddressOf`, which takes `e.mu` and loops from the tip
   to height 1 of every followed pool before returning. For the whole walk,
   a concurrent `/v1/status`, a transaction submit, and the node's own
   peer-item processing (which needs the engine) block.
4. Repeating the request back-to-back (or pointing several view-token
   clients at it) keeps `e.mu` held and starves peer processing and the
   control API — a sustained stall from a read-only credential.

A `pkg/watch` test that demonstrates the gap today: build a pool with a
tall ledger, call `AddressOf(..., limit=10)`, and assert it visited every
height (e.g. via an instrumented pool or by timing) even though at most 10
items are returned — i.e. `limit` did not bound the work. Contrast
`LedgerPage` with the same ledger, which stops after `50*limit` entries.

## Impact

Medium. It is an availability bug on the local RPC, like RED-006, so not
remotely reachable — but two things make it worse than a plain operator
foot-gun:

- **Lower privilege.** It is reachable with the read-only view token, whose
  whole purpose (D100) is to grant look-but-don't-touch access. A reader who
  is trusted only to *view* can freeze the node's engine, including its
  peer-facing processing, which the view token was never meant to allow.
- **Ordinary data triggers it.** Unlike RED-006, which needed a series with
  a hard minimum strength, this needs only a tall ledger, which every
  long-running node has. No crafted input is required; a normal address
  lookup on a busy node already does the full walk.

No value is stolen or forged and nothing is written, so it is a
denial-of-service, not a compromise — hence medium, not high. But the blast
radius is the whole engine for as long as the walk (or a stream of them)
runs, and the triggering credential is the least-privileged one the node
hands out.

## Recommendation

- Bound the address-history walk the way `LedgerPage` bounds its own: stop
  after a fixed number of entries looked at per request (e.g. `50*limit`),
  and return a `Next`/cursor so a caller can ask for the next, older page
  explicitly. Then `limit` bounds the work, not just the output.
- Thread the request `context` into `AddressOf` (and the other view walks)
  so a client disconnect or shutdown cancels an in-progress walk instead of
  pinning `e.mu` to the end.
- Consider serving these read-only walks against a snapshot or under a
  read-lock path that does not hold the single engine `Mutex` for an
  unbounded span, so a slow read query cannot starve peer processing and
  writes. (A broader change; the per-request bound above is the minimal fix
  and matches what `LedgerPage` already does.)
- Audit the other view walks for the same shape: `Holdings`/`unspent` and
  `Pending` iterate live state (bounded by the unspent/pending set) rather
  than the full chain, so they are far less severe, but they also run under
  `e.Ledger`'s lock and have no `context`.

Test: a `pkg/watch` test that `AddressOf` looks at a bounded number of
entries regardless of ledger height (mirroring `LedgerPage`), and an `rpc`
test that a long address lookup does not hold the engine (a concurrent
`Status` returns promptly), paralleling `TestBuildSearchHoldsNoLock` from
BLUE-003.

## Resolution

Fixed by BLUE-004 (decision D103). `watch.AddressPage` looks at no more
than 50×`limit` entries per request across all pools, stops a pool after
`limit` matches, starts from `below` (0: the tips), and returns `next`
(pool and `below`) for each unfinished pool. It checks the request's
context before each pool and every 256 entries. `/v1/address` and `poo
address` take `below`; `AddressOf` and `Client.Address` keep their
signatures. A snapshot or read lock for views was not added: with the
bound, one request holds the lock for at most 50,000 entries. Tests:
`TestAddressPageIsBounded` (`pkg/watch`: on a 120-entry ledger, pages of
50 entries for an address with no history, a page with history stops at
limit, cancelling stops it), and a `below` case in `pkg/rpc`.
