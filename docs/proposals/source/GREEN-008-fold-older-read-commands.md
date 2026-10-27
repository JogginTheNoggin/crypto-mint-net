# GREEN-008: Fold the older read commands into the views

| Field | Value |
| --- | --- |
| Status | proposed |
| Based on | `claude/m10-watch` @ `c71f774` |
| Area | `pkg/cli` (`status`, `holdings`, `payment`), `pkg/rpc`, docs; the GUI's "Pools and payments" view (GUI team) |
| Effort | small |
| Decided by | |

## Summary

M2's `poo holdings`, `poo payment` and `poo status` now overlap with M10's
`poo unspent`, `poo tx` and `poo pools`, and in each pair the M10
command shows more. Keep the old names, so scripts and the GUI keep
working, but document the M10 command as the one to use. Later, serve both
from the same `pkg/watch` code, so that there is only one answer to "what
does the node hold for mint 5". No prototype: the GUI calls `holdings` and
`payment` today, so any change in output needs GUI/Grey to agree first.

## Problem

Checked on a live node at `c71f774`:

| Older | Newer | What the newer one adds |
| --- | --- | --- |
| `holdings -serial N` (`[{tid, index, address, amount}]`) | `unspent -serial N` (`[{input:{tid,index}, serial, address, amount}]`) | the serial; any filter (address, pool, several serials); `-o table` |
| `payment -serial N -tid T` (`{pending, selected, entry, from, tx: HEX, rivals}`) | `tx T` (`{pool, pending, selected, entry, height, from, rivals, tx: {decoded…, raw}}`) | no `-serial` needed; the pool and height; the decoded transaction; `-o table` |
| `status` (pools with `waiting` and `stored_records`, plus `replay`) | `pools` (pools with `first_serial` and `last_serial`) | filters. `status` has fields `pools` lacks, and the reverse |

A new user who reads the help sees two ways to ask the same question,
gets two different JSON shapes back, and can't tell which one the docs
mean. `running-a-node.md` part 4 uses `payment` and `holdings`, and part
4c uses `tx` and `unspent`. The two shapes also have to be kept correct
separately in `pkg/rpc` (`/v1/holdings` and `/v1/payment`) and in
`pkg/watch`.

## Idea

1. **Now (docs only).** Mark `holdings` and `payment` in the usage and
   `poo.md` as "older form of unspent / tx, kept for scripts", and switch
   `running-a-node.md` part 4 to `tx` and `unspent`. Give `pools` the
   two fields only `status` has (`waiting`, `stored_records`), so `pools`
   is a superset and `status` becomes `pools` plus `replay`.
2. **Next (with GUI/Grey).** The GUI's "Pools and payments" view moves to
   `unspent` and `tx`, which it already renders elsewhere.
3. **Then.** `/v1/holdings` and `/v1/payment` are served from the
   `pkg/watch` functions behind `/v1/unspent` and `/v1/tx`, keeping their
   JSON shapes, so there is one code path. Removing them is the owner's
   call, at a version bump.

Smaller naming mismatches found in the same sweep, for Grey to fix or
leave:
- `poo series info` prints `series_id`, while every other reply says
  `series`.
- `/v1/tx` submit replies with `id`, while views say `tid`.

## Cost and risk

Step 1 is docs plus two fields on `watch.Pool`, which only adds JSON
fields. Step 3 is small and keeps the old shapes, which the existing rpc
tests check. The risk is breaking a script that reads the old shapes,
which is why nothing is removed.

## Prototype

None, because the output changes need GUI/Grey to agree first. Green can
build step 1 or step 3 once Grey picks one.
