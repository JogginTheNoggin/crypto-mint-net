# GREEN-021: GUI ideas for the grinders (M11)

| Field | Value |
| --- | --- |
| Status | adopted: the Work view on poon grind (M11) |
| Based on | claude/gui-4-watch @ 50a0edd |
| Area | a future Work view; asks for the M11 commands (Source/Grey) |
| Effort | medium |
| Decided by | GUI/Grey, 2026-10-28 (Round 5 Part A) |

## Summary

M11, the grinders, is Source/Grey's Round 5 priority, and its GUI view
comes after the commands exist. This proposal sketches that view now, so
the commands can be shaped to fit it. The GUI would still only run
`poon` commands and show what they print.

## Problem

Grinding is the most hardware-heavy thing a node does, and the most
visual: which device, how fast, on what, and is it paying off. It is easy
to build commands that are fine in a shell but give a GUI nothing to draw
except text, such as a speed printed as prose every few seconds.

## Idea

A **Work** view with four parts:

1. **Devices.** A table from a device-listing command (for example
   `poon grind devices -o json`): name, kind (CPU or GPU), cores or
   compute units, instruction sets, which hashes have a fast grinder,
   and which one auto would pick, with the reason. A select sets the
   device for the commands below, like the wallet file in GREEN-016.
2. **Run.** A form for a hand-run job (one entry or layer, or a target
   by series, pool or entry, using the M10 filter flags), plus a
   switch for auto mode. Start runs the command as a job. Stop is the
   job's cancel, the same as Ctrl-C in a shell.
3. **Live.** While a grinder runs, its NDJSON progress lines feed a
   rate chart (hashes per second over the last minutes), the best
   strength found so far, and each layer or entry found. A find shows
   up as a `layer` or `entry` event in Watch too, with a look-up link
   (GREEN-013).
4. **Schedule.** The schedule as the node policy holds it, shown and
   edited through commands like `follow`'s, not a GUI-only setting.

What the GUI needs from the commands. These are cheap to add while M11
is being written:
- `-o json` on every listing, as M10 does (see GREEN-022).
- A streaming mode that prints one JSON object per line: start (device,
  hash, target), periodic progress (`tries`, `elapsed_ms`,
  `rate_per_s`, `best_strength`), found (entry or layer hash, strength),
  and end (reason). This is the same convention as `poon watch`.
- Exit cleanly on cancel, with exit 0 and an `end` line, so Stop is not
  shown as a failure. Compare the `poon node` shutdown in GREEN-015.
- Report the chosen device and the reason in the start line, so "what
  was picked" is shown, not guessed.

Nothing here changes consensus. The view is the same headless commands
with a picture on top.

## Cost and risk

Medium, once the commands exist: one view, a small inline-SVG rate chart
(like GREEN-018's), and reuse of the job and stream code Watch already
has. The risk is the GUI getting ahead of the commands, which is why
this is only a sketch until Source/Grey has built them.

## Prototype

None. It waits for M11.
