# RED-068: Building on a parent checks pending transactions in quadratic time under the engine lock

| Field | Value |
| --- | --- |
| Severity | Low |
| Status | open |
| Found | Source Round 23 Part B, on `main` @ `215d428` (D161) |
| Area | `pkg/ledger` (`CheckOn`, `conflicts`), `pkg/engine` (`BuildEntryOn`), `pkg/rpc` (`POST /v1/build`) |
| For Black | No consensus rule is touched. `CheckOn` is a local pre-check; `AddEntry` still decides. |

## What is wrong

`poo build -parent HASH|root|before-claim` (and `POST /v1/build` with
`parent` and no `txs`) passes **every ready pending transaction** to
`ledger.Pool.CheckOn`, while `BuildEntryOn` holds `e.mu`, the engine lock
that every message the node handles needs. `CheckOn`:

- calls `tx.CheckStateless` (signature checks) for each, again, though the
  pending pool already did;
- calls `conflicts(taken, it)` for each, which compares the transaction
  against **every transaction taken so far**: `O(n²)` in the number of
  transactions, times the inputs.

Build without `-parent` does none of this (it passes `Ready()` straight to
`entry.BuildWith`), so the new option costs more than the old build, and the
cost grows as the square.

## How I checked it

`conflicts` alone, for n transactions of one input each, none conflicting
(a micro-test in `pkg/ledger`):

```
n 10000    0.29 s
n 50000   10.7 s
n 100000  44.9 s
```

And the whole call on a real engine (12,000 mints in one pool, 3,000
entries, then 7,000 pending claims, the default limit being 10,000):

```
BuildEntryOn(Parent: pool_id) with 7000 pending:  126 ms  (a plain Build of the same: 25 ms)
```

Fine at the default, but `max_pending` now goes up to 1,048,576 and
`poo policy limits -set max_pending=...` changes it on a live node (D160,
D161). A series with many mints lets a peer fill the pending pool with valid
claims at no cost (a claim has no input and pays the claimer). Then the
owner's `poo build -parent X`, or any `POST /v1/build` with a parent and
no `txs`, stops the whole node for the time above while it holds the lock.
The explicit `txs` path is bounded by `MaxBuildTxs` (1,024) and costs little.

## Suggested fix (Blue's call)

- Replace `conflicts` with two sets: the inputs taken and the mint serials
  claimed. `O(n)` per call.
- Bound the pending path as the explicit one is: take pending transactions in
  canonical order until a count (or a byte size) is reached, as a node
  builds an entry of a size peers will take, and check only those.
- Do the checks without the engine lock held for their whole length where it
  can: copy the transactions out, check them against the view, take the lock
  again to build (`AddEntry` rechecks anyway).
- A test: `CheckOn` with 20,000 transactions finishes in well under a
  second.

## Not tested

A pool of 100,000 pending claims through the real RPC; the entry size peers
accept (I found no per-entry transaction cap in `entry`, `series` or `wire`).
