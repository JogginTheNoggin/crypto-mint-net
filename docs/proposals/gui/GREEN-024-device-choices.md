# GREEN-024: Device fields offer the devices poon found

| Field | Value |
| --- | --- |
| Status | adopted (Round 6) |
| Based on | claude/gui-4-watch @ 8b044ac |
| Area | gui/web/static/work.js (deviceChoices, the Devices panel's onResult) |
| Effort | small |
| Decided by | (Grey) |

## Summary

Every Device field in the Work view (Run a job, Start a job in the
background, Benchmark) now offers the ids that `poon grind devices` just
printed, plus `auto`, as suggestions while you type. jerry asked that the
user can pick a device or leave it on auto. Before, picking meant reading
the Devices table and typing `cpu` or `gpu:0` correctly.

## Idea

- The Devices panel's `onResult` fills one `<datalist id=grind-devices>`
  from the printed `devices[].id` (labelled with `name`), with `auto`
  first, and points every `input[name=device]` at it.
- A datalist suggests and does not restrict, so any text still goes to
  poon, and poon decides what is valid, as before. A device that appears
  later shows up on the panel's next Refresh.

## Cost and risk

About 15 lines. If the devices command fails (no node needed, so that is
rare), the fields stay plain text fields.

## Prototype

On the branch. `gui/e2e/work.mjs` checks that the Run a job device field
points at the list, and that the list holds `auto` and `cpu`.
