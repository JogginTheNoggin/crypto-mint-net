# GUI-012: Pressing a form's button again leaks the previous job's event stream and leaves that job orphaned

| Field | Value |
| --- | --- |
| Severity | low |
| Status | open |
| Found on | claude/gui-4-watch @ b20bf8f |
| Component | gui/web/static/views.js (`runJob`, `form`; the ORANGE-010 `leave`) |
| Surface | local (browser tab, poo-gui job and stream slots) |
| Fixed by | _(pending — GUI/Blue)_ |

## Summary

ORANGE-010 made leaving a view close the event stream of a job still running
in a panel or form (`leave(area)` calls `area.unfollow`). That holds. Running a
**second** job in the same area does not do the same.

`runJob` begins with `area.textContent = ''`, starts the new job and stores the
new `area.unfollow`. It never closes the stream of a job that is still running
in that area. `form` does not stop a submit while its last job runs. So each
extra press of a form's button while the earlier job is still going:

- leaves the earlier job's `EventSource` open, with nothing that can close it.
  `leave` only knows the newest one. It stays open until that job ends, which
  for a job waiting for input is never;
- removes the earlier job's input box from the page. A job that was waiting for
  a passphrase or seed (most Wallet forms, Create or restore, Make a key with
  `-seed-file -`) then waits forever. It keeps one of the 16 running-job slots
  until it is stopped from Jobs or poo-gui exits.

poo-gui serves plain HTTP/1.1, so the browser allows about six connections to
it. After a handful of extra presses, the leaked streams use them all up and
every later fetch queues. The GUI stalls in the same way ORANGE-010 fixed for
view switches.

## Reproduction

1. Wallet → any form that asks for the passphrase (for example Balance with a
   wallet set, or Create or restore). Press the button. The masked passphrase
   field appears.
2. Without answering, press the button again, five or six times.
3. Jobs lists that many `poo wallet …` jobs still waiting. The page's
   `/events` requests stay open (devtools → Network), and any further panel or
   view hangs on "running…" until those jobs are stopped from Jobs.

A long job such as `grind bench -seconds 60` pressed repeatedly does the same
for as long as it runs.

## Impact

The user's own GUI stalls, and jobs that wait for input are orphaned. No other
origin can cause this (`SameSite=Strict`, `X-Poo-GUI`), and nothing leaks.
Rated **low**, as ORANGE-010 would have been.

## Recommendation

- At the start of `runJob`, close a stream still open in that area
  (`if (area.unfollow) { area.unfollow(); area.unfollow = null; }`), as `leave`
  does.
- Decide what happens to the earlier job. Either cancel it when it is waiting
  for input (its prompt is about to disappear), or keep the form's button
  disabled while its job runs, as the live Run panel does. Disabling is the
  simpler choice and matches the panels' `busy` guard.
- Test: in e2e, press a passphrase form's button six times without answering,
  then assert that at most one `/events` request for that form is open and
  that `/api/jobs` answers within a second.
