# ORANGE-030: Round 20 measured; read an unused pool without making its state

| Field | Value |
| --- | --- |
| Status | proposed |
| Based on | `main` @ `522e891` |
| Area | `pkg/engine` (`Status`, `Tip`, new `unstated`) |
| Effort | one function and a test |
| Decided by | Grey (for the record) |

## Summary

I measured what Rounds 19 and 20 changed in the engine and the p2p
host: D154 (a pool's state is kept only once an item leaves something
in it, and reads use a fresh state that is not kept) and D155 (settings
export and import). I compared against `172866d`, which is main with
ORANGE-029 and before D154.

**D154 traded memory for work on every read.** Before, the first status
call made and kept a state for every pool. Now each read of a pool with
no state builds a fresh ledger and mempool and throws them away. For one
series of 65,535 pools followed whole (just under `ListAll`, so every
pool is listed), reading `Engine.Status` for every listed pool measured:

| Every pool's `Status`, 65,535 pools | Time per pass | Allocated per pass | Kept |
| --- | --- | --- | --- |
| `172866d`, first pass | 344 ms | 119 MiB | +92 MiB |
| `172866d`, later passes | 16–44 ms | 13 MiB | none |
| `522e891` (D154), each pass | 54–81 ms | 105 MiB (1.7 KB a pool) | none |
| This change, each pass | 28–33 ms | 20 MiB | none |

`Engine.Tip` on a pool with no state went from 0.13 µs to 0.96 µs.
With this change it is 0.25 µs.

`/v1/status`, `/v1/metrics`, `watch` queries and the grind targets all
read `Status` for each listed pool. So each status call and each
metrics scrape made about 100 MiB of throwaway state.

**The fix.** A fresh state always reads the same way: its tip is the
pool id, at height 0 and weight zero, with no entries, branches,
pending or waiting transactions. D154 already relies on this ("every
pool's state starts the same"). So `Status` and `Tip` now answer a
followed pool with no state from its pool id, and build nothing.
- Errors are the same as before: an unknown series or a pool not
  followed.
- Every other `peek` caller (pre-verifying items, `EntryCost`,
  `WarmStored`) is about an item, so it still uses a fresh state.

End to end the gain is smaller, because the reply itself dominates.
`/v1/status` for 65,535 pools is a 16 MiB JSON reply. Each call
allocated:
- 220–327 MiB on `172866d`
- 313 MiB on `522e891`
- 228 MiB with this change

The time stays at 0.2–0.3 s and varies more between runs than between
commits. The other bytes are JSON encoding (52%) and the hex of every
hash, which this change does not touch. `/v1/metrics` is a 47 MiB text
reply for this series. The same shape applies, and I am not proposing a
change to it: it is read per pool by design.

**No regression elsewhere.**
- `BenchmarkHandleTxParallel` measured 21.1–23.2 µs on `172866d` and
  22.4–23.9 µs on `522e891`. That is within run-to-run noise, at most
  5%. The extra work per item (D154's check after each item that a new
  state holds something) is three length reads.
- The p2p index of a large series (D154) builds its sorted list only
  when a pool is added, at most once per check. That is fine.
- Settings export and import (D155) reuse the policy import path
  measured in ORANGE-028, once per user action. They are not on a hot
  path.

No consensus change. It changes only how two read calls answer a pool
the node holds no state for, and the answer is the same.

## Measurements

Same machine as ORANGE-012: 4 vCPU Xeon at 2.10 GHz, Go 1.27.3, ext4.

How they were taken:
- **Engine:** a test-only harness. An engine follows a series of 65,535
  pools (`PoolSize` 1) whole, with no items. It made three passes of
  `Status` over `ListedPools`, timing each and reading the bytes
  allocated and the heap kept from `runtime.MemStats`. Then it made
  100,000 `Tip` calls. It ran on `172866d`, `522e891` and this change.
- **RPC:** a test-only harness. A node with the same policy and
  `rpc.NewHandler` on an `httptest` server, with `GET /v1/status` and
  `/v1/metrics` three times each, on the same three commits. It also
  took an alloc profile of one call each.
- **Engine benchmark:** the repo's `BenchmarkHandleTxParallel`, 3 s ×
  3 on `172866d` and `522e891`.

The harnesses are not committed. The test below is.

## Idea

- **`Engine.unstated(k)`:** for a followed pool with no state, returns
  its pool id. It returns `peek`'s errors.
- **`Status` and `Tip`:** answer such a pool from it.
- **Test:** `TestUnstatedReadsAsFresh`. For pools of a series followed
  whole but one, `Status` and `Tip` equal what a fresh state reads. No
  state is made, and a pool left out or an unknown series is refused as
  before.

## Cost and risk

- About 30 lines and a test.
- If an empty ledger ever reads differently, the test fails, so the two
  cannot drift apart silently.
- The full suite passes with `go test -race ./...`.

## Prototype

On `claude/source-orange-r20`; see the commit "ORANGE-030".
