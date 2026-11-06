# pkg/bounty

## Summary

The bounty market (D196, jerry's plan
https://claude.ai/artifact/7LPNtYLPh4Gez7cAcqAxdQ): a node pays peers to
grind layers on its entries. It is node policy only, with no consensus
change: a bounty layer is an ordinary layer and the payment is an
ordinary wallet payment. It is off until node.json's `bounty` section
turns it on.

The package has two parts:

- **Messages** (`bounty.go`): `Offer`, `ListReq`, the offer list,
  `Take`, `Grant`, `Claim` and `Ack`, with their byte forms (wire types
  70 to 76, [docs/protocol/wire.md](../protocol/wire.md)). Decoders
  ignore trailing bytes, so a later release can add fields.
- **The book** (`book.go`): the offers this node posted, with their
  leases and claims, kept in `bounties.json` (kind `poo-bounties`,
  format 1); the offers peers sent, kept in memory; and, by peer, what
  each payer accepted and paid.

`pkg/p2p` (`bounty.go`) sends and answers the messages, `pkg/rpc`
(`bounty.go`) serves `/v1/bounty`, and `poo bounty` runs it.

## The steps

1. **Post.** `poo bounty post -entry HASH -strength S -amount A [-count
   N] [-open SECONDS] [-pool K | -serial N]` makes an offer on an entry
   the node holds. With no `-pool` or `-serial` the node finds the pool
   that holds the entry; a named pool that does not hold it is refused,
   since a layer on it would be paid for and not kept (GREEN-061). The
   strength must be at least the series' layer floor. What all open
   offers and unpaid layers owe must fit `budget`, and one offer must fit
   `max_amount`. The offer goes to every connected peer that lists the
   feature `bounty`.
2. **Find.** `poo bounty list -ask` asks those peers for their open
   offers. Offers paying less than `min_price` per layer are not kept;
   at most `held_offers` are kept.
3. **Work.** A grinder states the strongest offer it will work, as
   `bounty.max_strength` (1 to 255; unset sends 255, any) or
   `-max-strength S` on `poo bounty list -ask` and `poo bounty work`.
   Every list request and lease carries it (a required byte in bounty
   version 1), and a payer lists and leases only offers at or under it,
   so a payer with offers of many strengths gives each to a grinder that
   says it can work it; a fast machine states a higher one. A grinder
   also drops peers' offers above its own setting (D202), so
   `-max-strength` above the setting is refused with the command that
   raises it, `poo bounty list -max-strength S` lists only offers at or
   under S, and working an id the node never kept says which setting
   left it out. `poo grind bench` gives `minute_strength`, the strength
   found in about a minute, to start the setting from (GREEN-064).
   `poo bounty work ID -to ADDR` measures this node's rate,
   asks the payer for a lease, grinds from a random nonce (so two
   grinders, or two calls, find different layers; RED-109) until it finds a layer, the lease
   ends or `-max-seconds` (at most 240) passes, and claims the layer to
   be paid to `ADDR`.
4. **Pay.** `poo bounty list` on the payer shows each accepted claim.
   `poo bounty pay ID [-claim NONCE]` pays it from the wallet and tells
   the grinder the payment's tids. The payment's memo is `bounty `
   followed by the first 16 bytes of the offer's id in hex (32
   characters; RED-121, D203). It looks in the wallet first: a payment
   of that claim the wallet sent (that memo, the claim's address and
   amount, every leg sent, its tids on no other claim) that the node
   never recorded is recorded now, and nothing is sent again
   (`"recorded": true`). A payment sent only in part is refused, to
   settle by hand (RED-119, D201). A payment with a dropped leg is asked
   about again and its dropped legs are sent again, never replaced by a
   new payment: a dropped leg's inputs are unspent and `poo wallet
   resend` would send it later anyway (RED-120, D203). If the node still
   does not hold it, or one of its inputs was spent beside a live leg,
   the command stops for the payer to settle it by hand.
5. **Cancel.** `poo bounty cancel ID` closes an offer; accepted claims
   stay owed.

## Leases

One grinder works an offer slot at a time. A lease lasts about three
times the expected time at the rate the grinder states (`2^strength /
rate` seconds), inside `lease_floor_seconds` and `lease_ceiling_seconds`,
and can be extended once. A grinder starts with one lease and earns one
more for each layer it hands in, up to `max_leases_per_grinder`, while
the expected time of all its leases at its stated rates fits the
ceiling. The payout address is named when the lease is taken, and a
claim naming another address is refused (mismatch).

## Claims

The payer checks a claim with the series' hash. A layer at or above the
offer's strength is accepted and ends the lease. Any claimed layer at or
above the series' floor goes to the payer's engine as a layer from that
peer, whatever the answer, unless the pool holds it already (D198). A
claim below the offer's strength is refused (too weak); only one under
the series' floor counts as an invalid item against the peer. A claim needs the
grinder's lease (else "no lease"), and a layer the payer's pool already
holds is answered "known" and is not owed, so a public layer cannot be
sold (RED-110, D197). Other outcomes: filled, closed, reserved (leased
to another grinder) and repeat.

The leases from one network address (an IPv4 address, or an IPv6 /64)
are limited to `max_leases_per_grinder` in all, whatever node ids it
uses (RED-112, RED-116). The offers held from one address are bounded as
one peer's are (64), and when the held list is full the oldest offer
goes, but not one this node holds a lease on. When the grinder records
are full, the one that earned least goes (RED-116). An offer is at most
1,024 bytes, alone or in a list (RED-115, D199).
An offer received again from another peer keeps the peer that sent it
first as its payer, and only that peer's grants and acks answer this
node's take and claim (RED-111). A held offer keeps its newest 16 acks,
and the book remembers at most 4,096 payers and grinders (RED-113).
`bounties.json` is checked at start as offers and claims are when made:
what fails (a bad offer, more claims than layers, a weak or repeated
claim) is left out with a warning in the log, and what is owed never
wraps (RED-114).

## Settings

Each is a settable limit: `poo policy limits` lists it and `poo policy
limits -set bounty.post=on` sets it (D198); a running node takes a change
on reload. `bounty.post` and `bounty.take` also take `on` and `off`, and
the reply's `notes` name a setting that cannot work as set: posting on
without `max_amount` and `budget`, or `max_amount` over `budget`. A node
that only posts refuses `poo bounty list -ask` and `poo bounty work`,
naming `bounty.take` (GREEN-062).

| node.json `bounty` | Default | Range |
| --- | --- | --- |
| `post`, `take` | off | |
| `max_open` | 16 | 0 to 256 |
| `max_amount`, `budget` | none (posting needs both) | |
| `max_leases_per_grinder` | 4 | 0 to 64 |
| `lease_floor_seconds`, `lease_ceiling_seconds` | 60, 3600 | 10 to 86400 |
| `held_offers` | 256 | 0 to 4096 |
| `min_price` | 0 | |

## Files

`bounties.json` holds the offers that may still be owed, each with its
bytes (`offer.raw`, W-27), and keeps fields a later release wrote (W-28).
An offer that needs nothing more (cancelled, expired or filled, every
claim paid, no lease) moves to `bounties-done.jsonl`, one offer a line,
appended and synced first (ORANGE-046 P1, D198). At start only the
first bytes of each done line (where the id is) are read, so a line of
any length costs little; an offer found in both files is left out with a
warning in the log, as is a count of done lines with no id (RED-117,
D199). A lease is saved on a
short timer; an accepted claim is on disk before its ack is sent.

The done file's format 1 is frozen (W-31, D200): its first line is
`{"kind":"poo-bounties-done","format":1}`, and every later line is one
offer whose JSON begins `{"id":"<64 hex digits>"`, whatever fields follow.
A reader takes the id from that prefix alone; a later format is refused
with a message to run that release or move the file aside.
`bounties.json` records the done file's size when it was written
(`done_size`), so a start reads only what was appended after it, the
only part that can repeat an offer still in `bounties.json` after a crash
(ORANGE-047 P1). An offer that finished while the node was stopped, or by
the clock while it runs, moves to the done file too (GUI-GREEN-063).

`bounties.json` also records the id on the line that ends at `done_size`
(`done_last`). A start reads from `done_size` only when that line is still
there; a done file trimmed, rotated or edited since is read in full, and
the log says so (RED-118, D201). A file written before `done_last` existed
is read in full once. When the done file's last line was cut short by a
stop, the start says so and the next record begins on a new line; a write
that fails part way is taken back. A payment whose save fails is not kept
in memory either, so the node and a restart agree the claim is unpaid,
and `poo bounty pay` then records the wallet's payment (RED-119).

## Finished offers

`poo bounty list -done N` (`/v1/bounty?done=N`) adds `done`: the last N
offers of `bounties-done.jsonl` (at most 256), oldest first, streamed a
line at a time; a line over 1 MiB or with no offer is skipped and counted
in `done_skipped` (GREEN-063).

## What the lists say

Each offer has `state` and, when it has open seconds, `closes`. This
node's offers are open, filled, closed or expired; a filled offer stays
filled once its time runs out (D200). A peer's offer is
open, leased (this node holds a lease), taken (the payer said it is
leased to another grinder or filled), filled or closed. Each ack has
`outcome_name` beside `outcome`. `poo status` has `bounty.post` and
`bounty.take` (W-30), and so does `poo status -counts` (D200). In
`poo policy limits`, `bounty.post` and `bounty.take` carry `"switch":
true`, so a front end can draw them as tick boxes.

## Phase 1 limits

- A watch-only wallet cannot pay a bounty yet.
- No auto-pay: the payer runs `poo bounty pay`.
- Offers go only to direct peers (passing them on is phase 3) and are
  not signed (phase 2). Until then the payer is whoever holds the
  connection with that node id: a peer can claim the id (RED-111 a), so
  work only offers you have reason to trust.
- Offers from peers live in memory and are gone after a restart.
