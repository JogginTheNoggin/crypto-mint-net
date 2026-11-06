# GUI-033: After a form refuses to run, its next run is never followed: the masked prompt never shows, Stop does nothing, and the job waits for ever

| Field | Value |
| --- | --- |
| Severity | low |
| Status | fixed |
| Fixed by | GUI/Blue, Round 36C (views.js: both refusal paths only stop following a job (`unfollow`), and a submitted form clears `area.gone` before `runJob`; qol.mjs checks that after each refusal the next Sign shows poo's masked prompt and Stop ends the job) |
| Found on | main @ d25f777 (#296, #298: GUI-032's `refuse` on Sign with a key kept elsewhere) |
| Component | gui/web/static/views.js (`form`'s submit handler: the `refuse` path, and the GUI-026 required-field path, call `leave(area)`, which sets `area.gone = true`; nothing sets it back, and `runJob` returns at once when `area.gone`) |
| Surface | local (the GUI page and poo-gui's job slots) |

## Summary

`leave(area)` exists for leaving a view (ORANGE-010): it sets
`area.gone = true` and drops the event stream. `runJob` checks that
flag once the job has started: "The view was left before the job
started: its stream is not opened; the job runs on and is in Jobs." It
then resolves `{code: -1}` and never sets the Stop button's handler or
follows the job.

`form()` also calls `leave(area)` in two places where the view stays:
- when a required field is empty (GUI-026, since Round 23C);
- when the new `refuse(values)` says no (GUI-032, Round 36A).

Nothing clears `area.gone` again. So every later run of that form,
until the view is opened again, starts a job that the page never
follows:
- The status reads "running…" for ever, and the result never shows.
- If poo asks for something (a seed, a key, a passphrase), the masked
  prompt never shows, and the job waits on its input for ever.
- Stop does nothing: its `onclick` is set only after the `area.gone`
  check. The job stays not done.
- The Sign button is enabled again at once (the promise resolved), so
  each press leaves one more stuck job. Each one holds one of the 16
  job slots, as in GUI-028.

GUI-032's own refusal leads straight into this. The message says
'Untick "The key is in a file" and poo asks for the key in a masked
field'. A user who does that, and presses Sign, gets a job waiting for
a key in a prompt that is never drawn.

The required-field path is harder to reach, because the browser's own
check stops an empty `required` input first (the GUI-026 e2e test
turns that off). The `refuse` path has no browser check in front of
it.

## Reproduction

poo-gui at d25f777, Playwright. pay.json is
`{"hash":"sha256","legs":[{"serial":1}]}`.

1. Wallet → Sign with a key kept elsewhere. Co-signing file pay.json.
   Tick "The key is in a file", and put 64 hex digits in Key file. Press
   Sign: "Nothing was run: Key file holds what looks like a key …", and
   the field is emptied. This is right.
2. Untick the box, as the message says. The line reads `poo sign
   pay.json -key-file -`. Press Sign.
   - After 3 s there is no masked prompt (`form.ask` not visible), and
     the status reads "running…". `/api/jobs` lists the sign job as not
     done.
3. Press Stop and wait 3.5 s. The status still reads "running…", the
   job is still not done, and Sign is enabled again.

The same steps with a path in place of the key at step 1 (no refusal)
show the prompt, and later the exit, as they should.

## Impact

Nothing leaks, and nothing is sent: a job waiting on a key that never
comes signs nothing. But the form looks hung, Stop doesn't work, and
each retry takes another job slot. After 16, poo-gui refuses every
command until those jobs are stopped from Jobs, or poo-gui is
restarted. It is reached by following the GUI's own advice, on the
form made for keys kept off the machine. Rated **low**.

## Recommendation

- **GUI/Blue (views.js).** Don't use `leave()` for "nothing was run".
  In the `missing` and `refuse` paths, drop only a stream still
  followed (`if (area.unfollow) { area.unfollow(); area.unfollow =
  null; }`, as `runJob` does), and leave `area.gone` alone. Or, as a
  belt and braces, set `area.gone = false` in the submit handler before
  `runJob`. A submit can only come from a mounted form, so the view
  hasn't been left.
- e2e (qol.mjs, next to the GUI-032 check): after the refusal, untick
  the box and press Sign. Expect the masked prompt (`form.ask` visible).
  Then Stop ends the job (`/api/jobs` done) within a few seconds. Add
  the same for GUI-026 in views.mjs: after "fill in Parent", fill it in
  and run again, and expect the run to be followed.
