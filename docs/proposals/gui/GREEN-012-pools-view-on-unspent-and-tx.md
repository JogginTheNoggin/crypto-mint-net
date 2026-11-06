# GREEN-012: The Pools view's lookups on poon unspent and poon tx

| Field | Value |
| --- | --- |
| Status | adopted |
| Based on | claude/gui-4-watch @ 93ed36a |
| Area | gui/web/static/views.js (poolsView); goes with Source GREEN-008 |
| Effort | small |
| Decided by | GUI/Grey, 2026-10-28 (Round 3 Part A) |

## Summary

The Pools and payments view's two lookups now run the newer commands that
Source/Green's GREEN-008 recommends. "Holdings of a mint" runs
`poon unspent -serial N` instead of `poon holdings`. "Where a payment
stands" runs `poon tx TID` instead of `poon payment -serial N -tid TID`, so
the serial becomes optional (`-in-serial`). The GUI then shows the same
answers as Explore, in the same shape, and no longer depends on the older
commands. This is step 2 of Source GREEN-008, and it does not wait for
the Source side.

## Problem

Since G4, the GUI asks the same two questions in two places with two
commands, and gets two JSON shapes back:

| Pools view | Explore view | Difference |
| --- | --- | --- |
| `holdings -serial N` | `unspent -serial N` | Explore adds the serial and the filters |
| `payment -serial N -tid T` | `tx T` | Explore adds the pool, height and decoded transaction, and needs no serial |
| `status` | `pools` | each has fields the other lacks |

A user who looks up the same transaction in both views sees different
fields and can't tell which one is right. Source GREEN-008 also notes
that `running-a-node.md` uses both pairs.

## Idea

- Switch the two forms' argv builders. Nothing else changes: the titles
  and the command preview stay, and both commands exist today.
- Keep "Followed pools" on `poon status` for now. `pools` lacks `waiting`
  and `stored_records`, so switching it waits for Source GREEN-008 step 1,
  which adds them. After that, the Pools view and Explore's Pools form
  are the same command, and one of them can go.
- Later, the Pools view could become "Settle a payment" (build and work),
  with the lookups only in Explore. That is a layout decision for Grey.

## Cost and risk

About 6 changed lines. Scripts are not affected, since the older
commands stay in `poon`. If Source/Grey declines GREEN-008, this still
stands on its own, because `unspent` and `tx` are the documented M10
commands.

## Prototype

`poolsView` in `views.js`. `gui/e2e/watch.mjs` checks that both forms
preview `poon unspent -serial 5` and `poon tx TID`, and that the unspent
lookup succeeds against a live node.
