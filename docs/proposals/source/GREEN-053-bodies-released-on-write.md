# GREEN-053: Drop an entry's body when it is written, and keep doctor's columns in line

| Field | Value |
| --- | --- |
| Status | proposed (prototyped) |
| Based on | `main` @ `705639a` |
| Area | `pkg/node` (`HandleEntryFrom`), `pkg/cli` (`doctorCmd` text output) |
| Effort | small |
| Consensus | none. Where a body's bytes live, and the spacing of `poon doctor`'s text. No verdict reads a body that is not in memory (D172), so releasing it sooner changes what a node costs, never what it decides. The differential tests (`TestBodiesFromDisk`, the ledger's twin pool) still pass |
| Decided by | Grey. Black may look, since D172 is the area, but nothing selected or held changes |

## Summary

Green ran a node on the example series, took in a few entries, and read
`poon status` before and after a restart.

1. **The newest body of each pool stayed outside the cache.**
   - While running: `bodies` showed `held 2, in_memory 2, located 2, used_bytes 0`. Both entries were on disk and could be read back, yet both bodies were still in memory, and the cache was empty.
   - After a restart: `in_memory 0` and `used_bytes 2512`.
   - `Pool.releaseBodies` runs at the start of each `AddEntry` and at the end of start-up. A just-written entry is released only by the pool's next entry, so every pool keeps its newest body in memory, outside `entry_cache_mib`, for as long as it is quiet.
   - On a node following the reserve series' 1,024 pools, that is up to 1,024 bodies the cache bound does not count. With large entries it can reach hundreds of MB, against a 512 MiB default bound.
2. **`poon doctor`'s columns broke on `open-files`.** The name column was a fixed 8 letters, so D172's `open-files` check pushed its detail out of line with every other check and fix.

Both are fixed on the branch.

The rest of D172 behaved as documented:

- `entry_cache_mib` is listed with `in_place: true`, and `-set entry_cache_mib=1` applied in place.
- `file_limit` and the `open-files` check read 20,000 against 258 needed.
- Replay reported 8 records over 2 pools.

## Idea

- **Release on write.** `Node.HandleEntryFrom`, right after `record` has written the entry, calls `n.eng.Ledger(k, p.ReleaseBodies)`. The body moves into the bounded cache (`releaseBodies` puts it there), so forwarding the entry or serving it to a peer reads it from the cache, not the disk.
  - Lock order: `n.mu`, then the engine's lock. That is the order `HandleEntryFrom` already takes them in when it calls `eng.HandleEntry`.
  - Every entry, local or from a peer, goes through `HandleEntryFrom`, since `Build` uses `HandleEntry`.
- **Doctor columns.** The text output's name column is as wide as the longest check name, at least 8. A fix line starts under the detail column, as before.

## How it was tested

- **`TestEntryBodyReleasedOnWrite`** (`pkg/node`) runs the fixture's history live and checks `in_memory 0`, with every held body located and read back.
  - On `main` it fails with `Held:3 InMemory:1`.
- **`TestDoctorColumnsLineUp`** (`pkg/cli`) checks that every detail and every `fix:` starts at one column, with `open-files` present.
  - It fails on `main`.
- **Checks run locally:** gofmt, go vet, staticcheck, `go test -race ./...` (including D172's differential tests), and the nine GUI e2e scripts.

## Ideas for later

- `poon status`'s `bodies.bodies` is the number of bodies in the cache. Next to `held` and `in_memory` it reads as the total. `cached` would say what it is, but it is in the JSON the GUI reads, so this is left to Grey.
- `/v1/metrics` could carry the open file limit beside `file_limit` in status, for a dashboard to alert on.
