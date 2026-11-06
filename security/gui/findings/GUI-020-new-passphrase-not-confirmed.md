# GUI-020: A new wallet passphrase typed into the masked field is never asked for twice, so one typo locks the wallet

| Field | Value |
| --- | --- |
| Severity | low |
| Status | fixed by Source (asks twice), merged Round 15; the GUI shows the second ask through State.Asks; pending Red's verification |
| Found on | claude/gui-4-watch @ 3a7a39c |
| Component | pkg/cli/wallet.go (`walletPasswd` "new passphrase:", `walletCreate` "new passphrase:"), the GUI's "Change the passphrase" and "Create or restore" forms |
| Surface | local (access to the user's own wallet) |
| Fixed by | Source 46bcb9e (D145), routed by GUI/Blue in Round 14C via GUI/Grey; merged into GUI main in Round 15 |

## Summary

The new "Change the passphrase" form runs `poon wallet passwd`. poon asks for
the old passphrase, then for `new passphrase:` **once**, in a masked field,
and re-seals the seed under whatever was typed. `wallet create` does the
same for a new wallet. Nothing asks for the new passphrase a second time.

A typo in a masked field goes unnoticed, and the wallet is then sealed
under a passphrase the user doesn't know:

- After `passwd`, the wallet can be opened again only by restoring from a
  seed backup.
- After `create`, a wallet the user then receives funds into can be
  opened only through `wallet seed`, which needs the very passphrase they
  don't know. If they never backed the seed up, the funds are out of reach.

The terminal prompt has the same gap. The GUI form makes the one-shot,
masked entry the normal way to change a passphrase.

## Impact

Self-inflicted loss of access to the wallet, possibly permanent if the
seed was not backed up. No attacker is involved. Rated **low**.

## Recommendation

Source, for both the terminal and the GUI, since the prompts come from poon:

- When the new passphrase is read interactively (`readSecret`, not
  `-new-passphrase-file`, `$POON_NEW_PASSPHRASE` or `$POON_PASSPHRASE`), ask
  `new passphrase again:` and refuse on a mismatch before re-sealing or
  creating. The GUI then shows the second masked field with no change of
  its own.
- Test: `passwd` and `create` with two different interactive answers fail,
  and the wallet keeps its old seal.

## Routing (GUI/Blue, Round 14C)

GUI/Blue makes no GUI change. The prompt comes from poon, and the GUI shows
each prompt poon asks for. A second field that only the GUI shows would
make the GUI act differently from the terminal and headless poon. So the
fix belongs to Source. GUI/Blue passed it to Source through GUI/Grey.
Once poon asks `new passphrase again:`, the GUI shows that second masked
field unchanged, and GUI/Blue can add an e2e check that a mismatch is
refused.

## Notes — Round 15B verification (held up)

- `create` and `passwd` now read a typed new passphrase through
  `readNewSecret`. When it comes through `Stdio.Secret` (the headless
  terminal's hidden read, or the GUI's masked field), poon asks
  `new passphrase again:` and refuses a mismatch ("the two new passphrases
  differ, so nothing was changed") before it seals or creates anything.
  Piped input, `-new-passphrase-file`, `$POON_NEW_PASSPHRASE` and
  `$POON_PASSPHRASE` are read once, as scripts expect.
- **GUI, the second ask.** Each read that starts waiting raises
  `State.Asks`, and the event stream sends a new `state` event when
  `Asks` changes. So the form shows its field again for the second
  prompt, even when the stream never saw `waiting` go false in between.
- **Masking order.** `readSecret` sets `Secret` before its read starts
  waiting, and clears it only after the read has returned. A plain read
  that follows starts waiting only after that. No state the stream can
  read pairs a waiting secret prompt with `secret: false`, or a plain
  prompt with a stale `secret: true`.
- `gui/e2e/terminal.mjs`, `views.mjs` and `viewonly.mjs` pass locally on
  55cf745. They cover two different answers being refused, two matching
  ones making the wallet, and neither passphrase being echoed.
