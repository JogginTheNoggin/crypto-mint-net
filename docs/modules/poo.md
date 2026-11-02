# cmd/poo

## Summary

The command-line tool. `poo node` runs the full node daemon with its store,
its peer connections and local RPC; client commands talk to it; `poo policy` manages node
policies; offline commands make keys, inspect series and run an in-process
demo. `claim` and `pay` sign with a single raw key and exist so a node can
be exercised end to end; the wallet comes in milestone 6. See
[../running-a-node.md](../running-a-node.md) for a walk-through.

## Commands

| Command | Does |
| --- | --- |
| `poo init [-example] [-series FILE]... [-listen ADDR] [-p2p ADDR] [-peer ADDR]... [-force]` | Writes a node policy that knows and follows the given series, and makes it current |
| `poo node` | Runs the node until interrupted |
| `poo peers` | The node's id, its p2p address and its connected peers |
| `poo status` | Every followed pool |
| `poo claim -seed HEX -serial N` | Claims a whole mint for the key's 1-of-1 address |
| `poo pay -seed HEX -serial N -to ADDR -amount A` | Pays from the key's address, change back to it |
| `poo build -pool K [-work N]` | Builds an entry from the pending transactions, optionally adding a layer |
| `poo work -pool K [-entry HASH] [-start S] [-tries N]` | Adds the strongest layer found (default target: the selected tip) |
| `poo payment -serial N -tid HASH` | Where a transaction stands, and the weight behind it |
| `poo holdings -serial N` | Unspent portions of a mint |
| `poo policy list\|use\|archive\|unarchive\|delete [-root DIR] [NAME]` | Manages node policies (flags before NAME) |
| `poo version` | Prints the version |
| `poo keygen [-seed hex]` | A new Ed25519 key and its 1-of-1 address |
| `poo series example` | The example series parameters as JSON |
| `poo series info FILE` | Validates a series file and prints its id and pools |
| `poo demo [-quiet]` | Four in-process nodes: claims, payments, a rival double spend that wins, a carried payment, and the honest ledger restored |

Node and client commands choose a data directory with `-dir`, or with
`-root` (default `$POO_HOME` or `~/.poo`) and `-policy` (default: the current
node policy, else `default`). Client commands take `-series ID` when the
node follows more than one series.

## Code

`main.go` dispatches through `run(args, out)` so tests can call it
directly; `node.go` holds the daemon, client and policy commands;
`demo.go` holds the demo. `node_test.go` runs a daemon in-process and drives
it only through the commands, across a restart; `TestTwoNodes` runs two
daemons where the second lists the first as a peer.
