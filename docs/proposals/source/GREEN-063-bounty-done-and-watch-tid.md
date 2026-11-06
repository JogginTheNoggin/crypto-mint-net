# GREEN-063: A bounty from post to paid and through a restart

| Field | Value |
| --- | --- |
| Status | proposed (prototyped) |
| Based on | `main` @ `9c4dc92` |
| Area | `pkg/bounty` (`Book.Done`), `pkg/rpc` (`?done=N` on `/v1/bounty`, `done` and `done_skipped` in the reply, `Client.BountyDone`), `pkg/cli` (`poo bounty list -done N`, `poo wallet watch -tid HASH`, help lines), `gui/e2e/parity.mjs` (two LEFT_OUT lines), docs (running-a-node, modules/bounty, modules/poo) |
| Effort | small |
| Consensus | none: one read of a local file and one look-up of a transaction the node holds |
| Decided by | Grey; Black to confirm no rule changed; White may note the reply fields below; GUI Grey or Green for forms later |

## Summary

BASE.md asked Green to walk a bounty from post to paid and through a
restart, as a new user. Green used two nodes, set up with
`poo policy limits -set`:
- The payer posted 2 layers at 10 units each.
- The grinder worked one slot and was accepted. The payer paid, and the
  grinder got the "paid" ack with the tid.
- Both nodes restarted. The payer's offer came back open with one claim
  paid, and the grinder worked the second slot.
- After the second payment the offer was filled and fully paid, and it
  moved to `bounties-done.jsonl`.

Leases, claims, payment and the payer's book all work, and all survive
the restart. Two things a new user cannot get past:

1. **The grinder's wallet showed 0 after being paid.** running-a-node.md
   says "the grinder sees it with `poo wallet balance` once it settles".
   It does not, because the wallet watches no mint, and the payment comes
   from whichever mint the payer holds (serial 5 here). The paid ack
   names only the tid. `poo address ADDR` showed the 10 units, but there
   was no wallet command that takes the tid and watches its mint. After
   the grinder's node restarts, its held offers and acks are gone (they
   are kept in memory, as documented), so the tid is lost too unless it
   was noted.
2. **A finished offer vanished.** Once it was filled and fully paid, `poo
   bounty list` showed `"mine": []`. No command showed it again. Its
   record was only in `bounties-done.jsonl`, which the docs mention only
   as a file.

## The change

- `poo wallet watch -tid HASH` (repeatable) asks the node for each
  transaction (`/v1/tx`, the same look-up as `poo tx`) and watches its
  mint. If the node does not hold the transaction, the error says the
  node must hold it.
- `poo bounty list -done N` (`/v1/bounty?done=N`) adds `done`: the last N
  offers of the done file, at most 256, oldest first.
  - `Book.Done` streams the file a line at a time, keeping only the last
    N.
  - Lines over 1 MiB, or with no offer id, are skipped and counted in
    `done_skipped`.
  - A line cut short at the end (being written now, or cut off by a
    crash) is left out and not counted.
  - It holds no lock: the file is append-only, and a line written
    meanwhile is either whole or cut short.
- Docs: running-a-node.md gives the grinder's last steps (`bounty list`
  for the tid, `wallet watch -tid`, `wallet balance`), says to note the
  tid before a restart, and adds `-done`. modules/bounty.md and
  modules/poo.md list both flags.
- GUI parity: two LEFT_OUT lines (terminal-only for now).

## How it was tested

- `TestBountyDoneListAndWatchByTID` (pkg/cli/green063_test.go) runs
  against a real node:
  - a cancelled offer leaves the list and comes back with `-done` after
    a restart;
  - a second wallet holds 0 until `watch -tid`, then holds 10;
  - an unknown tid is refused.

  On main it does not build (no `done`).
- `TestDoneListsTheLastOffers` (pkg/bounty) checks the last 3 of 5 in
  order, two bad lines skipped and counted, the line cut short at the end
  left out, and the cap.
- Tested by hand with two nodes: the walk above, then `-done 5` on the
  payer and `watch -tid` on the grinder (balance 10).
- gofmt, go vet and staticcheck pass, as do `go test -race ./...` and the
  GUI e2e script `parity.mjs`.

## For Black

The change adds no rule. Both features only read: the done file, and a
transaction the node already holds.

## For White

`/v1/bounty` takes an optional `done` query, and the reply gains optional
`done` and `done_skipped` fields. An older node ignores the query, so a
new `poo` shows no `done`. No file or wire format changes: the done file
is read as frozen in D200.

## For Red

`Book.Done` reads the whole done file on each call. Each line is capped
at 1 MiB and at most 256 offers are kept, so memory stays bounded, but
the time grows with the file. Only the local RPC can ask for it.

## Not done (ideas for later)

- The paid ack could carry the payment's serials as well as its tids
  (a wire change for White), so `poo wallet watch` would not need the
  node to hold the transaction.
- The grinder could keep its paid acks across a restart, for example in
  a small `bounty-acks.jsonl`, so the tid is not lost.
