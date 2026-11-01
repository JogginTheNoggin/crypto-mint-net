# GREEN-035: Layer rate loose ends: the summary and the doctor fix

| Field | Value |
| --- | --- |
| Status | proposed (Round 10) |
| Based on | claude/gui-4-watch @ 4108866 |
| Area | gui/web/static/work.js (scheduleTable); pkg/cli/doctor.go (lowered check) |
| Effort | small |
| Decided by | (GUI/Grey for 1; Source/Grey for 2) |

The number skips 034 because Source's GREEN-034 (`-if-version`) already
uses it.

## Summary

There are two small gaps around `grind.layer_rate`:

1. **GUI, prototyped.** The Schedule summary line never mentions a layer
   rate while the floor is 0. GREEN-033 noted this. The rate stays in
   node.json and comes back with the next floor without warning. The line
   now ends "no layer floor (layer rate 5 kept for when one is set)".
2. **Source, a request.** When `poo doctor` warns that Load lowered
   `grind.layer_rate` to the new maximum (D129/D130), its fix is only
   text. The GUI's Health check can turn a fix into a "Run in terminal"
   button only when poo gives `fix_args`, as `init` and `node` already do.

## Problem

- (1) A user sets floor 200 and rate 5, then floor 0. The summary reads
  "device auto, 1 thread." Months later they set floor 100 and get rate 5
  back, which they no longer remember setting.
- (2) The warning says "grind.layer_rate N is over the most, M: M is
  used". Its fix is "lower it in PATH (poo grind schedule set, or edit
  the file)". The user has to work out the command on their own. If the
  file is never fixed, the warning shows on every doctor run.

## Idea

1. In `scheduleTable`, when there is no floor and `layer_rate` is set,
   the line says the rate is kept and has no effect for now. The text
   comes from poo's own reply, and nothing new is worked out.
2. In doctor's lowered check, set
   `FixArgs: ["grind", "schedule", "set", "-layer-rate", "<M>"]`, where M
   is the maximum Load used. It writes the value the node already runs
   with, so it changes nothing about what the node does. It only makes
   the file agree. The GUI needs no change, because `doctorReport`
   already adds the button for `fix_args`. Nothing here touches consensus.

## Cost and risk

- (1) Three lines plus an e2e change. The old exact-text check for
  "device auto, 1 thread." now first sees the kept rate, then sets rate 0
  and sees the plain line.
- (2) About two lines in Source and a doctor test. The note would need to
  carry M, or the check would read `nodepolicy.MaxLayerRate`.

## Prototype

(1) is on `claude/gui-green`. `gui/e2e/work.mjs` checks the "kept" line
after floor 0, then that rate 0 clears it. All eight e2e scripts pass.
(2) has no prototype, because it is Source's code.
