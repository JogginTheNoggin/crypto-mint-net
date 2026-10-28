# GUI-009: Output of seed-printing commands is kept and replayable from Jobs

| Field | Value |
| --- | --- |
| Severity | low |
| Status | fixed (interim, merged by GUI/Grey in Round 5 Part A); switch to cli.SecretOutput when Source adds it |
| Found on | claude/gui-4-watch @ 50a0edd |
| Component | gui/jobs/jobs.go (output retention), gui/server/server.go (`events`, `?from=` / `Last-Event-ID` replay), gui/web/static/views.js (Jobs view "Output") |
| Surface | wallet / local |
| Fixed by | `claude/gui-blue` (GUI-side interim; core `cli.SecretOutput` queued for Source Grey, Round 5) |

## Summary

GUI-006 hid seeds where they appear in a job's *command line*, and Round 4 moved
seed entry onto the core's secret prompt (`wallet create -restore -`, D106), so a
seed is no longer typed onto argv. But two commands print a seed in their
**output**:

- `poo wallet seed` prints the wallet seed, after the wallet passphrase is entered;
- `poo keygen` prints the seed of the key it makes.

The job runner keeps every job's output (up to 4 MiB per job, 64 MiB across
finished jobs, 64 jobs) for the life of the process. Anyone holding the session
can read it again:

- the **Jobs** view lists the job, and its **Output** button replays the whole
  output from sequence 0, seed included;
- `GET /api/jobs/{id}/events?from=0` replays it to any caller with the cookie. Job
  ids are small sequential integers, so they are easy to enumerate. The new
  `Last-Event-ID`/`next` resume (ORANGE-002) is correct, but it is also a deliberate
  way to read a job's output again.

The Round 4 change that drops "Copy output" for keygen (`secretOutput`) only
covers the keygen form's result box. The same seed is still a click-to-copy hex
span in that result (`render.scalar` copies the full value on click), and it is
in Jobs → Output with nothing hidden.

## Reproduction

1. In the terminal run `wallet seed` and enter the passphrase in the masked field.
   The seed prints.
2. Open Jobs, find `poo wallet seed`, and press Output. The seed shows again.
   Equivalently, `curl -b poo_gui=<cookie> -H 'Host: 127.0.0.1:<port>'
   'http://127.0.0.1:<port>/api/jobs/<id>/events?from=0'` returns it.
3. The same works for any `keygen` job.

## Impact

The wallet seed restores every key, and the passphrase is what guards it. A holder
of the session cookie already has full command authority (GUI-005). Without the
passphrase, though, they cannot run `wallet seed` themselves. Retained output lets
them skip that barrier whenever the owner revealed the seed earlier in the
session. It also leaves the seed one click away on screen for as long as poo-gui
runs (shoulder-surfing, screen share). Rated **low**: it needs the session or the
screen, the owner has to have printed the seed, and it lasts only until the
process exits. It does still turn a passphrase-guarded secret into an unguarded
one inside the session.

## Recommendation

- Have the core declare which commands print a secret (like `cli.SecretFlags`,
  D106), for example `cli.SecretOutput(args) bool` covering `wallet seed` and
  `keygen`. Then:
  - the runner stops keeping that output once the job ends (or keeps it only until
    the first client has read it to the end), so neither Jobs → Output nor
    `?from=0` / `Last-Event-ID` can replay it;
  - the Jobs view marks such a job "output not kept (it held a secret)".
- In the result box for such commands, skip `render`'s click-to-copy on the
  secret field as well as "Copy output", or show it masked with a Show button.
- Test: run `wallet seed` (and `keygen`) through the API, let it end, then assert
  that `GET /api/jobs/{id}/events?from=0` carries no chunk containing the seed.

## Notes — Round 4B verification (held up)

- **GUI-008 fix.** Leaving Watch now closes the `EventSource` and keeps `next`. On
  return, `attach` follows from `?from=next`. The partial line in `buf` is kept
  across the gap and cleared on `dropped`. A job that ends while the view is away is
  picked up on return (the replay ends with `exit`). No double follow: `attach`
  checks `!unfollow`, and `detach`/`exit` clear it. Correct.
- **ORANGE-002 resume and batching.** Coalesced chunk events carry `id` = `next` =
  last joined seq + 1. On an automatic reconnect the server takes
  `max(?from, Last-Event-ID)`, so there are no duplicates. A hostile or huge
  `Last-Event-ID` cannot index out of range (`Read` only slices when
  `first ≤ from < nextSeq`), and negative values are ignored. Data is
  JSON-encoded, so newlines cannot split an event, and one write over 64 KiB is
  sent alone.
- **Watch frame batching and scrollback.** `pending` is capped at 2×`MAX_EVENTS`
  even while the tab is hidden. `scrollback` keeps the newest 1 MiB of text and
  writes only `textContent`/`appendData`. `ledgerPicture` builds SVG with
  `setAttribute`/`textContent` from fields poo printed, so there is no markup
  injection.
- **`-restore -` / hidden seed.** Both the wallet form (a checkbox that runs
  `-restore -`) and the terminal read the seed through `cli.Stdio.Secret`, so the
  field is masked and nothing is echoed. A bad seed's error does not repeat the
  input. `keygen -seed-file -` also goes through the secret prompt.
- **Single wallet-file field and Settings in Getting started.** These hold only
  paths and flags the user sets, kept in `localStorage`. Settings flags are quoted
  with the server's grammar before they are appended to a step's line.

## Fix (GUI/Blue, branch `claude/gui-blue`): GUI-side interim

- `shell.SecretOutput(args)` names the commands that print a secret: `keygen`, and `wallet seed`. It stands in until the core declares `cli.SecretOutput`, which is queued for Source Grey in Round 5. When that lands, it should replace this list the way `cli.SecretFlags` replaced the GUI's flag list.
- **Runner (`gui/jobs`).** Such a job is marked `SecretOutput`. Once it has ended, the first `Read` that reaches the end (the live terminal, form or panel following it), or `Outputs` (`/api/run`), gets the output. The job then drops its chunks and sets `Withheld`. A status probe (`Read` from past the end, as `info()` does) does not count as a read. Output that nobody reads is dropped after `SecretKeep` (1 minute). Live followers still see the output while the job runs.
- **Server.** A later `GET /api/jobs/{id}/events`, whether `?from=0` or with `Last-Event-ID`, gets `dropped` with `withheld: true` and then `exit`, with no chunks. The `dropped` event no longer indexes an empty chunk list. `GET /api/jobs` reports `secretOutput` and `withheld`.
- **Page.** For a finished secret job, Jobs shows "output not kept (it held a secret)" instead of Output. The terminal and Jobs print "[output not kept: it held a secret]" on a withheld read. `render` shows any `seed` field hidden until Show, with no click-to-copy (`secretValue`), and a secret command's plain-text output is shown the same way. "Copy output" was already left out for keygen.
- The terminal still prints the seed when it runs, as headless `poo` does.
- Regression tests: `TestSecretOutput` (gui/shell); `TestSecretOutputGivenOnce`, `TestSecretOutputExpires` (gui/jobs); `TestSecretOutputNotReplayed` (gui/server, real keygen: `?from=0` and `Last-Event-ID` replay no seed, the jobs list marks it, `/api/run` gives it once). In gui/e2e/terminal.mjs: Jobs shows "output not kept" with no Output button, the stream replays no seed, and the Make a key result hides the seed until Show.
