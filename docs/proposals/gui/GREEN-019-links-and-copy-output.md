# GREEN-019: Small polish: themed links and Copy output

| Field | Value |
| --- | --- |
| Status | adopted; Copy output left out where output holds a seed (keygen) |
| Based on | claude/gui-4-watch @ 9b4b19c |
| Area | gui/web/static (views.js showResult, style.css) |
| Effort | small |
| Decided by | GUI/Grey, 2026-10-28 (Round 4 Part A) |

## Summary

Two small fixes found while exploring:
1. The node banner's link used the browser's default blue, which clashes
   with the theme and is hard to read in dark mode. All links in the
   content now use the accent colour.
2. Every command result gets a "Copy output" button that copies stdout
   exactly as poo printed it, for a script, a bug report or a co-signer.

## Problem

- The new banner was the only place in the GUI with an unstyled link.
- Results are rendered as tables, which is good for reading but loses
  the JSON. Getting the raw output meant re-running the command in the
  terminal and selecting text.

## Idea

- One CSS rule, `main a:not(.skip) { color: var(--accent) }`.
- In `showResult`, a small button after the rendered output. Copying is
  allowed to fail, and the button then says "Copy refused".
- Copy is always user-initiated, and copies only what is already on
  screen. Output that holds a secret, such as `poo wallet seed`, is
  copied only if the user clicks. Grey or Red may want such outputs to
  skip the button. That can be a per-form flag.

## Cost and risk

About 15 lines.

## Prototype

On the branch. `gui/e2e/qol.mjs` checks that the Series and keys
view's result has the button.
