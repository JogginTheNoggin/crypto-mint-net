# RED-120: A dropped leg counts as unsent, so `bounty pay` pays again, and `poon wallet resend` later sends the old payment too

| Field | Value |
| --- | --- |
| Severity | Low (it needs a send the node refused or lost, a retry, and a later `poon wallet resend`; it costs the payer one extra payment) |
| Status | fixed |
| Found | Source Round 39 Part B, on `main` @ `fa8dad7` (D202, W-34: `legSent`, `sentFor`; `Wallet.Resend`) |
| Area | `pkg/cli/bounty.go` (`legSent`, `sentFor`, `bountyPay`), `pkg/wallet/pay.go` (`Leg.open`, `Spendable`), `pkg/wallet/watch.go` (`Resend`), `pkg/cli/wallet.go` (`walletResend`) |
| Fixed by | |
| Fixed by | `claude/project-thread-ggo8dt` @ a29af78 (BLUE-040) |
| For Black | Not a rule: both payments are ordinary transactions. |

## What is wrong

W-34 freezes the leg states, and `legSent` reads `dropped` (with `unsigned`, `void` and no state) as **unsent**.
So `sentFor` skips a payment whose legs are all dropped, and `bounty pay` makes a new one.
But a dropped leg is not dead. The wallet's own words (`pay.go`): "not held by the node, and its inputs are still unspent: it can be sent again (Resend)".
Two things keep it alive:

- `Spendable` reserves only the inputs of **open** legs (`unsigned`, `pending`, none). A dropped leg's inputs are free, so the new payment may take **other** inputs. `choose` takes the fewest inputs, then the least change, so a deposit that fits better is enough.
- `poon wallet resend` refreshes **every** payment of the series and sends every dropped leg of all of them. It does not ask which are still wanted.

So: a bounty payment is dropped (the node refused the submit, or lost it and the wallet refreshed), the person runs `poon bounty pay` again, it lands, and a later `poon wallet resend` (a habit, a script, or the answer to "dropped" in `poon wallet watch`) sends the first one too. The grinder is paid twice. The node records one tid in `Paid`; the other is in the wallet only.

## How I checked it

Wallet test (throwaway, removed; copy kept with Red's notes): alice holds two mints; she pays carol 30 (leg 1); the node reports it not held and `Refresh` sets it `dropped`; bob pays alice 30 and an entry is built; alice pays carol 30 again (leg 2); then `Resend` of the first payment:

```
p1 inputs [41ae883e:0] change 70
p1 state after refresh: dropped
p2 inputs [d09c8f38:1] change 40 state pending
resend of p1: 1 legs, err <nil>; shared inputs false
```

The two payments share no input, and both are pending: carol is owed 60. The W-34 test table in `pkg/cli/white034_test.go` has `dropped` reading as not sent, so `sentFor` finds nothing for payment 1. I did not run `bounty pay` end to end over a dropped payment; the link from `sentFor` to the second send is that `sentFor` returns nil and `bountyPay` calls `w.Pay`.

## Suggested fix (Blue's call)

- In `bountyPay`, a payment of the claim (same memo, address, amount, series, not used by another claim) with a dropped leg is **resent**, not replaced: `Refresh`, then `Resend`, then record it. Only a leg that is `void` (an input was spent) or `unsigned` is safe to replace.
- Or, when it must pay again, mark the old payment so `Resend` skips it (a new state such as `abandoned`; W-34 already says an unknown state reads as sent, so older readers stay safe).
- `poon wallet resend` could take `-tid` or a payment number, and list what it will send.

## Not tested

The end-to-end command line path; a node that really loses its pending pool across a restart (the test hides the tid).

## Resolution

Fixed in BLUE-040 (D203). `sentFor` finds a payment of the claim whose every leg is sent or dropped, and says when some are dropped. `poon bounty pay` refreshes it from the node (a leg the node holds reads pending again; one whose input was spent reads void), resends its dropped legs, saves the wallet and looks again. It records the payment when every leg is sent. It stops, for the payer to settle by hand, when a leg is still dropped or when a void leg sits beside a live or dropped one. Only a payment whose legs are all void or unsigned is replaced by a new one. So `poon wallet resend` never finds an old dropped payment of a claim that was paid anew. No new leg state was needed.

Tests: `TestBlue040DroppedIsResent`; `TestW34UnknownLegStateIsSent` (dropped now found, to be resent); `TestBountyMarket` (a leg marked dropped is asked about, found held, and recorded: one amount at the grinder).
