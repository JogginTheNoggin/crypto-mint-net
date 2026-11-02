# GUI-GREEN-041: Forms take the flags poo takes: Build/Work grinding, the wallet's series, named transactions

| Field | Value |
| --- | --- |
| Status | proposed (Round 15) |
| Based on | claude/gui-4-watch @ 55cf745 |
| Area | gui/web/static/views.js (Pools and payments, Wallet) |
| Effort | small |
| Decided by | (Grey) |

The first proposal under the GUI-GREEN prefix.

## Summary

GREEN-039 and GREEN-040 made every poo command reachable. This round
checks a level deeper: for each form, the flags `poo CMD -h` lists
against the flags the form can send. Three real gaps remained:

1. **Build an entry and Add a layer** had no `-device`, `-threads`,
   `-for` or `-serial`. Build also lacked `-max-tries`, and Work lacked
   `-start`. These are the same grinding choices the Work view offers for
   jobs, but a one-off build or layer always used the defaults and could
   not be timed.
2. **The Wallet view** sent `-series` from only 3 of its 17 forms
   (Balance, Pay, Claim). Every `poo wallet` command takes `-series`. With
   two series followed, New address, Request, Receive and the rest
   failed with no way to say which.
3. **What has arrived** had no `-serial N -tid HASH` pairs. Those are the
   transactions a payer named, which `poo wallet receive` checks first.

## Idea

1. A shared `grindFields()` (Device, using the `grind-devices`
   suggestions; Threads; For) and `grindFlags(v)`, used by Build and Add
   a layer. Both forms also get the Mint serial field that Submit already
   uses ("or a mint serial"). Build gets "Most entry tries" and Work gets
   "First nonce".
2. The Wallet view's top panel gets **Series (-series)** next to the
   wallet file. `wf()` adds it to every wallet command, and it is kept
   per browser like the file. The three per-form Series fields go, so
   there is one place to set it.
3. Receive gets two list fields: "Transactions the payer named" and
   "Their mint serials", in the same order. poo pairs them and refuses
   counts that differ ("pairs with -tid").

No logic is added: each field is one flag, and poo checks the values.

## Checked and left alone

- `-wide` on the listing commands only changes poo's text tables. The GUI
  reads JSON.
- The filter flags (`-address`, `-tid`, `-kind`, …) on `ledger`,
  `pending`, `pools` and `watch` are already in Explore and Watch,
  through `filterFields`.
- `poo node -log-level/-log-format` aren't in "Run the node". The live
  log there is for reading, and a level choice would be a small follow-up
  if Grey wants it.
- `poo sim`'s 14 tuning flags: GREEN-040's form names the terminal for
  those.

## Cost and risk

About 30 lines. Two forms are longer. The Wallet change removes three
fields and adds one. A browser that had typed a series into Balance loses
nothing stored, because those fields were never kept.

## Prototype

On `claude/gui-green`.

- `gui/e2e/views.mjs` checks the Build preview `poo build … -serial 5
  -device cpu -threads 2 -for 5s -bytes`, and the Work preview with
  `-device cpu -start 7`.
- `gui/e2e/qol.mjs` checks that the view's series reaches Balance, and
  checks Receive's pairs (`-serial 3 -serial 5 -tid aa -tid bb`).

All eight e2e scripts pass.
