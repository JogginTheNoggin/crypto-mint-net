# GUI-022: A wallet command opened before a passphrase change still works with the old passphrase, and a stale `passwd` overrides the change

| Field | Value |
| --- | --- |
| Severity | medium |
| Status | fixed by Source (D149 `Hold`, merged Round 18; D151 `Unlock` re-checks the seal, merged Round 19); verified by GUI/Red R19B @ e71e317 |
| Found on | main @ 052d032 (Source BLUE-016 / D146, `Wallet.Hold`) |
| Component | pkg/wallet/wallet.go (`Hold`: on a changed file it adopts the new file, seal included, and keeps the seed unlocked from the old one), pkg/cli/wallet.go (`unlocked` checks the passphrase against the copy `Open` read, before `Hold`); the GUI's Wallet forms, which can wait at a passphrase prompt for as long as the page is open |
| Surface | local (the wallet's passphrase seal) |
| Fixed by | Source, in part: D149 (`Hold` refuses a changed seal). The rest (`wallet seed` and `wallet cosign`) was routed to Source by GUI/Blue in Round 18C via GUI/Grey and queued for Source Round 18. The GUI needs no change. |

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

## Notes — Round 18B verification (partly held)

Checked on main @ 3804c55 with the same two-process scripts.

- **D149 holds for every command that `Hold`s.**
  - Stale `passwd`: the owner rotates `old` to `rotated`, then a `passwd`
    that unlocked with `old` is answered with `mine`. It is refused:
    "the wallet's passphrase was changed meanwhile, so nothing was done".
    `rotated` still unlocks the wallet; `mine` and `old` don't.
  - Stale `address` answered with `old` after a rotation: refused the
    same way, and the new passphrase stands.
  - `sameSeal` compares every `KDF` field (name, iterations, memory,
    threads, salt) and the sealed bytes, so any re-seal is caught.
  - Every saving command (`passwd`, `address`, `watch`, `claim`, `pay`,
    `request`, `receive`, `status`, `resend`, `policy`, `submit`) goes
    through `holdWallet`.
- **Still open: commands that unlock without `Hold`.**
  - `wallet seed` and `wallet cosign` check the passphrase against the
    copy `Open` read, and never `Hold`, so D149 never runs for them.
  - Reproduced: `wallet seed` started before the rotation, waiting at
    `passphrase:`. The owner rotates `old` to `rotated`. Answered with
    `old`, the stale `seed` **prints the seed**. A fresh `seed` with
    `old` fails with "wrong passphrase".
  - So, after a passphrase leak and a rotation, a `seed` form that was
    left open still gives the seed (the whole wallet) to whoever knows
    the leaked passphrase. A stale `cosign` still signs a co-signing file
    with the old passphrase.
  - The GUI's "Show seed" form, like every Wallet form, can wait at its
    masked prompt for as long as the page is open.
- **Recommendation (Source).** Check the passphrase against the current
  file in one place, for every command:
  - In `walletFlags.unlocked`, re-read the file after the passphrase is
    answered and refuse with `ErrPassphraseChanged` if its seal differs
    from the copy unlocked. Or open the file only after the prompt, for
    commands that need a passphrase.
  - Or give `seed` and `cosign` the same short `Hold` (or a read-only
    seal check under the lock) before they use the seed.
  - Test: stale `seed` and stale `cosign` after a rotation are refused.

## Routing (GUI/Blue, Round 18C)

The part still open is in pkg/cli: `wallet seed` and `wallet cosign`
unlock the copy `Open` read and never call `Hold`, so a stale passphrase
still works for them. GUI/Blue passed it to Source through GUI/Grey, as
Red suggested: check the seal in `walletFlags.unlocked`, or `Hold` in
those two commands. Source/Grey has it queued for Round 18. The GUI's
"Show seed" and cosign forms run these commands as they are, and need no
change.

## Notes — Round 19B verification (held up)

- **D151, verified.** `Unlock` reads the file again before it opens the
  seed. If the file's seal is no longer the one `Open` read, it returns
  `ErrPassphraseChanged`, so a command that never `Hold`s is covered too.
  - The R18B race was re-run on e71e317. A stale `wallet seed` answered
    with `old` after the owner rotated `old` to `rotated` now fails with
    "the wallet's passphrase was changed meanwhile, so nothing was done".
    No seed is printed.
  - `cosign` goes through the same `Unlock`, so it is covered by the
    same check.
  - The R17B/R18B `passwd` and `address` races are still refused.
- **Edge, not a finding.** If the re-read fails (the file was removed or
  can't be read), or doesn't parse, `Unlock` skips the check and opens
  the copy already in memory. A rotation always writes the file
  atomically, so this needs the file to be removed after the rotation.
  That isn't a passphrase change. Source can make the check fail closed
  if it wants to.
