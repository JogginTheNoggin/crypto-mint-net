# GUI-019: Wallet commands read the whole wallet file and write it back, so one waiting at a passphrase prompt undoes another, even a passphrase change

| Field | Value |
| --- | --- |
| Severity | medium |
| Status | fixed by Source (D144/D145), merged Round 15; pending Red's verification |
| Found on | claude/gui-4-watch @ 3a7a39c |
| Component | pkg/wallet/wallet.go (`Open`, `Save`), pkg/cli/wallet.go (every subcommand that saves: `passwd`, `address`, `pay`, `claim`, `watch`, `request`, `policy`, `cosign`, `submit`, `resend`, …); the GUI's Wallet view runs them as independent forms |
| Surface | local (wallet.json: its seal, keys, payment records) |
| Fixed by | Source 46bcb9e (D145), routed by GUI/Blue in Round 14C via GUI/Grey; merged into GUI main in Round 15 |

## Summary

Each `poon wallet` command does the same three things:

1. `wallet.Open` reads the **whole** wallet.json into memory.
2. It asks for the passphrase if it needs one (`f.unlocked`). In the GUI,
   that is a masked field that waits until the user answers.
3. It changes its copy and `Save`s it, which replaces the whole file through
   one fixed temporary name, `wallet.json.tmp`.

No lock, version or re-read sits between step 1 and step 3. Whichever
command saves last overwrites everything the others saved since it read the
file, and every command reports success.

The new Wallet forms (GREEN-039) make this easy to hit. Every wallet command
now has its own form, the forms run independently (there is no shared
gate, unlike the Schedule panel), and a form's job can wait at its masked
passphrase prompt for as long as the page is open. A user who opens
"Balance" or "Get an address", gets the passphrase prompt, and does
something else first keeps a stale copy of the wallet alive in that job.

Two concrete effects, both reproduced:

- **A passphrase change is silently reverted.** A command that read the
  wallet before `passwd` ran and saves after it writes back the old seal.
  The new passphrase then fails, and the **old** one, the one the user
  meant to retire (for example because it leaked), unlocks the wallet
  again.
- **Keys and payment records are lost.** `passwd`, or any long-waiting
  command, saving after another command drops that command's changes:
  - an address given out (the key's `used` flag and label);
  - a payment's `Sent` record, which `wallet status` and `wallet resend`
    rely on;
  - watched mints, requests, and shared policies.

  A lost `used` flag means the same address, or a change address, is
  handed out again.

Two `Save`s at the same moment can also both write `wallet.json.tmp`
before either renames it, so the file renamed into place can be a mix of
both. `Save` doesn't fsync before the rename either.

## Reproduction

Both runs use the `poon` binary at 3a7a39c, a data directory from
`poon init -example`, and a wallet made with passphrase `old`. Driving
script: two processes, one waiting on its stdin prompt.

**Passphrase reverted:**

1. Start `poon wallet address -label slow` with no `POON_PASSPHRASE`. It reads
   the wallet and waits at `passphrase:`. This is the GUI's "Get an address"
   form waiting at its masked field.
2. Run `poon wallet passwd` with the old passphrase `old` and the new
   passphrase `new`. It prints `passphrase changed`.
3. Answer step 1's prompt with `old`. It gives out an address and saves.
4. `wallet seed` with `new` fails with "wallet: wrong passphrase". With
   `old`, it works.

**Address lost:**

1. Start `poon wallet passwd` (old passphrase from a file). It waits at
   `new passphrase:`.
2. Meanwhile, `poon wallet address -label during` gives out `d38c799f…`,
   and `addresses` lists it as used.
3. Answer `new` to step 1. `addresses` (with `new`) now lists only the
   `before` address as used. `d38c799f…` is unused again, and its label is
   gone.

## Impact

- The user's passphrase change, a security action, is undone with no
  error, so a leaked passphrase keeps working.
- Address reuse, and loss of the wallet's own payment records (status and
  resend), without any message.

No attacker is needed; this is the user's own GUI with two wallet forms in
use. The wallet's funds are still on the ledger and the seed is unchanged,
so nothing is stolen by this alone. Rated **medium** because it silently
defeats passphrase rotation.

## Recommendation

Source (pkg/wallet / pkg/cli), so the terminal and GUI are fixed together:

- Make `Save` refuse to overwrite a file that changed since `Open` read it.
  Keep a hash of the bytes read, and under an exclusive lock on
  `wallet.json.lock`, re-read and compare before writing. On a mismatch,
  fail with "the wallet changed meanwhile (another poon wallet command
  saved it): run this again". Then a stale copy can never write back an
  old seal or drop another command's keys or payments.
  - The lock is held only around compare-and-write, not across the
    passphrase prompt, so a form waiting at its prompt never blocks the
    others.
  - Alternatively, take the lock across the whole command, and have `Open`
    wait or fail if it is held. That blocks other commands while one waits
    at a prompt, which is worse in the GUI.
- `passwd` in particular: re-read the file under the lock right before
  `Rekey`, so it never seals a stale copy.
- Give `Save` a unique temporary file (`os.CreateTemp` in the same
  directory) and fsync before the rename, as `nodepolicy.Save` does (D114).
- Test:
  - Open the wallet twice. Save the first after a change, then expect
    the second's `Save` to fail.
  - The `passwd`-then-stale-`address` sequence above should leave the new
    passphrase in force.

## Routing (GUI/Blue, Round 14C)

GUI/Blue makes no GUI change. The read-then-write-back is in pkg/wallet
and pkg/cli, which the GUI's forms and terminal run as is. A GUI-only
guard, such as letting only one wallet form run at a time, would leave the
terminal and other shells open to the same race. It would also make the
GUI act differently from headless poon, which the GUI must never do. So
the fix belongs to Source. GUI/Blue passed it to Source through GUI/Grey.
Once Source's fix lands, GUI/Blue can add a GUI e2e check: start two
wallet forms, let one wait at its prompt, save the other, then expect the
waiting one to be refused.

## Notes — Round 15B verification (held up)

- **D145, verified.**
  - `Save` takes an exclusive lock on `wallet.json.lock`, re-reads the
    file, and writes only if it is byte for byte what `Open` read or this
    wallet last wrote. Otherwise it returns `ErrChanged` and writes
    nothing. The lock is held only around compare-and-write, so a form
    waiting at its prompt blocks no other form.
  - Both R14B races were re-run on the new binary:
    - `passwd` waiting at `new passphrase:` while `address` saves:
      `passwd` is refused ("the file changed since it was read … run the
      command again"), and the `during` address stays used.
    - `address` waiting at `passphrase:` while `passwd` saves: `address`
      is refused. `new` unlocks the wallet and `old` does not, so the
      passphrase change stands.
  - `Create` over a file that appeared meanwhile returns `ErrExists`, and a
    save over a removed file returns `ErrChanged`.
  - The temporary file is still the fixed `wallet.json.tmp`. It is now
    written only under the lock, so two poon commands can no longer mix
    their writes.
  - Windows has a real lock (`LockFileEx`). `GOOS=windows` builds.
  - `go test ./pkg/wallet -run 'Save|Concurrent' -count=3` passes.
- **Covered by Source, not repeated here.** A refused `Save` after
  `pay`, `claim`, `submit` or `resend` already sent something loses the
  record and says "run the command again". That is RED-039, and Source
  BLUE-016 / D146 fixes it in the Source Round 16 queue. The GUI's Wallet
  forms run those same commands, so they are fixed along with it.
- **Residual, durability (optional, Source).** `Save` still doesn't
  fsync the temporary file before the rename. After a power loss, the
  rename can land without the data, and a wallet file the user saw as
  saved can come back empty. `nodepolicy.Save` already syncs (D114).
  This isn't a finding on its own, since no attacker or second command
  is involved. It is noted so the recommendation above is closed in full.
- **Windows for GUI-017.** The wallet now has a Windows lock, but
  `nodepolicy.Lock` is still a no-op off unix. The same `LockFileEx` code
  would close GUI-017's Windows residual.
