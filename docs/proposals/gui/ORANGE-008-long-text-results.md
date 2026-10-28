# ORANGE-008: Show the last 1 MiB of a long text result

| Field | Value |
| --- | --- |
| Status | proposed |
| Based on | claude/gui-4-watch @ 50a0edd |
| Area | gui/web/static/views.js (showResult) |
| Effort | small |
| Decided by | |

## Summary

When a command prints a long text result (up to the server's 4 MiB a
job), `showResult` put all of it into one `<pre>`. A 3.9 MB result took
0.8 s to show and kept the main thread busy that long. JSON results were
already fine, because render caps tables at 500 rows: 5000 rows show in
0.2 s. Now a text result over `MAX_CHARS` (1 MiB, the scrollback bound)
shows its last 1 MiB, with a line saying how much is not shown. Copy
output still copies all of it. The 3.9 MB result now shows in 0.18 s.

## Measurements

`gui/perf/measure.mjs render`: `runJob` running a perfgui command that
prints a large result, until the result is drawn.

| Result | before (50a0edd) | after (ORANGE-007 + 008) |
| --- | --- | --- |
| JSON, 500 rows (150 KB) | 111 ms | 120 ms |
| JSON, 5000 rows (1.5 MB, 500 shown) | 175 ms | 210 ms |
| text, 50,000 lines (3.9 MB) | streamed 320 ms, shown **792 ms**, busy 0.76 s | streamed **78 ms**, shown **181 ms**, busy **0.14 s** |

The JSON times are within run-to-run noise. Nothing there needs changing.

Watch paused at full rate (20,000 events, 1000 held) was also measured.
It ran at 47 fps, with a longest gap of 33 ms on Resume, so it is fine.

## Cost and risk

Seeing the first part of a long text result now takes Copy output, or
running it in the terminal and scrolling (its scrollback is also 1 MiB).
JSON output is unchanged.

## Prototype

Branch `claude/project-thread-f0df7u`: `showResult` in `gui/web/static/views.js`. The
`render` and `paused` scenarios and the `bigjson` command were added to
`gui/perf`.
