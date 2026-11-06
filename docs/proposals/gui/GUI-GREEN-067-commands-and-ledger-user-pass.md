# GUI-GREEN-067: Where the GUI still needs commands, and ledger navigation

| Field | Value |
| --- | --- |
| Status | proposed |
| Based on | main @ 56c5e0a |
| Area | gui/web/static/views/ledger.js (a mint screen, mint links, Find by number), views/pools.js (Build an entry offers Add a layer), views.js (the file picker's line), gui/e2e/ledger.mjs, docs/gui.md, docs/FEATURES.md, CHANGELOG.md |
| Effort | small (built) + medium (proposed) |
| Decided by | (Grey) |

## Summary

jerry's principle (R41): the GUI abstracts commands and is user friendly. The terminal is only for what the UI can't reasonably do.

I used poon-gui on main 56c5e0a as a person would, with command lines hidden (the new default):
- a fresh node on the worked example's series, with four claims, a payment and five entries;
- every view, read for text that still asks the reader to think in commands;
- the Ledger view, walked from the pools down to an address.

**The Ledger view works well.**
- Pools lead to pages, an entry, a transaction and an address, and back.
- The trail, Back, Older/Newer and Find all behave.

**One gap:** a mint is a dead end.
- "Mint 5" shows as a plain number in the entry's table, the transaction's facts and an address's holdings.
- Find takes no mint number.
- The only way to see who holds a mint is Pools and payments, Holdings of a mint, where you type the serial.

**Elsewhere, the views still talk in commands.** The list is under Proposed. One spot made me leave the GUI:
1. Build an entry with no layer answers "the entry is selected once a layer lands on it: poon work -pool 0 -entry 0995b2…, or add -work N to the build".
2. To follow it, I had to copy the hash into Add a layer, further down the page.

## Built here

1. **A mint screen in the Ledger view** (`#/ledger?serial=5`).
   - It shows the mint's pool (linked) and who holds its units now: address, amount, and the transaction and output that made each portion, all linked. It reads `poon unspent -serial N`, and finds the pool from `poon pools`' serial ranges.
   - An unclaimed mint says "the mint is not claimed yet, or its claim is still pending". A number no followed pool holds says so.
   - The trail reads Pools › Pool 1 › Mint 5.
2. **Mint numbers link to it** wherever the Ledger view shows one: entry and pending tables, a transaction's facts, an address's holdings.
3. **Find takes a mint number.** A text of digits only opens the mint. The placeholder says "A transaction id, an address or a mint number".
4. **Build an entry offers "Add a layer to it".**
   - When poon's reply says the new entry waits for a layer (its `next` line), the result shows "The entry is built. It is selected once a layer lands on it." with a button.
   - The button fills Add a layer with the pool (or serial), the series and the entry hash, then moves to it. Nothing runs until Work is pressed.
5. **The file picker's line.** "Sent to poon as its input (- above); nothing is stored by the GUI" now reads "Read in place of a path: the field above then shows -, which means the picked file. The GUI keeps no copy."
   - The old line explained the "-" in a path field by pointing at a command line that is now hidden by default.
   - It shows under every Import form: node, peers, policies, series, pools.

## Proposed (not built here)

These want wording or layout decisions across views, so they are Grey's to schedule.

1. **About lines name commands and files instead of saying what happens.** With command lines hidden, these read as jargon. Each could say what happens and leave the command to Show commands:
   - Work: "Each part is a poon grind command; node.json's grind section sets the defaults".
   - Work: "Adds a job to node.json's grind section with poon grind schedule add".
   - Work: "Starts a job in the running node with poon grind run".
   - Explore: "Each is one poon command".
   - Wallet: "all through poon wallet".
   - Pools: "(poon pools)".
   - Node: "What poon doctor finds".
2. **Getting started is a list of commands.**
   - It opens "Each step is one poon command" and shows `poon version`, `poon init`, `poon node`, `poon status` and so on, even with command lines hidden.
   - A first-run person needs three things: make a node policy, start the node, and see that it is healthy. Each step could be a button that does it, or a link that goes to the view where it is done (Node, Health check, Work), with the command shown only under Show commands.
3. **Explore and Ledger overlap.**
   - Both look up pools, ledgers, pending, unspent, a transaction and an address.
   - Explore is the filter form (the Watch filters), and Ledger is the one to click through.
   - Two views for one job make a person guess. Either fold Explore's filters into Ledger as "Filter", or rename Explore to something like "Ledger queries" and say on it that Ledger is the place to browse.
4. **Pools and payments overlaps Ledger too.** "Holdings of a mint" and "Where a payment stands" are now the Ledger's mint and transaction screens. They could become links ("Open mint N in the Ledger") or go away.
5. **poon's own next-step lines name commands.** Examples: Health check's "fix: … poon policy limits -set series_ceiling=N", and Build's "poon work -pool 0 -entry …". Where the GUI has the matching form, a button next to the line can do what it says, as Build now does. Health check's fixes are the next place to do this.
6. **An unclaimed mint could link to Claim a mint** (Wallet) from the new mint screen, once the Wallet view can be opened at that form by its address (as `#/ledger?…` does).

## Seen on the way (for Source)

- Test nodes from before R40/R41 come up following nothing: "follow rule names a series not known or not accepted". The worked example's series id changed with the label rename. The Ledger view then says "This node follows no pools yet". That is right, but a person cannot tell why. Health check could say "a followed series is not known" with the id.
- Two such nodes peered before refuse each other: "no mint policy in common was verified". The cause is the same, and both are expected while nothing is released.

## White's checklist, for this change

| Line | Answer |
| --- | --- |
| 2. CHANGELOG and FEATURES | A sentence on the GUI bullet; the Ledger and Pools and payments rows; docs/gui.md's Ledger row |
| 6. Only through pkg/cli | Unchanged; the mint screen runs `poon unspent` and `poon pools` |
| 8. Browser storage | Unchanged |
| 9. `/api/` shapes | Unchanged |
| 14. e2e | All 11 scripts pass locally on this commit |

## Cost and risk

About 50 lines in ledger.js, 20 in pools.js and one line in views.js. No rule, consensus, policy or `/api/` change. The mint screen lays out what `poon unspent` prints. The only thing it works out is which followed pool's serial range holds the number.

## Prototype

This PR. ledger.mjs, on a real node, checks:
- the Mint link on a transaction opens Mint 6 in Pool 1, with its one holder and amount;
- Find "2" opens an unclaimed mint, and "99" says no followed pool holds it;
- Build an entry with no layer, then "Add a layer to it", fills Add a layer with pool 1 and the hash poon printed.
