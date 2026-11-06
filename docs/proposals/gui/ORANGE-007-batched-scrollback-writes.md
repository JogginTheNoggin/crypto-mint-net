# ORANGE-007: Batch frequent writes in scrollback panes

| Field | Value |
| --- | --- |
| Status | adopted |
| Based on | claude/gui-4-watch @ 50a0edd |
| Area | gui/web/static/scroll.js, terminal.js |
| Effort | small |
| Decided by | |

## Summary

ORANGE-003 made the terminal and the node log bounded and flat in cost.
They still laid out once for every write, though: a node logging 50
lines a second caused 50 layouts a second and kept the main thread 11%
to 13% busy. Now the first write after a quiet spell is drawn at once,
so typing and short commands feel the same. Writes that follow within
100 ms are queued and drawn together. The node log drops to 4% to 5%
busy, and `poon watch` in the terminal finishes in 0.62 s instead of
1.05 s.

## Idea

- `write()` queues `[text, cls]`. If the last draw was at least
  `DRAW_EVERY` (100 ms) ago, it draws now. Otherwise one timer draws the
  queue when the 100 ms is up. The queue is bounded by the same
  `MAX_CHARS`, so a tab whose timers are slowed stays bounded.
- `flush()` draws what is queued now. The terminal calls it before it
  marks a command finished (`data-runs`), so the last output is always
  on screen when a command ends.
- The text, order, classes, blocks and trimming are unchanged.

## Measurements

`gui/perf/measure.mjs node terminal` against `perfgui`. Before is base
50a0edd.

**Node view, 50 log lines a second, main-thread busy in each 10 s:**

| | 10 s | 20 s | 30 s | 40 s | 50 s | 60 s | layouts in 60 s |
| --- | --- | --- | --- | --- | --- | --- | --- |
| before | 1.09 s | 1.12 s | 1.11 s | 1.25 s | 1.21 s | 1.35 s | 3001 |
| after | 0.50 s | 0.42 s | 0.44 s | 0.46 s | 0.41 s | 0.43 s | **620** |

**`poon watch` typed in the terminal, 20,000 events at full rate:**

| | before | after |
| --- | --- | --- |
| time to the last event | 1050 ms | **622 ms** |
| frames per second | 14 | **26** |
| main-thread busy | 1.03 s | **0.46 s** |

The run pane of a form or panel uses the same helper. A 4 MB text result
now streams in 78 ms instead of 320 ms (see ORANGE-008).

## Cost and risk

Output can show up to 100 ms late while a command is printing fast.
Sparse output, such as echoes, prompts and a short command's lines,
shows at once as before. All six e2e checks pass.

## Prototype

Branch `claude/project-thread-f0df7u`: `gui/web/static/scroll.js` and `finished()` in
`terminal.js`.
