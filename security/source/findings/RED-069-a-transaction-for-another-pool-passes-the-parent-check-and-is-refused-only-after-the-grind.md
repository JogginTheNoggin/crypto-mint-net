# RED-069: A transaction for another pool passes the parent check and is refused only after the grind

| Field | Value |
| --- | --- |
| Severity | Low |
| Status | fixed |
| Found | Source Round 23 Part B, on `main` @ `215d428` (D161) |
| Area | `pkg/ledger` (`CheckOn`), `pkg/engine` (`BuildEntryOn`), `pkg/rpc` (`POST /v1/build`) |
| For Black | No consensus rule is touched. |
| Fixed by | `claude/project-thread-ggo8dt` @ HASH (BLUE-024) |

## What is wrong

`CheckOn` checks each transaction's signatures, inputs and claim against the
state after the parent. It does not check that the transaction's mint is in
the pool being built. `AddEntry` does (`entry.CheckShape`, "a transaction is
not on a mint of the entry's pool"), but only after `entry.BuildWith` has
ground the entry.

`POST /v1/build` takes the pool from the request and the transactions from
`txs`, so the two can disagree. `poo claim -parent` names the pool from the
serial, so it cannot. A script or a GUI that builds with the wrong pool can.

## How I checked it

An engine on the example series; pool 1 (mints 4 to 7) and a claim of mint 0
(pool 0):

```
BuildEntryOn(pool 1, Txs: [claim of mint 0])  -> entry built, refused: [], err nil
HandleEntry(pool 1, entry, Local)             -> "a transaction is not on a mint of the entry's pool"
```

On a series that sets a minimum entry strength, the build in between is a
search of up to `max_tries` nonces (16,777,216 by default) that can never
produce a usable entry. A repeated duplicate in `txs` (`c, c, c`) is
handled: two are listed as refused.

## What could happen

Wasted time and heat for a mistake, and an error that arrives after the
work. Not a break of any rule.

## Suggested fix (Blue's call)

Check the pool in `CheckOn` (or in `BuildEntryOn` before it): leave out any
transaction whose mint is not in the pool, with the same error `CheckShape`
gives, so `refused` names it and the grind never starts. A test beside
`TestBuildOnRefusals`.

## Resolution

Fixed (D162), as Red suggested. `CheckOn` and `CheckPendingOn` leave out a
transaction whose mint is not in the pool, with `entry.ErrWrongPool` (the
error `CheckShape` gives). So `refused` names it, and when nothing is left
the build returns `ErrNothingPending` before any grind.

- Test: `TestBuildOnRefusesAnotherPool` (pkg/engine) sits beside
  `TestBuildOnRefusals`. A claim of mint 0 built on pool 1 is refused and
  no nonce is tried; next to a claim of the pool, only the stranger is left
  out. It fails on the old code.
