# RED-039: when `Save` is refused after a payment or claim was already submitted, the wallet has no record of it and the message says to run the command again

| Field | Value |
| --- | --- |
| Severity | medium |
| Status | fixed |
| Found on | `claude/m10-watch` @ 46bcb9e (Source Round 15 Part B) |
| Component | pkg/wallet/wallet.go (`Save`, `ErrChanged`), pkg/cli/wallet.go (`walletPay`, `walletClaim`) |
| Surface | local operator (CLI, GUI, a second terminal) |
| Fixed by | `claude/project-thread-ggo8dt` @ 47ad630 (BLUE-016) |

## Summary

D145 made `Save` a compare-and-write: if wallet.json no longer holds the
bytes the wallet read, nothing is written and `ErrChanged` is returned ("the
file changed since it was read (another command saved it), so nothing was
saved; run the command again"). That fixes GUI-019 for commands that only
change wallet state (passwd, new address). It does not fit `pay` and `claim`,
because they act on the node before they save: `Pay`/`Claim` call
`Node.Submit`, record the `Sent` in memory, and only then does the CLI call
`w.Save()`. If another command saved in between, the payment is already on
the node, the save is refused, the `Sent` record (and the change address
the payment created) is dropped, and `walletPay` returns the error without
printing the `Sent` JSON. The operator is told to "run the command again".

## Reproduction

Test `TestRedPaySubmittedThenSaveRefused` (scratchpad
`red_r15b_wallet_test.go.txt`, goes in `pkg/wallet`). Alice has a claimed,
mined 100 on mint 4. Two copies of her wallet are opened, as two commands
would. The second (`other`) makes a request and saves. The first (`pay`)
pays a request of 30 and then saves:

```
pay: legs 1, unsigned false
save after the payment was submitted: wallet: the file changed since it was read (another command saved it), so nothing was saved; run the command again
the node holds the payment: pending=true
rerun after the first landed: err=<nil>
second payment legs: 1, change 40
```

The node has the first payment (pending, then mined). The wallet file has no
record of it. Following the message and running the command again, once the
first has landed, pays the request a second time: change 40, which is
100 - 30 - 30. (I did not check whether the first payment's change address is
also missing from the file; `Pay` creates it in memory before the save.)

## Impact

Money moves twice because of an instruction in an error message. The window
is a few milliseconds in a CLI, but the GUI and the CLI are meant to be used
side by side on one wallet (that is how GUI-019 was found), and `claim` has
the same shape. The failure also costs the operator the receipt (`Sent`
JSON) of a payment that did happen, and the error does not say it did.

## Recommendation

For `pay` and `claim`, on `ErrChanged` after a successful submit:

1. Do not tell the operator to run the command again. Print the `Sent` JSON
   anyway, and say the payment was submitted and must not be repeated.
2. Better, re-open the file under the lock and merge the new `Sent` records and
   addresses into it (append-only data, so a merge cannot lose the other
   command's changes), then write. `Save` could take a merge function:
   `SaveWith(func(cur *file) error)`, run on the freshly read file.
3. At least check `Sent()` for the same request before paying it a second
   time (a request has an id), and refuse with "already paid".

No consensus rule is involved.

## Tried and found fine

- `readNewSecret`: empty, mismatched and EOF second entries are refused; piped,
  env and file passphrases read once, as documented.
- Lock file: it follows a symlink at `<wallet>.json.lock` and the `.tmp` name
  is fixed, but both need write access to the wallet directory, which already
  gives control of wallet.json. A removed or replaced wallet.json gives
  `ErrChanged`; a new wallet whose file appeared gives `ErrExists`.
- `POST /v1/entry` without the body cap needs the full token. The decoder
  holds the hex string and the bytes (about three times the body at peak) and
  `entry.Decode` refuses a count the bytes left cannot hold, so the memory
  follows the body and not a small field.

## Resolution

Fixed by BLUE-016 (D146). `pay`, `claim` and `submit` take `Wallet.Hold` before
they act on the node and keep it through `Save`, so no other command's save
comes between; `Hold` re-reads a changed file first. If the save still fails,
`saveSent` prints the record and says not to run the command again.
Re-checked in Source Round 16 Part B: the same test with `Hold` taken first
saves the payment (wallet file records it) with no `ErrChanged`.
