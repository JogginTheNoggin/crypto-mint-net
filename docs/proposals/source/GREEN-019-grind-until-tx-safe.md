# GREEN-019: Grind until my payment is safe (`tx:TID` target)

| Field | Value |
| --- | --- |
| Status | proposed |
| Based on | `claude/m10-watch` @ `29b08ce` (code `a92c2ca`) |
| Area | `pkg/grind`, `pkg/cli` (`grind start`) |
| Effort | small to medium |
| Decided by | Grey |

## Summary

The most likely reason a person grinds is to protect a payment they just
made or received. Today they must find the entry that holds the payment,
then pass its hash to `-until-from`. They have a TID, not an entry hash.
The idea is a target `tx:TID`: grind the pending layer until the payment
is in an entry, then the layers on top of that entry, until the weight
from it, from(e), reaches `-until-from W`. Proposal only.

## Problem

`poo pay` and `poo tx TID` give a TID. `grind start` takes `tip`,
`pending` or an entry hash, and `-until-from` also takes an entry hash. To
say "until my payment is safe", the person runs `poo tx TID`, copies the
entry, and starts the job, and if the payment is still pending there is no
entry yet, so they have to come back later.

## Idea

- Accept `tx:TID` as a target. While the TID is pending, the job works
  like `pending`. Once it is in an entry, it works like `tip` with
  `-until-from` set to that entry.
- `-until-from W` stays required with a `tx:` target. A later step could
  let a policy name a default W, but the node has no notion of a "safe"
  weight today, and Green does not propose one here.
- The job ends `done` when from(e) of the payment's entry reaches W, and
  `failed` if the payment is dropped or double spent, with that reason.

## Risks

The job follows one TID through its states, so it needs a test for each
path (pending then included, already included, dropped). The safe weight
must come from the same code that `poo tx` uses, so the two can never
disagree.
