# GUI-014: Schedule changes are read, changed and written back by the client, so two at once lose one

| Field | Value |
| --- | --- |
| Severity | low |
| Status | GUI part fixed (PR #41, merged Round 9; verified by GUI/Red R9B @ 6fae281); core edits on the node (D125, merged Round 10); the GUI's Remove also sends `-if-version` (GREEN-034, Round 10) |
| Found on | claude/gui-4-watch @ a93ccb6 |
| Component | pkg/cli/grind.go (`grindSchedule`: GET, then PUT of the whole section), pkg/rpc/grind.go (`PUT /v1/grind/schedule`), gui/web/static/work.js (Schedule a job, Schedule defaults, Remove) |
| Surface | node (node.json's grind section, applied at once by a running node) |
| Fixed by | GUI/Blue on claude/gui-blue (GUI stopgap); core fix queued for Source Grey via GUI/Grey |

## Summary

RED-013 added `policyMu`, so two `PUT /v1/grind/schedule` calls no longer
interleave inside the node. That lock does not cover the change itself.
`poo grind schedule add | remove | set` works in the client:

1. `GET /v1/grind/schedule` reads the whole section;
2. the change is made to that copy in the CLI process, and that copy is also
   what the GREEN-031 `-target` check looks at;
3. `PUT /v1/grind/schedule` replaces the **whole** section with the new copy.

Two changes that overlap between step 1 and step 3 both start from the same
copy. The second PUT writes its copy over the first, so one change is lost.
Both commands still print success. The `-target` guard cannot catch this,
because each client checks its own stale copy.

The GUI makes this easy to hit. The Work view's **Remove** (now one at a time
within the table, GUI-013), **Schedule a job** and **Schedule defaults** run in
separate areas with no shared guard. A Remove, then an Add pressed before the
Remove's reply comes back, gives one of:

- the removed job back, because the Add's PUT carries the old list plus the new job;
- the new job gone, because the Remove's PUT lands last.

A terminal or script `poo grind schedule …` running at the same time as the
GUI has the same race.

**Residual from GUI-013.** `-target` checks the target only. Several scheduled
jobs often share a target, since `tip` is the default and jobs differ by hours,
pools, series or device. A stale table can still remove a different `tip` job
than the row showed. For example, rows [tip 22-06, tip always, pending] are
shown, the first job is removed elsewhere, and Remove on row 1 sends
`remove -target tip 1`, which removes "tip always".

## Reproduction

1. With a node running, schedule jobs `tip` (hours 22-06) and `pending`.
2. In Work → Schedule, click Remove on row 1. Straight away, before the table
   is drawn again, submit Schedule a job with target `tip` and hours 08-12.
3. List the schedule. Depending on which PUT landed last, either `tip 22-06`
   is back or `tip 08-12` is missing, although both commands reported success.

From a shell, two `poo grind schedule add …` commands started together leave
only one of the two new jobs.

## Impact

Wrong grind schedule with no error, applied at once by the running node: a job
the user removed comes back, or one they added is missing. This is an
integrity issue with the user's own configuration and needs no attacker. Rated
**low**.

## Recommendation

Core (Source, via GUI/Grey), so headless and GUI are fixed together:

- Make each change a single server-side operation under `policyMu`:
  `POST /v1/grind/schedule/jobs` (add), `DELETE /v1/grind/schedule/jobs/{n}`
  with an expectation, and `PATCH` for the defaults. The node then reads,
  checks and writes in one step. The offline path, which edits node.json,
  needs the same read, check and write under a file lock.
- Alternatively, add **optimistic concurrency**. GET returns a version (a hash
  of the section). PUT takes `If-Match` and refuses with 409 when the section
  changed, and the CLI then reads again or reports the conflict.
- Make the remove guard match the **whole job**, not only its target. For
  example, `-expect` takes the job's JSON or a content hash that `schedule`
  prints with each job, so duplicate targets cannot be confused.

GUI, as a stopgap:

- Run one schedule change at a time across the whole Work view. Disable Remove,
  Add and Set while any of them runs, and draw the table again from the reply.
- Test: e2e clicks Remove and submits Add together, then checks that the final
  schedule shows both changes or that one of them was refused visibly.

## Notes — Round 8B verification (held up)

- **GUI-012 fix, verified.** `runJob` closes `area.unfollow` before clearing
  the area. `form` holds a `busy` flag, so the submit button is off and
  `requestSubmit` or `run` is dropped while the job runs. The first job keeps
  its prompt and its Stop. If the view is left during a run, `leave` closes the
  stream. The pending `runJob` promise then never settles and that form stays
  busy, but the form is rebuilt on the next mount, so nothing is stuck. One job
  per form, one stream per area.
- **GUI-013 fix, verified for its own case.** A Remove disables every Remove in
  the table until it is drawn again, so a double click no longer shifts
  numbers. `-target` refuses a number whose target changed. The remaining gaps,
  duplicate targets and concurrent changes from other areas or clients, are
  described above.
- **GREEN-030 jump list, safe.** Titles come from the view's own `h2`
  `textContent`, and the buttons only scroll and focus. The hash router is not
  touched.
- **ORANGE-011, safe.** At most one `poo crypto` runs at a time from the key
  fields.
- **Source m10-watch @ 0fd59fa, for what the GUI uses.** `policyMu` now
  serialises follow, unfollow and the schedule PUT inside the node (RED-013),
  which removes the in-node interleaving noted in GUI-013. The client-side gap
  above is what remains. The table output changes (`pkg/cli/table.go`:
  prefixed hashes, times cut to the second) only affect `-o table` text, and
  the GUI renders JSON.

## Fix (GUI/Blue, Round 8C), the GUI stopgap

- **One schedule change at a time across the Work view.** A new `gate()` in
  `gui/web/static/views.js` lets one of several actions run at a time, and
  while it is held every button added to it is off. The Work view gives
  Remove, Schedule a job (Add) and Schedule defaults (Set) one gate (`edits`
  in work.js):
  - A Remove takes the gate before it runs.
  - A form with `gate` takes it on submit. While it is held, a press, or a
    submit by other means (`requestSubmit`, a link into the view), is
    dropped, not queued.
- **The next change waits for the redraw.** `panel` now returns
  `reload()`, which runs the panel again (after the run in progress, if
  there is one) and resolves once that new run is drawn. Each schedule change
  holds the gate until `reload()` has resolved, so the next change can only
  start from a table read after the last one. Add and Set now redraw after a
  failure too, not only after success.
- **Test.** `e2e/work.mjs` clicks Remove and, in the same task in the page,
  checks that every other Remove, Add and Set is off. It then calls
  `requestSubmit()` on Add and checks that Add started no job. After the
  redraw it checks that exactly the expected jobs are left, that Add is on
  again, and that a new Add lands next to the remaining job. Without the
  fix, the check fails at "one schedule change at a time".
- **Still open, in the core.** This only orders changes made from one GUI
  page. Another tab, the terminal or a script can still change the schedule
  between a GET and a PUT, so one change is lost while both report success.
  `-target` also can't tell scheduled jobs with the same target apart. The
  fix is the one recommended above: server-side add, remove and set under
  `policyMu`, or an `If-Match` version on the PUT, and a remove guard that
  matches the whole job. It goes to Source through GUI/Grey, and the
  coordinator has queued it for Source Grey's next Part A.
