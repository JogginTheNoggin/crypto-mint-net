# Running a node

## Summary

How to run the full node today: write a node policy, start the daemon, and
use the local commands to claim a mint, pay, build an entry, add work and
check a payment, then restart and see everything come back from disk, and
then connect a second node and watch what you do on one reach the other.
Keys made with `poo keygen` are raw seeds printed on screen; use them only
for testing, never for anything of value.

## 1. Build

```sh
go build -o poo ./cmd/poo
```

Optionally, turn on tab completion of commands, subcommands, flags and
policy names. With `poo` on your `PATH`:

```sh
source <(poo completion bash)                          # bash, this shell
poo completion zsh > "${fpath[1]}/_poo"                # zsh
poo completion fish > ~/.config/fish/completions/poo.fish   # fish
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

It prints how many pools it follows and what it replayed, and listens for
peers on port 7733 and for local commands on `127.0.0.1:7732` (`poo
init` wrote both into `node.json`; change them there). Leave it running
and use another terminal. Stop it with Ctrl-C; it compacts its stores and
exits.

## 4. Claim, pay and settle

```sh
./poo keygen > alice.json          # note "seed"
./poo keygen > bob.json            # note "address_sha256"
export POO_SEED=ALICE_SEED        # or -seed-file FILE; never on the command line
./poo claim -serial 5
./poo build -pool 1 -work 100000   # an entry, then a layer that selects it
./poo pay -serial 5 -to BOB_ADDRESS -amount 30
./poo build -pool 1 -work 100000
./poo tx -in-serial 5 TID_FROM_PAY  # where the payment stands
./poo unspent -serial 5
./poo pools
```

`claim` and `pay` read the key's seed from `POO_SEED` or `-seed-file`
(`-` for standard input), never from an argument: other users of the
machine can read a command line, and the shell keeps it in its history
(D98). For everyday spending use `poo wallet`, which keeps the seed sealed.

Mint 5 is in pool 1 (pools hold serials 0 to 3 and 4 to 7). A new entry
has no weight until a layer lands on it, so it is held but not selected;
`-work` adds one right away. `payment` reports `from`, the weight a rival
would have to beat to reverse the payment. That is the number a receiver
compares with its threshold.

## 4b. The same with a wallet

`poo claim` and `poo pay` sign with a raw key. A wallet keeps every key
behind one seed and a passphrase, uses a fresh address for every payment,
and follows each payment's weight:

```sh
export POO_PASSPHRASE='a long passphrase'   # or -passphrase-file FILE
./poo wallet create -wallet alice.wallet     # then: poo wallet seed, and back it up
./poo wallet create -wallet bob.wallet
./poo wallet watch -wallet alice.wallet -serial 5
./poo wallet watch -wallet bob.wallet -pool 1
./poo wallet claim -wallet alice.wallet -serial 4
./poo build -pool 1 -work 100000
./poo wallet request -wallet bob.wallet -amount 30 -memo lunch -out req.json
./poo wallet pay -wallet alice.wallet -request req.json   # note each leg's tid
./poo wallet receive -wallet bob.wallet -request req.json -serial 4 -tid TID
./poo build -pool 1 -work 100000
./poo wallet receive -wallet bob.wallet -request req.json -threshold 50000
./poo wallet status -wallet alice.wallet
```

The request carries the policy behind Bob's address, so Alice's wallet
checks it before paying. `receive` shows each leg as pending, then
selected with its `from` and any known rival; `settled` is true once
every leg is selected, ahead of every rival and at the threshold. A leg
the node no longer holds is `dropped` (send it again with `poo wallet
resend`) or `void` if another transaction spent its input.

A shared policy needs every co-signer's signature:

```sh
./poo wallet address -wallet bob.wallet          # note "public"
./poo wallet policy -wallet alice.wallet -name joint -k 2 -key self -key BOB_PUBLIC
./poo wallet pay -wallet alice.wallet -to JOINT_OR_OTHER_ADDRESS -amount 10 -out pay.json
./poo wallet cosign -wallet bob.wallet pay.json
./poo wallet submit -wallet alice.wallet pay.json
```

For a lighter node, `poo wallet follow` rewrites the node policy to follow
only the pools of the mints the wallet watches, and a running node applies
it at once. See [modules/wallet.md](modules/wallet.md).

## 4c. View, watch and change what the node follows

Every view takes the same filter flags: `-series`, `-pool`, `-serial`,
`-address`, `-address-policy`, `-tid` and, for `watch`, `-kind`. Values
are comma-separated; fields combine with "and" ([modules/filter.md](modules/filter.md)).

```sh
./poo pools                               # every followed pool: tip, height, weight, pending
./poo ledger -pool 1 -limit 10            # the newest ten entries of pool 1, with their transactions
./poo ledger -pool 1 -serial 5            # only entries holding mint 5's transactions
./poo pending -address ADDR               # pending transactions paying or spending ADDR
./poo tx TID                              # where a transaction stands, its rivals and contents
./poo address ADDR                        # balance, holdings, history, pending
./poo unspent -serial 5                   # unspent portions of mint 5
```

To carry a ledger, or part of one, to another node:

```sh
./poo ledger export -pool 1 -from 1 -to 500 -out pool1.json
./poo ledger import pool1.json            # on the other node: every entry is checked in full
```

The file holds each entry's raw bytes and layers, so nothing in it is
trusted; a range imports only from where the other node's ledger already
reaches.

`poo watch` prints events as they happen, one JSON line each, until you
stop it. In another terminal:

```sh
./poo watch -serial 5 -kind tx,selected,reverted
```

then pay from mint 5 and build an entry: the watcher prints the `tx`
line, then `selected` once the entry is built. The event lines are
described in [modules/watch.md](modules/watch.md).

What the node follows can change while it runs:

```sh
./poo follow                              # what node.json follows, and what the node follows now
./poo unfollow -series SERIES_ID -pool 0  # stop following pool 0
./poo follow -series SERIES_ID -pool 0    # follow it again: its store replays from disk
```

The node applies each change at once by reloading its node policy in
place (`poo reload` or SIGHUP does the same after a hand edit of
`node.json`). A running `poo watch` reconnects by itself; events during
the reload, which takes a moment, are not replayed, so look at the views
for anything that happened then. With the node stopped, `poo follow`
edits `node.json` and the node applies it when it starts.

## 4d. Claim a mint someone else claimed

A claim is a transaction like any other, and which claim of a mint
counts is decided by ledger weight, as a double spend is. The pending
pool checks a transaction against the selected ledger, so a second claim
of a mint it has claimed is refused there. To compete, build it on a
branch from before that claim and add weight to the branch (D161):

```sh
./poo claim -seed-file bob.seed -serial 5 -parent before-claim -work 4000
# prints the entry, and next: the command that adds weight to it
./poo grind start ENTRY -serial 5 -for 2h      # or poo work -serial 5 -entry ENTRY
./poo holdings -serial 5                       # bob holds it once the branch outweighs the other
```

`-parent` takes an entry hash, `root` (the pool's first entry) or
`before-claim` (the parent of the selected entry that holds the mint's
claim). `poo build -parent ...` does the same with the pending
transactions that fit on that parent. The branch is kept like any
other: it is selected once it is heavier, everywhere, and while it is
lighter it may be pruned if more than `k` lighter branches are held
(`k: -1` in node.json keeps every branch). Everything after the old
claim that depends on it is not on the new branch.

## 5. Restart

Stop the node and start it again. `poo status` shows the same tips and
weights, and the replay line reports how many records it read back. Every
record is checked again on the way in; a file damaged by a crash loses at
most its last record.

Memory: the node keeps every entry's skeleton in memory and reads entry
bodies back from the pool logs when it serves them, keeping up to
`entry_cache_mib` (256 MiB by default) of them cached. `poo status` shows
the cache under `bodies`; `poo policy limits -set entry_cache_mib=N`
changes it in a running node. Each pool in use keeps its log open, so
`poo doctor` and the node's start-up warn when the open file limit is
below the followed pools plus 256 (raise it with `ulimit -n` or
`LimitNOFILE=`). Entries more than `recent_window` (10,000 by default) below each tip
also leave their transactions and undo records on disk; a deep switch
reads them back, which is slower but never refused.

Each pool also keeps a snapshot beside its log (`<index>.log.snap`),
written at a clean stop, after each compaction and every
`snapshot_every` records (50,000 by default). A start restores it and
replays only newer records, so it no longer re-checks the whole history.
A damaged, stale or foreign snapshot, one written by another release,
or one that does not restore the pool it was written from is ignored and
the log replayed as before; `poo status` and `poo doctor` show how old they are. To check
everything once, start with `poo node -reverify`; to never use them, set
`"no_snapshots": true` in node.json.

## 6. A second node

Give the second node its own policy, its own RPC port, and the first node
as a peer:

```sh
./poo init -policy second -example -listen 127.0.0.1:7742 -p2p off -peer 127.0.0.1:7733
./poo node -policy second            # in a third terminal
./poo peers -policy second           # the first node, connected
```

`init` makes the new policy current, so give the first node's commands
`-policy default` from now on. A listed peer such as `127.0.0.1:7733` is
always dialled, but a node does not take loopback or private addresses
it hears of from other nodes (D96): for several nodes on one machine to
find each other, set `"allow_local_peers": true` in each policy's `p2p`
section. On connecting, the second node asks the first for its whole ledger of each
pool. From then on, whatever either node keeps is forwarded to the other:
claim, pay and build on the first node and `./poo status -policy second`
shows the same tips and weights a moment later. To let the first node dial
the second as well, or to connect nodes on different machines, give each a
`-p2p` address the other can reach and list it with `-peer`, or edit the
`p2p` section of `node.json` (see
[modules/nodepolicy.md](modules/nodepolicy.md)).

Listed peers are only a start. Each node asks its peers what they follow,
asks them for their peer lists, and broadcasts itself to the network, so
nodes that follow the same pools find each other. It keeps what it learns
in `peers.json` in the data directory and dials from there after a restart.
Two `init` flags help on a real network:

- `-advertise host:port` is the address other nodes should dial, such as a
  public address or port forward. Without it the node gives no address, so
  it still finds and dials peers but other nodes are not offered it.
- `-port-map auto` asks your router to forward the peer port, so other
  nodes can reach a node at home with no port forward set by hand. See
  [Behind a router](#behind-a-router-nat) first: it is off by default
  for a reason.
- `-registry URL` (repeatable) names a bootstrap list: a JSON file over
  `https://`, `file://` or a plain path, read on start-up and every hour.
  See [modules/registry.md](modules/registry.md) for the format.

`poo peers` shows the connected peers and the peer book: every address the
node knows, whether it is a priority peer, a backup, a candidate or
archived for each mint policy, and its points. The node relies on its
priority peers, keeps backups ready, and swaps them as their points
change (see [modules/p2p.md](modules/p2p.md)).

The node also records what each peer does in `tracking.json`. A peer that
sends invalid data or floods the node is flagged; enough flags time it out
for 10 minutes or block it for a day. You can look and act yourself:

```sh
./poo tracking                          # every tracked address and its status
./poo tracking 203.0.113.5              # one address: flags, measures, points
./poo peer block -seconds -1 203.0.113.5:7733   # until you unblock it
./poo peer block -range 203.0.113.5:7733        # its whole network range
./poo peer unblock 203.0.113.5:7733
./poo peer template -template detailed 203.0.113.5:7733   # record more
./poo chases                            # heavier ledgers waiting to be checked
```

### Behind a router (NAT)

A node behind a home router, a firewall or a VPN works without any of
this. It dials out to its peers, and outbound connections carry
everything: it follows, catches up and forwards like any other node.
What it lacks is inbound connections: other nodes cannot dial it, so it
is never offered as a contact and adds no reachable node to the network.

There are two ways to take inbound connections:

- **Forward the port by hand.** In the router, forward TCP port 7733 (or
  your `-p2p` port) to this machine. Then set `p2p.advertise`
  (`poo init -advertise`) to the router's public address and that port.
- **Let the node ask the router.** Set `p2p.port_map` to `auto`
  (`poo init -port-map auto`). The node asks the router by PCP, then
  NAT-PMP, then UPnP. It talks only to the gateway, at the default
  route's address on Linux or at `p2p.port_map_gateway` elsewhere. It
  offers the address the router reports only after dialling it and
  reaching itself. It renews the lease (`p2p.port_map_seconds`, an hour
  by default) and removes the mapping when it stops cleanly. A
  `p2p.advertise` you set always wins. On a node set up already, add
  `"port_map": "auto"` to the `p2p` section of `node.json` and run
  `poo reload`.

`poo doctor`'s `port-map` line, and `port_map` in `poo status`, say what
happened:

- **advertised:** it works.
- **unconfirmed:** the router mapped the port, but could not loop a
  connection back to itself. Many routers cannot. If you know the
  address reaches you from outside, set `p2p.advertise` to it.
- **not_public:** the router's own address is private. This is
  carrier-grade NAT or a second router, and no mapping can reach you;
  the node stays outbound only.
- **failed:** the router has PCP, NAT-PMP and UPnP turned off, or no
  gateway was found. `gateway` names the router the node asked, and
  `next_try` when it asks again: the wait starts at a minute and doubles
  to 30 minutes. After you change the router, `poo reload` asks at once.
- **skipped:** `reason` says why: `p2p.advertise` is set, or `p2p.listen`
  is off.

**What it costs in privacy.** An advertised address, by hand or mapped,
is passed from peer to peer:

- Anyone on the network learns your home's public IP address and that a
  Proof of Ownership node runs there.
- The open port can be found by scanners.
- A UPnP mapping that the router grants only as permanent stays in the
  router if the node crashes, until the next clean stop or until you
  remove it in the router.

Leave both off if that matters more to you than taking inbound
connections.

A node inside a private VPN mesh reaches only the peers you list with
`-peer`, as addresses on the mesh. Addresses learned from other nodes
must be public unless `p2p.allow_local_peers` is on. Peer connections
are not encrypted (D30), so use the VPN if the link needs privacy.

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

### Sharing node policies and backups

One file carries node policies to another node or machine:

```sh
./poo policy export -out shop.json                 # the current node policy
./poo policy export -series 1a2b... -out s.json    # one series and how it is followed
./poo policy export -all -full -out backup.json    # every saved policy, this machine's settings too
./poo policy import shop.json                      # shows the plan; changes nothing
./poo policy import shop.json -apply               # merges in what to follow
./poo policy import mine.json -apply -settings     # and the node settings, from a file you trust
./poo policy import backup.json -apply -full       # restores a backup on this machine
```

### Moving settings to another machine

A node's settings are everything in its node policy but what it
follows: its peers, registries, limits, p2p switches, tracking, logs and
grind jobs. `poo settings` writes them to one file and reads them back
(D155). node.json has no key, seed, passphrase or token, and the wallet
is a file of its own. A plain export also leaves out what names you or
opens a door: registry files, registry URLs with a user name, password
or query, and grind jobs on one of your payments or entries (`-full`
keeps them). It still holds your peer addresses and `tracking.peers`,
since a setup is made of them; the export says how many, so check them
before you share the file (D156).

```sh
./poo settings export -out settings.json            # the settings, without this machine's own
./poo settings export -full -out settings.json      # with rpc, listen addresses, grind device and threads
./poo settings import settings.json                 # on the new machine: shows the plan
./poo settings import settings.json -full -map-path /home/old=/home/new
./poo settings import settings.json -apply          # merges them in
```

The preview lists, as "may not fit this machine", anything the file
brings that the new machine may lack:

- a grind device, or more threads than it has;
- a registry file that is not there (`-map-path OLD=NEW` points it at one,
  and a mapped file is taken without `-full`);
- a listen address that is not one of its own;
- an advertised address from the old machine;
- a grind job for a series this node does not list.

A settings import never changes what the node follows. To carry that
too, export the node policy with `poo policy export`.

By default an import takes only what to follow: series, follows and
accepted issuers. Peers, registries, grind jobs, tracking and the p2p
switches and limits are node settings; the plan lists them as skipped,
and `-settings` takes them. With `-settings` the plan names first every
line that turns off a defence or makes the node reach out. Read those
before applying a file from someone else.

An import adds what the node lacks and keeps the node's own value where
both set one; the plan lists each such conflict, and `-prefer-file` takes
the file's instead. `-replace` takes the file's node policy whole (only
what it follows without `-settings`). The RPC and peer listen addresses,
the advertised address, `no_sync`, the grind device and threads, and
registries that are local files stay this machine's unless the file was
exported with `-full` and imported with `-full`. A bundle of several node
policies is planned whole first; if one cannot be imported, none is
changed. A running node applies an import at once and never takes an
`rpc` section from one.

### Adding and sharing peers

`poo peer add ADDR` puts an address in the peer book by hand, as a
candidate the manager may dial; `-label TEXT` names it and `-keep` also
lists it in the node policy's `p2p.peers`. `poo peer remove ADDR` takes it
out of both, `poo peer label ADDR -label TEXT` renames it, and `poo peer
connect ADDR` dials it now. `poo peers` filters the book with `-category`,
`-mint-policy`, `-label` and `-addr`, and `poo tracking -blocked` or
`-timed-out` lists only those addresses (D157).

```sh
./poo peers export -out peers.json                         # the whole book, labels and blocks
./poo peers export -category priority,backup -out good.json
./poo peers export -category blocked -out blocklist.json   # a block list to share
./poo peers import good.json                               # on the other node: shows the plan
./poo peers import good.json -apply                        # adds every address as unlisted
./poo peers import blocklist.json -blocks -apply           # takes the blocks too
```

An imported address always lands unlisted: the node knows it but has not
picked it. When a mint policy is short of peers, the manager promotes the
oldest imported addresses to candidates and dials them before it asks its
peers and registries for more, so a new node with an imported book finds
peers by itself. `poo peer connect -imported` does that at once, and
`p2p.no_dial_imported: true` in node.json turns it off, leaving imported
addresses for you to promote by hand.

An import never removes, unblocks or shortens anything. It takes the
file's blocks only with `-blocks`, and only where they are longer than
what the node holds; it never blocks an address the node policy lists,
and network range blocks need `-ranges` as well. It refuses addresses
blocked here, local addresses unless `p2p.allow_local_peers` is on, and
anything past the book's limits. Labels are yours: an export holds them,
so read the file before you share it.

## Security notes

- The RPC listens on loopback only. Every call needs the token in
  `rpc.token` in the data directory, readable only by your user. Anyone who
  can read that file controls the node.
- The p2p port (7733) listens on every interface by default, because peers
  are on other machines. It accepts only protocol messages, and every item
  is checked before it is kept; nothing it receives can change the node's
  settings. Set `p2p.listen` to `"off"` to accept no connections (the node
  still dials its listed and discovered peers). Connections are not
  encrypted (decision D30).
- What peers say about other peers is only a lead. A node dials an address
  and checks its follows itself before it relies on it, limits how many
  addresses it keeps from one network range, and rate-limits each peer's
  requests (decisions D40 to D49). Set `p2p.no_relay` to stop relaying
  other nodes' discovery requests.
- A peer claiming a heavier ledger is never believed on its word: the node
  checks the work behind the branch before it fetches any entry, and
  switches only on weight it computed (decisions D52 to D54). Nothing the
  tracker records is sent to other nodes.
- `poo claim` and `poo pay` take a raw seed on the command line, which your
  shell may save in its history; use `poo wallet` instead. The wallet file
  holds the seed sealed with your passphrase (decision D64) and is
  readable only by your user, but the seed printed by `poo wallet seed`
  restores every key: keep it offline. Reading the passphrase from
  standard input echoes it; prefer `-passphrase-file` or
  `POO_PASSPHRASE` in a private shell.
