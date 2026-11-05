# ORANGE-043: Round 33 measured; ledger export and import, and the hello's cost

| Field | Value |
| --- | --- |
| Status | proposed |
| Based on | `main` @ `941049d` |
| Area | `pkg/cli` and `pkg/node` (ledger import, D188), `pkg/wire` (hello, D186) |
| Effort | two small changes and one test; two proposals |
| Decided by | Grey; Black in Part D for the import change |

## Summary

D188 adds `poo ledger export` and `poo ledger import`. D186 adds a
release, features and extensions to the hello. I measured both. The
ledgers have 10,000 and 100,000 entries, each with four transactions and
a layer, made with Ed25519 or ML-DSA-65 owners.

**1. Export is fast. The file is about 1.45 times the log.**

| owners | entries | export | file | per entry | slowest page |
| --- | --- | --- | --- | --- | --- |
| Ed25519 | 10,000 | 0.21–0.24 s | 16.9 MB | 1,689 B | 7.5–8.6 ms |
| Ed25519 | 100,000 | 3.0–3.5 s | 169 MB | 1,690 B | 9.8–10.4 ms |
| ML-DSA-65 | 10,000 | 2.3–2.7 s | 292 MB | 29,237 B | 60–82 ms |

- The log holds 1,158 bytes an entry with Ed25519 and 21,818 with
  ML-DSA-65. The file adds base64 (4/3) and each entry's JSON summary.
- Each page is read outside the engine's lock where it can be (D174).
- The client writes each page as it comes, so the exporting side's
  memory does not grow with the ledger.

**2. Import read the whole file into memory: 1.8 GB for a 292 MB file.
Fixed here.**

`poo ledger import` decoded the whole file, every item as bytes, before
sending the first batch.

| owners | entries | peak heap before | with the fix |
| --- | --- | --- | --- |
| Ed25519 | 10,000 | +130 MiB | +95–113 MiB |
| Ed25519 | 100,000 | +1,068 MiB | +921 MiB |
| ML-DSA-65 | 10,000 | +1,761 MiB | +740–754 MiB |

The harness runs client and node in one process, so these heaps also
hold:
- the importing node, which grows with what it takes;
- its entry cache (256 MiB by default);
- the GC's headroom.

The client's own share was the file times about six. It is now one batch
(at most 256 entries or 8 MiB). A 1,000,000-entry Ed25519 ledger, a
1.7 GB file, would have needed about 10 GB on the client.

**The fix.** `readLedgerFile` reads the header fields and then streams
the `entries` list one entry at a time, handing each to the batcher. The
file format is unchanged.
- **Key order.** When the entries come before the fields that name the
  file and its pool (`kind`, `format`, `series`, `pool`), which `poo
  ledger export` never writes, they are held and given after the header,
  as before.
- **A file that turns out bad.** Today a broken file sends nothing.
  Streaming may send the batches before a malformed entry and then stop
  with "not a ledger file". Those batches have been checked in full like
  any import, so a partial import keeps only valid entries.
- **Test.** `TestReadLedgerFileStreams` covers:
  - the header first, the entries first, and the entries between fields;
  - null entries;
  - six malformed files;
  - an error from the header or an item stopping the read.

**3. Import checked every witness on one core. It now checks each batch
on every core first, about twice as fast. Fixed here.**

In a profile of an Ed25519 import, 77% of `ImportItems` was witness
checks through `PreverifyEntry`, one entry after another.
`ImportItems` now:
1. decodes the batch;
2. hands the leading run of entries that decode to
   `Engine.PreverifyPage`, as a chase does with a page from a peer. That
   checks their witnesses on every core, outside the engine's lock.
3. takes each entry in order through the same `HandleEntryFrom` checks
   as before.

Preverifying only fills the list of verified witnesses. It never changes
what a check returns, and an entry it cannot preview is checked as
before.

| owners | entries | import before | with the fix | slowest batch, before → after |
| --- | --- | --- | --- | --- |
| Ed25519 | 10,000 | 3.8–4.0 s (2,500/s) | 1.86 s (5,400/s) | 115–129 → 60 ms |
| Ed25519 | 100,000 | 44.2 s (2,260/s) | 23.3 s (4,300/s) | 803 → 1,056 ms |
| ML-DSA-65 | 10,000 | 18.0–21.5 s (465–556/s) | 10.9 s (913/s) | 735–1,002 → 580 ms |

Importing the same file again, when every entry is already held, takes
0.4–0.7 s for 10,000 Ed25519 entries and 5.8–8.0 s for ML-DSA-65. That is
mostly moving and decoding 292 MB of JSON.

**4. The hello costs well under a microsecond per connection, and at most
35 KB.**

| hello | bytes | encode | decode |
| --- | --- | --- | --- |
| bare (no release, no features) | 53 | 101 ns | 86 ns |
| typical (release, the four own features) | 124 | 133 ns | 463 ns |
| largest allowed (64 B release, 64 features, 32 × 1 KiB extensions) | 35,564 | 64 µs | 28 µs |

- Ping and pong grow by one byte each, for an empty extension list.
- A typical hello is 71 bytes more than a bare one.
- Each peer keeps the hello it got, extensions included. At most that is
  35 KB a peer, or about 1.1 MB at the default 32 peers.

## Proposals

**Proposal 1: keep only registered extensions from a peer's hello.** No
extension tag is registered yet. Yet a peer may fill the 32 KiB of
extension values, and the node keeps them for the life of the
connection. Dropping unknown tags after decoding would bound that memory
by what this release reads. This matches term 4, which says to skip an
unknown tag. It changes no verdict.

**Proposal 2: stream the import request too.** One batch is at most
8 MiB of items. As JSON base64 that is a request of about 11 MiB, which
the handler decodes whole. That is fine at today's limits, so this only
matters if `watch.ExportBytes` grows. Nothing to do now.

## Measurements

The machine has 4 vCPUs (Xeon with SHA-NI, AVX2 and AVX-512), has 15 GiB
of memory and runs Go 1.27.3. The harness was an internal test in
`pkg/cli`, outside the commit.

- **History:** the example series with Ed25519 and ML-DSA-65 allowed,
  pool 1, four mint chains. Each entry spends each mint once and has a
  16-try layer. The node is closed and reopened before the export, so its
  bodies are on disk as on a running node.
- **Export:** `writeLedgerFile` through `rpc.NewHandler` on an `httptest`
  server, to a buffered file. Then the pages alone, timed one by one.
- **Import:** into a fresh node with the same policy, through its own
  handler, with the CLI's batching (`node.MaxImportItems`,
  `watch.ExportBytes`). "Before" decodes the file whole as the CLI did,
  and "with the fix" uses `readLedgerFile`. Peak heap is sampled every
  2 ms against the heap after two GCs.
- **Profile:** a CPU profile of the 10,000-entry Ed25519 import before
  the preverify change.
- **Hello:** `Hello.Encode` and `DecodeHello`, 200,000 times each.

## What changed

- `pkg/cli/ledgerport.go`: `readLedgerFile` streams a ledger file's
  entries. `ledgerImportCmd` dials the node once it has the header, and
  sends each batch as it fills.
- `pkg/node/import.go`: `ImportItems` decodes the batch and runs
  `Engine.PreverifyPage` on its leading run of entries before taking each
  one in order.
- Test: `pkg/cli/orange043_test.go`, `TestReadLedgerFileStreams`.
  `TestLedgerExportImport` and the node and watch tests pass unchanged.

## Consensus impact

None.
- Every imported entry and layer still goes through `HandleEntryFrom`
  and `HandleLayerFrom` with origin `Requested`, in file order.
- Preverifying only fills the list of verified witnesses, as it does for
  a chase.
- Streaming changes only when the client sends its batches.
- No verdict changes and no depth limit is added.
