# ORANGE-013: Send a long pasted line in parts, and end the input only once it arrived

| Field | Value |
| --- | --- |
| Status | adopted (Round 17 Part A) |
| Based on | main @ 6a1cdce |
| Area | gui/web/static/api.js (`input`), gui/web/static/views.js (Send and end input), gui/server/server.go (`/input` takes `more`), gui/jobs/jobs.go (`InputPart`) |
| Effort | small |
| Decided by | |

## Summary

Round 16's "Send and end input" (GUI-021) lets Submit read a pasted entry
to its end with `submit -f -`. But any paste over 1 MiB never reached
`poo`. The whole line goes in one request, and the server takes at most
1 MiB per request (`MaxBody`). In hex, that means no entry over about
500 KiB could be pasted. Worse, the button ended the input even though
the send had failed. `poo` then read nothing and reported "entry: decode:
encoding: input too short", which hides the real cause. The
"request body too large" error only showed in the output after that.

With the change, the page sends a long line in parts of at most 256 KiB,
in order, and the server adds each part with no end of line. "Send and
end input" now ends the input only after the whole line has arrived. If
the send fails, the input stays open and the error is shown. Terminal
lines go through the same `api.input`, so the terminal gains the same
fix.

| Pasted into Submit (`-f -`) | Before | After |
| --- | --- | --- |
| 128 KiB entry, 359 KiB of hex | reaches `poo`, 185 ms | reaches `poo`, 184 ms |
| 450 KiB entry, 1,078 KiB of hex | refused: "request body too large", then "input too short" | reaches `poo`, 341 ms |
| 1 MiB entry, 2,156 KiB of hex | refused, 1.15 s | reaches `poo`, 0.81 s |
| 4 MiB entry, 8,265 KiB of hex | refused, 5.7 s | reaches `poo`, 4.7 s |

"Reaches `poo`" means `poo` decoded the whole entry and went on to ask
the node. The test had no node running, so each run ended with "is the
node running?". Most of the 4.7 s for 8 MiB is the browser handling an
8 MiB text field. A file (`-f FILE`) is still the better way for an
entry that size.

## Change

- `api.input(id, line)` sends `line.slice(i, i + PART)` with
  `more: true` while more than `PART` (256 KiB) remains, then the rest
  as an ordinary line.
- `POST /api/jobs/{id}/input` takes `{line, more}`. With `more`, the
  server calls the new `Job.InputPart`, which adds the text with no end
  of line. Each request stays under `MaxBody`, which is unchanged.
- In `runJob`, the ask form's submit returns whether the send arrived.
  "Send and end input" ends the input only if it did.

## Checks

- `TestInputInParts` (gui/server) sends two `more` parts and a final
  line, then checks that the command read them as one line ending in a
  single newline.
- `gui/e2e/streams.mjs` pastes 1.5 MiB into Submit with `-f -` and
  requires that no "request body too large" comes back. It fails on
  6a1cdce and passes with the change.
- The terminal, views, qol and work e2e checks pass, and so do
  `go test ./server ./jobs`.

## Consensus

None. This only changes how the page hands typed text to `poo`.
