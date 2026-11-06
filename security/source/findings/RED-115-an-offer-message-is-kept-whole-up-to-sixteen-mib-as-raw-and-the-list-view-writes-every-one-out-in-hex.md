# RED-115: An offer message is kept whole, up to 16 MiB, as `Raw`, and the list view writes every one out in hex

| Field | Value |
| --- | --- |
| Severity | Medium |
| Status | fixed |
| Found | Source Round 37 Part B, on `main` @ `bf90ffd` (D198, W-27) |
| Area | `pkg/bounty/bounty.go` (`DecodeOffer`, `Offer.Raw`), `pkg/bounty/book.go` (`Receive`, `HeldOffers`), `pkg/p2p/bounty.go` (`handleBounty`), `pkg/rpc/bounty.go` (`GET /v1/bounty`) |
| Fixed by | `claude/project-thread-ggo8dt` @ HASH (BLUE-038) |
| For Black | Not a rule: the offer is not part of any ledger. |

## What is wrong

W-27 names an offer by the bytes as received, so `DecodeOffer` now keeps the
whole body in `Offer.Raw` ("fields a later release added after Salt"). Nothing
bounds it. A list reply takes each offer from a 1,024-byte `VarBytes`, but a
single `bounty-offer` message is decoded from its whole body, which a peer may
make `wire.MaxFrame` (16 MiB). `Receive` checks the fields only (`Check`,
`MinPrice`) and keeps the offer with its `Raw`.

So each offer held can cost up to 16 MiB. The held list is 256 offers by
default (4,096 at most) and 64 from one peer, so one peer can make the node hold
64 × 16 MiB = 1 GiB, and several peers up to 4 GiB (from the counts, not run).
The bytes are then written out each time the market is listed: `Offer.Raw` is
JSON `raw`, as hex, in `GET /v1/bounty` (what `poo bounty list` and a GUI view
read), so the reply is twice the held bytes and is built in memory in one piece.

Offers are not passed on, so the node's memory is the only cost.

## How I checked it

`pkg/p2p` test (throwaway, removed; copy kept with Red's notes): a node with
the market on (`take`), a raw peer that lists `bounty`, 12 offers, each the
real offer plus 8 MiB of trailing bytes, with a different salt so each is its
own offer:

```
sent 12 offers of 8 MiB each; held 12; heap +96 MiB
the list view (json of HeldOffers): 192 MiB in 4.7s
```

Nothing refused them, and the message budget (120 a minute, discovery) allows
more than the 64 a peer may hold. I did not run 16 MiB offers or the full
256.

## Suggested fix (Blue's call)

- Bound an offer's bytes: refuse (or ignore) a body over a few KiB, which
  is room for many later fields (the list reply already says 1,024).
- Do not put `Raw` in the JSON of a held offer; show its length and id. Keep
  it in memory only for an offer this node passes on or hashes, or keep just
  the hash.
- Optionally count the held bytes, not only the offers, and drop the oldest over a
  bound.

## Not tested

16 MiB offers; the 256-offer and 64-per-peer totals; the same through the real
RPC and `poo bounty list`.

## Resolution

Fixed in BLUE-038 (D199). An offer is at most 1,024 bytes (`MaxOfferBytes`), alone or in a list, so a held offer's `raw` is at most 1 KiB, or 2 KiB as hex. `raw` stays in the JSON: `bounties.json` names an offer by its bytes (W-27). Test: `TestBlue038OfferBytesBounded`.
