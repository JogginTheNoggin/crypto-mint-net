# GREEN-005: A command palette (Ctrl-K)

| Field | Value |
| --- | --- |
| Status | deferred |
| Based on | claude/gui-5-release @ f93508e |
| Area | gui/web/static (new palette.js) |
| Effort | small |
| Decided by | GUI/Grey, 2026-10-27 |

## Summary

Ctrl-K (Cmd-K on macOS) opens a search box over every view, every form
title and every Getting started step. Choosing one jumps to it, with the
form focused. It is navigation only.

## Problem

The views now hold more than thirty forms across eight screens. Finding
"Where a payment stands" means knowing it lives under Pools and payments.

## Idea

- Index the titles the views already declare (`panel` and `form` titles)
  plus the nav entries. Fuzzy-match on typing, Enter to go there.
- Typing a line that starts with `poon ` or `>` offers "Run in terminal",
  so the palette is also a quick way to the terminal.

## Cost and risk

About 100 lines. It needs the views to expose their form titles without
mounting them, which is a small refactor of `views.js` (a static list of
titles per view). Not prototyped yet, to avoid touching `views.js` while
Grey is still building views.

## Prototype

None.

## Decision

Deferred. Navigation and Tab completion cover most of it today. Revisit with GREEN-004, which would give the palette its command list.
