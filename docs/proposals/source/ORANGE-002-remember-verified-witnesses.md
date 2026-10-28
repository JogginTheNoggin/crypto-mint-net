# ORANGE-002: Remember witnesses already verified

| Field | Value |
| --- | --- |
| Status | adopted |
| Based on | `claude/m10-watch` @ `7e68fe0` (base `23e268f`) |
| Area | `pkg/policy`, `pkg/series` (`CheckWitness`) |
| Effort | small |
| Decided by | Grey, Source Round 4 (D105); Red to review the cache key in Round 4 Part B |

## Summary

A node verifies the same signature several times: when a transaction
arrives and becomes pending, again when it arrives inside an entry, again
for each rival entry that holds it, and again when pending transactions
are revalidated after a switch. Signature checks were 45% of all CPU in a
64-node simulation. Each series now keeps a bounded list of witnesses that
passed for a tid and skips only the signature step for a remembered one.
A repeated check drops from about 70 µs to 1.2 µs, and with ORANGE-001
the simulation runs 2.5 times faster than before. Results are unchanged:
failures are never remembered and the cheap checks always run.

## Problem

`policy.Registry.Check` runs the size ceiling, the address hash, the
allowed-types check and then the policy type's `Satisfy`, which for the
threshold type is one Ed25519 verification per key. `tx.CheckAgainst`
calls it for every input, from `mempool.Pool.Add`, `entry.CheckAgainst`
(every entry, on every branch) and `AddCarried`. Nothing is shared between
these calls.

## Measurements

Machine: 4 vCPU Intel Xeon @ 2.10 GHz (cloud container), Linux, Go 1.27.3.

Simulator CPU profile before (see ORANGE-001 for the scenario):
`ed25519.Verify` 44.9% of all CPU, reached from `entry.CheckAgainst`
(24.1%) and `mempool.Pool.Add` (11.5%) among others.

`go test -run '^$' -bench Check -benchmem -count 3 ./pkg/tx`. `first`
checks a one-input spend on a series that has not seen it; `again` checks
it once more, as a node does when a pending transaction arrives in an
entry. Before, every check was a first check.

| Benchmark | Before | After |
| --- | --- | --- |
| `Check` (one-input spend), first | 73.8 µs, 13 allocs | 70.5 µs, 17 allocs |
| `Check`, again | 73.8 µs | 1.2 µs, 10 allocs |

The 64-node simulation (same report, identical to the last digit):

| Run | Wall time | Allocated | Allocations |
| --- | --- | --- | --- |
| Before | 41.0 s | 6.65 GB | 135 M |
| ORANGE-001 alone | 32.5 s | 4.30 GB | 65 M |
| ORANGE-001 + 002 | 16.7 s | 4.33 GB | 64 M |

## Idea

- `policy.Verified` is a fixed-size ring (default 16,384 keys, grown on
  demand, safe for concurrent use) of SHA-256 keys over
  `"PoO-verified-v1" ∥ encode(witness) ∥ tid`. The witness encoding holds
  the policy (type and parameters) and the witness data, and the tid
  fixes the message, so the key covers everything `Satisfy` reads.
- `Registry.CheckCached` is `Check` with a `*Verified`: the size, address
  and allowed checks run every time, then `Satisfy` is skipped when the key
  is remembered, and a new pass is remembered. `Check` is `CheckCached`
  with nil.
- Each `series.Series` holds one list, and `CheckWitness` uses it.

## Cost and risk

- Memory: at most about 1.5 MB per series when full (16,384 × 32-byte keys
  in a slice and a map). A node follows few series.
- A first check costs one extra SHA-256 of about 150 bytes (about 1 µs
  next to 70 µs).
- Safety rests on `Satisfy` being a pure function of the policy, the data
  and the tid, true of every type today (threshold, hashlock, k-of). A
  future type that reads state or time (the deferred timelocks) must not be
  remembered; the comment on `Verified` says so. **For Red:** please review
  the key and the claim that a remembered pass can never make an invalid
  witness pass.
- Only passes are remembered, so a flood of bad signatures cannot fill
  it with anything useful, and it cannot turn a pass into a failure.

## Prototype

On `claude/source-orange`, commit `4e2ff4e`:
- `pkg/policy/verified.go`: `Verified`, `NewVerified`, the key.
- `pkg/policy/policy.go`: `CheckCached`.
- `pkg/series/series.go`: the list, used by `CheckWitness`.
- `pkg/policy/verified_test.go`: a remembered pass is not reused for
  another tid, tampered data fails every time and is never remembered, a
  remembered witness still fails a wrong address, disallowed rules and the
  size ceiling, and the ring forgets its oldest key.
- `pkg/tx/bench_test.go`: `BenchmarkCheck/first` and `/again`.
