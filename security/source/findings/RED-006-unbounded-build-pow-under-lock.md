# RED-006: `/v1/build` runs unbounded, uninterruptible entry proof-of-work while holding the global node lock

| Field | Value |
| --- | --- |
| Severity | medium |
| Status | fixed |
| Found on | `claude/m10-watch` @ a226c79 |
| Component | pkg/rpc/rpc.go (build, `BuildRequest.MaxTries`), pkg/engine/engine.go (Build), pkg/entry/entry.go (Build), pkg/node/node.go (Build) |
| Surface | local RPC |
| Fixed by | `claude/project-thread-ggo8dt` @ 16b68ed (BLUE-003) |

## Summary

`POST /v1/build` takes `max_tries` and passes it straight through with no
upper bound, and `0` means "try forever":

```go
// pkg/rpc/rpc.go
type BuildRequest struct {
    ...
    MaxTries uint64 `json:"max_tries,omitempty"`
}
func (h *Handler) build(...) {
    ...
    en, res, err := h.n.Build(req.key(), req.MaxTries)   // no cap, 0 allowed
```

```go
// pkg/entry/entry.go, Build — when the series sets a minimum entry strength
min := s.MinEntryStrength()
for nonce := uint64(0); maxTries == 0 || nonce < maxTries; nonce++ {
    h := HashOf(s, parent, tids, nonce)
    if h.Strength() >= min { ... return }
    if nonce == ^uint64(0) { break }
}
```

With `maxTries == 0` the loop runs up to 2^64 iterations. The whole search
runs **while two locks are held**: `node.Build` takes `n.mu` for its entire
body, and `engine.Build` takes `e.mu` and does the proof-of-work before
unlocking:

```go
// pkg/engine/engine.go, Build
e.mu.Lock()
...
en, h, err := entry.Build(p.s, parent, txs, maxTries)   // PoW under e.mu
...
e.mu.Unlock()
```

```go
// pkg/node/node.go, Build
n.mu.Lock()
defer n.mu.Unlock()
...
e, res, err := n.eng.Build(k, maxTries)                 // PoW under n.mu too
```

There is no `context` and no cancellation, so once started the search cannot
be stopped short of killing the process.

Contrast `/v1/work`, where the same kind of search *is* bounded:

```go
// pkg/rpc/rpc.go
const MaxWorkTries = 1 << 24
func (h *Handler) work(...) {
    if req.Tries == 0 || req.Tries > MaxWorkTries {
        writeError(w, badRequest("tries must be 1 to %d", MaxWorkTries))
        return
    }
```

The default makes this easy to hit by accident: `poon build` defaults
`-max-tries` to `0` ("0: no limit", pkg/cli/node.go), so the ordinary
`poon build` command on a series that sets a minimum entry strength starts an
unbounded search.

## Reproduction

1. Run a node for a series that sets a non-trivial `MinEntryStrength` (so a
   nonce meeting it is rare), with at least one pending transaction so
   `Build` has something to assemble.
2. Call `POST /v1/build {"series":…,"pool":…}` with no `max_tries` (or run
   `poon build`).
3. The handler enters `entry.Build`'s loop and spins the CPU searching for a
   qualifying nonce, holding `n.mu` and `e.mu` the whole time.
4. While it runs, every other endpoint that needs the node or engine lock —
   `/v1/status`, `/v1/metrics`, `/v1/ledger`, `/v1/tx`, submits — blocks, and
   the node stops processing peer items through the engine. The only way out
   is to kill the process.

## Impact

A single trivial request (the default `poon build`, or a GUI "build" action
that omits `max_tries`) wedges the **whole node**, not just the caller: the
engine lock is held, so peer-item handling, forwarding and chasing stall too,
and the control API stops answering. It is reachable only with the token
(local owner / their tools / the GUI, which runs the same CLI and RPC per the
project's hard rule), so it is a local / operator foot-gun and availability
bug rather than a remote one — hence medium, not high. But because it freezes
the node's peer-facing processing for as long as the search runs and cannot
be cancelled, its blast radius is the whole node.

## Recommendation

- Bound `max_tries` on `/v1/build` the way `/v1/work` bounds `tries` (reject
  `0` or clamp to a maximum), and change the `poon build` default from `0`
  to a finite number with a documented meaning.
- Do the proof-of-work **outside** the node and engine locks: assemble the
  transaction set under the lock, release it, search for the nonce, then
  re-acquire to insert — so a long search never freezes peer processing or
  the control API.
- Thread the request `context` into `Build`/`Work` so a client disconnect or
  a shutdown cancels the search.

Test: a `pkg/engine`/`pkg/node` test that `Build` with a large/zero
`maxTries` on a hard-strength series does not hold the lock for the search
(e.g. a concurrent `Status` call returns promptly), and an `rpc` test that
`/v1/build` rejects or clamps an unbounded `max_tries`.

## Resolution

Fixed by BLUE-003 (decision D99), all three recommendations. `/v1/build`
caps `max_tries` at `MaxBuildTries` (2^24; 0 means the cap, more is a bad
request) and passes the request's context. The search runs outside both
locks: `Engine.BuildEntry` reads the pool under the engine lock and
releases it before searching, and `Node.Build`/`BuildContext` take the
node lock only to store the result. `Node.Work` likewise grinds before
locking. `entry.BuildContext` checks the context every 4,096 nonces.
`poon build -max-tries 0` now means the node's limit. Tests:
`TestBuildContextCancels` (`pkg/entry`), `TestBuildSearchHoldsNoLock`
(`pkg/node`: a transaction goes in promptly during an endless search,
and cancelling ends it), and an unbounded `max_tries` rejected in
`pkg/rpc`.
