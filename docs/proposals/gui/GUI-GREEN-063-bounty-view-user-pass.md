# GUI-GREEN-063: Bounty view, a user pass after Round 37

| Field | Value |
| --- | --- |
| Status | proposed |
| Based on | main @ 7eb16ab |
| Area | gui/web/static/views/bounty.js (Cancel on open offers, `when`, the off line's wording), gui/e2e/qol.mjs, CHANGELOG.md, docs/FEATURES.md |
| Effort | small |
| Decided by | (Grey) |

## Summary

Round 37 Part A gave the Bounty view the market's state from `poo
status` (W-30), each offer's state and closing time (D197, D198), and an
off line that points to Set limits now that the bounty settings are
settable (D198). I used it as a person would, on real nodes on the
worked example series.

- **A new node:**
  1. Offers says "The market is off on this node. In Node policies, Set
     limits: bounty.post 1 …".
  2. In Set limits I typed 1 in the bounty.take field and saved. poo said
     "Saved to node.json; the running node is reloading to apply it."
  3. Back on Bounty market, the panel read "This node does not post
     offers and takes offers from peers. No offers yet."

  This is the step that needed a hand edit of node.json last round. It
  works.
- **The payer from Round 36,** with three offers past their time (two
  paid, one with a claim unpaid), showed three things to change:
  - **Cancel on every row**, including the three expired offers. A
    closed offer cannot be cancelled again, so the button only leads to
    a refusal.
  - **Times as printed to the nanosecond:** "closes
    2026-11-06T14:39:37.232046814Z" in UTC. That is hard to read at a
    glance, and it is not the reader's clock.
  - **The off line said "A running node takes the change at once".**
    Set limits lists bounty.post and bounty.take as taken "on reload",
    and the save said the node "is reloading".

## Idea (built here)

- **Cancel only on an open offer.** An offer of this node's gets Cancel
  only when poo says it is `open`, or when poo gives no state (an older
  node). This is the same rule Round 37 Part A gave Work on a peer's
  offer. Pay still shows while a claim is unpaid, whatever the state, so
  the expired offer above keeps its Pay. poo still decides either
  action.
- **Times in the reader's own words.** `when(s)` draws a time poo
  printed in the reader's time zone and locale, as a `<time>` element.
  It has poo's exact text in `datetime` and in the tooltip, for example
  "Nov 6, 2026, 2:39 PM" over `2026-11-06T14:39:37.232046814Z`. Text
  that is not a time is shown as printed. It is used for closes and
  received in both tables.
- **The off line** now ends "Saving reloads a running node to apply
  it.", which matches what Set limits says.

## For Source (not built here)

1. **A filled offer reads as expired.** Offer c56fa6ae… was filled and
   paid at 13:39, an hour before its time ran out. After 14:39, `poo
   bounty list` gives it `"state": "expired"`. When its time runs out, a
   filled offer is closed like any other (book.go, the expiry sweep), and
   `Mine.state` checks `Closed && expired` before it counts the claims.
   A payer looking back sees "expired" for an offer that did its job.
   Checking filled first, or a separate "filled" for a closed offer whose
   claims reached its count, would keep the word right.
2. **Done offers are still listed.** That offer and 373be10b… are closed,
   fully paid and have no lease, which is ORANGE-046 P1's test for moving
   to `bounties-done.jsonl`. After a node restart on 7eb16ab, both are
   still in `bounties.json` and in `poo bounty list`, and no done file
   exists. They were written before D198, so the move may happen only at
   the next save. If so, a check at load would tidy older books.
3. **Switches shown as ranges.** `poo policy limits` gives bounty.post
   and bounty.take a range of "1 to 1, default 0". The field works (1 on,
   0 back to the default, off), but it reads oddly for an on/off setting.
   A `"switch": true` on such limits would let Set limits draw a tick box
   with no GUI rule.

## White's checklist, for this change

| Line | Answer |
| --- | --- |
| 2. CHANGELOG and FEATURES | A sentence on the GUI bullet under "Unreleased (0.1.0)", and the Bounty market row in FEATURES.md |
| 6. Only through pkg/cli | Page only; no Go change |
| 8. Browser storage | Unchanged |
| 9. `/api/` shapes | Unchanged |
| 14. e2e | All 10 scripts pass locally on this commit |

## Cost and risk

About 20 lines in bounty.js. No rule, consensus, policy or `/api/`
change. The GUI reads only the state poo prints, and shows poo's time
text in the tooltip.

## Prototype

This PR. qol.mjs checks that:
- the expired offer has no Cancel, and the acts are pay, cancel on the
  offer with no state, and work on the open peer offer;
- the closes cell is a `<time>` with poo's text in `datetime` and the
  title, and readable text of its own;
- `when` returns text that is not a time unchanged, and nothing for an
  empty value.
