# GUI-016: A Remove that poon refuses says nothing, so the job just stays

| Field | Value |
| --- | --- |
| Severity | low |
| Status | fixed (PR #49, merged Round 11; verified by GUI/Red R11B @ 4be2230) |
| Found on | claude/gui-4-watch @ 4108866 |
| Component | gui/web/static/work.js (`scheduleTable`, the Remove button) |
| Surface | local (what the Work view tells the user) |
| Fixed by | GUI/Blue on claude/gui-blue (Round 10C) |

## Summary

Each Schedule row's Remove now runs
`poon grind schedule remove -if-version V -target T N`, where V is the
version the table was read at (GREEN-034). That is the right guard. poon
now refuses the removal whenever the schedule changed at all since the
table was drawn, for example after an Add from another tab, the terminal or
a script. It says so with exit code 1 and
"the grind schedule changed meanwhile: list it again".

The button never looks at the result:

```js
try { await api.run(['grind', 'schedule', 'remove', ...]); } finally {
  await reload();
  edits.release();
}
```

`api.run` resolves with `{code, out, err}` for any exit code. It throws
only if poon-gui itself refuses the request. So a refused Remove gives no
message, no `run-status bad`, nothing. The table is drawn again, and the
job the user removed is still there. Nothing says why, or that they should
press Remove again on the fresh table.

The same happens when `-target` refuses, or when the removal fails for any
other reason, for example the node's reply timing out or view-only. With
`-if-version`, a refusal is now an expected outcome whenever anything else
edits the schedule, not a rare one.

## Reproduction

1. With a node running, schedule two jobs and open Work → Schedule.
2. In a terminal: `poon grind schedule add -hours 10-11 tip`.
3. In the GUI, without refreshing, press Remove on row 1.
4. The table is redrawn with the new job, and row 1's job is still listed.
   No message appears. Jobs shows the `grind schedule remove` job with
   exit 1 and the "changed meanwhile" error. That is the only place it is
   visible.

The CLI part was checked directly on a running node:
`remove -if-version <old> -target pending 1` after an Add exits 1 with
"the grind schedule changed meanwhile: list it again".

## Impact

The user thinks they stopped a scheduled job, the table shows it plainly,
and the node keeps running it. It fails safe: nothing wrong is removed, and
the job is visible. But the cause is hidden, and a user who doesn't notice
leaves a job grinding they meant to stop. No attacker is involved. Rated
**low**.

## Recommendation

- When `code !== 0`, show poon's `err` next to the table, the way forms show
  a `run-status bad` line. For example: "Not removed: the grind schedule
  changed meanwhile. The table is current now; press Remove again." Keep
  the redraw.
- Handle an exception from `api.run` the same way, for example poon-gui
  refusing the request.
- Test: in `e2e/work.mjs`, draw the table, add a job through `api.run` from
  the page (or from the test), press Remove, then assert that the error is
  shown and the row is still listed.

## Fix (GUI/Blue, Round 10C)

- **A refused Remove says why** (`gui/web/static/work.js`). Remove now reads
  `api.run`'s `{code, err}`. When `code !== 0`, it keeps poon's `err` (or its
  output, or the exit code if both are empty). An exception from `api.run`,
  for example poon-gui refusing the request, is kept the same way. After the
  table has been read again, the view puts "Not removed: <poon's words> The
  table is current now." above it, as a `run-status bad` line with
  `role="alert"`. The text is capped at its last 2,000 characters. A Remove
  that works shows no message, and the next redraw clears an old one.
- **Test.** `e2e/work.mjs` covers both refusals:
  - the existing stale-row case, refused by `-target`, now also checks the
    message;
  - a new case adds a job from the terminal (`poon grind schedule add`), so
    the schedule's version changes, then presses Remove on a row that is
    still correct. It checks that `-if-version` refuses it, that the message
    is shown, that the row is still listed and the new job appears, and that
    a Remove after the redraw works with no message left.

  On the old code the check fails because no message appears.

## Notes — Round 11B verification (held up)

- **The fix, verified.**
  - Remove reads `{code, err}` and catches an exception from `api.run`.
  - The reason is shown only after `reload()` has drawn the table again. It
    goes above the table as a `role="alert"` line, built with `el` (so as
    `textContent`, not HTML) and capped at the last 2,000 characters.
  - The next redraw clears it, because `runJob` empties the area.
  - The gate is still released in `finally`.
  - `runJob` always resolves, including when `api.start` fails, so a failing
    redraw cannot hold the gate. The one exception is leaving the view during
    the redraw. That gate belongs to the old mount, as noted in R9B.
  - `gui/e2e/work.mjs` passes locally on 4be2230, including the new
    `-if-version` case.
- **Optional nits, wording only.**
  - poon's errors end without a full stop, so the line reads "…changed
    meanwhile: list it again The table is current now." Adding ". " when the
    reason doesn't end in punctuation would fix it.
  - If the redraw itself failed, for example because the running-job limit
    was hit, "The table is current now" is not true. The line could leave
    that sentence out when the panel's last run did not exit 0.
- **GREEN-035, safe.** The summary line is plain text built in `el`. A rate
  kept with no floor is now shown rather than hidden, and nothing new is
  sent.

## Follow-up (GUI/Blue, Round 11C): Red's two wording notes

- **Closing punctuation.** If poon's reason doesn't end in `.`, `!` or `?`,
  the message adds a period before what follows, so it reads "Not removed:
  <reason>. The table is current now."
- **Only claim the table is current when it is.** `panel.reload()` now
  resolves with the new run's `{code, out, err}`. "The table is current
  now." is added only when that run exited 0. If reading the schedule again
  failed, the message stops after poon's reason, and the panel shows that
  read's own error below it.
- **Test.** `e2e/work.mjs` checks the closing punctuation on the stale-row
  refusal. For the changed-elsewhere refusal, the test makes the read after
  the Remove fail: it routes the schedule's `POST /api/jobs` to a 503. It
  then checks that the refusal is shown without "current now", and that
  Refresh draws the table again. On the old code this check fails.
