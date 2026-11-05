# ORANGE-021: Let the Followed pools refresh leave out the per-pool rows

| Field | Value |
| --- | --- |
| Status | proposed |
| Based on | main @ 4e1c601 |
| Area | gui/web/static/views/node.js (the Followed pools panel), and one `poo status` output choice in pkg/cli (Source) |
| Effort | small |
| Decided by | |

## Summary

The Node view's Followed pools panel runs `poo status` every 5 s. `poo
status` prints one row for every followed pool. Since D170, a new node
follows the built-in reserve series out of the box, which has 1,024
pools. So the first thing a newcomer sees on the Node view refreshes
352 KB every 5 s and redraws a 500-row table each time. The page is 5%
busy, with a pause of about 150 ms on every refresh.

A node that follows 4,096 series has 8,192 pools. There each refresh is
2.8 MB, the pauses reach 280 ms with frame gaps up to 433 ms, and
poo-gui holds 140 MB.

With a series of 2 pools, the same panel costs 1% with no pauses. This
proposal brings the timed refresh down to that cost:

- **Source:** a `poo status` option that prints the per-series totals
  and the notes (entry cache, file limit, policy changed) without a row
  for every pool. It works the way `poo peers -counts` leaves out the
  book (ORANGE-017).
- **GUI:** the timed panel uses that option. The per-pool table stays
  one press away, either in a "Pools" form that runs plain `poo status`
  when asked or as a Refresh on a second panel that has no timer.

This is the same approach ORANGE-017 took for the peer book, and it is
about output only.

## Problem

The panel exists to show where the node stands, and that changes often.
The per-pool rows of a series mostly do not change: on a new reserve
node, all 1,024 rows read height 0, weight 0 and no entries. Yet each
refresh sends every row through poo-gui's job stream, parses it in the
page and draws 500 rows again. `render.js` caps the table at 500 rows,
but it still builds and lays out 10,500 elements every 5 s.

Round 29 Part A did not cause this. Its additions (the entry cache and
file limit lines, the Followed series panel and the ceiling column) cost
nothing measurable, as the measurements below show. The reserve series
default (D170) is what made the 1,024-pool case the common one.

## Measurements

These were measured with a real `poo node` and the real `poo-gui` at
4e1c601, in Chromium through Playwright. The probe opened the Node view,
waited 8 s, then sampled it for 60 s. Each home was run twice.

| Node follows | `poo status` prints, in | Page busy | Long tasks a minute, longest | Longest frame gap | poo-gui RSS |
| --- | --- | --- | --- | --- | --- |
| a series of 2 pools | 1 KB | 1% | none | 17 to 33 ms | 17 MB |
| the reserve series, 1,024 pools (`poo init`) | 352 KB, 21 to 33 ms | 5% | 12 to 13, 145 to 161 ms | 167 ms | 30 MB |
| 4,096 series, 8,192 pools | 2.8 MB, 201 ms | 6% | 20 to 21, 224 to 279 ms | 367 to 433 ms | 136 to 145 MB |

With the reserve series, the two pool tables in the Node view are most
of it. Out of 23,477 elements:

- Followed pools (`poo status`, timed) has 10,594 elements and 500 of
  1,024 rows.
- What the node follows (`poo follow`, run once) has 12,546 elements and
  501 rows.

Opening the Node view on the reserve series, three runs of each build
back to back:

| Build | All panels answered | Busy, first 10 s | Longest task | Elements |
| --- | --- | --- | --- | --- |
| Round 28 (adcf216) | 0.61 to 1.25 s | 0.8 to 1.5 s | 190 to 605 ms | 23,385 |
| Round 29 (4e1c601) | 0.63 to 0.79 s | 0.8 to 0.9 s | 184 to 282 ms | 23,477 |

The first run of each build was the slowest, so the 605 ms outlier is a
cold start, not Round 28.

## Idea

1. **Source:** `poo status` gets an output choice that leaves out the
   pool rows, with its name for Source to pick (for example `-counts` or
   `-series`). It should still print, per followed series, the counts
   poo already knows (pools, entries, pending, waiting, stored records),
   plus `bodies`, `file_limit`, `replay` and `policy_changed` as today.
   The GUI must not add up the rows itself (D84). That is why this needs
   Source.
2. **GUI:** the Followed pools panel's `refresh: 5000` runs that choice.
   The entry cache, file limit and policy-changed notes come from the
   same fields as now. The per-pool table moves to a panel or form
   without a timer.
3. What the node follows (`poo follow`) is not on a timer and costs
   12,546 elements once. It is left as it is.

## Cost and risk

- **Source:** one output choice, about the size of `peers -counts`.
- **GUI:** one argument on the timed panel, plus one panel for the pool
  rows.
- **Risk:** none to consensus. It changes what is printed, not what is
  valid.

## Prototype

None. The change starts in Source, like ORANGE-017. The 2-pool row
above is the cost the timed panel would have with a short answer.

## Modularity

Nothing to challenge.

## Consensus

None.
