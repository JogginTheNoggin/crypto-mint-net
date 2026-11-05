# BLUE-029: An unreadable entry body is not held, never a crash; queries say what they could not read; the cache counts real bytes

| Field | Value |
| --- | --- |
| Fixes | RED-084 (high) |
| Branch | `claude/project-thread-ggo8dt` (PR against `main`) |
| Base | `main` @ `705639a` (Source Round 28 Part C), with ORANGE-038 (#201) merged in |
| Decisions | D173 |
| With | ORANGE-038 (PR #201), carried: it changes `pkg/ledger/pool.go` and the chase walk that this fix also touches. Its proposal 2 (count real bytes, 256 MiB default) is built, as jerry approved. Its proposal 1 (serve outside the engine lock) is left for Grey. Not carried: Green #200 (no overlap except the doctor's column width, which this PR does not touch). |
| Review | No consensus change. For Black: (1) no verdict reads a body (D172); an unreadable body is now missing information for peers and queries, which is what it always was for selection. (2) `Rivals` reads the skeleton's items, which are the same transactions without witnesses; a rival is found by its inputs only. (3) The recover in the work loop closes a peer, and does not count it as an offence. |

## Summary

- `Pool.Entry(h)` returns `(*entry.Entry, error)`, so `e, _ :=` cannot
  hand a caller a nil entry to use.
- p2p: `entries` lists an unreadable entry as not held, `parts` replies
  `not held`, an extension ends before it, and nothing is encoded.
- `Rivals` searches the skeleton's items, so it is always complete and
  reads no body. `Payment` says `unreadable` and still finds rivals.
- watch ledger and address pages count `unreadable` entries.
- A panic while handling one peer message closes that peer only.
- `poo doctor` warns (`bodies`).
- The body cache counts byte form plus 1 KiB, and `entry_cache_mib`
  defaults to 256.

## Consensus

None.

## Tests

- `pkg/p2p/blue029_test.go`: `TestDamagedLogDoesNotCrashServing`
- `pkg/node/blue029_test.go`: `TestDamagedLogQueriesSayIncomplete`
- `pkg/ledger/bodies_test.go`: `TestBodyCacheBound` (a failed read returns `ErrUnreadable`)
