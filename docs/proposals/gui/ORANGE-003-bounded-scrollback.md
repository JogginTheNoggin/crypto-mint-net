# ORANGE-003: Bounded scrollback for the terminal, the node log and running commands

| Field | Value |
| --- | --- |
| Status | adopted |
| Based on | claude/gui-4-watch @ 9b4b19c |
| Area | gui/web/static/scroll.js (new), terminal.js, views.js (runJob, Node view log, Jobs output) |
| Effort | small |
| Decided by | |

## Summary

The terminal, the running node's log, a running command's live output
and the Jobs output pane each kept every write as a new `<span>`,
forever. The terminal and the node log also read `scrollHeight` after
every write, which forced a full layout of everything printed so far.
Typing `poon watch` in the terminal under load froze the page for two
minutes. A node logging 50 lines a second cost a little more every
minute. At 200 lines a second the page stopped answering altogether.

A small `scrollback` helper now writes into blocks of whole lines of
about 4 KiB. It keeps the newest 1 MiB of text, as a terminal keeps a
bounded scrollback, and scrolls to the end at most once a frame. With
it, the same `poon watch` takes 1.1 s and a busy node log costs the same
in minute 1 and minute 60. The text is shown exactly as printed, and
only the oldest text is let go.

## Problem

- `out.append(span); out.scrollTop = out.scrollHeight` on every write.
  Reading `scrollHeight` lays out the whole `<pre>`, and an inline run
  of text is laid out as a whole, so each write cost grew with
  everything above it. The cost is quadratic over a session.
- Nothing was ever removed. The server keeps 4 MiB per job, but the
  page kept everything that ever streamed past.

## Idea

`scroll.js` exports `scrollback(pre, { max, follow })`, which returns
`{ write(text, cls), clear() }`:

- Writes go into `div.blk` blocks. A new block starts once the current
  one holds about 4 KiB and ends with a newline, so a line is never
  split. Inside a block, a write of the same class extends the last
  text node with `appendData` instead of adding a node.
- Once more than `max` characters are kept (default 1 MiB), the oldest
  blocks are removed. Trimming happens on each write, so a hidden tab
  stays bounded too.
- With `follow`, it scrolls to the end in a `requestAnimationFrame`,
  once a frame. The terminal and the node log follow. The run pane and
  Jobs output do not, as before.

The terminal's Clear and Ctrl-L call `clear()`. Everything that showed
before still shows, in the same order and the same classes.

## Measurements

`gui/perf/measure.mjs terminal node` against `perfgui` (ORANGE-005).

**`poon watch` typed in the terminal, 20,000 events as fast as possible
(about 6 MB of text):**

| | before | after |
| --- | --- | --- |
| time to show the last event | 126 s (page frozen) | **1.1 s** |
| frames per second | 0 | 12 |
| longest frame gap | 56,098 ms | **117 ms** |
| layouts | 8,795 | **13** |
| main-thread busy | 204 s | **1.0 s** |
| text kept | 6.0 MB in 10,545 spans | **1.0 MB in 83 blocks** |

**Node view, a node logging 50 lines a second, main-thread busy in each
10 s:**

| at | 10 s | 20 s | 30 s | 40 s | 50 s | 60 s |
| --- | --- | --- | --- | --- | --- | --- |
| before | 1.45 s | 2.03 s | 2.40 s | 3.05 s | 3.75 s | 4.35 s |
| after | 1.23 s | 1.33 s | 1.48 s | 1.36 s | 1.31 s | 1.47 s |

Before, the cost rose steadily and DOM nodes grew by about 100 a second
(+5,857 in a minute). After, the cost is flat and DOM nodes grew by 197
in a minute. At 200 lines a second, the before build stopped answering
the measuring script for over ten minutes and the run was killed.

## Cost and risk

- A user who scrolls back more than about 1 MiB in one pane no longer
  sees the oldest text. The command's output is still in Jobs (the
  server's 4 MiB) and `poon` is unchanged. The 1 MiB is a constant
  (`MAX_CHARS`) Grey can tune.
- Output is now `pre > div.blk > span` instead of `pre > span`. The
  e2e checks read `innerText` and pass.

## Prototype

Branch `claude/project-thread-f0df7u`: `gui/web/static/scroll.js`, with calls from
`terminal.js` and `views.js`.
