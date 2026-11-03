# GUI-GREEN-046: Copy a command line, to run it in a shell

| Field | Value |
| --- | --- |
| Status | proposed (Round 20) |
| Based on | main @ 9628426 |
| Area | gui/web/static/views.js (cmdLine, a shared copyButton), docs/FEATURES.md |
| Effort | small |
| Decided by | (Grey) |

## Summary

Every panel and form shows the exact `poo` line it runs, quoted the way
the platform's shell takes it (`api.quote`, pinned to `shell.Quote` by
the tests). The only thing you could do with that line was "Run in
terminal". This adds **Copy** next to it, so the line can go into a
shell, a script, a service file or a note, and run the same command
there.

That is the point of jerry's rule that the GUI is a visual layer over the
headless tool. The GUI then teaches headless use: build a command with a
form, copy it, and run it on a server that has no GUI.

## Problem

- Selecting the `code` text by hand is fiddly on long lines, and on a
  phone, where the line wraps.
- Results already have "Copy output", but the command that made them had
  no copy button.

## Idea

- A shared `copyButton(label, text, class)` in `views.js`. "Copy output"
  now uses it too, so there's one copy behaviour: "Copied" for a moment,
  or "Copy refused" when the browser refuses.
- `cmdLine` adds `Copy` (`button.copy-cmd`). It copies `api.quote(args)`,
  the line as shown, which includes Settings' `-root` and `-policy`, so
  the shell acts on the same node policy.
- **Secrets.** Copy is off, with a tooltip saying why, while the line
  holds a secret flag such as `-seed` or `-restore`. It uses the same
  `api.secretLine` the terminal uses to keep such lines out of its
  history (GUI-006), so a seed typed into "Make a key" doesn't go to the
  clipboard. It turns back on when the field is cleared.
- Modular: one helper, used in two places, with no per-view code. Every
  view gets it through `cmdLine`.

## Cost and risk

- About 20 lines. One more small button on each command row. The row
  already wraps, and bounds.mjs (no sideways scroll at phone width)
  passes.
- The clipboard is the browser's. The GUI reads nothing back and stores
  nothing.

## Prototype

In the PR. `gui/e2e/qol.mjs` checks three things in "Make a key":

- Copy puts exactly the previewed line on the clipboard.
- With a seed typed in, Copy is off.
- With the seed cleared, Copy is on again.

All nine e2e scripts pass.
