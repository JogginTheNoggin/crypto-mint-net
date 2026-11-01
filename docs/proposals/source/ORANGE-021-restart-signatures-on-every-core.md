# ORANGE-021: Restart measured; check the store's signatures on every core

| Field | Value |
| --- | --- |
| Status | proposed |
| Based on | `claude/m10-watch` @ `1ae02ca` |
| Area | `pkg/policy` (`Registry.Warm`), `pkg/series` (`WarmWitness`), `pkg/engine` (`WarmStored`, `MaxWarm`), `pkg/node` (`load`, `replayWindow`) |
| Effort | small |
| Decided by | Grey, with Red (Black reviews in Part D) |

## Summary

On start-up a node replays its own store record by record, and checks
every witness again, so a tampered store is refused
(`TestTamperedStoreIsCheckedAgain`). That is right, but it all runs on
one core. For a ledger whose entries each hold a spend, 72% of restart
time is `ed25519.Verify` on one core.

| 4 vCPU, 20,000 entries with a spend each | Before | After |
| --- | --- | --- |
| Restart (60,000 records) | 1.72–2.15 s (86–108 µs an entry) | 0.86–0.95 s (43–48 µs) |
| Restart, 5,000 entries | 420–460 ms | 205–222 ms |
| 5,000 entries, `GOMAXPROCS=1` | 505 ms | 517 ms |

That is 2.1× faster on 4 cores, and the same on one core. The time
grows with the ledger, so a node with a million such entries would
restart in about 45 s instead of about 100 s (estimated, not run).

**The fix.** The store is replayed in windows of 1,024 records:
1. Decode the window.
2. Check every witness of its transactions on all cores, and remember
   each pass (`Engine.WarmStored`).
3. Replay the window in order, as before. Each witness check now finds
   its pass remembered.

Step 2 needs no owners. Their addresses come from the state the replay
builds, which does not exist yet. A pass depends only on the policy,
the witness data and the tid, and that is exactly what the list of
verified witnesses is keyed on. The address, size and allowed checks
still run in step 3, in order, for every witness. A forged witness
fails in step 2, is not remembered, and is refused in step 3 exactly as
before.

No consensus change. What is checked, and what each check returns, are
the same.

## Measurements

Same machine as ORANGE-012: 4 vCPU Xeon at 2.10 GHz, Go 1.27.3.

How they were taken:
- A test-only harness on `node.Open`, with the default policy.
- It grows N entries, each with one Ed25519 spend and one layer, so the
  store holds a transaction, an entry and a layer record per entry.
- It closes the node and times `node.Open`.
- The state after restart is checked equal to the state before.
- "Before" is the same harness on `1ae02ca`.

| Run | Before | After |
| --- | --- | --- |
| 5,000 entries | 420, 460 ms | 222, 205 ms |
| 20,000 entries | 1.90, 2.15, 2.04, 1.83 s | 0.88, 0.86, 0.94, 0.90 s |
| 20,000, second restart | 1.92, 1.72 s | 0.88, 0.95 s |
| 5,000, one core | 505 ms | 517 ms |

Where the time went before (CPU profile, 20,000 entries):

| Where | Share |
| --- | --- |
| `HandleTx` (stored transactions) → `ed25519.Verify` | 72% |
| `HandleEntry` (state, ledger) | 8% |
| `HandleLayer` | 6% |

After, `WarmStored` runs the checks on all cores, and the in-order
replay is about 11% of the CPU.

The first prototype checked each transaction twice: once for its
stored record and once for the entry holding it. They ran at the same
time, so neither found the other's pass. `WarmStored` now checks each
tid once per call. That took 20,000 entries from 1.29–1.40 s to
0.86–0.95 s.

## Also measured: overlapping catch-up with its checks

After ORANGE-020, catching up spends its time like this:

| Step | Share of b's reply handling |
| --- | --- |
| Taking the entries in, in order (state, store, events, layers) | about 25 µs an entry, 60% |
| Waiting for the checks | 30% |
| Finding the owners (`PageOwners`) | 5% |

I tried taking the entries in while the workers checked the later
ones. Over three runs each, the median time went:
- 5,000 entries: 294 ms → 238 ms;
- 20,000 entries: 1,097 ms → 954 ms.

The gain was 13 to 19%, with run-to-run noise of about 20%. That is not
enough for the extra goroutines and waiting, so I am not proposing it.

The remaining in-order cost has no single hotspot. The network wait is
only 70–90 ms of about 900 ms.

## Idea

- **`policy.Registry.Warm(rules, w, tid, v)`:** the size and allowed
  checks, then the satisfaction function. It remembers a pass under
  the same key `CheckCached` uses, and checks no address.
- **`series.Series.WarmWitness(w, tid)`:** wraps it with the series'
  list.
- **`engine.Engine.WarmStored(txs)`:**
  - Under the lock, it finds each transaction's series and tid, and
    drops repeated tids.
  - Outside the lock, it checks at most `MaxWarm` (8,192) witnesses on
    `GOMAXPROCS` workers. That is half the list, so they are still
    there when replayed.
  - On one core it does nothing, and the replay checks them as before.
- **`node.load`:** decodes `replayWindow` (1,024) records at a time,
  calls `WarmStored` with the transactions in them (stored ones and
  those inside entries), and then applies them in order.
- **Tests:**
  - `TestWarmWitness`: a witness for another tid is not remembered, and
    the address is still checked after a warm.
  - `TestWarmStored`: 12 transactions listed twice cost 12 checks ahead
    and none in replay. A forged witness is not remembered, and its
    entry is refused.
  - The existing restart tests pass, including
    `TestTamperedStoreIsCheckedAgain`.

## For Red

- `Warm` checks a signature before the address it must satisfy. RED-009
  (D107) asks for the opposite order for items from peers, so a bad
  item costs one check. `WarmStored` is used only for the node's own
  store on start-up, which a peer cannot write. Someone who can write
  the store can already make the node do any amount of work. If Red
  wants it kept to peer-free paths, that is where it is today.
- Nothing is remembered that would not be remembered by replaying the
  same records in order.

## Cost and risk

- About 150 lines with tests.
- A window's decoded records are held at once: 1,024 records, under
  1 MB for ordinary entries. A large entry is one record of its own
  size, as before.
- The full suite passes: `go test -race ./...`,
  `TestHeavierLedgerHuge`, `TestHeavierLedgerLongBareRun`, and
  `GOEXPERIMENT=simd` for the changed packages.

## Prototype

On `claude/source-orange-r11`; see the commit "ORANGE-021".
