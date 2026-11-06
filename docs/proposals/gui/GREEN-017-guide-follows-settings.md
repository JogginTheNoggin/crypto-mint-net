# GREEN-017: Getting started follows the Settings flags

| Field | Value |
| --- | --- |
| Status | adopted |
| Based on | claude/gui-4-watch @ 9b4b19c |
| Area | gui/web/static/start.js |
| Effort | small |
| Decided by | GUI/Grey, 2026-10-28 (Round 4 Part A) |

## Summary

A mismatch: when Settings names a root or node policy, the views use it,
but the Getting started steps did not. "Write a node policy" then wrote to
`~/.poon` while the Node view looked in the root from Settings. Now the
steps that take `-root` and `-policy` (`init`, `node`, `status`,
`wallet create`, `wallet address`) add them, and show them in the line.

## Problem

A user who set a root in Settings, followed the guide and then opened the
Node view saw "No node is answering". The guide had started a node under
a different root. Nothing on the page showed why.

## Idea

- Each step that acts on a node policy is marked `policy: true`, and its
  line gets `where.flags()`, quoted as a shell would quote them.
- With nothing set in Settings, every line is exactly what it was, so the
  guide still teaches the plain shell commands.
- `version`, `demo` and `crypto` take no policy, and stay as they are.

## Cost and risk

About 10 lines. The lines are still run through the terminal, which adds
nothing of its own: the flags are visible in the step before it runs.

## Prototype

`start.js`. `gui/e2e/qol.mjs` sets a root in Settings and checks that
"Write a node policy" shows `poon init -example -root /tmp/qol-root`, while
"Check the tool" still shows `poon version`.
