# Every screen

## Summary

Every screen of `poon-gui`, in menu order, with its panels and forms and
the page of this guide that explains it. Panels show information (most
refresh by themselves); forms act when you press their button.

| Screen | What it is for | Guide |
| --- | --- | --- |
| Getting started | The first steps, one at a time | [Start here](1-start-here.md) |
| Node | Run the node and see what it follows | [Run, stop and update](4-how-to-run-stop-and-update.md), [Follow mints](4-how-to-follow-mints.md) |
| Ledger | Browse pools, entries, transactions and addresses by clicking | [Browse the ledger](4-how-to-browse-the-ledger.md) |
| Watch | What the node does, live | [Browse the ledger](4-how-to-browse-the-ledger.md#watch-it-live) |
| Explore | The same lookups as forms, with every filter | [Browse the ledger](4-how-to-browse-the-ledger.md) |
| Work | Grinding on CPU and GPU | [Grind](5-how-to-grind.md) |
| Bounty market | Pay or be paid for grinding | [Bounty market](5-how-to-use-the-bounty-market.md) |
| Pools and payments | Build entries and layers, rival claims, ledger files | [Claim a mint](5-how-to-claim-a-mint.md), [Move or back up](5-how-to-move-or-back-up-a-node.md) |
| Wallet | Your wallet | [Receive](3-how-to-receive.md), [Send](3-how-to-send.md), [Balance](3-how-to-check-your-balance.md) |
| Peers | Other nodes | [Manage peers](4-how-to-manage-peers.md) |
| Node policies | Saved setups, limits, export and import | [Move or back up](5-how-to-move-or-back-up-a-node.md), [Settings](6-reference-settings.md) |
| Series and keys | Keys, and making your own series | [Cold keys](3-how-to-use-cold-keys.md) |
| Jobs | Everything the GUI ran | below |
| Settings | Which node, theme, command lines | [Every setting](6-reference-settings.md) |
| Terminal | The headless tool, typed | below |

## Getting started

The first steps in order. Each has **Run in terminal** and a link to the
screen that does the same with forms.

## Node

- **Run the node**: **Start node**, **Stop node**, and the box to check
  every record again on this start.
- **Followed pools**: totals per series, refreshed every 5 seconds; the
  node's version, snapshots, entry cache and router port mapping.
- **Each pool**: every pool's row, when you press **Show**.
- **Write a node policy**: create a node policy.
- **What the node follows**, **Follow more**, **Follow less**, **Reload
  the node policy**.
- **Followed series** and **Variable series**: the series followed, and
  the parts of series that can grow; **Add a release**.
- **Health check**: problems and their fixes.

## Ledger

Pools, then a pool's ledger page by page, an entry, a transaction, an
address. **Find** takes a transaction id or an address.

## Watch

**Live events**: filter fields, **Kinds**, **Start watching**, **Pause**,
**Stop**, **Clear**. Click an event to see it whole.

## Explore

**Pools**, **Selected ledger**, **Pending transactions**, **Unspent
portions**, **A transaction**, **An address**, each a form with filter
fields.

## Work

**Devices**, **Jobs**, **Schedule**, **Schedule a job**, **Schedule
defaults**, **Run a job**, **Start a job in the background**, **Stop a
job**, **Benchmark**.

## Bounty market

**Offers**, **Ask peers for offers**, **Post an offer**, **Work an
offer**, **Pay a claim**, **Cancel an offer**.

## Pools and payments

- **Followed pools**, **Holdings of a mint**, **Where a payment stands**.
- **Build an entry** from pending transactions; **Add a layer** to an
  entry; **Build a rival claim**; **Submit an entry or layer** made on
  another node.
- **Export a pool's ledger**, **Import a ledger**.

## Wallet

At the top, **Wallet file** and **Series** pick the wallet for every
form. Then: **Keep the key off this machine**, **Balance**, **Addresses**,
**New address**, **Pay**, **Record a shared policy**, **Co-sign a
payment**, **Sign with a key kept elsewhere**, **Submit a co-signed
payment**, **Request a payment**, **What has arrived**, **Payments
sent**, **Send dropped legs again**, **Watch mints**, **Claim a mint**,
**Follow only the watched mints**, **Change the passphrase**, **Show the
seed**, **Create or restore a wallet**.

## Peers

**Peers**, **Find peers**, **Add, remove, label or connect a peer**,
**Act on a peer**, **Tracking**, **Tracking records**, **Chases**,
**Export peers**, **Import peers**.

## Node policies

**Policies**, **Use, archive, unarchive or delete**, **Export node
policies**, **Export node settings**, **Import node policies**, **Import
node settings**, **Settable limits**, **Set limits**.

## Series and keys

**Hashes, key algorithms and policy types**, **Make a key**, **New
series parameters**, **Series file info**, **Sign a series**, **Parts of
a variable series**, **Release a part**, **Check a release**, **The
worked example's series**, **Simulate the network**. Making your own
series is an issuer's task; see the
[headless guide](../headless/6-reference-commands.md).

## Jobs

Every command the GUI ran, with its output, **Stop** while it runs, and
**Run again in terminal**. Output that held a secret (a seed) is shown once and not
kept.

## Terminal

The prompt is `$ poon`: type any command of the headless tool after it,
for example `status`. It runs exactly as in a shell, with history (up and
down arrows), Tab to complete, Ctrl-C to stop, Ctrl-D to end input and
Ctrl-L to clear. Use it for
anything the screens don't cover; the
[headless guide](../headless/index.md) lists every command.

In the terminal: [Every command (headless)](../headless/6-reference-commands.md).
