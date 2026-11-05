# GUI-GREEN-057: Pools and payments, pool list on demand

| Field | Value |
| --- | --- |
| Status | adopted (Round 32 Part A, merged as is) |
| Based on | main @ 55c5efa |
| Area | gui/web/static/views/pools.js (Followed pools), views/node.js (statusNotes wording), gui/e2e/views.mjs, gui/e2e/qol.mjs, docs/FEATURES.md |
| Effort | small |
| Decided by | (Grey) |

## Summary

GUI-GREEN-056 stopped the Node view from drawing every pool when it
opens. I then checked every panel that runs by itself when its view opens
(`panel(...)` without `start: false`), on a running reserve node
(1,024 pools):

| View: panel | Command | Output on the reserve |
| --- | --- | --- |
| Pools and payments: Followed pools | `poo pools` | **381 KB, a row per pool** |
| Peers: Peers | `poo peers -counts` | 283 bytes |
| Peers: Tracking, Chases | `poo tracking`, `poo chases` | under 40 bytes |
| Work: Jobs, Schedule, Devices | `poo grind …` | small |
| Policies: Policies, Settable limits | `poo policy …` | small |
| Series and keys: crypto | `poo crypto` | small |

Only one is left. **Pools and payments** runs `poo pools` as soon as it
opens and draws 500 rows before anyone can use the forms below it, which
are what people open the view for (holdings, a payment, build, work,
submit). Measured over four opens in Chromium:

| | Rows | Page elements | First panel ready |
| --- | --- | --- | --- |
| main @ 55c5efa | 501 | 12,755 | 177 to 549 ms |
| this PR | 0 | 234 | 63 to 65 ms |

## Idea

- **Pools and payments → Followed pools** uses `panel({ start: false })`
  (GUI-GREEN-056): it says "Not run yet: press Show." and runs `poo
  pools` when asked. The forms are unchanged.
- **Wording:** the Node view's replay line said "1 pools from their
  snapshots". It now says "1 pool from its snapshot", and the plural
  otherwise.

## Idea for Orange and Grey (not built here)

- **A reserve-scale guard.** 056 and 057 were found by hand, by opening
  each view on a reserve node. A small e2e check could do it every
  round:
  - start a node on `poo init` (the reserve), with p2p off;
  - open every view in the navigation;
  - fail if any view draws more than a few thousand elements before a
    button is pressed.

  It would be an eleventh script and needs a CI job, so it is Grey's
  call. It would catch the next panel that prints one row per pool.

## Cost and risk

One option on one panel and one sentence. A person who wants the pool
list presses Show once. No rule, consensus or policy change.

## Prototype

This PR.
- views.mjs checks that the Pools and payments pool list shows "Not run
  yet" and draws its rows after Show.
- The replay line's check now accepts the singular.
- qol.mjs's no-node banner check presses Show first: the banner still
  comes, as soon as the list runs with no node.
