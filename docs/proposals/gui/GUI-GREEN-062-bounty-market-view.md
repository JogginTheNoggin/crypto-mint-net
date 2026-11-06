# GUI-GREEN-062: A Bounty market view

| Field | Value |
| --- | --- |
| Status | proposed |
| Based on | main @ d25f777 |
| Area | gui/web/static/views/bounty.js (new), views/index.js, views/wallet.js (`walletFile` exported), gui/e2e/qol.mjs, views.mjs, parity.mjs, docs/gui.md, docs/FEATURES.md, CHANGELOG.md |
| Effort | medium |
| Decided by | (Grey) |

## Summary

The bounty market (D196) is terminal-only in phase 1. This proposal adds
a **Bounty market** view, between Work and Pools and payments, that runs
the same five `poo bounty` commands. I walked the whole market twice:
first in the terminal, then in this view. Both times I used two real
nodes on the worked example series, joined over P2P, one posting and one
taking.

1. **Payer, Post an offer:** pool 1, its tip entry, strength 8, 5 units.
   Offers then lists it.
2. **Grinder, Ask peers for offers:** the offer arrives. **Work** on its
   row fills the offer id into Work an offer. With a pay-to address,
   Work found a strength-19 layer, and the line says "the payer
   answered: accepted".
3. **Payer, Refresh:** the offer shows 1 claim, 1 unpaid, and a **Pay**
   button. Above the table: "1 accepted claim waits to be paid".
4. **Payer, Pay:** poo asks for the wallet's passphrase in the masked
   field. The line reads "Paid 5 to e51c5ebaf4…7450cf for the layer with
   nonce 1009664; the grinder was told." The row then shows 0 unpaid.

## Idea

- **Offers** is a panel that runs `poo bounty list` when the view opens.
  It is small, a row per offer.
  - It says what this node does in the market ("This node posts offers
    and does not take offers from peers").
  - **Your offers** is a table: offer, pool, entry, strength, units per
    layer, layers, claims and unpaid. Each row has Cancel, and Pay while
    it has an unpaid claim.
  - **Offers from peers** is a table with Work on each row.
  - The buttons fill the form below and go to it, as GUI-GREEN-061's
    steps do. Nothing runs until its form is pressed.
  - poo's output stays under Output as printed.
  - "Unpaid" counts the claims with no `paid` list. Nothing else is
    computed.
- **When the market is off**, poo refuses with "the bounty market is
  off: set bounty.post or bounty.take in node.json". The GUI does not edit
  node.json, so the panel adds where and how: a `bounty` section with
  `post` (which also needs `max_amount` and `budget`) or `take`, then a
  restart. This matches docs/running-a-node.md.
- **The forms** are Ask peers for offers (`list -ask`), Post an offer,
  Work an offer, Pay a claim and Cancel an offer.
  - Every flag `poo bounty … -h` lists is offered, so parity covers the
    view and the five LEFT_OUT_SUBS entries go. Parity now finds 82
    commands.
  - Pay sends `-wallet` from the Wallet view's wallet file
    (`walletFile`, now exported), as the Wallet view's own forms do.
  - Post, Pay and Cancel ask before they run.
  - Work prints what the payer answered, and Pay what was paid and
    whether the grinder was told. Each reads poo's own fields.

## For Source (not built here)

1. **The bounty settings are not settable limits.** `poo policy limits`
   lists none of `bounty.*`, so the market is turned on only by editing
   node.json by hand, then restarting. jerry's principle (D159) prefers
   settable limits with floors and ceilings. `max_open`,
   `max_leases_per_grinder`, the lease floor and ceiling, `held_offers`
   and `min_price` already have ranges. With them settable, the
   Settable limits panel would turn the market on with no GUI change.
2. **Print outcome names in `poo bounty list`.** A held offer's `acks`
   print `"outcome": 1` and `"outcome": 2`, while `poo bounty work`
   prints `"outcome": "accepted"`. The GUI would have to keep poo's table
   of codes to show them, so this view leaves them under Output as
   printed.
3. **An offer's state and closing time.** Neither list says whether an
   offer is open, filled, closed or past its time, or when it closes.
   `open_seconds` and `posted` are there, but the arithmetic and the
   "filled" rule are poo's. A filled offer still shows Cancel here. On
   the grinder, an offer that was filled and paid stays in "Offers from
   peers" with Work on it until a restart. `"state"` and `"closes"` on
   each offer would let both tables say it.
4. **A second Pay waits for the first payment's change.** After one
   `bounty pay`, the next failed with "not enough spendable units: 0
   held, 5 asked" until an entry was built. That is ordinary wallet
   behaviour, but a payer with several claims meets it at once. poo could
   say so in the error ("the change of a payment not yet in an entry").

## White's checklist, for this change

| Line | Answer |
| --- | --- |
| 2. CHANGELOG and FEATURES | A sentence on the GUI bullet under "Unreleased (0.1.0)", and a Bounty market row in FEATURES.md's GUI table; docs/gui.md's view table too |
| 6. Only through pkg/cli | Page only; no Go change |
| 8. Browser storage | Reads the Wallet view's saved wallet file; writes nothing new |
| 9. `/api/` shapes | Unchanged |
| 14. e2e | All 10 scripts pass locally on this commit |

## Cost and risk

One view file of about 145 lines, one line in index.js, and one word in
wallet.js. No rule, consensus, policy or `/api/` change. The view runs
only `poo bounty` command lines, which it shows. A person can do
everything it does in the terminal.

## Prototype

This PR.
- views.mjs, on the e2e node (market off): Offers shows the steps to
  turn it on under poo's refusal, and the Work and Pay forms preview
  their lines.
- qol.mjs checks `marketView` on its own:
  - the headings, the owed line and the unpaid counts;
  - Pay only on an offer with an unpaid claim, Cancel on every offer of
    this node's, and Work on a peer's offer, each calling the form it
    fills;
  - the steps for poo's market-off refusal only;
  - the pay and work lines.
- parity.mjs covers every `poo bounty` flag.
