# GREEN-016: One wallet file for the whole Wallet view

| Field | Value |
| --- | --- |
| Status | adopted |
| Based on | claude/gui-4-watch @ 9b4b19c |
| Area | gui/web/static/views.js (walletView), style.css |
| Effort | small |
| Decided by | GUI/Grey, 2026-10-28 (Round 4 Part A) |

## Summary

The Wallet view had a "Wallet file" field in all ten of its forms. Now it
has one, at the top of the view. Every form uses it, it is remembered per
browser like the Settings flags, and changing it updates every form's
command preview at once.

## Problem

A user with a second wallet (`-wallet savings.json`) had to type the file
into each form before using it, and could easily check the balance of one
wallet and then pay from another. Ten copies of the same field also made
the view long.

## Idea

- One "Wallet file (-wallet)" field in a small panel above the forms.
  Empty means `wallet.json` in the root, exactly as in a shell.
- Every form adds `-wallet FILE` through the same `wf()` as before, so
  each preview still shows the full command, `-wallet` included.
- The value is kept in `localStorage` under its own key, so it never
  touches Settings' saved flags.

## Cost and risk

About 20 lines. A user who wanted a different file for one form now
changes the top field, or uses the terminal. If Grey would rather keep it
with -root and -policy, it fits in Settings as a third field.

## Prototype

`walletView` in `views.js`. `gui/e2e/qol.mjs` checks that there is one
wallet field and that the Balance preview reads
`poon wallet balance -wallet w2.json`. `views.mjs` still creates a wallet
through the view.
