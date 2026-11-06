# GREEN-007: Next steps under errors, and helpful empty states

| Field | Value |
| --- | --- |
| Status | adopted |
| Based on | claude/gui-5-release @ 7209952 |
| Area | gui/web/static (new hints.js, showResult and panel in views.js) |
| Effort | small |
| Decided by | GUI/Grey, 2026-10-27 |

## Summary

When a command fails with an error a new user meets often, the view
still shows the error exactly as printed, and adds one line under it that
says what to do, with a link to the view that does it. A panel can also
say what an empty result means instead of "No output."

## Problem

Opening the GUI without a running node fills most views with
`poon: rpc: is the node running? open …/rpc.addr: no such file or
directory`. It is accurate, but a newcomer does not know that the fix is
the Node view's "Start node" button. Without a node policy the wallet
says `open …/node.json: no such file or directory`. Node policies with no
saved policy shows "No output."

## Idea

- `hints.js` is a short table of `{match, text, view, label}`. In
  `showResult`, the first hint whose pattern matches stderr is shown under
  the error as a line with a link to that view. Three hints for now:
  - No node answering: link to Node.
  - No node policy (`node.json` missing): link to Node.
  - No wallet file (`wallet.json` missing): link to Wallet.
- The error text is never changed or hidden. A hint that no longer
  matches, because a message changed in `pkg/cli`, simply is not shown.
- `panel()` takes an optional `empty` text. The Node policies panel uses
  it: "No saved node policies yet. Write one on the Node view."
- The terminal is left alone, so it stays exactly like `poon` in a shell.

A sturdier version is for `pkg/cli` to print stable error codes, for
example `poon: E_NO_NODE: …`, so hints match a code instead of text. That
is a headless change and Grey's call, so it is only noted here.

## Cost and risk

About 40 lines. Matching on error text is brittle, but a miss only means
no hint. Each hint is prose, so it needs to be kept true when views move.

## Prototype

`gui/web/static/hints.js`, `showResult` and `panel` in `views.js`, the
`.next-step` and `.hint.empty` CSS, and checks in `gui/e2e/qol.mjs` (the
error keeps its exact text, the hint links to Node, and the empty
policies text shows).

## Decision

Adopted. The command's error stays exactly as printed; the hint only links to a view. Hints match poon's error text, so a reworded error just drops its hint.
