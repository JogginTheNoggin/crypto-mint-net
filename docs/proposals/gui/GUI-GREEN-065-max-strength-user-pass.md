# GUI-GREEN-065: Max strength, a user pass after Round 39

| Field | Value |
| --- | --- |
| Status | proposed |
| Based on | main @ 21c34c8 |
| Area | gui/web/static/views/bounty.js (Ask's about text), gui/e2e/views.mjs, CHANGELOG.md, docs/FEATURES.md |
| Effort | small |
| Decided by | (Grey) |

## Summary

Round 39 Part A added `-max-strength` to Ask and Work and put `bounty.max_strength` in Set limits (D202). I walked the max-strength flow on two real nodes on the worked example series, joined over P2P:
- a payer with two open offers, one asking strength 20 and one asking 12;
- a grinder that takes offers.

1. **Grinder, Set limits:** bounty.max_strength read "0 to 255, default 0". I set it to 16 and saved, and the node reloaded.
2. **Grinder, Ask peers for offers** (empty Strongest offer): only the strength-12 offer was kept. The limit works.
3. **Grinder, Ask with Strongest offer 30:** still only the strength-12 offer. Working the strength-20 offer then gave "no offer with that id was received".
   - The book drops every offer above bounty.max_strength when it arrives (book.go, `ReceiveFrom`). So an ask can narrow below the limit but never widen it.
   - The field's placeholder ("default: bounty.max_strength, else any") and poon's help suggest it overrides the limit.
4. **Grinder, bounty.max_strength back to 0, then Ask at once:** "No offers yet". The save had reloaded the node:
   - the held offers, kept only in memory, were gone;
   - the payer had not connected again yet, so the ask reached no one.

   The same Ask a few seconds later kept both offers.
5. **Grinder, Work the strength-20 offer with Strongest offer 16:** "poon: the offer asks strength 20, over the 16 this node works (bounty.max_strength, or -max-strength)". It is clear and comes at once, with no grinding.
6. **Grinder, Work it with an empty Strongest offer:** "Found a layer of strength 23 in 1048576 tries; the payer answered: accepted."
   - On the payer the offer reads `filled`.
   - On the grinder it still reads `open`, with Work on its row. Its `acks` hold the accepted claim, and its count was 1.

The market notes (D202) show over the tables. On a node with bounty.post on and no budget, poon's note "… so no offer can be posted" sits under "This node posts offers".

## Idea (built here)

- **Ask's about text** now says two things:
  - Strongest offer can ask for fewer offers than bounty.max_strength allows, never for more.
  - A reload (a save in Set limits) empties the offers held from peers, so ask again a few seconds after it.

  This is words only. The GUI still sends whatever is typed, and poon decides.

## For Source (not built here)

1. **`-ask -max-strength` above the limit.**
   - Today it asks peers for offers up to 30, and the node then drops everything above 16 as it arrives.
   - Two fixes would make it honest:
     - poon refuses the value, or prints a note ("at most bounty.max_strength 16 is kept").
     - The received offers honour the asked value.
   - The help line "default: bounty.max_strength, else any" reads as an override.
2. **How many peers an ask reached.** `poon bounty list -ask` drops the count `BountyAsk` returns. Right after a reload, or with no peer that runs the market, "no offers" looks the same as "peers have none". An `"asked": N` field (or a note when N is 0) would let both the terminal and the GUI say so.
3. **A grinder's held offer stays open after it is filled.** When accepted acks reach the offer's count, the held state could read `filled`, as the payer's does. Then Work would not stay on a done offer. This is GUI-GREEN-062's note 3, still true for held offers.
4. **What 0 means for a limit.** `poon policy limits` gives no word on 0:
   - bounty.max_strength 0 is any strength;
   - bounty.max_amount, bounty.budget and bounty.min_price 0 is "not set".

   An `about` line (or `"zero": "any"`) on each limit would let Set limits say it without a GUI table.

## White's checklist, for this change

| Line | Answer |
| --- | --- |
| 2. CHANGELOG and FEATURES | A sentence on the GUI bullet under "Unreleased (0.1.0)"; the Bounty market row in FEATURES.md |
| 6. Only through pkg/cli | Page only; no Go change |
| 8. Browser storage | Unchanged |
| 9. `/api/` shapes | Unchanged |
| 14. e2e | All 10 scripts pass locally on this commit |

## Cost and risk

One sentence in bounty.js. No rule, consensus, policy or `/api/` change.

## Prototype

This PR. views.mjs checks that the Ask form says Strongest offer asks for fewer and never for more, and that a reload empties the held offers.
