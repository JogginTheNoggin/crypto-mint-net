# GUI-013: The Schedule panel's Remove goes by position, so it can remove a different scheduled job

| Field | Value |
| --- | --- |
| Severity | low |
| Status | open |
| Found on | claude/gui-4-watch @ b20bf8f |
| Component | gui/web/static/work.js (`scheduleTable`, Remove), pkg/cli/grind.go (`grindSchedule remove N`) |
| Surface | node (node.json's grind section, applied at once by a running node) |
| Fixed by | _(pending — GUI/Blue; optional core part for Source)_ |

## Summary

Each row of the new Schedule panel has a Remove button that runs
`poo grind schedule remove N`, where N is the row's number **when the table
was drawn**. The core removes whatever job is at position N **now**. It loads
the section again from the node or node.json, then drops index N-1. The panel
does not refresh itself (no `refresh:` interval), and each Remove disables only
its own button. So any change to the schedule between the drawing and the click
shifts the numbers, and Remove takes out a different job.

That includes:

- **Two quick Removes in the same table.** With jobs `[A, B, C]`, Remove A runs
  `remove 1`, leaving `[B, C]`. Remove B, clicked before the refresh lands,
  runs `remove 2` and removes **C**. B stays.
- **A change from elsewhere.** The terminal, another tab, a script, or a hand
  edit followed by a reload. The panel still shows the old list, and every
  Remove below the change hits the wrong row.

With a running node, the change applies at once (`PUT /v1/grind/schedule`
then `Manager.Schedule`). The wrong scheduled job stops grinding and is gone
from node.json. The job the user meant to remove keeps running.

## Reproduction

1. With a node running, add three scheduled jobs (Schedule a job: targets
   `tip`, `pending`, `tip` with hours `22-06`).
2. In the Schedule table, click Remove on row 1, then quickly click Remove on
   row 2.
3. Refresh. Rows 1 and **3** are gone. The `pending` job that row 2 showed is
   still scheduled.

Or: draw the panel, run `grind schedule remove 1` in the terminal, then click
Remove on the last row. It errors with "numbered 1 to N", or, with more rows,
removes the wrong one.

## Impact

The user's grind schedule loses the wrong job without warning, and the job
they meant to stop keeps going. This is an integrity issue with the user's own
configuration, and it needs no attacker. Rated **low**.

## Recommendation

- GUI: while any Remove in the table is running, disable every Remove, and
  redraw the table from the reply before allowing another. Also refresh the
  panel when the view is shown or when a form changes the schedule (the
  forms already call `afterSchedule`).
- Better, with a small core part (Source): let `schedule remove` say *which*
  job, not only where. For example, `remove N -expect TARGET[,HOURS…]` refuses
  if job N no longer matches, or jobs carry a stable id or content hash that
  `remove` takes. The GUI passes what the row showed, so a stale table fails
  safely rather than removing the wrong job. The headless command gets the
  same protection for scripts.
- Test: in e2e, with three scheduled jobs, click Remove on rows 1 and 2 in
  quick succession, then assert that the job row 2 showed is gone and the
  other two are not, or that the second click was refused.

## Notes — Round 7B verification (held up)

- **ORANGE-010 event-stream leak, verified for view switches.** `panel` and
  `form` close their area's stream on `stop` (`leave`). `runJob` does not open
  one for a view that was already left (`area.gone`). The Node view's log
  stream closes on leave, and `showRunning` stops once `gone`. Watch, Work's
  live panel and Jobs close theirs. `gui/e2e/streams.mjs` covers ten Node-view
  visits. The one gap left is re-running inside the same area: **GUI-012**.
- **GUI-011 fix, verified.** `hangup(ignored, tty)`: under nohup (SIGHUP
  ignored) nothing is caught, so it stays ignored. With a terminal, SIGHUP
  stops poo-gui through its cancel path. With no terminal, SIGHUP is absorbed,
  so Go's default exit does not run, and a hosted node reloads on it as
  `poo node` does. `cmd/poo` keeps SIGHUP as reload for `poo node`
  (`ReloadsOnHangup`), and as stop for other commands only when it is not
  ignored. `isTerminal` uses TCGETS/TIOCGETA. Windows never sends SIGHUP.
- **GUI-010 core lease (Source 9972137), verified.** `poo grind run` starts its
  job with a 15 s lease and renews it every 5 s. A follower that dies or
  freezes (SIGKILL, crash, sleep) lets the lease run out, and the node stops the
  job with the reason "its lease ran out". `MaxLease` is 10 minutes. The old
  `source` label escape is closed: jobs made over RPC are always `user`.
  For Source (reliability, not GUI): `renew` returns on the **first** failed
  renew, so one RPC timeout while the node is busy stops renewal, and the
  job is killed 15 s later while its follower is still alive. Retrying until
  the lease is nearly gone would avoid that.
- **Schedule panel rendering and view-only, safe.** Every field from
  `poo grind schedule` is drawn with `el`/`render` (`textContent`). View-only
  allows `grind schedule` only to show it; `add`, `remove` and `set` are refused
  by `grindSchedule` itself, and over RPC by the view token, which is GET-only.
  `schedule add` and `set` go through `grind.Check` and `nodepolicy.Validate`
  (at most 16 jobs, thread and lease bounds).
- **node.json rewrites (pre-existing, noted).** `schedule add`, `remove` and
  `set`, like `follow` before them, load node.json, change it and `Save` it
  with no lock. Two concurrent changes, such as a follow from the Node view and
  a schedule change, can lose one. `Load` also drops keys it does not know, so
  hand-added keys vanish on the first rewrite. Neither is new in this round.
- **GREEN-028/029, safe.** `only:` hides and *does not send* a field the
  chosen action does not take, which matches the core's `flagsFor`. The crypto
  `<datalist>`s are filled from `poo crypto` with `el`, and the one-time
  `focusin` lookup runs only the read-only `poo crypto`.
