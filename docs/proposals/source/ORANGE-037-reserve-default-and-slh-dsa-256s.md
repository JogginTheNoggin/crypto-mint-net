# ORANGE-037: Round 27 measured; the reserve default, SLH-DSA-256s and witnesses with no network-wide cap

| Field | Value |
| --- | --- |
| Status | proposed |
| Based on | `main` @ `1a1c729` |
| Area | `pkg/policy`, `pkg/node`, `pkg/series`, node policy `series_ceiling` |
| Effort | measurements only; two small proposals for Grey |
| Decided by | Grey; Red for the `series_ceiling` default |

## Summary

D170 removed the network-wide witness size caps, added SLH-DSA-SHA2-256s
(algorithm 11), made the reserve series the `poon init` default and made far
part ids cancellable. I measured each one. Nothing needs fixing now, and I
make two proposals.

**1. SLH-DSA-256s verifies in about 0.9 ms and signs in about 0.65 s. It
is the cheapest key per byte, but the dearest per signature.**

| | verify, 1 core | verifies a second, 4 cores | sign | key from seed | signature | largest k-of-k in 128 KiB |
| --- | --- | --- | --- | --- | --- | --- |
| Ed25519 | 51 µs | 48,000 | 67 µs | 1 ms | 64 B | 16 |
| ML-DSA-65 | 248 µs | 15,800 | 1.4 ms | 0.5 ms | 3,309 B | 16 |
| ML-DSA-87 | 304–358 µs | 8,400–11,400 | 0.4–1.2 ms | 0.5 ms | 4,627 B | 16 |
| SLH-DSA-SHA2-128s | 323–345 µs | 8,000–10,800 | 345 ms | 43–55 ms | 7,856 B | 16 |
| SLH-DSA-SHA2-256s | 852–941 µs | 3,900–4,300 | 650–680 ms | 41–62 ms | 29,792 B | 4 |

- A node checks SLH-DSA-256s at a third of ML-DSA-87's rate per
  signature, but at 29 ms per MiB of witness against 41 ms for ML-DSA-87.
  So per byte received it is the cheapest key the reserve series allows.
- At two thirds of a second a signature, it suits cold keys and issuers,
  not everyday spends. Only 4 fit in the reserve series' 128 KiB.
- Deriving the key from its seed takes 40 to 60 ms, which a wallet pays
  each time it builds the signer.

**2. The reserve series' worst witnesses cost 3.5 to 5.6 ms to check.**

These are the largest k-of-k witnesses that fit the 128 KiB ceiling,
checked through the series' registry:

| witness | size | check, 1 core |
| --- | --- | --- |
| 16-of-16 Ed25519 | 1,786 B | 1.26 ms |
| 16-of-16 ML-DSA-65 | 84,426 B | 3.9 ms |
| 16-of-16 ML-DSA-87 | 115,754 B | 4.7 ms |
| 16-of-16 SLH-DSA-128s | 126,458 B | 5.6 ms |
| 4-of-4 SLH-DSA-256s | 119,506 B | 3.5 ms |

Checking runs outside the engine lock (`PreverifyTx`) and its result is
cached, so a slow witness does not stall the node's other work.

Ed25519 is the dearest per byte: 16-of-16 costs about 0.7 ms per KiB of
witness, against 0.03 to 0.05 ms for the post-quantum keys. Under the
reserve ceiling, one witness made of nested k-of policies over Ed25519
keys is bounded at about 128 × 0.7 ≈ 90 ms. Under a series with no
ceiling, which D170 left valid, one witness can fill a 16 MiB frame,
about 12 s of one core.

**3. A witness over the ceiling is refused at the speed of copying it.**

The ceiling is checked before any hash or signature:

| witness bytes | decode and refuse |
| --- | --- |
| 128 KiB | 96 µs |
| 1 MiB | 1.4 ms |
| 15 MiB | 28 ms |

The profile is all `encoding.Reader.Raw` copying the witness. That is
about 1.9 ns a byte, so the node spends far less time than it took to
receive the bytes. No change is needed.

**4. The reserve default opens in under a millisecond, but uses one open
file per pool. Proposal 2 below.**

- A node that follows the reserve series whole opens in 0.33 ms, because
  pool state is made on first use.
- A claim takes 167 µs to decode and take in when it is the first in its
  pool.
- 1,024 pending claims, one per pool, add 7.9 MiB of heap.

Each pool that holds an item keeps its store file open. With all 1,024
reserve pools in use the process held 1,033 open files. Go raises the
soft limit to the hard limit at start, so this is fine wherever the hard
limit is a few thousand or more. Where the hard limit is 1,024, as in
some containers and older systems, the last pools' stores and new peer
connections fail with "too many open files". Today the node does not say
why.

**5. Cancelling a far part id takes 2 to 3 ms. No change needed.**

`PartIDContext` checks its context once every 16,384 hashes:
- SHA-256 works out 2,000,000 parts in 0.52 s (258 ns each), and a
  cancel returned in 2.4 ms;
- SHA-512/256 takes 0.85 s (424 ns each), and a cancel returned in 3.3 ms.

The plain methods pay one `ctx.Err()` per chunk, which cannot be
measured.

## Proposals

**Proposal 1: a `series_ceiling` default of 4 MiB.** D170 lets Orange and
Red recommend one. 4 MiB is `CeilingLimit`, so every series that
`poon series new` writes without `-any-ceiling` is still taken. It bounds
the worst single witness a node can be asked to check at about 3 s of one
core (Ed25519 at 0.7 ms per KiB), against about 12 s with no ceiling. The
reserve series and every series made with the tool's default are
unaffected. Declining is a lack of information, not a verdict (D170), so
it cannot be a hard fork. If Red wants a tighter bound, 1 MiB gives about
0.75 s.

**Proposal 2: warn at start when the open file limit is too low.** When
the soft `RLIMIT_NOFILE` (after Go raises it) is below the followed pools
plus room for peers, about 256, `node.Open` should log one warning with
the limit and the number needed. That is a Unix-only call beside
`lock_unix.go`. Closing idle stores would remove the need, but it is a
larger change to `pkg/node` and I do not propose it now.

## Measurements

The machine has 4 vCPUs (Xeon with SHA-NI, AVX2 and AVX-512), and runs Go
1.27.3. The harnesses were test files outside the commit.

- **Algorithms:** for each one I derived a key from a seed and signed
  once. Verify is the mean of 200 single-core verifies; the 4-core rate is
  4 goroutines doing 200 each. Ranges are three runs.
  `MaxK` is `policy.MaxK(alg, 131072)`.
- **Worst witnesses:** `SignThreshold` with K distinct keys over one claim's
  tid in the reserve series, then `Registry.Check` with the series as
  rules, 10 times each.
- **Refusing a witness over the ceiling:** a claim in the reserve series
  whose witness data was replaced by N zero bytes, encoded, then
  `tx.Decode` and `node.HandleTx`, 20 times each. Every call was refused
  with "witness larger than the series allows".
- **Reserve node:** node.json listing `series.Reserve()` followed whole,
  `node.Open`, then 1,024 Ed25519 claims to serials 0, 32, …, one per
  pool. The heap is measured after two GCs, and the open files are counted
  from `/proc/self/fd`.
- **Part ids:** a variable series (part 1 of 8 mints, then parts of 4),
  `PartID(2,000,000)` from a fresh chain. For the cancel test,
  `PartIDContext(ctx, 2^40)` on another fresh chain, cancelled after
  200 ms.

## Consensus impact

None. This PR adds only this document. Proposal 1 is a node policy
default and proposal 2 is a log line.
