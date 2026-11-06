# ORANGE-026: Say when a result lost part of its output, and offer no Save as file then

| Field | Value |
| --- | --- |
| Status | adopted (GUI Round 34 Part A, the prototype as is) |
| Based on | main @ 3c4ae00 |
| Area | gui/web/static/views.js (`runJob`, `drawn`, new `incomplete`), gui/e2e/qol.mjs; it touches every form that offers Save as file (ledger, node policies, settings, peers and series exports) |
| Effort | small |
| Decided by | |

## Summary

Round 33 added "Export a pool's ledger" to the Pools view (D188). With
no file given, poon prints the ledger into the page, and the form offers
Save as file.

poon-gui keeps only the last 4 MiB of a command's output
(`jobs.DefaultMaxOutput`). A page that falls more than that far behind
loses the part in between. The event stream then sends a `dropped`
event, but `runJob` does not listen for it, so nothing tells the user.

With a 100,000-entry ledger (58.6 MB), the page received only 19 to 27
MB on every run. In 5 of 7 runs the text that arrived still parsed as a
ledger file: the header said "from 1 to 100000", but it held 38,008 to
46,214 entries. Every run offered Save as file. A person who pressed it
got a file with most of the middle missing and nothing to say so. Copy
output copied the same incomplete text, even though the page said it
"copies all of it".

The page also froze while the 58 MB arrived: 14 to 24 s busy, tasks up
to 2.0 s and frame gaps up to 2.5 s.

The prototype on this branch listens for `dropped`. When output was
dropped, it removes Save as file from the result and puts an alert at
the top: part of the output was dropped, what is shown has a gap, and
the user should write it to a file instead. Exports to a file were never
affected: 100,000 entries are written in 1.4 s, with the page idle.

## Problem

- **The server:** `gui/jobs` keeps up to 4 MiB of output for each job,
  and drops the oldest chunks past that. `/api/jobs/ID/events` sends
  `event: dropped {next, withheld}` when a reader asks for chunks that
  are gone (server.go).
- **The page:** `api.follow` passes `dropped` to a handler if one is
  given. `runJob` gives handlers for chunk, state, exit and error only,
  so it adds up what arrives as if it were everything.
- **Why the page falls behind:** each chunk event is parsed and written
  to the live pane. At 58 MB, that takes longer than poon takes to print
  it (1.4 s to a file), so the reader is soon more than 4 MiB behind.
  A reader that does nothing else (a probe that only adds up chunks)
  kept up and got all 58.6 MB with no drop. The page's own work is what
  makes it lag.
- **Why the gap can read as whole:** `poon ledger export` writes one
  entry per line. The dropped chunks are whole writes, so what is left
  is often still valid JSON with fewer entries.

The same applies to any command that prints more than 4 MiB quickly
with Save as file offered. Node policies, settings, peers and series
exports use the same pattern. None of them was measured over 4 MiB
here, but a large peer export could get there (the 16,384-address book
prints 12.8 MB in `poon peers`).

## Measurements

The ledgers came from a throwaway Go test, which is not committed. It
builds one pool of N entries in a test series, one claim and one layer
per entry. The real `poon` (main @ 3c4ae00) served them, and the real
`poon-gui` ran in Chromium through Playwright. A second node, following
the same series and holding nothing, received the imports.

On the command line:

| Entries | `ledger export -out` | Printed to stdout | File size | `ledger import` (fresh node) |
| --- | --- | --- | --- | --- |
| 2,000 | 0.06 s | 0.03 s | 1.2 MB | 0.70 s |
| 20,000 | 0.30 s | 0.29 s | 11.7 MB | 8.4 s |
| 100,000 | 1.6 s | 1.5 s | 58.6 MB | 44.5 s |

In the GUI (Pools view), main @ 3c4ae00:

| Entries | Export to the page | Export to a file | Import by path | Import of a picked file |
| --- | --- | --- | --- | --- |
| 2,000 | 0.23 s, 0.19 s busy, longest task 95 ms | 0.15 s, idle | 0.84 s, idle | 1.4 s, idle |
| 20,000 | 4.7 s, 4.6 s busy, longest 663 ms, gap 817 ms; all 11.7 MB arrived | 0.36 s, idle | 7.9 s, idle | 9.0 s, idle |
| 100,000 | 14 to 23 s, all of it busy, longest 1.4 to 2.0 s, gaps up to 2.6 s; 19 to 27 MB of 58.6 MB arrived, 2 drops each time | 1.4 s, idle | 48.7 s, idle | 51.3 s, idle |

The 100,000-entry export to the page, run seven times on main:

- In 5 runs the text parsed as JSON. The page drew "Showing 500 of
  38008 rows" (also 38,622 and 46,214), under a header that said
  `to 100000`.
- In 2 runs it did not parse, and the page showed the last 1 MiB as
  text, with "The first 19173427 characters are not shown; Copy output
  copies all of it."
- Save as file was offered in every run checked.

With the prototype, four runs alternated with main on the same node:

| 100,000 entries to the page | main | this branch |
| --- | --- | --- |
| Arrived | 22.3 and 27.1 MB | 26.2 and 23.7 MB |
| Save as file offered | yes, both | no, both |
| Says output was dropped | no | yes, both |
| Busy | 19.8 and 21.6 s | 24.5 and 20.8 s |

The prototype does not make the page faster. That needs the output to
stay off the page (see Idea, point 3).

## Idea

1. **Built here:** `runJob` listens for `dropped` and returns
   `dropped: true` with the result. After `onResult`, `panel` and
   `form` call `incomplete(area)` for such a result. It removes any
   Save as file button and puts this alert at the top:

   > Part of this output was dropped: poon printed it faster than the
   > page could take it, and poon-gui keeps only the last 4 MiB of a
   > command's output. What is shown, and what Copy output copies, has
   > a gap. Write it to a file instead: the form's file field where it
   > has one, or -out in the terminal.

2. **Small follow-up:** the line "Copy output copies all of it", under a
   cut text result, should not show when output was dropped. The alert
   already says the copy has a gap.
3. **For GUI Grey to weigh:** the export form could say that a long
   ledger belongs in a file, for example in its "To a file" field's
   placeholder. A Save as file that streams from poon-gui would not help,
   because poon-gui keeps only 4 MiB itself. The file field is the way
   that never loses anything.

The GUI computes nothing new. It reports what the stream said, and
keeps poon's output as it arrived.

## Cost and risk

- About 20 lines in views.js and one qol.mjs check.
- **Risk:** a result that only lost part of a long live log (a node log
  or a long `poon watch` in a form) now carries the alert too. That is
  true, and those panels offer no Save as file.
- **Compatibility (White):** no API path or stored key changes. The page
  reads an event the server already sends.

## Prototype

Built on this branch. `gui/e2e/qol.mjs` checks that `incomplete` removes
Save as file from a result and puts the `output-dropped` alert first.
The end-to-end case, a 100,000-entry export, was checked with the probe
above, in four runs. All ten e2e scripts pass.

## Modularity

`incomplete` is one helper called from `drawn`, which runs after every
`onResult`, so no view has to check for drops itself.

## Consensus

None.
