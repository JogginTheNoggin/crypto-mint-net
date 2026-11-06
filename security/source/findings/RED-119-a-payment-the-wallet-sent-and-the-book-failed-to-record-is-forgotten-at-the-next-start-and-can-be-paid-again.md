# RED-119: A payment the wallet sent and the book failed to record is forgotten at the next start, and can be paid again

| Field | Value |
| --- | --- |
| Severity | Low (it needs a write error or a stop between two steps; it costs the payer's money when it happens) |
| Status | fixed |
| Found | Source Round 38 Part B, on `main` @ `9c4dc92` (D196, D200; `bounty pay` and `Book.Paid`) |
| Area | `pkg/cli/bounty.go` (`bountyPay`), `pkg/bounty/book.go` (`Paid`, `save`, `archive`) |
| Fixed by | `claude/project-thread-ggo8dt` @ 8ee7aa6 (BLUE-039) |
| For Black | Not a rule: the payment is an ordinary transaction. |

## What is wrong

`poo bounty pay` sends the payment with the wallet first, and then tells the node
(`BountyPaid`) to record it. The two are not one step. If the second fails, the
claim is still unpaid on disk, and nothing connects it to the money that left the
wallet:

- `Book.Paid` sets `Claims[i].Paid` in memory and then calls `save()`. If
  `save()` fails (the done file cannot be written, the disk is full) it returns the
  error, but the claim stays marked paid in memory and unpaid in the file. The
  command prints "paid, but the node did not record it". A second `poo bounty pay` is refused
  ("no unpaid accepted claim") while the node runs; after any restart the claim
  is unpaid again and `poo bounty pay` pays it again.
- A stop between the wallet's send and the RPC (a Ctrl-C, a node restart, a crash)
  does the same, with no error at all to read.
- Nothing in `bountyPay` looks at the wallet's own record of the payment (its
  `Sent` list has the memo `bounty <short id>` and the tid), and the command
  line has no way to record a payment that was made.

## How I checked it

`pkg/bounty` test (throwaway, removed; copy kept with Red's notes): an offer with
one accepted claim, a directory standing where the done file goes (so the
append fails), then `Paid`:

```
Paid error: open …/bounties-done.jsonl: is a directory
in memory after the failure: live=true unpaid=0
after a restart: live=true unpaid claims=1
```

The restart brings the claim back as unpaid. I did not run a stop between the
wallet and the RPC; that is from the code.

## Suggested fix (Blue's call)

- Before sending, mark the claim "paying" in the file (with the amount and
  address); after the wallet's send, record the tids; at the start, a claim that
  is "paying" is shown as such, not as unpaid, and `bounty pay` refuses it
  until the person says (`-force`) or the wallet's `Sent` list shows the tid.
- Or have `bountyPay` search the wallet's `Sent` for the memo and address and
  refuse a second payment.
- In `Paid`, on a failed save, put the claim back as it was (or keep retrying
  the save) so memory and disk do not disagree.
- Add `poo bounty paid ID -claim N -tid T` to record a payment by hand.

## Not tested

A stop between the wallet and the RPC; the same through the CLI end to end.

## Resolution

Fixed in BLUE-039 (D201).

- `Book.Paid` puts the claim back to unpaid when its save fails, so memory and disk agree. When the save failed only after the offer reached the done file (synced, with the payment), the payment is recorded and `bounties.json` is written again on the timer.
- `poo bounty pay` looks in the wallet before it sends. A payment with the memo `bounty <short id>`, the claim's address and amount and series, every leg sent (pending or selected), and no tid already on another claim of the offer, is recorded with the node, and nothing is sent again (`"recorded": true`). That covers a failed save, a stop between the send and the record, and a restart. A payment sent only in part is refused with a message to settle it by hand.
- Not taken: a "paying" mark in `bounties.json` before the send, and `poo bounty paid` to record by hand. The wallet's `Sent` list is that mark, and the next `poo bounty pay` is the way to record it, with no new flag or command for the GUI. A stop inside the wallet's own send, before it saves its `Sent` list, is a wallet-wide window, not this command's.

Tests: `TestBlue039PaidSaveFails`; `TestBountyMarket` makes the node's record fail, then pays again and checks the payment was recorded and the grinder holds one amount, not two.
