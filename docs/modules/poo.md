# cmd/poo

## Summary

The command-line tool. Today it creates keys, prints and inspects series,
and runs an in-process demo of four full nodes. Milestone 2 adds the node
daemon (`poo node`) and a local RPC client.

## Commands

| Command | Does |
| --- | --- |
| `poo version` | Prints the version |
| `poo keygen [-seed hex]` | A new Ed25519 key and its 1-of-1 address |
| `poo series example` | The example series parameters as JSON |
| `poo series info FILE` | Validates a series file and prints its id and pools |
| `poo demo [-quiet]` | Four nodes: claims, payments, a rival double spend that wins, a carried payment, and the honest ledger restored |

## Code

`main.go` dispatches through `run(args, out)` so tests can call it directly;
`demo.go` holds the demo, built only on the public engine API.
