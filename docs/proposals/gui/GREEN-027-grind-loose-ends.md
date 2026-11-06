# GREEN-027: Grinding loose ends for Source

| Field | Value |
| --- | --- |
| Status | done by Source (D112) |
| Based on | claude/gui-4-watch @ 8b044ac |
| Area | pkg/grind, pkg/cli grind (Source/Grey) |
| Effort | small |
| Decided by | (Grey) |

## Summary

Small things seen while using `poon grind` through the GUI, for
Source/Grey to fix or leave:

1. A `pending` job that built an entry reports `tries: 0` and `rate: 0`
   in `poon grind jobs` (job 1 in the e2e run: 0 tries, 0 H/s, 0 layers,
   1 entry). Entry searches seem not to be counted, so the GUI shows a
   job that did work as if it did none.
2. `poon grind run` prints two `start` lines. The first is
   `{"job":2,"kind":"start","state":"running"}` with no plan, and the
   second has the series and plan. A reader that takes the first start
   as the plan gets none. One start line per plan, or a `kind` such as
   `accepted` for the first, would be clearer.
3. On a fresh node with no entry yet, a `tip` job waits with "nothing to
   work on yet" for as long as it runs. That is correct, but the reason
   could point at the fix ("no entry in pool K yet: claim or pay, or use
   the pending target"), as doctor's fixes do.

## Cost and risk

Small, and all in Source code. The GUI is not affected by any of them
except that it shows what is printed.

## Prototype

None.
