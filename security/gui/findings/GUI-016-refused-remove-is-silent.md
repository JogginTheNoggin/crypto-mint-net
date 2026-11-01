# GUI-016: A Remove that poo refuses says nothing, so the job just stays

| Field | Value |
| --- | --- |
| Severity | low |
| Status | open |
| Found on | claude/gui-4-watch @ 4108866 |
| Component | gui/web/static/work.js (`scheduleTable`, the Remove button) |
| Surface | local (what the Work view tells the user) |
| Fixed by | _(pending — GUI/Blue)_ |

## Summary

Each Schedule row's Remove now runs
`poo grind schedule remove -if-version V -target T N`, where V is the
version the table was read at (GREEN-034). That is the right guard. poo
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
only if poo-gui itself refuses the request. So a refused Remove gives no
message, no `run-status bad`, nothing. The table is drawn again, and the
job the user removed is still there. Nothing says why, or that they should
press Remove again on the fresh table.

The same happens when `-target` refuses, or when the removal fails for any
other reason, for example the node's reply timing out or view-only. With
`-if-version`, a refusal is now an expected outcome whenever anything else
edits the schedule, not a rare one.

## Reproduction

1. With a node running, schedule two jobs and open Work → Schedule.
2. In a terminal: `poo grind schedule add -hours 10-11 tip`.
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

- When `code !== 0`, show poo's `err` next to the table, the way forms show
  a `run-status bad` line. For example: "Not removed: the grind schedule
  changed meanwhile. The table is current now; press Remove again." Keep
  the redraw.
- Handle an exception from `api.run` the same way, for example poo-gui
  refusing the request.
- Test: in `e2e/work.mjs`, draw the table, add a job through `api.run` from
  the page (or from the test), press Remove, then assert that the error is
  shown and the row is still listed.
