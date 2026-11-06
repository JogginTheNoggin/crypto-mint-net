# BLUE-003: Bounded, cancellable builds that hold no lock while searching

| Field | Value |
| --- | --- |
| Fixes | RED-006 (medium) |
| Branch | `claude/project-thread-ggo8dt` |
| Base | `claude/m10-watch` @ `5ea6828` |
| Decision | D99 |

## Summary

A build request with no `max_tries`, on a series with a high minimum
entry strength, used to search for up to 2^64 nonces while holding the
node's and the engine's locks, which froze the RPC and all peer
processing until the process was killed. Now the request is capped at
2^24 nonces, the search runs with neither lock held, and it stops when
the client goes away. Layer grinding (`Work`) also no longer holds the
node's lock. No byte form, wire or consensus change.

## What changed

- `pkg/entry`: `BuildContext(ctx, …)`, which checks the context every
  4,096 nonces; `Build` calls it with a background context.
- `pkg/engine`: `BuildEntry(ctx, k, maxTries)` reads the ready
  transactions and tip under the lock, then searches without it, and
  reports `ErrAlreadyBuilt` if the result is already held. `GrindLayer`
  is the same for layers. `Build` and `Work` are these plus
  `HandleEntry`/`HandleLayer`, with the same signatures as before.
- `pkg/node`: `BuildContext`; `Build` and `Work` search first and then
  take the result in through `HandleEntry`/`HandleLayer`, so storing and
  events keep their order.
- `pkg/rpc`: `MaxBuildTries` (2^24); 0 means it, more is
  `bad_request`; the build passes `r.Context()`.
- `pkg/cli`: `poon build -max-tries` help says 0 is the node's limit.
- Docs: D99, `docs/modules/rpc.md`, `engine.md`, `node.md`.

## Tests added

- `pkg/entry`: `TestBuildContextCancels`.
- `pkg/node`: `TestBuildSearchHoldsNoLock`: during an endless search a
  transaction goes in within a second, and cancelling ends the search
  with `context.Canceled`.
- `pkg/rpc`: `max_tries` of `MaxBuildTries + 1` is a bad request.

## How Grey can check it

```sh
go test -race -run 'BuildContext|BuildSearch' -v ./pkg/entry ./pkg/node
go test -race ./pkg/rpc ./pkg/engine ./pkg/sim ./pkg/cli
```
