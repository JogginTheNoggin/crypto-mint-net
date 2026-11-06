# How to browse the ledger

## Summary

The **Ledger** screen lets you look through everything your node holds
by clicking: the pools, each pool's ledger page by page, every entry,
every transaction and every address. **Find** jumps to a transaction id
or an address. **Watch** shows what happens live.

## Start at the pools

Open **Ledger**. The table lists every pool the node follows: its mints,
its height (how many entries its ledger has), its weight, what is
pending, and other branches it knows about. Click a row to open that
pool.

## A pool's ledger

- The newest entries come first, 25 to a page, drawn as bars (taller
  means more weight of its own) and as a table.
- **Older**, **Newer** and **Newest** move between pages.
- **Pending** at the bottom lists transactions waiting to go into an entry.
- Click a bar or a row to open that entry.

## An entry

An entry is a block of transactions at one height. Its page shows its
hash, its parent, its own weight, the ledger's weight up to it, the
**weight behind it** (what a rival would have to beat), its layers and
when your node first saw it, then its transactions. **Older: height N**
and **Newer: height N** step one entry at a time.

## A transaction

A transaction's page says where it stands: in the selected ledger at a
height (click it to open the entry), pending, or held but not selected.
It shows the mint, the amount, the weight behind it, what it spent (each
linked to the transaction that made it) and what it paid (each address
linked). If a rival branch spends the same coins, it says so and lists
the rivals. The raw bytes are in a fold at the bottom.

## An address

An address's page shows its balance, what it holds (each linked to the
transaction that made it), its history (each linked to its entry) and
anything pending. It works for any address; no wallet is needed.

## Find

Type a transaction id or an address in the box at the top and press
**Find**. It tries a transaction first, then an address. Links to transactions and addresses on every other
screen also open here.

## Moving around

The trail under the search box (Pools › Pool 1 › Height 3 ›
Transaction …) links back up. The browser's Back button works, and you
can bookmark the address of any page to come back to it.

## Watch it live

**Watch**, **Start watching** shows events as they happen: transactions
arriving, entries and layers kept, the ledger switching to a heavier
branch. Tick **Kinds** or fill in the filter fields to narrow it. It keeps
running while you use other screens; **Stop** ends it.

**Explore** has the same lookups as forms with every filter, for when you
want to narrow a page by series, mint, address or transaction.

In the terminal: [How to browse the ledger (headless)](../headless/4-how-to-browse-the-ledger.md).
