# BLUE-016: A payment holds the wallet from before it is sent until it is saved

| Field | Value |
| --- | --- |
| Fixes | RED-039 (medium) |
| Branch | `claude/project-thread-ggo8dt` |
| Base | `claude/m10-watch` @ `46bcb9e` (Source Round 15 Part C) |
| Decisions | D146 |
| Review | No consensus change. |

## Summary

`pay`, `claim`, `submit` and `resend` act on the node and then save the
wallet. Under D145 a save that found the file changed was refused, so a
payment already on the node lost its record, and the message said to run
the command again, which paid twice.

These commands now call `Wallet.Hold` after the passphrase prompt. It
takes the wallet's lock until the command ends and reads the file again.
Nothing can save between the send and the save, so the record is kept,
along with what other commands saved before. The change address is a key
no other command took. If the save still fails, the record is printed
and the error says not to run the command again.

## What changed

- `pkg/wallet/wallet.go`: `Hold`, `Wallet.held`; `Save` skips the lock
  while held.
- `pkg/cli/wallet.go`: `walletPay`, `walletClaim`, `walletSubmit` and
  `walletResend` hold the wallet; `saveSent` reports a failed save after
  a send.
- Docs: D146, `docs/modules/wallet.md`.

## Tests added

- `TestPaymentHoldsTheWallet` (pkg/wallet): Red's shape. A request saved
  by another copy before the hold is kept. The payment's save is not
  refused, and its change takes a key of its own. A save from a copy
  read before the hold waits for the release and is then refused.
- `TestSaveSentSaysDoNotRepeat` (pkg/cli): a sent payment whose save
  fails is printed, and the error says not to run the command again.
  When nothing was sent, the send's own error is kept.

## Not taken

- A merge of new records into the changed file: with the hold there is
  nothing to merge.
- An "already paid" check by request: the hold removes the cause.

## What is left

- Another command's save waits while the node answers a held command.
