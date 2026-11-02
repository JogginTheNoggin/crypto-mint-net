# GREEN-001: A Getting started guide

| Field | Value |
| --- | --- |
| Status | adopted |
| Based on | claude/gui-5-release @ f93508e |
| Area | gui/web/static (new start.js, app.js nav) |
| Effort | small |
| Decided by | GUI/Grey, 2026-10-27 |

## Summary

A first view that walks a new user through poo as eight command lines,
each with a "Run in terminal" button: version, demo, crypto, init, node,
status, wallet create, wallet address. It teaches the headless tool while
using the GUI, and adds no logic.

## Problem

The GUI opens on the Node view, which assumes the user knows what a node
policy, a pool and a series are. A first-time user has no path from
"I downloaded this" to "I have a node running and an address to be paid
at". The design documents explain the concepts, but nothing in the GUI
points at an order of steps.

## Idea

- A "Getting started" view, first in the navigation, with numbered steps.
  Each step is a title, one sentence, the `poo` command line in code, and
  a button that types it into the terminal and runs it.
- The button is `openInTerminal`, the same path the views use. The guide
  never runs anything itself, and every line works unchanged in a shell,
  so the guide doubles as a CLI tutorial.
- Steps use the terminal, which adds no -root or -policy flags. That keeps
  the lines identical to what the user would type in a shell. (Grey may
  prefer the views' Settings flags instead; that is one `where.flags()`.)

Later, if Grey wants: mark a step done when its job exits 0 (read from
`/api/jobs`), and link each step to the view that covers it.

## Cost and risk

About 50 lines of JavaScript and a few CSS rules. It changes the default
landing view from Node to Getting started; an alternative is to place it
second, or show it only while `poo policy list` is empty.

The step texts are prose about commands, so they can go stale when a
command changes. They only describe; the command validates.

## Prototype

`gui/web/static/start.js`, the nav entry in `app.js`, the `.steps` CSS,
and checks in `gui/e2e/qol.mjs` (a step's button runs its line in the
terminal).

## Decision

Adopted as the landing view. Every step is a poo command line typed into the terminal, so it teaches the headless tool too.
