# GREEN-036: Getting started covers health, Watch and grinding, and links each view

| Field | Value |
| --- | --- |
| Status | proposed (Round 11) |
| Based on | claude/gui-4-watch @ 4be2230 |
| Area | gui/web/static/start.js, style.css |
| Effort | small |
| Decided by | (Grey) |

## Summary

The Getting started guide still stops at wallets. It has never mentioned
`poo doctor`, `poo watch` or `poo grind`, even though the Health check,
the Watch view and the Work view are now some of the GUI's largest
parts. This adds four steps after "Look at the node":

- Check its health: `poo doctor`
- Watch it work: `poo watch`
- See what can grind: `poo grind devices`
- Help secure the ledger: `poo grind start tip`

Each step about the node or wallet also gets an "Open the X view" link to
the view that runs the same command with forms.

## Problem

- A new user who follows the guide runs a node and makes a wallet. They
  never learn that their node can grind layers to make payments safer,
  which is the reason the Work view exists.
- The guide teaches only the terminal. Nothing tells someone who likes
  the forms where each step lives in the GUI.

## Idea

- Four new entries in `steps`, in the same shape as the rest. Each is one
  `poo` command line and runs only in the terminal. The commands and
  their wording come from `poo help` and `poo grind help`.
- An optional `view` on a step adds a plain `#/view` link after the
  "Run in terminal" button. The steps that get one are status and doctor
  (Node), watch (Watch), the two grind steps (Work), and the two wallet
  steps (Wallet).
- The order follows what each step needs: the node runs first, then
  health, watching, grinding, and the wallet.

## Cost and risk

About 20 lines. No logic. The steps run nothing on their own: the
terminal runs exactly what is shown. `grind start tip` needs a running
node, as the step before it says, and poo explains if there isn't one.

## Prototype

On `claude/gui-green`. `gui/e2e/qol.mjs` checks that "Help secure the
ledger" shows `poo grind start tip` and links `#/work`. It also checks
that the "Check its health" link opens the Node view. All eight e2e
scripts pass.
