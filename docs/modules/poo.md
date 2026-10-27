# cmd/poo

## Summary

The command-line tool. `poo node` runs the full node daemon with its store,
its peer connections, its peer tracker (`tracking.json`) and local RPC; client commands talk to it; `poo policy` manages node
policies; offline commands make keys, inspect series and run an in-process
demo. `poo wallet` keeps a holder's keys and pays, requests, co-signs and
follows payments through the node ([wallet.md](wallet.md)). `claim` and
`pay` sign with a single raw key and exist so a node can be exercised
without a wallet. See
[../running-a-node.md](../running-a-node.md) for a walk-through.

## Commands

| Command | Does |
| --- | --- |
| `poo init [-example] [-series FILE]... [-listen ADDR] [-p2p ADDR] [-advertise ADDR] [-peer ADDR]... [-registry LOC]... [-force]` | Writes a node policy that knows and follows the given series, and makes it current |
| `poo node` | Runs the node until interrupted |
| `poo peers` | The node's id, its p2p address, its connected peers with their categories and points, and its peer book |
| `poo peer timeout\|block\|unblock\|clear\|template\|delete ADDR [-seconds N] [-range] [-what flags\|measures\|all] [-template NAME]` | Acts on one peer: pause it, block it (or its range; `-seconds -1` until unblocked), lift either, clear its records, choose its tracking template, or delete it from the book (flags before ADDR) |
| `poo tracking [ADDR]` | Every tracked address's status and the blocked ranges, or one address's whole record |
| `poo chases` | The chase queue: heavier stated weights waiting or being checked |
| `poo status` | Every followed pool |
| `poo claim -seed HEX -serial N` | Claims a whole mint for the key's 1-of-1 address |
| `poo pay -seed HEX -serial N -to ADDR -amount A` | Pays from the key's address, change back to it |
| `poo build -pool K [-work N]` | Builds an entry from the pending transactions, optionally adding a layer |
| `poo work -pool K [-entry HASH] [-start S] [-tries N]` | Adds the strongest layer found (default target: the selected tip) |
| `poo payment -serial N -tid HASH` | Where a transaction stands, and the weight behind it |
| `poo holdings -serial N` | Unspent portions of a mint |
| `poo policy list\|use\|archive\|unarchive\|delete [-root DIR] [NAME]` | Manages node policies (flags before NAME) |
| `poo wallet COMMAND` | The wallet; see below |
| `poo version` | Prints the version |
| `poo keygen [-seed hex]` | A new Ed25519 key and its 1-of-1 address |
| `poo series example` | The example series parameters as JSON |
| `poo series info FILE` | Validates a series file and prints its id and pools |
| `poo sim load\|partition\|attack\|sizing [flags]` | Runs a scenario of the network simulator and prints JSON; `-seed` and `-runs` pick the seeds, other flags shape the network and the attack ([sim.md](sim.md), [../simulation.md](../simulation.md)) |
| `poo demo [-quiet]` | Four in-process nodes: claims, payments, a rival double spend that wins, a carried payment, and the honest ledger restored |

Node and client commands choose a data directory with `-dir`, or with
`-root` (default `$POO_HOME` or `~/.poo`) and `-policy` (default: the current
node policy, else `default`). Client commands take `-series ID` when the
node follows more than one series.

## Wallet commands

The wallet file is `-wallet FILE`, default `wallet.json` in `-root`. The
passphrase is read from `$POO_PASSPHRASE`, `-passphrase-file FILE`, or a
line on standard input (which is echoed: prefer the file or the
variable in scripts). The series comes from the node policy, so commands
that do not need the node (`create`, `seed`, `address`, `addresses`,
`watch`, `request`, `policy`, `cosign`, `follow`) work with the node
stopped. Flags go before positional arguments.

| Command | Does |
| --- | --- |
| `create [-restore HEX] [-gap N]` | Makes a wallet with a new random seed, or restores one from its seed |
| `seed` | Prints the seed: everything needed to restore the wallet |
| `passwd [-new-passphrase-file FILE]` | Seals the seed under a new passphrase (`$POO_NEW_PASSPHRASE`) |
| `address [-label TEXT]` | Gives out a fresh address, with its public key for co-signers |
| `addresses` | Every address the wallet watches: derived keys (the gap included) and shared policies |
| `watch [-serial N]... [-pool K]...` | Adds mints to look for portions in |
| `follow` | Rewrites the node policy to follow only the pools of the watched mints (light mode); restart the node to apply |
| `balance` | Spendable units per watched mint |
| `claim -serial N` | Claims a whole mint for a fresh address |
| `pay -to ADDR -amount A [-mint N]... [-memo TEXT] [-out FILE]` | Pays an address; with a shared policy, writes the payment for co-signers to `-out` |
| `pay -request FILE [-out FILE]` | Checks a payment request and pays it |
| `request -amount A [-memo TEXT] [-out FILE]` | Writes a payment request to a fresh address |
| `receive (-address ADDR \| -request FILE) [-serial N -tid HASH]... [-threshold W]` | What has arrived for a request, and whether it is settled |
| `status [-threshold W]` | Every payment sent: each leg's state, from(e), rivals, and settled |
| `resend` | Sends dropped legs again |
| `policy -name NAME -k K -key self\|ALG:HEX\|HEX... [-label TEXT]` | Records a shared k-of-n policy and prints its address |
| `cosign FILE` | Adds this wallet's signatures to a payment file, in place |
| `submit FILE` | Sends a payment whose signatures are complete |

## Code

`main.go` dispatches through `run(args, out)` so tests can call it
directly; `node.go` holds the daemon, client and policy commands;
`wallet.go` holds the wallet commands; `sim.go` the simulator command; `demo.go` holds the demo. `node_test.go` runs a daemon in-process and drives
it only through the commands, across a restart; `TestTwoNodes` runs two
daemons where the second lists the first as a peer.
