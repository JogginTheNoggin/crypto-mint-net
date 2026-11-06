# RED-114: `bounties.json` is trusted as written: an offer the posting check would refuse loads, the owed sum wraps, and `poo bounty pay` sends what the file says

| Field | Value |
| --- | --- |
| Severity | Low (it needs write access to the node's data directory) |
| Status | fixed |
| Found | Source Round 36 Part B, on `main` @ `6b56136` (D196) |
| Area | `pkg/bounty/book.go` (`Open`, `Owed`), `pkg/cli/bounty.go` (`bountyPay`) |
| Fixed by | `claude/project-thread-ggo8dt` @ afd0dc1 (BLUE-037) |
| For Black | Not a rule. |

## What is wrong

`Open` keeps every offer whose `id` equals the hash of its bytes and checks
nothing else: not `Offer.Check()` (strength 0, amount 0, count 0), not the
number of offers (`max_open`), not the claims. `Owed()` multiplies unpaid
claims and unfilled layers by `Amount` without an overflow check (`Post`'s own
sum uses `mulCheck`), so a count of 4 and an amount of 2^62 owes 0.
`bountyPay` sends the amount of the offer to the `pay_to` of the claim as
the file has them, with no preview and no question.

The file is 0600 in the node's directory, as the wallet is; the wallet itself
needs its passphrase. Someone who can write the node's directory (a plugin, a
backup tool, a shared account) can add a claim that `poo bounty pay` will
pay when the owner next runs it, with a wallet the attacker cannot open.

## How I checked it

`pkg/bounty` test (throwaway, removed; copy kept with Red's notes): a file
with one offer of strength 0, amount 2^63, count 3, one claim with a made-up
`pay_to`: `Open` returned it; `Owed()` was 9223372036854775808 (a wrap of
3 × 2^63); a new offer of 10 against a budget of 100 was refused only
because the wrapped sum was still large. I did not run `bounty pay` on it.

## Suggested fix (Blue's call)

- `Open` re-runs `Offer.Check` and drops (and reports) what fails; caps the
  number of offers and claims; uses `mulCheck` in `Owed`.
- `poo bounty pay` prints what it is about to pay (to whom, how much, which
  nonce, whether the layer was new) and takes a `-yes` for scripts.
- Optionally, keep a MAC of the file under a key from the wallet seed.

## Not tested

A file of a million offers; `bounty pay` on a forged claim.

## Resolution

Fixed in part in BLUE-037 (D197). `Open` re-checks each offer and its claims, leaves out what fails, and the daemon logs it. `Owed` saturates instead of wrapping. The `-yes` preview for `poo bounty pay` is not added, since a new flag needs a GUI form. Test: `TestBlue037FileChecked`.
