# ORANGE-028: Round 18 measured; imports are linear, Unlock's check is free

| Field | Value |
| --- | --- |
| Status | measured; no change proposed |
| Based on | `main` @ `533dbcd` |
| Area | `pkg/nodepolicy` (`ReadBundle`, `Merge`, `Validate`), `pkg/node` (`Open`), `pkg/wallet` (`Unlock`), `pkg/p2p` (`registryClient`) |
| Effort | none |
| Decided by | Grey (for the record) |

## Summary

I measured what Round 18 changed. BLUE-018 changed the merge (RED-044),
D151 added the `Unlock` check, and the registry fetch now checks
addresses. I also measured what a large import leads to: validating the
result and opening a node that follows it. Nothing needs a change.

**Imports are linear now.** A bundle of n followed series, read with
`ReadBundle` and merged into a default node policy:

| Series (file size) | Merge on `b17e892` | Merge on `533dbcd` | Read | Validate | Merge again into the result |
| --- | --- | --- | --- | --- | --- |
| 1,000 (469 KiB) | 1.05 s | 0.13 s | 8 ms | under 1 ms | 0.17 s |
| 4,000 (1.8 MiB) | 11.2 s | 0.42 s | 35 ms | under 1 ms | 0.69 s |
| 8,000 (3.7 MiB) | (not run) | 0.87 s | 63 ms | under 1 ms | 1.36 s |

That matches BLUE-018's own figures: 4,000 series went from 10.4 s to
about 0.4 s. About 0.1 ms a series is left, and almost all of it is
`series.New`, which runs once per series on each side. Merging the same
file again, which is the common re-import, costs a little more, because
every series in the result is built as well. At the 16 MiB bundle cap
(about 35,000 series) a merge would take about 4 s, estimated from the
line. It runs under `policyMu` on a running node. That is fine for a
deliberate import of that size, so I propose no cap.

**What the import leads to.** `node.Open` with 1,000 followed series
took 17 ms and 9 MiB of heap; with 4,000, 81 ms and 16 MiB. Nothing
after the merge grows faster than the series.

**`Unlock`'s new check (D151)** reads the wallet file once more and
compares it. The key derivation is the whole cost, and Unlock with the
default settings measures the same before and after: 56–62 ms on
`b17e892`, 53–90 ms on `533dbcd` (one outlier; the other two runs were
53 and 61 ms). Reading a file of a few KB is tens of microseconds next
to that.

**The registry fetch (RED-043)** now builds an HTTP client per registry
per fetch, from a clone of the default transport. So no connection is
reused between fetches: each fetch makes a new TCP and TLS connection,
and the idle one closes after 90 s. Fetches run once an hour
(`RegistryEvery`), so this costs nothing that matters. If that interval
is ever made short, keep one client per registry location.

No consensus change, and none proposed.

## Measurements

Same machine as ORANGE-012: 4 vCPU Xeon at 2.10 GHz, Go 1.27.3, ext4.

How they were taken:
- **Import:** a test-only harness. It saved a node policy of n
  `ExampleParams` series (each with its own `Mints`, so each is a
  different series), each followed in pool 1, as node.json. It then read
  the file with `ReadBundle` (a plain node.json is a bundle of one),
  merged it into `nodepolicy.Default()` with the default options,
  validated the result, and merged the file into the result again. It
  ran on `b17e892` and on `533dbcd`.
- **Open:** a test-only harness that saved the same policies to a data
  directory and timed `node.Open`, with the heap growth from
  `runtime.MemStats`.
- **Unlock:** a test-only benchmark of `Wallet.Unlock` with the default
  `Options`, 10 runs, 3 times each, on both commits.
- **Registry:** read from the code; not run.

None of the harnesses are committed.

## Prototype

None.
