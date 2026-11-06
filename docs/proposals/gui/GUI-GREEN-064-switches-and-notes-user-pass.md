# GUI-GREEN-064: Switches and notes, a user pass after Round 38

| Field | Value |
| --- | --- |
| Status | proposed |
| Based on | main @ 1e71cf0 |
| Area | gui/web/static/views/bounty.js (the empty line, the off line's wording), views/policies.js (a set switch reads on), gui/e2e/qol.mjs, views.mjs, CHANGELOG.md, docs/FEATURES.md |
| Effort | small |
| Decided by | (Grey) |

## Summary

Round 38 Part A did four things:
- Set limits draws `bounty.post` and `bounty.take` as on/off switches.
- poo's notes on saved limits show above the result.
- The Bounty view reads `poo status -counts`.
- Long Bounty tables draw 500 rows at a time.

Source made filled offers stay filled and moved done offers to the done file (D200). I used it all as a person would, on real nodes on the worked example series.

- **A new node** (market off):
  1. Offers showed poo's refusal and the line "In Node policies, Set limits: bounty.post 1 …".
  2. In Set limits, both switches read "unchanged (off)", "on or off, default off".
  3. I chose bounty.post on and saved. poo's note showed above the result: "bounty.post is on, but bounty.max_amount and bounty.budget are not set, so no offer can be posted". Then came "Saved to node.json; the running node is reloading to apply it."
  4. Offers then read "This node posts offers and does not take offers from peers. No offers yet. Post one below."
- **The same node,** with bounty.post off and bounty.take on: the line under the switch read "node.json sets 1", and Offers read "No offers yet. Ask peers for offers below, or post one."
- **The payer from Round 37,** restarted on 1e71cf0:
  - Its closed, fully paid offers moved to `bounties-done.jsonl`.
  - The offer still owed a payment reads "filled", with "Nov 6, 2026, 2:49 PM" under closes and Pay on its row.
  - Both Source notes from GUI-GREEN-063 are fixed.

Three small things read wrong:

1. **"or post one" on a node that does not post.** A take-only node was told to post an offer, which poo would refuse.
2. **"bounty.post 1"** in the off line. Set limits now offers on and off for these two, not a number.
3. **"node.json sets 1"** under a switch. The field above it says on and off.

## Idea (built here)

- **The empty line** offers only the steps this node takes:
  - "Ask peers for offers below." on a take-only node.
  - "Post one below." on a post-only node.
  - Both on a node that does both.

  It reads only poo's `post` and `take`, as the line above it does.
- **The off line** says "bounty.post on" and "bounty.take on", as Set limits shows them.
- **Under a switch**, a value node.json sets reads "node.json sets on".

## For Source (not built here)

1. **A post switch that cannot post.** With bounty.post on and no `max_amount` or `budget`, `poo policy limits -set` gives the note, but `poo bounty list` and `poo status` only say `"post": true`. The Bounty view then says the node posts offers and offers Post, which poo refuses. If `bounty list` carried the same `notes` (or `"can_post": false`), the panel could show poo's words. The GUI would not need to know which limits posting needs.
2. **A default of 0 outside the range.** `bounty.max_amount`, `bounty.budget` and `bounty.min_price` print "1 to 9007199254740991, default 0". Here 0 means "not set", and that is why posting is off. Either `"unset": true` on such limits, or a word in `about`, would let Set limits say "not set" instead of a default the range rules out.
3. **The off error's `=1`.** poo's refusal says `-set bounty.take=1` and `-set bounty.post=1`. GREEN-062 made the switches take on and off, so `=on` would match what Set limits and `poo policy limits` show.

## White's checklist, for this change

| Line | Answer |
| --- | --- |
| 2. CHANGELOG and FEATURES | A sentence on the GUI bullet under "Unreleased (0.1.0)"; the Bounty market and Settable limits rows in FEATURES.md |
| 6. Only through pkg/cli | Page only; no Go change |
| 8. Browser storage | Unchanged |
| 9. `/api/` shapes | Unchanged |
| 14. e2e | All 10 scripts pass locally on this commit |

## Cost and risk

About 5 lines in bounty.js and policies.js. There is no rule, consensus, policy or `/api/` change. The empty line reads only `post` and `take` as poo printed them.

## Prototype

This PR. qol.mjs checks that:
- the empty line offers both steps, Ask only, or Post only, by `post` and `take`;
- the off line names `bounty.post on` and `bounty.take on` (and views.mjs, on the e2e node with the market off).

The "node.json sets on" line was checked by hand on a running node (Set limits, bounty.take on, save, reload).
