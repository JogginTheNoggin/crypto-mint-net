# GUI-022: A wallet command opened before a passphrase change still works with the old passphrase, and a stale `passwd` overrides the change

| Field | Value |
| --- | --- |
| Severity | medium |
| Status | open (with Source) |
| Found on | main @ 052d032 (Source BLUE-016 / D146, `Wallet.Hold`) |
| Component | pkg/wallet/wallet.go (`Hold`: on a changed file it adopts the new file, seal included, and keeps the seed unlocked from the old one), pkg/cli/wallet.go (`unlocked` checks the passphrase against the copy `Open` read, before `Hold`); the GUI's Wallet forms, which can wait at a passphrase prompt for as long as the page is open |
| Surface | local (the wallet's passphrase seal) |
| Fixed by | _(pending — routed to Source by GUI/Blue in Round 17C via GUI/Grey; already flagged to Source/Grey for its Round 17. The GUI needs no change.)_ |

## Summary

D146 (RED-039) added `Wallet.Hold`. Each saving command now runs in this
order:

1. `Open` reads the file.
2. `unlocked` asks for the passphrase and checks it against the seal in
   **that** copy.
3. `Hold` takes the lock and re-reads the file. If another command saved
   meanwhile, `Hold` **replaces** `w.f` with the new file, including its
   `KDF` and `Sealed` seal, and keeps `w.seed` open. Its comment explains
   why: "The seed stays open: it is the same seed under any passphrase."
4. The command acts and saves.

Under D145, a command whose copy was stale was refused at `Save`. That
is how GUI-019's passphrase revert was closed. `Hold` now turns that
refusal into a merge. As a result, a command that opened the wallet
before a passphrase change still takes the **old** passphrase, and then
goes on:

- **A stale `passwd` overrides the rotation.** Say a `passwd` was started
  by someone who knows only the old passphrase, and it is waiting at
  `new passphrase:`. The owner then changes the passphrase. When the stale
  `passwd` is answered, it adopts the owner's file and re-seals it under
  its own new passphrase. The owner's new passphrase stops working, and
  the person who knew only the old one now holds the wallet.
- **Any other stale command runs on the old passphrase.** `address`,
  `request`, `claim`, `pay`, `submit`, `resend` and the rest, if opened
  before the change and answered afterwards with the old passphrase,
  unlock against their stale copy, then `Hold` and act. A `pay` sends a
  payment that the old passphrase alone authorized. The new seal is kept
  in the file, so the damage is the action itself.

Rotating the passphrase is how a user revokes a passphrase that leaked.
After this change, rotation does not revoke commands that are already
waiting. In the GUI those are easy to leave behind: a Wallet form's job
can sit at its masked prompt indefinitely, and it is listed in Jobs.

## Reproduction

poo at 052d032, `poo init -example`, and a wallet made with passphrase
`old`. The driving script uses two processes, one waiting on its stdin
prompt.

**Stale `passwd` overrides the rotation:**

1. Start `poo wallet passwd -passphrase-file old.txt` (holding `old`). It
   unlocks with `old` and waits at `new passphrase:`.
2. Run `poo wallet passwd` with `POO_PASSPHRASE=old` and
   `POO_NEW_PASSPHRASE=rotated`. It prints `passphrase changed`.
3. Answer step 1 with `mine`. It prints `passphrase changed`.
4. `wallet seed` with `rotated` fails with "wallet: wrong passphrase".
   With `mine` it succeeds, and with `old` it fails.

**Stale command accepted on the old passphrase:**

1. Start `poo wallet address -label slow` with no passphrase in the
   environment. It reads the wallet and waits at `passphrase:`.
2. `passwd` changes `old` to `new`.
3. Answer step 1 with `old`. It gives out an address, saves, and exits 0.
   After that, `new` unlocks the wallet and `old` doesn't, but the old
   passphrase has just authorized a wallet action.

On 6a1cdce, before `Hold`, the stale `address` in step 3 was refused
with `ErrChanged`, and so was the stale `passwd`.

A `pay` started before the change behaves like `address`. It sends a
payment authorized only by the old passphrase. Note that `pay` already
sent before its `Save` under D145, so that part is older than D146. D146
made it succeed cleanly.

## Impact

- A passphrase rotation can be undone by a `passwd` that started before
  it, run by someone who knew only the old passphrase. The owner is then
  locked out of their wallet until they restore it from the seed.
- A command left waiting before a rotation still spends with the
  leaked passphrase.

This needs a command started before the rotation, and the old
passphrase. Nobody remote can do it. It is rated **medium**, as GUI-019
was, because it defeats passphrase rotation, the one control a user has
over a leaked passphrase. Funds can move (stale `pay`), and the owner can
be locked out (stale `passwd`).

## Recommendation

Source (pkg/wallet), so the terminal and the GUI are fixed together.
Keep D146's merge for everything but the seal:

- In `Hold`, when the file changed, compare its `KDF` and `Sealed` with
  those of the copy this wallet was unlocked against. If they differ,
  release the lock and return an error, without adopting the file. The
  error could read: "the wallet's passphrase was changed meanwhile; run
  the command again and give the current passphrase". Nothing has been
  sent at that point, so running again is safe.
  - The other option is to make `unlocked` open the file after the prompt
    is answered, or check the passphrase again against the file read
    under `Hold`. That way the passphrase is always checked against the
    current seal.
- `passwd` in particular must never re-seal a file whose seal it didn't
  unlock.
- Tests:
  - Two `Open`s; the first rekeys and saves; the second, unlocked with
    the old passphrase, gets an error from `Hold`, and the first's
    passphrase stands.
  - The `passwd` race above leaves `rotated` in force.

## Routing (GUI/Blue, Round 17C)

GUI/Blue makes no GUI change. The seal check and `Hold` live in pkg/wallet
and pkg/cli, which the GUI's forms and terminal run as is. A guard only
in the GUI could not cover the terminal or other shells, and would make
the GUI act differently from headless poo. GUI/Blue passed the finding
to Source through GUI/Grey. Once Source's fix lands, GUI/Blue can add a
GUI e2e check: leave one wallet form at its passphrase prompt, change the
passphrase in another, and expect the waiting one to be refused with the
old passphrase.
