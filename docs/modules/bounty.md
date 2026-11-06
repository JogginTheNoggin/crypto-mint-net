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
3. **Work.** `poo bounty work ID -to ADDR` measures this node's rate,
   asks the payer for a lease, grinds until it finds a layer, the lease
   ends or `-max-seconds` (at most 240) passes, and claims the layer to
   be paid to `ADDR`.
4. **Pay.** `poo bounty list` on the payer shows each accepted claim.
   `poo bounty pay ID [-claim NONCE]` pays it from the wallet and tells
   the grinder the payment's tids.
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
offer's strength is accepted, ends the lease, and goes to the payer's
engine as a layer from that peer. A claim below it is refused (too weak)
and counts as an invalid item against the peer. Other outcomes: filled,
closed, reserved (leased to another grinder) and repeat.

## Settings

| node.json `bounty` | Default | Range |
| --- | --- | --- |
| `post`, `take` | off | |
| `max_open` | 16 | 0 to 256 |
| `max_amount`, `budget` | none (posting needs both) | |
| `max_leases_per_grinder` | 4 | 0 to 64 |
| `lease_floor_seconds`, `lease_ceiling_seconds` | 60, 3600 | 10 to 86400 |
| `held_offers` | 256 | 0 to 4096 |
| `min_price` | 0 | |

## Phase 1 limits

- A watch-only wallet cannot pay a bounty yet.
- No auto-pay: the payer runs `poo bounty pay`.
- Offers go only to direct peers (passing them on is phase 3) and are
  not signed (phase 2).
- Offers from peers live in memory and are gone after a restart.
