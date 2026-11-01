# ORANGE-014: Large entries measured; check many witnesses once, on every core

| Field | Value |
| --- | --- |
| Status | adopted (Source Round 8, D122) |
| Based on | `claude/m10-watch` @ `0fd59fa` |
| Area | `pkg/policy` (`CheckAll`, new `checkall.go`), `pkg/series` (`CheckWitnesses`), `pkg/tx` (`checkWitnesses`), `pkg/encoding` (`Writer.Reset`) |
| Effort | small |
| Decided by | Grey (Black reviews in Part D) |

## Summary

I measured the large entry from Round 7's P0 test
(`TestHeavierLedgerHuge`).

| Item | Measured |
| --- | --- |
| The claim | 150,000 outputs, 26.4 MB as an item, sent in parts |
| CI test time | 58 s |
| Signature checks | 80% of that time |
| Moving the parts | Under 0.2 s |
| Peak heap | 283 MB for both nodes |

The parts transfer is cheap. Signatures dominate because every node
checks the claim's 150,000 witnesses several times, one core at a time:
- Each node checks them twice: once before its lock and once in `Check`.
- The node that builds the entry checks them four times: twice taking
  the transaction, and twice taking its own entry.

The list of witnesses already verified (ORANGE-002) holds 16,384. A
150,000-witness item pushes all of its own keys out before the second
check reaches them, and pushes out every other remembered witness too.

`Registry.CheckAll` checks an item's witnesses as a set:
- The cheap checks run on every witness, every time.
- From 64 witnesses up, the signatures run on every core.
- A set that passes is remembered under one key.

Results for the 150,000-witness claim:

| Step | Before | After |
| --- | --- | --- |
| Taking the claim in | 16.6 s | 2.8 s |
| Building the entry that holds it | 17.2 s | 0.7 s |
| A peer reaching that entry | 17.3 s | 3.1 s |
| `TestHeavierLedgerHuge` | 58 s | 12.9 s |

The result is the same as checking each witness in order. Same pass, or
the same first failing index and error. A bad witness still costs about
one check per core (RED-009). No consensus change.

## Measurements

Same machine as ORANGE-012: 4 vCPU Xeon 2.10 GHz, Go 1.27.3.

### Where the large entry's time goes

The base is `0fd59fa`. The run used a test-only harness doing what
`TestHeavierLedgerHuge` does, timed step by step.

| Step | Time | What the CPU profile shows |
| --- | --- | --- |
| Sign the 150,000-output claim (test only) | 7.2 s | Ed25519 signing and keys |
| `a.HandleTx` | 16.6 s | 2 × 150,000 `ed25519.Verify`: `PreverifyTx`, then mempool `Check` |
| `a.Build` | 17.2 s | 2 × 150,000 again: `PreverifyEntry`, then `AddEntry` |
| `a.Work` | 0.05 s | |
| b reaches a, entry in parts | 17.3 s | 2 × 150,000: `PreverifyEntry` and `AddEntry` |

About 53 µs a verify.

For the whole test, 46 s of 58 s is `ed25519.Verify`. Encoding is 1.9 s;
the 26 MB entry is encoded again for `EntryItem.Size`, in `HandleEntryFrom`
and in `onKept`, about 1.8 GB allocated in all. Reading the parts
(`fetchParts`) is 130 MB allocated and too little CPU to show in the
profile.

### With `CheckAll`

| Step | Before | After |
| --- | --- | --- |
| `a.HandleTx` | 16.6 s | 2.76 s |
| `a.Build` | 17.2 s | 0.70 s |
| b reaches a | 17.3 s | 3.13 s |
| Peak heap in use | 282 MB | 283 MB |
| `go test -run TestHeavierLedgerHuge ./pkg/p2p` | 58.4 s | 12.9 s |

`BenchmarkCheckAll`, 2,000 single-key witnesses:

| How | Time |
| --- | --- |
| `CheckCached`, one at a time, no list | 101 ms |
| `CheckAll`, every core | 33 ms (3.1×) |
| `CheckAll`, set already remembered | 2.3 ms (44×): address checks and the set key's hash |

## Idea

`policy.Registry.CheckAll(h, rules, ws, addrs, tid, v)` returns
`(index, error)` of the first witness that fails, as calling
`CheckCached` on each in turn would.

Below `ManyWitnesses` (64), it is exactly that loop.

From 64 up:
1. The size, address and allowed checks run on every witness, in order.
   A failure there is noted, and only the witnesses before it go on.
2. If all pass the cheap checks, a set key is looked up:
   `SHA-256("PoO-verified-set-v1" ∥ tid ∥ count ∥ each witness encoded)`.
   If `v` holds it, the item passes.
3. The satisfaction functions run on `GOMAXPROCS` workers over contiguous
   ranges, with `Message(tid)` built once. They stop when one fails.
4. On a failure, the witnesses no worker reached are checked in order up
   to the first failure. So the index and error are exactly those of the
   in-order loop.
5. A set that passes adds one key to `v`. Its witnesses are not added
   one by one, so they do not flush the list.

`series.CheckWitnesses` wraps it, and `tx.checkWitnesses`, used by
`Preverify` and `CheckStateless`, calls it.

`TestCheckAllMatchesInOrder` covers 3, 64 and 500 witnesses:
- valid;
- a bad signature first, last, or twice;
- a wrong address;
- both together in either order.

Each case is checked twice, the second time with the list.

## Cost and risk

- About 150 lines with the test. The set key hashes the witnesses
  (estimated at tens of milliseconds for 26 MB; not measured on its own); a 1-witness spend goes through the old loop
  unchanged.
- **Under a lock.** Checks under the node's lock (`Check`) find the set
  that `Preverify` remembered before the lock, so the parallel path runs
  outside the lock in practice.
- **Pool size.** Workers are short-lived goroutines, at most
  `GOMAXPROCS`, for items of 64 or more witnesses.

## Next

- **Encode the large entry once.** `EntryItem.Size`, `HandleEntryFrom`
  and `onKept` each encode the 26 MB entry (1.8 GB of allocation in the
  test). A size function that adds up lengths, and `Policy.EncodeTo` in
  place of `enc.Raw(w.Policy.Encode())`, would remove most of it.
- D117 says an entry is put together in memory. Peak heap is 283 MB for
  two nodes holding this entry, so about 5× the item per node. That is
  fine at this size, and worth watching for larger ones.

## Prototype

On `claude/source-orange-r7`, commit `0d2aeb8`.

The full suite passes:
- `go test -race ./...`;
- `TestHeavierLedgerHuge`;
- `GOEXPERIMENT=simd` for the changed packages.
