# RED-086: The view cost counts transactions, but a cold revert reads the maker's body for every input, so one transaction with many inputs costs thousands of reads for a few units of revert budget

| Field | Value |
| --- | --- |
| Severity | Medium at a small `recent_window` or with a very large merge; low at the default window with ordinary merges (by arithmetic below) |
| Status | open |
| Found | Source Round 29 Part B, on `main` @ `d1a9128` (D175) |
| Area | `pkg/ledger` (`cold.go`: `undoOf`; `pool.go`: `unitsOf`, `viewCost`, `MaxPreviewCost`), `pkg/p2p` (the `revert` budget, D111) |
| For Black | No consensus rule is touched. A deep-parent entry is still taken, as jerry's rule says; this is about what it costs and who pays. |
| Fixed by | |

## What is wrong

D111 and D116 bound the work a deep-parent entry can cost by counting view
units: one per entry reverted or applied plus one per transaction (`unitsOf
= 1 + nitems`), and the sender is charged that many from a `revert` budget
(200,000 a minute per address by default). The calibration was memory: a
revert is about 1.5 µs a unit.

D175 recomputes a cold entry's undo from its transactions' inputs: for each
input `undoOf` finds the maker entry through `p.tids` and reads that entry's
body (`itemsOf` → `loadItems` → `BodyRef.Get`: cache, else the log, decode
and hash) to get the spent output. A maker is usually the next entry down,
which the same call has already read, so ordinarily the cost is small (below).
But the makers need not be among the entries being reverted. A transaction
that spends outputs of M different older entries reads M bodies, and its
view cost is 2 (the entry and the transaction), whatever M is. The number of
inputs a transaction may have is not bounded by anything but the size of the
entry.

## How I checked it

`pkg/ledger` test (throwaway, removed). Series with supply 1,500 and a
window of 16; the `mapSource` twin (decode and hash from bytes, no
syscalls), body cache set to 0 after the build.

Chain: entry 1 claims mint 4 as 1,500 outputs; entries 2 to 1,501 each spend
one of them; entry 1,502 spends all 1,500 results in one transaction; 40
more spends on top. Then a rival entry (one valid spend, no work) whose
parent is entry 1,501:

```
view cost of the rival:   82 units     (the 40 spends and the merge, 2 each)
AddEntry, cold:           6.9 ms,  1,525 body loads
AddEntry, all in memory:  0.29 ms
```

24 times slower, 1,525 reads for 82 units, which is 18 reads a unit against
0.44 for an ordinary chain (a 300-entry transfer chain with a rival 120
entries down: cost 240, 105 loads, 312 µs cold against 115 µs in memory, so
2.7 times). Charged 82 units, an address may send 200,000 / 82 = 2,439 such
rivals a minute: 16.8 s of engine lock a minute from one address with the
fast source, and `PreverifyEntry` pays the same view once more outside
`AddEntry`'s. Each rival is a different transaction on the same coin (a
different output address), so one coin gives unlimited rivals. A real log
read is a syscall, a CRC and a decode: at 20 µs a read the same 2,439 rivals
are 74 s a minute, more than all of the lock.

The merge is not a contrived shape: it is a wallet's consolidation, and one
in the history is enough; the sender needs no part of building it, only a
coin that was unspent just before it. If instead the sender builds it, it has
to put 1,500 entries on the selected ledger, which costs work, but the merge
is then permanent and every node pays for every rival for ever. The 16 MiB
frame holds about 150,000 inputs (Ed25519 witnesses), so M is not small.

**How far this reaches at the defaults.** Reads happen only for entries
below `recent_window`, and the sender pays for every entry between the tip
and its parent, so a rival below a cold merge costs at least `recent_window`
entries of budget. At the default 10,000 that is 20,000 units or more, so
200,000 units a minute is about 10 rivals a minute, and 10 × 1,525 loads is
harmless (arithmetic, not measured). It matters (a) with `recent_window` near
its floor of 16, which a low-memory node is meant to use: the 82-unit case
above; and (b) with a merge of about 10^5 inputs at the default: 10 rivals a
minute × 100,000 reads × 20 µs is 20 s of lock a minute from one address
(arithmetic; I did not build a merge that large).

A second effect: `undoOf` keeps every maker's items in `memo` until it is
done. M makers of 1 MiB each are 1.5 GB held at once, which `entry_cache_mib`
does not bound (the memo is outside the cache).

## What could happen

- The `revert` budget no longer bounds the lock an address can hold, so the
  p2p cost model of D111 (RED-011, RED-012) is wrong again for any series with
  a consolidating transaction in its history. A few addresses can keep the
  engine lock saturated: replies, RPC and honest entries queue behind it. It is
  slowness, not a split: every node still decides alike.
- Memory use during the call can pass `entry_cache_mib` by far.

## Suggested fix (Blue's call)

- Charge the real cost: store the number of inputs in the skeleton
  (`ninputs`) and make `unitsOf = 1 + nitems + ninputs` for the entries a view
  reverts (it is already summed per chain entry in `p.units`). An ordinary
  chain's cost changes by a small factor and the budget and `MaxPreviewCost`
  may need a new calibration. Nothing is refused: the entry is still taken,
  the sender pays more.
- Or avoid the repeated work: keep the recomputed undo of the entries just
  below the window in a small LRU (`entry_cache_mib` counts it), so a second
  rival on the same stretch reads nothing. Or keep each cold entry's spent
  portions (a few tens of bytes an input) in a side file written with the
  record, so no maker body is read. That is the cheapest in reads, but it is a
  second source of truth, so it should be checked against the maker when read.
- Count the memo's items against the cache bound, or read the maker body only
  as far as the one transaction needed (the entry's `Items` for a single tid).
- Test: the merge shape above in `pkg/ledger` with a counting source, asserting
  the loads per unit of cost stay under a stated number.

## Not tested

A merge of 10^5 inputs (the figures for it are arithmetic); a real `store.Log` with a cold page cache (the source is in-memory bytes, so
the times are a lower bound); the default `entry_cache_mib` of 256 MiB with
small maker entries (the second rival then finds them in the cache and costs
little: the amplification is against makers that do not fit, or a first read);
the p2p `revert` charge end to end (read from D111 and `EntryCost`, which is
`ViewCost`); the random differential of undo recomputation against `Apply`'s
record **did** pass for a merge of 40 inputs in shuffled order across two
transactions per entry, and a deep switch afterwards matched the in-memory
pool at every step.
