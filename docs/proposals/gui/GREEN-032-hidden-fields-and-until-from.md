# GREEN-032: Hidden form fields were still drawn; Until from follows the target

| Field | Value |
| --- | --- |
| Status | proposed (Round 8) |
| Based on | claude/gui-4-watch @ a93ccb6 |
| Area | gui/web/static/style.css, views.js (form `only`), work.js (job forms) |
| Effort | small |
| Decided by | (Grey) |

## Summary

This fixes two things:

1. **A bug in GREEN-028.** Fields that `only:` hides were never hidden on
   screen. `.fields label { display: flex }` overrides the `hidden`
   attribute. So the Peers and Node policies forms still showed every
   flag, even though they did not send the hidden ones. The e2e check
   only looked for the attribute, so it did not catch this. One CSS line
   fixes it.
2. **Grey's Round 7 loose end.** "Until from reaches" now shows only for
   a target that takes it. It is hidden for tip, pending and an empty
   target (which means tip), in all three job forms: Run a job, Schedule
   a job, and Start a job in the background.

## Problem

- Peers, "Act on a peer": choosing unblock still shows Seconds, Whole
  range, Clear and Tracking template. They look usable but are dropped
  from the command, which is worse than not showing them.
- `poo grind` refuses `-until-from` with tip or pending ("until_from
  needs an entry or tx:TID as the target"). It also needs it for a
  tx:TID target. The forms showed the field for every target. The Run
  form's target hint also left out tx:TID, and the Start form's target
  had no hint at all.

## Idea

- CSS: `.fields label[hidden] { display: none; }`.
- `only` takes `not` as well as `values`. With `only: {field, not}`, the
  field is hidden, and not sent, while the other field's trimmed text is
  one of `not`. The job forms use `not: ['', 'tip', 'pending']`. These are
  poo's own target keywords, compared as text, so the GUI checks nothing
  itself. poo still checks the command, for example that a tx: target
  has an until-from.
- The Run a job panel builds its own form, so it applies the same rule.
- The three target fields share one hint: "tip, pending, tx:TID or an
  entry hash". The Run form's help now says what tx:TID does. The
  until-from placeholder now says "a weight; a tx:TID target needs one".
- `form().run` (a link that fills and submits a form) now updates the
  hidden fields too, so a filled-in target shows the right fields.

## Cost and risk

About 15 lines of JS and 1 line of CSS. Grey may have a test that clicks
or fills a field that `only` hides. That now fails, because the field is
really hidden. All eight e2e scripts pass.

## Prototype

On `claude/gui-green`.

- `gui/e2e/qol.mjs` now checks that a hidden Peers flag is not visible.
  That check failed before the CSS line.
- `gui/e2e/work.mjs` checks the Schedule a job form:
  - For tip, Until from is hidden.
  - For a tx: target, it shows and is sent.
  - Back to tip, it is hidden and not sent.
  - The Run a job form also hides it for tip.
