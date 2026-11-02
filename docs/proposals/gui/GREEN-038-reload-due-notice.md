# GREEN-038: The Node view says when a node.json change waits for a reload

| Field | Value |
| --- | --- |
| Status | proposed (Round 12) |
| Based on | claude/gui-4-watch @ c9c9e1f, checked with claude/source-green @ 557534b merged in |
| Area | gui/web/static/views.js (Node view: Followed pools, Reload the node policy) |
| Effort | small |
| Decided by | (Grey; it needs Source GREEN-037 merged first) |

## Summary

Source Green's GREEN-037 makes `poo status` report `"policy_changed":
true` when node.json differs from the policy the running node uses
(outside the grind section, which is applied at once). Doctor's rpc check
also warns about it, with `poo reload` as the fix.

The GUI shows doctor's warning already, through the Health check and its
"Run in terminal" button. But the Health check runs only when asked,
while Followed pools runs `poo status` every 5 seconds. So Followed pools
now shows a notice above its output: "The node policy file has changed
since the node loaded it, and the change waits for a reload." The notice
has a **Reload now** button.

## Problem

- "Follow more" and "Follow less" have an "Only write the policy (no
  reload)" box. Edits by hand or from a shell also leave node.json ahead
  of the node. Until now, nothing in the GUI said the node was still
  running the old policy. Followed pools showed the old follows with no
  hint why.
- Even with GREEN-037, you only find out if you happen to press Refresh
  on the Health check.

## Idea

- An `onResult` on the Followed pools panel. When `poo status` printed
  `policy_changed: true`, it puts a `p.next-step.policy-changed` (role
  status) above the output. The GUI decides nothing: it shows what poo
  reports, every 5 seconds as before.
- **Reload now** runs the Node view's own "Reload the node policy" form
  (`form.run`). So the command, its output and its exit show where they
  always do, as `poo reload`.
- A successful Reload (from the notice or the form) reloads Followed
  pools. The notice goes away as soon as poo stops reporting the field.
- Without GREEN-037, `poo status` never prints the field, and nothing
  changes.

## Cost and risk

About 15 lines. One extra JSON.parse of `poo status`'s output every 5
seconds, which is small next to drawing it.

## Prototype

On `claude/gui-green`. `gui/e2e/work.mjs`, before Stop node, runs
`poo unfollow -series S -pool 1 -no-reload`. If `poo status` then reports
`policy_changed`, it checks three things: the notice shows, Reload now
runs the Reload form to exit 0, and the notice goes away.

- With claude/source-green @ 557534b merged into a scratch copy, that
  path ran and passed.
- On the GUI base alone it is skipped, and all eight e2e scripts pass.
