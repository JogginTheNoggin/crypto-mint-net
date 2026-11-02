# ORANGE-011: Ask `poo crypto` once while a key field waits for its suggestions

| Field | Value |
| --- | --- |
| Status | adopted (Round 8) |
| Based on | claude/gui-4-watch @ b20bf8f |
| Area | gui/web/static/views.js (the key-field focus listener), gui/e2e/streams.mjs |
| Effort | small |
| Decided by | |

## Summary

Round 7 added suggestions to the key algorithm and hash fields. The
first time one of them gets focus, the page runs `poo crypto` to fill
the list. Until the reply comes back, every further focus on a key
field runs `poo crypto` again. Each run is a job, listed in Jobs, and
the GUI keeps only the newest 64 jobs, so the extra runs can push real
commands out of the list. On a slow machine, or with a busy node, the
reply takes long enough for this to happen by tabbing through a form.

The fix lets one ask be in flight at a time. Measured with eight focuses
before the reply: 8 runs and 8 jobs before, 1 run and 1 job after.
Nothing else changed in this round's measurements: the 5-minute soak
still holds, and every earlier number held.

## Measurements

All on the same container, Playwright Chromium, the real `poo-gui`
(focus check) or `perfgui` (the rest), base b20bf8f.

| What | Before | After |
| --- | --- | --- |
| `poo crypto` runs for 8 focuses before the reply | 8 | 1 |
| Jobs listed after them | 8 | 1 |

Re-checked on b20bf8f, no change needed:

| Scenario | Round 7 |
| --- | --- |
| 5-minute soak (views, commands, node log) | ran 300 s; heap 3.1 MB, ~27k nodes, ~1050 listeners, 1 open stream, server RSS 40 MB, slowest click 72 ms, all flat |
| Watch at 200 events/s | 60 fps, 19% busy |
| Watch at full rate (20k events) | 32 fps, done in 0.55 s |
| Terminal, `poo watch` (20k events) | 0.6 s |
| Node log at 50 lines/s for 60 s | answers in 2 to 6 ms, flat |
| Work at 200 lines/s | 60 fps, 12% busy |
| Reload a running watch (40k events) | caught up in 238 ms |

The new Schedule panel and the Work view's per-pool plans draw only when
`poo` answers or a plan arrives, which is once per pool per job. They
did not show in any measurement.

## Change

```js
let askingCrypto = null;
document.addEventListener('focusin', e => {
  const l = e.target.getAttribute && e.target.getAttribute('list');
  if ((l === 'crypto-algs' || l === 'crypto-hashes') && !document.getElementById(l) && !askingCrypto) {
    askingCrypto = api.run(['crypto']).then(r => { if (r.code === 0) cryptoChoices(r.out); }).catch(() => {})
      .finally(() => { askingCrypto = null; });
  }
});
```

If `poo crypto` fails, the next focus asks again, as before. The GUI
still takes any text; the list is only a suggestion from `poo`.

## Check

`gui/e2e/streams.mjs` now focuses a key field eight times before `poo`
answers and requires one run. It fails on b20bf8f ("poo crypto ran 8
times for eight focuses") and passes with the change. The views, qol,
terminal and work checks pass too.

## Consensus

None. This is a page-only change.
