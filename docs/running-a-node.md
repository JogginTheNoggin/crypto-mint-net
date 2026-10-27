# Running a node

## Summary

How to run the full node today: write a node policy, start the daemon, and
use the local commands to claim a mint, pay, build an entry, add work and
check a payment, then restart and see everything come back from disk. The
node does not talk to peers yet (milestone 3), so this is a single node.
Keys made with `poo keygen` are raw seeds printed on screen; use them only
for testing, never for anything of value.

## 1. Build

```sh
go build -o poo ./cmd/poo
```

## 2. Write a node policy

```sh
./poo init -example
```

This writes `~/.poo/policies/default/node.json`, which knows and follows
the worked example's series (8 mints of 100 units, pool size 4) and makes
it the current node policy. Set `POO_HOME` to keep everything somewhere
else. To follow your own series, write its parameters as JSON (start from
`./poo series example`) and pass `-series FILE`. Edit `node.json` to follow
only some pools or mints, or to change K and the other limits; see
[modules/nodepolicy.md](modules/nodepolicy.md).

## 3. Start the node

```sh
./poo node
```

It prints how many pools it follows and what it replayed, and listens on
`127.0.0.1:8645`. Leave it running and use another terminal. Stop it with
Ctrl-C; it compacts its stores and exits.

## 4. Claim, pay and settle

```sh
./poo keygen > alice.json          # note "seed"
./poo keygen > bob.json            # note "address_sha256"
./poo claim -seed ALICE_SEED -serial 5
./poo build -pool 1 -work 100000   # an entry, then a layer that selects it
./poo pay -seed ALICE_SEED -serial 5 -to BOB_ADDRESS -amount 30
./poo build -pool 1 -work 100000
./poo payment -serial 5 -tid TID_FROM_PAY
./poo holdings -serial 5
./poo status
```

Mint 5 is in pool 1 (pools hold serials 0 to 3 and 4 to 7). A new entry
has no weight until a layer lands on it, so it is held but not selected;
`-work` adds one right away. `payment` reports `from`, the weight a rival
would have to beat to reverse the payment. That is the number a receiver
compares with its threshold.

## 5. Restart

Stop the node and start it again. `poo status` shows the same tips and
weights, and the replay line reports how many records it read back. Every
record is checked again on the way in; a file damaged by a crash loses at
most its last record.

## Node policies

```sh
./poo init -policy shop -series shop-series.json
./poo policy list
./poo policy use default        # picks up where it left off
./poo policy archive shop       # kept apart, with its data
./poo policy use shop           # an archived policy starts from scratch
./poo policy unarchive shop     # or take it out by hand and pick up
./poo policy delete shop        # gone for good
```

Stop the node before archiving or deleting the policy it uses; a running
node holds a lock on its directory.

## Security notes

- The RPC listens on loopback only. Every call needs the token in
  `rpc.token` in the data directory, readable only by your user. Anyone who
  can read that file controls the node.
- `poo claim` and `poo pay` take a raw seed on the command line, which your
  shell may save in its history. They are developer tools until the wallet
  (milestone 6).
