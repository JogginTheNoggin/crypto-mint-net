# How to move or back up a node

## Summary

You can carry a node's setup to another computer, or keep a backup, in
three parts: its settings and what it follows (**Node policies**), its
peers (**Peers**), and its ledgers (**Pools and payments**). Every import
shows what it would change first, and every ledger entry imported is
checked in full, so a file can never make your node accept something
invalid.

## What to carry

| What | Where | Holds |
| --- | --- | --- |
| Node policy | **Node policies**, **Export node policies** | Settings and what the node follows |
| Settings only | **Node policies**, **Export node settings** | Everything but what it follows: peers, registries, limits, grind settings |
| Peers | **Peers**, **Export peers** | The peer book and blocks |
| Ledger | **Pools and payments**, **Export a pool's ledger** | A pool's entries and layers |
| Wallet | your `wallet.json` file | Keys (sealed), labels, payments; back up the seed too ([How to back up your wallet](3-how-to-back-up-your-wallet.md)) |

No export holds a key, seed, passphrase or token.

## Export

- **Export node policies**: choose **this node policy**, **every saved
  node policy** or **one series**. Tick **Keep machine-local settings**
  for a backup or a clone of this same machine (it keeps addresses and
  the grind device). Fill in **To a file**, or leave it empty and use
  **Save as file**.
- **Export node settings**: the same, without what it follows.
- **Export peers**: tick categories, or none for all.
- **Export a pool's ledger**: **Pool**, and **From height** / **To height**
  for part of it. A line says what the file holds.

## Import

Each import has a preview: press **Preview** to see what is new, changed
or in conflict, then **Apply** to make the change. Nothing changes before
**Apply**.

- **Import node policies** and **Import node settings**: choose whether
  the file wins where both set a value differently, or replace this
  policy with it. Tick the machine-local box only when restoring onto the
  same kind of machine. **Registry paths from the other machine** maps the
  other machine's file paths to this one's.
- **Import peers**: adds; it never removes or unblocks anything.
- **Import a ledger**: give the file (a path, or pick it). Tick **Only
  check the file** to read it without the node first. The reply counts
  what was kept, already known, and refused.

## Use, archive and delete node policies

**Node policies**, **Use, archive, unarchive or delete** switches between
saved policies (each is a separate node setup with its own data), puts
one away, or deletes it. Tick **Fresh (drop stored data)** to start one
over.

In the terminal: [How to move or back up a node (headless)](../headless/5-how-to-move-or-back-up-a-node.md).
