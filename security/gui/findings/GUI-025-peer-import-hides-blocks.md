# GUI-025: The Import peers preview hides the blocks it will apply below the first 500 lines

| Field | Value |
| --- | --- |
| Severity | low |
| Status | open |
| Found on | main @ f4f7945 (PR #129, the Peers view's "Import peers" form, D157) |
| Component | gui/web/static/views/peers.js (`showPeerPlan`: block lines are put after every added, more and label line, in one table drawn 500 rows at a time, with no "Read first" line) |
| Surface | node (the tracker's address and range blocks, from a peer file someone else made) |
| Fixed by | (GUI/Blue) |

## Summary

`poo peers import -blocks [-ranges]` takes a peer file's blocks. Each
one makes this node refuse an address, or a whole /16 network range,
until it is unblocked. poo treats these as the lines to read first. Its
text plan prints them at the top, under "read first: these blocks refuse
addresses on this node:", before the addresses it adds.

The GUI's preview does the opposite:

- `showPeerPlan` builds one list in this order: added, more, label,
  label kept, **block**, skipped, refused. `planTable` draws the first
  500 lines (ORANGE-015), and the rest wait behind "Show more".
- Nothing outside the table names the blocks. The node policy plan has a
  "Read first" alert for its defences (D150). The peer plan has none, and
  its notes line counts no blocks.

So a peer file with more than 500 new addresses pushes every block out of
sight. The preview shows 500 harmless "added … unlisted" rows and Apply,
and the blocks it would apply aren't on the page unless the user presses
"Show more" or opens the raw JSON. With fewer addresses the blocks are
drawn, but at the bottom of the table, after the additions.

A block list is one of the things D157 says people will share ("share a
block list"). So a file that pairs a long, plausible peer list with
blocks of the peers the user relies on, or of their ranges, is the case
the review gate is for.

## Reproduction

poo-gui at f4f7945, `init -example` in a fresh home, no node running.

big.json has 600 public addresses (`20.1.7.9:7000` to `49.20.7.9:7000`,
one mint policy each) and two blocks: address `198.51.100.20:7000` and
range `v4:203.0`, both until 2099.

1. `poo peers import big.json -blocks -ranges` (the CLI) starts with:
   ```
   read first: these blocks refuse addresses on this node:
   ! block 198.51.100.20:7000 until unblocked
   ! block range v4:203.0 until unblocked
   + 20.1.7.9:7000 (unlisted, 1 mint policy)
   ```
2. In the GUI, go to Peers → Import peers. Tick "Take the file's address
   blocks too" and "With blocks, take its network range blocks too", pick
   big.json, and press Preview. Checked with Playwright:
   - the table shows 500 rows, and none of them is a block (`tr.plan-block`
     count 0);
   - the first row is `added 20.1.7.9:7000 unlisted, 1 mint policy`;
   - the plan has no alert;
   - the notes read "0 already the same. Nothing changed yet: press Apply
     to make this change.";
   - "Showing 500 of 602 lines" is shown.
3. Press Apply. The dialog reads `Run poo peers import -blocks -ranges
   -json -apply -if-digest 67c8ef01… - with the previewed file big.json?`.
   Accept it. `poo peers export -category blocked` then lists
   `198.51.100.20` and `v4:203.0`.

The digest pin works as designed. Apply makes exactly the plan
previewed, but the preview never showed the blocks.

## Impact

Blocks the user didn't see are applied: an address, or a /16 range,
refused until unblocked. Used against the peers the user relies on, this
helps cut a node off from honest peers (eclipse), and the user doesn't
know why those peers stopped. It needs the user to tick "address blocks
too" (and "range blocks" for ranges) and press Apply on a file someone
else made. A file never blocks an address node.json lists (`p2p.peers`),
or a range holding one. Blocks can be undone with `poo peer unblock`. It
is node policy, not consensus. Rated **low**.

## Recommendation

GUI/Blue, in `showPeerPlan` (peers.js):

- Put the blocks first, as poo does. Add a "Read first" alert above the
  table when `p.blocks` is not empty, like the policy plan's
  `plan-defences`. For example: "Read first: 2 blocks refuse addresses
  on this node: 198.51.100.20:7000 until unblocked, range v4:203.0 until
  unblocked." Use `someOf` / `allOf` so a long list stays bounded
  (ORANGE-016).
- Order the table's kinds as poo prints them: block, added, more, label,
  label kept, skipped, refused. Then the first 500 rows start with the
  blocks.
- Count the blocks in the notes line, as poo counts what it skipped.
- e2e (views.mjs): import a file with 600 addresses and one block, with
  blocks ticked. Expect a `plan-blocks` alert that names the block, and a
  first table row of kind `block`.
