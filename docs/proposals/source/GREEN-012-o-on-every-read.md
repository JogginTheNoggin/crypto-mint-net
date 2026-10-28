# GREEN-012: `-o table` on the read commands that lacked it

| Field | Value |
| --- | --- |
| Status | proposed |
| Based on | `claude/m10-watch` @ `7e68fe0` |
| Area | `pkg/cli` (`payment`, `wallet addresses/balance/status`) |
| Effort | small |
| Decided by | |

## Summary

GREEN-003 gave `-o table` to the node's read commands. A sweep with the
completion probe found four read commands still without it:
`poo payment`, and `poo wallet addresses`, `balance` and `status`. This
change adds it to all four. Prototyped, with a test that lists every read
command and checks each one takes `-o` and refuses an unknown form before
doing anything.

## Problem

`poo wallet addresses` prints 21 address objects (the gap of unused keys)
as about 130 lines of JSON. With `-o table` it is 22 aligned lines.
Wallet users are the people least likely to want raw JSON. Having `-o`
on some read commands and not on others is the kind of mismatch that
makes a tool feel unfinished.

## Idea

The same `addOutput` and `o.check()` / `o.write()` pattern used
elsewhere. It goes only on commands that read, never on ones that change
state. That way a mistyped `-o` can never fail after a payment has been
sent: the check runs right after the flags are parsed, before any work.
JSON stays the default.

Left out on purpose: `follow` (it changes state when given flags),
`policy list` and `crypto` (short and fixed). They can be added the
same way if wanted.

## Cost and risk

About 20 lines. Output is unchanged without `-o`.

## Prototype

On `claude/source-green`:
- `-o` and `-wide` on `paymentCmd` (`node.go`) and on `walletAddresses`,
  `walletBalance` and `walletStatus` (`wallet.go`)
- `TestEveryReadCommandTakesO` in `table_test.go`
- the list in `docs/modules/poo.md`

I checked `wallet addresses -o table` by hand.
