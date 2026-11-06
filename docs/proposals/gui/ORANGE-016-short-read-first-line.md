# ORANGE-016: Name the first 20 items of a plan's "Read first" line, with Show all

| Field | Value |
| --- | --- |
| Status | adopted (Round 22 Part A) |
| Based on | main @ 4dc7d3d |
| Area | gui/web/static/views/policies.js (`showPlan`: defences, now used, follows), gui/e2e/views.mjs |
| Effort | small |
| Decided by | |

## Summary

Above the plan's table, an import draws a "Read first" line that names
every defence the import changes (D150, D152). It also draws a "Now
used" line for series that become used (D156), and a note on what each
series' follow covers. Each line joined its whole list into one
paragraph, and the "Read first" and "Now used" lines are alerts, which a
screen reader reads out whole. Every new peer, registry or grind job
taken with the settings is a defence. So a settings file with 50,000
peers (1 MB) made a "Read first" line of 1.5 million characters and
183,000 pixels tall. The table and Apply came after it. ORANGE-015
limited the table to 500 rows, but this line was still drawn whole.

With the change, each of these lines names the first 20 items and says
how many more there are, as in "Read first: 50000 lines change a
defence of this node or where it reaches out: p2p.peers[10.0.0.0:7000],
…, and 49980 more." A "Show all 50000" fold below lists every one, and
draws the list only when opened. The alert is now that short line.
Nothing is dropped: each item is still in the fold, in the table (500 at
a time) and in "Output as printed".

| Import node settings (new peers) | Before: until the plan shows, "Read first" height, page work | After |
| --- | --- | --- |
| 10,000 (190 KB file) | 0.34 to 0.61 s, 35,344 px (293,195 characters), 0.19 to 0.40 s | 0.26 s, 151 px (684 characters), 0.10 s |
| 50,000 (1 MB file) | 1.6 to 2.1 s, 183,041 px (1,500,371 characters), 1.1 to 1.3 s | 1.46 s, 151 px (685 characters), 0.52 s |

`poon` alone makes the 50,000-peer plan in 0.46 s. The 1.5 s that is
left is mostly the browser reading 6.9 MB of JSON and the result view
(`render.js`) drawing it. The main gain is not time. Apply and the
table are now in reach, and a screen reader no longer reads out 1.5
million characters.

## Measured how

The probe was the ORANGE-015 Playwright script, pointed at the Import
node settings form. It used a saved node policy with one peer and a
settings file listing N new peers, and timed Preview until the plan
showed. The "Read first" line's height is its bounding box. "Page work"
is the Chrome TaskDuration added during the preview. The before and
after builds ran back to back; the before build was run twice.

## Checks

- `gui/e2e/views.mjs` imports settings with 30 new peers. The alert
  line must end "and N more." without naming the last peer. "Show all"
  must then list all 30.
- The views and qol checks pass, including the existing one that
  `p2p.peers` is named in the line.

## Modularity

Nothing to challenge. `someOf` and `allOf` are two small helpers, used
the same way by every list in the plan.

## Consensus

None. This only changes how much of a list the page names at once.
