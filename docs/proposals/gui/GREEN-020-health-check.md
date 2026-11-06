# GREEN-020: A Health check panel on poon doctor

| Field | Value |
| --- | --- |
| Status | adopted; the banner on the Node view points at the Health check |
| Based on | claude/gui-4-watch @ 50a0edd |
| Area | gui/web/static/views.js (doctorReport, panel onResult, nodeView), style.css |
| Effort | small |
| Decided by | GUI/Grey, 2026-10-28 (Round 5 Part A) |

## Summary

Source Round 4 added `poon doctor`, which checks a node policy, its files,
series, RPC and P2P and suggests a fix for each problem. The GUI had no
way to show it. Now the Node view opens with a "Health check" panel that
runs `poon doctor -json` and draws the report as a checklist: an ok, warn,
fail or skip pill, the check's name and detail, and its fix. A fix that
is exactly one `poon` command line (for example `poon node`) gets a "Run in
terminal" button. The output as printed is kept under a fold.

## Problem

- New users meet problems one view at a time (no policy, then no node,
  then files readable by others), and each shows up as a different
  error. `poon doctor` answers all of them at once, but only in a shell.
- The GUI's own hints (GREEN-007) and node banner (GREEN-011) match
  error text. `poon doctor` is the headless tool's own diagnosis, so the
  GUI should show that instead of guessing.

## Idea

- `panel()` gains the same `onResult` hook as `form()`. The Health
  check panel uses it to replace the generic rendering with
  `doctorReport`, and moves the raw result into a "Output as printed"
  fold. Everything shown is a field doctor printed.
- A fix becomes a button only when it matches one plain command line.
  Prose fixes ("poon init (or poon init -example …), or pick one with
  -policy or -dir") are shown as text. GREEN-022 asks Source for a
  structured fix, so the GUI need not tell the two apart.
- The panel runs when the view opens, and on Refresh. It has no timer,
  because doctor reads files and dials the RPC.

A loose end it shows up: with no node policy at all, the node banner
says "No node is answering … Start one on the Node view", even on the
Node view itself, while doctor says the real problem is the missing
policy. Once doctor is there, the banner could defer to it. For example,
it could say "See the health check" when the policy check fails, and
drop the link when it is already on the Node view.

## Cost and risk

About 40 lines. Doctor exits 1 when a check fails, so the panel still
shows "exit 1" above the report, which is honest but a little loud.

## Prototype

On the branch. `gui/e2e/qol.mjs` checks the failing policy check, its
printed fix, the `poon doctor -json` preview and the raw fold.
`gui/e2e/watch.mjs` checks that, after `poon init`, policy is ok and the
rpc warning offers `poon node` as a button.
