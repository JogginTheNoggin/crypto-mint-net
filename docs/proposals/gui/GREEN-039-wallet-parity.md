# GREEN-039: Every poo wallet command has a form, so a shared payment can be finished in the GUI

| Field | Value |
| --- | --- |
| Status | adopted (Round 14): wallet forms; Follow more's Replace on Source `follow -replace`; Submit form on `poo submit` |
| Based on | claude/gui-4-watch @ 376e964 |
| Area | gui/web/static/views.js (Wallet view) |
| Effort | small |
| Decided by | (Grey) |

## Summary

`poo wallet help` lists 17 subcommands, and the Wallet view had forms for
10 of them. These 7 were missing: `policy`, `cosign`, `submit`, `resend`,
`follow`, `passwd` and `seed`. The worst result was a dead end. The Pay
form has a "Co-signing file" field for a shared policy, but nothing in the
GUI could record that policy, co-sign the file, or submit it. A shared
payment started in the GUI had to be finished in a shell.

This adds the seven forms. It also adds an e2e check that fails when
`poo wallet help` lists a subcommand the Wallet view never runs, so the
two stay level from now on. This is the GUI side of Source Green's parity
test.

## Problem

- The headless rule is that the GUI is the same `poo` with a visual
  layer. A user who works only in the GUI could not:
  - finish a k-of-n payment (`policy`, `cosign`, `submit`);
  - send dropped legs again (`resend`), although "Payments sent" shows
    them;
  - switch to light mode (`follow`);
  - change their passphrase (`passwd`);
  - back up their seed (`seed`). Creating a wallet even prints "back up
    the seed (poo wallet seed)".
- Nothing would catch the next subcommand Source adds.

## Idea

- Each is one form running one `poo wallet` line with the view's
  `-wallet`. They follow the same rules as the forms there today:
  - Passphrases and the new passphrase go through poo's own masked
    prompts. I checked this with a real create, then passwd, then seed run.
  - "Show the seed" uses `secretOutput`, as "Make key" does, so the seed
    stays hidden until shown and Jobs hides it. It asks for confirmation.
  - "Submit a co-signed payment" and "Follow only the watched mints" ask
    for confirmation, as Pay and Claim do. Follow replaces the node's
    follow list.
- They go next to the forms they belong with:
  - The shared-payment trio goes after Pay.
  - "Send dropped legs again" goes before Watch mints.
  - Follow, passwd and seed go before Create.
- The e2e reads `poo wallet help` at test time, so the list of commands
  comes from poo, not the GUI.

## Not covered here (follow-ups once Source merges them)

- Source Green's GREEN-038, `poo follow -replace` (claude/source-green @
  1e3ee01). The Node view's follow forms would get a "Replace the follow
  list" checkbox once it lands. Note that Source's GREEN-038 and the GUI's
  GREEN-038 (the reload notice) share a number.
- Source's proposed commands for submitting an entry, submitting a layer
  and showing one grind job. Each would get a form, or a row action in
  Work's Jobs table, when it exists.
- The same help-driven check could cover `poo grind`, `poo peer` and
  `poo policy`. Wallet was the only one with a gap today.

## Cost and risk

About 25 lines of form definitions. No new logic, and every action is a
`poo` line shown in its preview. View-only mode refuses these commands
the same way it refuses the existing wallet writes; the view-only e2e
still passes.

## Prototype

On `claude/gui-green`.

- `gui/e2e/qol.mjs` checks the previews for policy (`-k 2 -key self -key
  ed25519:ab`), cosign, submit, resend, follow, passwd and seed. It then
  checks that every subcommand in `poo wallet help` is run by some
  Wallet form.
- I ran a scratch script once against a real wallet:
  - create, then passwd (old and new passphrase prompts, both masked);
  - then seed (masked prompt, and the output hidden behind Show).
- All eight e2e scripts pass.
