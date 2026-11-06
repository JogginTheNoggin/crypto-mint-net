# GUI-GREEN-056: Node view, pool rows on demand

| Field | Value |
| --- | --- |
| Status | adopted (Round 31 Part A, merged as is) |
| Based on | main @ 0ade046 |
| Area | gui/web/static/views.js (panel `start`), views/node.js (Each pool, What the node follows, followView), gui/e2e/views.mjs, docs/FEATURES.md |
| Effort | small |
| Decided by | (Grey) |

## Summary

Round 30 Part A moved Followed pools' 5 s refresh to `poon status -counts`
(ORANGE-021), so the timer no longer fetches a row for every pool. On the
reserve series (1,024 pools), though, the Node view still drew every pool
row **each time it opened**:

- **Each pool** ran `poon status` (350 KB) as soon as the view mounted.
  Its comment and help said it runs "only when shown or asked", but every
  panel runs once on opening.
- **What the node follows** ran `poon follow`, which prints the follow
  rules and then every pool they cover (410 KB on the reserve), all drawn
  as a table.

I measured this with a real reserve node and Chromium, opening the Node
view four times:

| | Rows in the view | Page elements | Followed series drawn after |
| --- | --- | --- | --- |
| main @ 0ade046 | 1,008 | 23,550 | 333 to 712 ms |
| this PR | 6 | 451 | 121 to 135 ms |

## Idea

- **`panel(ctx, { start: false })`.** The panel does not run when it
  opens. It says "Not run yet: press Show.", and its button reads Show
  until the first run, then Refresh. Every other panel is unchanged.
  **Each pool** uses it, so the full `poon status` runs only when a person
  asks for it.
- **What the node follows** still runs on opening, so the follow rules
  show at once. A new `followView` draws the rules as printed and folds
  the pools under "1024 pools followed", drawn only when opened. The raw
  output stays under "Output as printed", drawn lazily as before.
- Nothing is computed: the count is the length of poon's own `pools` list.

## Notes for others (not built here)

- **Source:** the `poon status -counts` rows name the series by id only.
  On the reserve, the Followed pools row reads `fe52c1ac…` while
  Followed series, just above, says "the built-in reserve series".
  Adding `builtin` to each `-counts` row, as `series followed` has it,
  would let both panels name it without the GUI matching ids.
- **Grey:** Followed pools now draws the entry cache line and the file
  limit alert from `bodies` and `file_limit`, but its result also prints
  both objects in full under the table, every 5 s. The same fold as
  above ("Output as printed") would leave just the table and the lines.
  This is cosmetic, so I left it.

## Cost and risk

About 25 lines across views.js and node.js. A person who wants every
pool's row presses Show (Each pool) or opens the fold (follow list),
once. Nothing a node decides or a command prints changes.

## Prototype

This PR. views.mjs checks:
- Each pool says "Not run yet" with a Show button, draws after Show, and
  then reads Refresh;
- the follow list shows "N pools followed" closed, with no table until
  it is opened, then the pool rows.
