# RED-042: `Hold` keeps the wallet lock for a whole command, including its node calls, and every other wallet command waits for it with no limit and no message

| Field | Value |
| --- | --- |
| Severity | low |
| Status | fixed |
| Found on | `main` @ b502960 (Source Round 16 Part B) |
| Component | pkg/wallet/wallet.go (`Hold`), pkg/wallet/lock_unix.go, pkg/cli/wallet.go |
| Surface | local operator (CLI, GUI) |
| Fixed by | `claude/project-thread-ggo8dt` @ c2964ed (BLUE-017) |

## Summary

D146's `Hold` takes the lock beside the wallet file at the start of `pay`,
`claim`, `submit`, `receive`, `status`, `resend`, `watch`, `address`,
`request`, `policy` and `passwd`, and holds it until the command returns.
Several of these talk to the node while holding it (`pay` and `claim` per
leg, `receive` and `status` scanning the ledger, `resend` per sent record),
and the node client waits up to 5 minutes per request (`http.Client{Timeout:
5 * time.Minute}`). `lockFile` is a blocking `flock(LOCK_EX)`. So one command
stuck on a slow or black-holed node makes every other wallet command, the
read-only `status` and the GUI's wallet calls included, wait silently, one
node timeout per leg or record, with no way to tell why.

## Reproduction

`TestRedHoldWaitsForever` (scratchpad `red_r16b_wallet_test.go.txt`, goes in
`pkg/wallet`): with one wallet holding, a second `Hold` on the same file
does not return for as long as the first holds (3 s in the test), with no
error and no timeout, and returns 3 s after the first releases. I did not
run it end to end against a stalled node: the 5-minute figure and the
per-leg loop are from the code.

## Impact

A hang, not a loss: nothing is paid twice and nothing is corrupted. But a
person running `poo wallet status` to see whether a payment landed waits
behind the `pay` that is waiting for the node, and cannot see why; the GUI
and the terminal share the wallet by design (jerry's rule), so the GUI's
wallet tab freezes the same way.

## Recommendation

1. Hold only the commands that send and then save (`pay`, `claim`,
   `submit`, `resend`), and let the read-only ones (`status`, `receive`
   without `-tid`) read without the lock.
2. Wait with a limit and a word: try `flock(LOCK_EX|LOCK_NB)` in a loop,
   and after a second print "waiting for another poo wallet command to
   finish (it is using <wallet>)", giving up after a stated time with an
   error that says the same.
3. Optionally, hold the lock only across the submit+save of a `pay`, not
   across the balance/selection scan before it, if the scan can be made safe
   (the select-then-spend order is why it is held; say so in the doc).

## Resolution

Fixed (D148), points 1 and 2 in part. `status` and `receive` ask the node
without the wallet held and hold it only to save (`status` keeps a leg
state another command changed meanwhile). The lock is tried first
without waiting; a command that must wait says so on its error output,
the GUI's terminal included, then waits. No time limit: the holder is
saving a payment's record, and giving up would leave the waiter's change
unsaved. The send-then-save commands still hold across their node calls
(D146). Tests: `TestHoldSaysWhenBusy`, `TestHeldWalletWaitersSayWhy`.
