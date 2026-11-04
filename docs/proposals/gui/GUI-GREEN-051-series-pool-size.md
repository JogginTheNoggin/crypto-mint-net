# GUI-GREEN-051: New series takes its units per mint and pool size

| Field | Value |
| --- | --- |
| Status | proposed (Round 25 Part B) |
| Based on | main @ 88bd534 |
| Area | gui/web/static/views/crypto.js (New series parameters), gui/e2e/parity.mjs, gui/e2e/views.mjs |
| Effort | small |
| Decided by | (Grey) |

## Summary

A series is defined by N (mints), S (units in every mint) and P (pool
size). A variable series adds M (part size), and the design document and
D163 require N and M to be whole multiples of P.

The New series parameters form took N and, since Round 25 Part A, M. It
did not take S or P: parity.mjs left `-supply` and `-pool-size` out as
"the form says poo series new -h has the rest". So when poo refused a
variable series with "the number of mints and the part size must both be
whole multiples of the pool size", the form had no field for the pool
size it named. The only way out was the terminal, or changing N and M to
fit a default of 4 that the form never showed.

This proposal adds **Units in every mint** (`-supply`) and **Pool size**
(`-pool-size`) to the form. Their placeholders state poo's defaults, 100
and 4, and GUI-GREEN-043 checks those against `poo -h`. Mints now states
its default, 8, too. The two flags leave parity.mjs's LEFT_OUT list.

## Problem

1. **An error named a field the form lacked.** poo was right to refuse
   the series, and the GUI correctly does not restate the rule (see
   GUI-GREEN-050). But the error could not be acted on in the form.
2. **The form showed only part of the definition.** N, S and P are the
   three numbers the design document's table of terms starts with, and
   each one changes the series id. A series made in the GUI always had
   S = 100 and P = 4, and nothing on the form said so.
3. **Mints did not state its default**, unlike the other number fields
   that have one.

## Idea

- crypto.js, in New series parameters:
  - **Mints** gets the placeholder "default 8".
  - **Units in every mint** sends `-supply`, with the placeholder
    "default 100".
  - **Pool size** sends `-pool-size`, with the placeholder "mints per
    pool, default 4".
  - The new fields sit beside Mints, before Issuer and Variable, because
    they hold for both kinds of series.
- parity.mjs: `supply` and `pool-size` leave `LEFT_OUT['series new']`, so
  a form that stops sending them fails the check. GUI-GREEN-043 now also
  holds the three stated defaults to `poo series new -h`.
- The GUI checks nothing. poo still refuses a bad definition with its
  own message.

This adds no consensus rule and changes nothing in `poo`.

## For Source

I found two things in Phase 1 while checking the forms against the CLI.
Neither is a GUI change, so they are noted here for Source:

1. **`poo series info` prints nothing about a variable series.** For a
   variable series it prints `series_id` and the pools of part 1 only. It
   leaves out the supply type, part size and largest number of parts. GUI
   Grey's plan (docs/gui-variable-supply-plan.md, "Series info") expects
   these fields, and the GUI can only show what poo prints (D84). Until
   then, `poo series parts` is the only place they show.
2. **`poo series info -h` tries to open a file named `-h`**
   ("poo: open -h: no such file or directory"). Every other command
   prints its flags.

## Cost and risk

- Small: three field definitions and one args line in crypto.js, and one
  line in parity.mjs.
- The form grows by two fields, both optional. Left empty, they send
  nothing, so poo's defaults hold exactly as before.

## Prototype

Built on this branch.

- views.mjs, after poo refuses part size 3 with the default pool size:
  - Fills Pool size 3, Mints 9 and Units 50.
  - Checks that the preview is
    `poo series new -mints 9 -supply 50 -pool-size 3 -issuer … -variable -part-size 3 -max-parts 5`.
  - Checks that poo accepts it.
- parity.mjs passes (73 commands), now with `-supply` and `-pool-size`
  sent by a form and their stated defaults matching `poo -h`.
- All nine e2e scripts and the gui Go tests pass locally.
