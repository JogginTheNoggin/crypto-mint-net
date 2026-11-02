# ORANGE-027: Round 17 measured; the modular seams cost nothing worth an exception

| Field | Value |
| --- | --- |
| Status | measured; no change proposed |
| Based on | `main` @ `b17e892` |
| Area | `pkg/hash` (`Grinder`, `Hasher`, `Func`), `pkg/fsutil`, `pkg/wallet`, `pkg/peers`, `pkg/tracking`, catch-up |
| Effort | none |
| Decided by | Grey (for the record) |

## Summary

The owner's standing rule is that features are modular and extensible,
and that Orange may challenge it, with measurements, where modularity
really hurts performance. This round I measured the seams on the
node's hot paths, and the durable writes that D148 and D149 added. I
found nothing to challenge and no regression.

| What | Measured | Verdict |
| --- | --- | --- |
| Layer grinding through `hash.Grinder` (a `hash.Hasher`, over Go's `hash.Hash` interface) vs a concrete `sha256.Sum256` | 94.5 ns a try through the interface, 99.2 ns direct (one core) | Keep the interface. SHA-256's compression is all the cost. |
| Catch-up of 20,000 entries with a spend each (ORANGE-020's case) | 902 ms, 45 µs an entry (ORANGE-020: 43–44 µs) | No regression. 53% is Ed25519, and no indirection shows in the profile. |
| `tx.TID` through `hash.Func`, which takes byte slices, so the body is built first | 381 ns, 3 allocations, for a small claim | Not worth changing. The body encoding is about 1% of catch-up CPU. |
| `Wallet.Save`, now synced file and directory (`fsutil.WriteFile`, RED-041) | 20 keys: 0.16–0.19 ms → 0.57–0.64 ms; 1,000 keys: 1.1 → 1.6 ms | Fine: once per command, and a crash can no longer lose a payment's record. |
| Peer book and tracker saves, now synced | Once per `ManageEvery` (30 s) and at shutdown | Not on a hot path. |
| Node policy export and import (D149) | Run once per user action; reading is bounded at 16 MiB and 256 policies | Not on a hot path. Not measured further. |

### Why the grinding seam is free

`hash.Grinder` keeps one `hash.Hash` per worker and calls `Reset`,
`Write`, `Write` and `Sum` through the interface for every nonce. That
looks like the place where a pluggable hash would cost speed. Measured
against `sha256.Sum256` on the same 46-byte message, the interface path
is not slower: 94.5 ns against 99.2 ns. The difference is noise.
One SHA-256 compression is
about 90 ns on this CPU. The calls through the interface and the
buffering cost a few nanoseconds. That is 1–3% of a try, and less than
run-to-run noise.

`GrindHash` itself, at 106 ns a try for SHA-256 on one worker (26.6 ns
on 4), matches ORANGE-009's 110 ns and ORANGE-012's figures. The
AVX-512 lanes (ORANGE-011/012) already bypass the interface where it
would matter: there the cost per hash is about 10 ns.

### Catch-up after Round 17

| 20,000 entries, 4 vCPU | ORANGE-020 (D135) | `b17e892` |
| --- | --- | --- |
| Time | 854–871 ms | 902 ms |
| µs an entry | 43–44 | 45 |

Where the CPU went (profile, 1.74 s over both nodes):

| Where | Share |
| --- | --- |
| Ed25519 verification (`edwards25519`), all cores | 53% |
| `memmove` (decoding, copies) | 7% |
| Taking layers in (`takeLayers`, `HandleLayerFrom`) | 10% |
| SHA-256 (tids, entry hashes) | 3% |
| Map lookups (`maps.*`), `Seen.Add`, `Pool.Tip`/`Layers` | under 2% each |

Nothing there is the cost of a module boundary. The registries (hash,
policy), `Seen` and the pool's maps each sit under 2%.

## Measurements

Same machine as ORANGE-012: 4 vCPU Xeon at 2.10 GHz, Go 1.27.3, ext4.

How they were taken:
- **Grinding:** the repo's `BenchmarkGrindHash` (`pkg/layer`). Beside
  it, a standalone benchmark compared `sha256.Sum256` on a 46-byte
  buffer (tag, entry, nonce) with `Reset`/`Write`/`Write`/`Sum` through
  `hash.Hash`, the path `hash.Grinder` takes for a prefix shorter than a
  block.
- **Catch-up:** ORANGE-020's harness, rebuilt test-only. Two
  `startNode` nodes; node a grows N entries, each with one Ed25519
  spend, and node b connects. The time runs until b holds a's state.
  There was a CPU profile over the catch-up.
- **TID:** the repo's `BenchmarkTID` (`pkg/tx`).
- **Wallet:** a test-only benchmark of `Wallet.Save` with `Gap` 20 and
  1,000, under `t.TempDir` on ext4, run twice each on `b502960` (before
  D148) and `b17e892`.

None of the harnesses are committed.

## For the next rounds

If modularity ever costs measurable time, it will be where a seam
forces bytes to be built that a stream could hash: `hash.Func` takes
byte slices, so a tid encodes the body first. Today that is about 1% of
catch-up. It would matter only for entries with very many small
transactions, where TIDs are computed several times per transaction
(the old lead). A `hash.Func` that can also take a writer would keep
the seam and remove the copy. I would propose it only with a case that
shows it above noise.

## Prototype

None.
