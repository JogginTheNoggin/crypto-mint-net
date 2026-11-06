# ORANGE-029: Let `poon address` print a page of holdings, and the Ledger view ask for one

| Field | Value |
| --- | --- |
| Status | proposed |
| Based on | main @ 56c5e0a |
| Area | `poon address` in pkg/cli (Source); the Ledger view's address screen in gui/web/static/views/ledger.js (GUI) |
| Effort | small |
| Decided by | |

## Summary

The Ledger view (Round 41) opens an address with `poon address -limit 50 ADDR`. `-limit` bounds the history only. Holdings, the address's unspent portions, are printed whole. An address that holds many portions prints all of them, and the page then draws only the first 500 (`rowsTable`).

On a 100,000-entry ledger where every entry paid one address:

- `poon address` printed 26.9 MB, 21.1 MB of it holdings, in 0.93 to 1.06 s.
- The Ledger view took 0.90 to 1.07 s to open that address.
- `-limit 1` made no difference (26.9 MB, 1.06 s), because the time is in the holdings.

Every other Ledger screen opened in 14 to 55 ms at every ledger size, so the address screen is the only one that grows with the ledger.

The proposal:

1. **For Source:** let `poon address` print a page of holdings, with the count and the balance still worked out over all of them.
2. **For the GUI:** ask for one page, the 500 rows it draws anyway.

## Problem

```js
read(['address', ...where.flags(), ...sflag(series), '-limit', String(HISTORY), ...])
```

`poon address`'s reply has `holdings`, an array of every unspent portion. The GUI's address screen shows the first 500 rows with Show more, and the count in the heading ("Holdings (100000)"). It reads the whole array only for that count and those 500 rows. The rest is printed by poon, carried over `/api/run`, parsed in the page, and dropped.

An address with many small portions is a likely case: a pool of mints all claimed to one key, an exchange, or a grinder paid per layer through the bounty market.

## Measurements

The ledgers come from a throwaway Go test, not committed. It builds one pool of N entries in a test series, one claim and one layer per entry, every claim paying the same address. The real `poon` and `poon-gui` at 56c5e0a were used, with poon-gui in Chromium through Playwright.

`poon address -limit 50 ADDR` on the command line:

| Entries (holdings of the address) | Output | Time |
| --- | --- | --- |
| 2,000 | 576 KB | 32 ms |
| 20,000 | 5.4 MB | 231 ms |
| 100,000 | 26.9 MB (holdings 21.1 MB, history 30 KB) | 0.93 to 1.06 s |

The Ledger view, from setting the address to the screen being drawn:

| Screen | 2,000 | 20,000 | 100,000 |
| --- | --- | --- | --- |
| Pools | 30 ms | 24 ms | 27 ms |
| A pool, newest 25 | 55 ms | 34 ms | 36 ms |
| A pool, 25 at the middle | 24 ms | 25 ms | 18 ms |
| An entry | 18 ms | 20 ms | 14 ms |
| A transaction | 18 ms | 22 ms | 24 ms |
| The address | 85 ms, 615 KB | 234 to 292 ms, 5.8 MB | 0.90 to 1.07 s, 26.9 MB, task up to 105 ms |

The address screen draws 4,025 elements at every size, because it already shows only 500 holdings. The page work is 0.07, 0.17 to 0.19 and 0.30 to 0.33 s busy. Most of the wait is poon printing, and the transfer.

## Idea

1. **Source:** add a holdings page to `poon address`, for example `-holdings N`. The default stays all, so the reply is unchanged for scripts. Add `holdings_count`, and keep `balance` as now, over every holding. A `-holdings-after` (or the history's own `next`) could page on later. That is Source's choice.
2. **GUI:** the address screen passes `-holdings 500`. It shows "Holdings (N)" from `holdings_count`, and draws the page it got. For more than 500, it says how many there are and gives the command line, or pages if Source adds a cursor.
3. Until Source has it, nothing changes: the screen works, just slowly for an address with tens of thousands of portions.

## Cost and risk

- **Source:** a flag and a count in one command. The balance is still summed by poon, over every holding.
- **GUI:** one flag and one heading.
- **Risk:** none to consensus. Only how much of a reply is printed changes.
- **Compatibility (White):** with the default left as all, the reply and every script stay the same. A GUI that passes `-holdings` needs a poon that knows it: poon-gui runs its own poon code, so the two ship together.

## Prototype

None: the change starts in Source's command. The numbers above are the baseline.

## Modularity

Nothing to challenge.

## Consensus

None.
