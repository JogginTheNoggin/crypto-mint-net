# GUI-GREEN-049: A field for each settable limit

| Field | Value |
| --- | --- |
| Status | proposed (Round 23 Part B) |
| Based on | main @ 2cacdd5 |
| Area | gui/web/static/views/policies.js (limitsParts), style.css, gui/e2e/views.mjs |
| Effort | small |
| Decided by | (Grey) |

## Summary

The Node policies view has two parts for the limits in D159 to D161:

- **Settable limits**, a panel over `poo policy limits` that lists each
  limit with its range, default, the value node.json sets and the value
  in effect.
- **Set limits**, a form with one text field for typed
  `NAME=VALUE` pairs, such as `p2p.max_book=8192 tracking.log_size=512`.

To change a limit, you read its exact name in the panel, then typed it
in the form above or below, without a typo. This proposal makes Set
limits draw one field per limit from the panel's own output. Each field
is named for its limit, its placeholder shows the value in effect, and
its hint gives the range and default. Fill only the ones you want to
change. A field left empty sends nothing.

## Problem

1. **The names had to be remembered and typed.** Some are long, such as
   `p2p.verified_per_range` and `p2p.registry_seconds`, and they are easy
   to mistype. A typo only shows up after Save, as poo's error. GUI-GREEN-048
   made the peer book states tick boxes for the same reason: the GUI
   should offer poo's choices, not ask you to remember them.
2. **The range and the value sat in another panel.** To pick a value you
   had to read the range in the table and then switch to the form.
3. **The two parts drifted apart.** The panel already reads every limit
   from poo, so a limit Source adds appears there with no GUI change, but
   the form knew nothing of that list.

## Idea

- When the Settable limits panel's `poo policy limits` run succeeds,
  Set limits draws a field for each limit in the JSON:
  - The label is the limit's name.
  - The placeholder is its `used` value.
  - The hint reads "`min` to `max`, default `default`", plus "node.json
    sets `set`" when node.json sets it.
- A filled field adds `-set NAME=VALUE`, so the preview shows the exact
  line that will run.
- After a save, the panel reloads (as it already did) and the fields are
  drawn again. They come back empty, with the new values as placeholders.
- The free-text field stays as **Other limits (NAME=VALUE)**, for a name
  the list lacks, such as when the panel could not run.
- The GUI checks nothing itself. poo checks every value against its range
  and refuses the whole save if one is out (D159). The hint only repeats
  what poo printed.
- The list still comes only from poo, so a limit Source adds gets its own
  field with no GUI change.
- The range and default go in the hint, not the placeholder, so
  GUI-GREEN-043's check of stated defaults does not mistake a limit's
  default for the `-set` flag's.

This adds no consensus rule and changes nothing in `poo`.

## Cost and risk

- Small: about 25 lines in policies.js and one CSS rule (the fields sit in
  the form's grid with `display: contents`).
- The fields are drawn from the panel's run, so they appear once the
  panel answers. Until then, and if the panel fails, the Other limits
  field still works.

## Prototype

Built on this branch. views.mjs now does the following:

- Waits for the `p2p.max_book` field. It checks that the placeholder is
  4096 (the value in effect) and that the hint says "256 to 65536,
  default 4096".
- Fills `p2p.max_book` with 8192 and `tracking.log_size` with 512, and
  checks that the preview is
  `poo policy limits -root … -set p2p.max_book=8192 -set tracking.log_size=512`.
- Saves, sees 8192 in the panel, and sees the field drawn again, empty,
  with 8192 as its placeholder. Nothing is sent again.
- Types `p2p.max_book=1` into Other limits and checks that poo refuses
  it and names the limit.

All nine e2e scripts and the gui Go tests pass locally. GitHub Actions is
not running this round.
