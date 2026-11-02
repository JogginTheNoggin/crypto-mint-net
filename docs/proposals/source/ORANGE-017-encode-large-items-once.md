# ORANGE-017: Large entry memory measured; encode a large item once

| Field | Value |
| --- | --- |
| Status | adopted (D126) |
| Based on | `claude/m10-watch` @ `ae8a103` |
| Area | `pkg/encoding` (`NewCounter`), `pkg/entry` and `pkg/tx` (`Size`, presized `Encode`), `pkg/policy` (`Policy.EncodeTo`), `pkg/wire` (`EntryItem.Size`), `pkg/p2p` (`onKept`) |
| Effort | small |
| Decided by | Grey (Black reviews in Part D) |

## Summary

I measured the memory of Round 7's large entry: one claim of 150,000
outputs, 25 MB encoded, as in `TestHeavierLedgerHuge`.

Taking the claim, building the entry, and a peer fetching it in parts
allocated **2.4 GB** for a 25 MB item. One third of that was the entry
being encoded again only to learn its size:
- `EntryItem.Size` encoded the whole entry to count its bytes;
- `onKept` encoded the entry before it knew the entry was too large to
  push, and then sent only a notice;
- every `Encode` started from a 256-byte buffer and grew it by doubling,
  so a 25 MB encode made about 50 MB of garbage on the way;
- each witness encoded its policy into a fresh slice and copied it in.

The prototype adds a counting `Writer`. `Entry.Size` and `Tx.Size` run
the same encoding code into it, so the count is the encoded length by
construction, with nothing allocated. `Encode` then allocates exactly
once. `onKept` checks the size before it encodes.

| | Before | After |
| --- | --- | --- |
| All allocations, 150,000 outputs | 2,426 MB | 1,160 MB |
| `entry.Encode` | 813 MB | 50 MB |
| Building the entry (`a.Build`) | 1.04 s | 0.50 s |
| A peer reaching it | 3.71 s | 3.3 s |
| Peak heap while the peer fetched | 334 MB | 259 MB |

No consensus change: the bytes are the same; only how often they are
made changes.

## Measurements

Same machine as ORANGE-012: 4 vCPU Xeon at 2.10 GHz, Go 1.27.3. A
test-only harness does what `TestHeavierLedgerHuge` does, timed step by
step. The heap is `HeapInuse`, sampled every 20 ms.

### Before (`ae8a103`)

| Outputs | Item | Take | Build | Peer | Heap before | Peak on a | After a | Peak while b fetched | After b |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| 150,000 | 25 MB | 3.39 s | 1.04 s | 3.71 s | 53 MB | 262 MB | 77 MB | 334 MB | 158 MB |
| 50,000 | 8 MB | 1.03 s | 368 ms | 1.10 s | 29 MB | 107 MB | 35 MB | 115 MB | 53 MB |

Where the 2,426 MB went (`alloc_space`):

| Where | MB |
| --- | --- |
| `entry.Encode` (about 31 whole encodes, with growth garbage) | 813 |
| `tx.Encode` | 488 |
| `onKept` (encoding an entry it then sent as a notice) | 333 |
| `EntryItem.Size` (encoding only to count) | 322 |

### With the prototype

| Outputs | Take | Build | Peer | Peak while b fetched | After b |
| --- | --- | --- | --- | --- | --- |
| 150,000 | 2.83 s | 0.50 s | 3.3 s | 259 MB | 158 MB |
| 50,000 | 1.06 s | 157 ms | 937 ms | 108 MB | 53 MB |

| Where | Before | After |
| --- | --- | --- |
| All allocations | 2,426 MB | 1,160 MB |
| `entry.Encode` | 813 MB | 50 MB |
| Build path | 524 MB | 208 MB |
| `HandleTx` | 452 MB | 138 MB |
| `fetchAll` | 461 MB | 307 MB |

"Take" moves within noise; its time is the signatures (ORANGE-014).

### What is left

- **The entry is held decoded.** After b takes it, b's heap is about
  81 MB above where it started: about 3× the item. That is the decoded
  outputs, which the ledger needs. D117 says an entry is put together in
  memory; this is the cost of that at this size, and it scales with the
  item.
- **Serving parts** (`replier.parts`, `partsCache.get`, `EncodeItem`):
  about 220 MB. The sender encodes the item once per parts cache fill.
- `state.add`, about 190 MB, and `Address` encoding its policy for each
  witness, about 63 MB.

None of these is large enough to change before items grow past the
current limits.

## Idea

- `encoding.NewCounter()` returns a `Writer` that only counts. `Raw`,
  `U8` to `U64` add to the count, `Len` returns it, `Reset` clears it.
- `Entry.Size()` and `Tx.Size()` encode into a counter. `Encode()`
  presizes its buffer with `Size()`.
- `Policy.EncodeTo(w)` writes a policy into an existing writer.
  `Witness.Encode` uses it instead of `Raw(Policy.Encode())`.
- `EntryItem.Size` is `Entry.Size() + 1`.
- `onKept` checks `Size()` against `PartsOver` before encoding a
  transaction or an entry.

Because `Size` runs the same code path as `Encode`, it cannot drift from
it. The existing round-trip tests in `encoding`, `entry`, `tx`,
`policy` and `wire` cover it.

`go vet`, `go test -race ./...`, `TestHeavierLedgerHuge`, and
`GOEXPERIMENT=simd` for the changed packages all pass.

## Cost and risk

- About 60 lines.
- `Size` costs one pass over the item without allocating; `Encode` costs
  one pass more. Before, `Encode` also copied about twice the item as
  its buffer doubled.

## Prototype

On `claude/source-orange-r8`; see the commit "ORANGE-017".
