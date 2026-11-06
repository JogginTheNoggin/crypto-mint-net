# GREEN-003: Accessibility fixes

| Field | Value |
| --- | --- |
| Status | adopted |
| Based on | claude/gui-5-release @ f93508e |
| Area | gui/web/static (index.html, app.js, terminal.js, style.css) |
| Effort | small |
| Decided by | GUI/Grey, 2026-10-27 |

## Summary

Small changes that make the GUI usable with a keyboard and a screen
reader: a skip link, `aria-current` on the active view, a page title per
view, visible focus outlines, and a terminal that announces when a
command ends instead of reading every chunk of output aloud.

## Problem

- The terminal output was `aria-live="polite"`, so a screen reader would
  read a streaming `poon node` log line by line, forever.
- The active view was shown only by colour and a bar.
- There was no way to jump past the navigation, and buttons had no
  visible focus ring in some themes.
- Every tab was titled "poon GUI", which is unhelpful with several open.

## Idea

- Output stays readable but is no longer live; a hidden `role=status`
  region says "Command finished." or "Command failed with exit N." when a
  command ends.
- The active nav link gets `aria-current="page"`, and `document.title`
  becomes "View · poon GUI".
- A "Skip to content" link, visible on focus, moves focus to `main`
  without touching the hash the router uses.
- `:focus-visible` gets a clear outline in the accent colour.

Further ideas, not prototyped: label every form field with its flag name
for screen readers (`aria-describedby` on the command preview), honour
`prefers-reduced-motion` if animation is ever added, and an audit with
axe-core in the e2e run (it would be a dev-only dependency).

## Cost and risk

About 30 lines across four files. No behaviour changes for mouse users.

## Prototype

On the branch, with checks in `gui/e2e/qol.mjs` (aria-current, the status
announcement, and the skip link).

## Decision

Adopted as prototyped.
