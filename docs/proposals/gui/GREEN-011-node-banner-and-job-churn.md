# GREEN-011: One node banner instead of an error per panel

| Field | Value |
| --- | --- |
| Status | accepted, later |
| Based on | claude/gui-5-release @ 7209952 |
| Area | gui/web/static (app.js, views.js); maybe gui/server |
| Effort | medium |
| Decided by | GUI/Grey, 2026-10-27 |

## Summary

Two related observations from using the GUI with no node running, as
ideas for Grey, not prototyped:

1. Show a single banner at the top, "No node is answering for this node
   policy", with a Start node button. Panels that need the node then show
   a short line instead of the same red error three times on one page.
2. Stop panel refreshes from pushing user jobs out of the job list.

## Problem

- The Peers view shows the same `is the node running?` error three times,
  once for Peers, Tracking and Chases. Pools and Node show it again.
- Panels refresh every 5 to 10 seconds, and each refresh is a job. The
  server keeps at most 64 jobs, so a few minutes on the Node view drops
  the user's own earlier terminal and form jobs from the Jobs list.

## Idea

- The banner comes from the result of a command the GUI already runs
  (for example the Node view's `poo status`). It decides nothing new: it
  shows that one command's failure in one place. Panels whose command
  fails with the same error collapse to "Needs a running node (see
  above)", with a toggle to see the raw error.
- For job churn, two options for Grey:
  - (a) The server keeps refresh jobs in their own smaller ring, marked
    by a `refresh: true` field the views send.
  - (b) A panel reuses one job id and the Jobs view shows it once.

  Option (a) is the smaller change. Both touch `gui/server` or
  `gui/jobs`, which is why this is only a proposal.

## Cost and risk

Medium. The banner must never hide a different error behind the generic
one, so it should match only that one error and leave everything else as
printed.

## Prototype

None. GREEN-008's "Group repeats" makes the churn easier to read in the
meantime, but does not stop the eviction.

## Decision

Accepted. The job churn matters more now that GUI-004 caps jobs; Grey will keep panel refreshes from evicting user jobs, and add the banner, in a later GUI change.
