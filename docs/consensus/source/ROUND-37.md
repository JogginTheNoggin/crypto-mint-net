# Source Round 37: consensus review (Black, Part D)

## Summary

This round finishes the bounty market's first phase. Part A (D198) does
five things:
- names an offer by its bytes (W-27);
- keeps later fields in `bounties.json` (W-28);
- writes the rule for later bounty changes into wire.md (W-29);
- moves finished offers to a done file (ORANGE-046);
- makes the bounty settings settable limits (GUI-GREEN-062).

It also adopts both of Black's Round 36 recommendations. Blue's D199 fixes
RED-115 to RED-117 and carries Green's GREEN-062.

**Result: 9 clear, 1 with a condition, 0 blocked.**

- **No consensus rule or depth limit is added.**
  - The engine, the ledger rules and byte form v1 are untouched.
  - `TestByteFormV1Frozen` passes on `main` and on PR #317, and the
    Round 24 and Round 25 cross-version tests give the same bytes on
    both.
- **Black's Round 36 recommendations are built as asked.**
  - Every claimed layer at or above the series' floor now goes to
    `HandleLayerFrom(k, l, engine.Pushed, p.id)`, whatever the market
    makes of it, unless the pool holds it already. That is the ordinary
    pushed-layer path with every check, so no rule is added.
  - Only a layer under the series' floor is an `InvalidItem`. A valid
    layer under the offer's target is answered "too weak" with no
    offence.
- **D198's question about budgets.** The claim path is a lane of its own,
  not part of the push budget. Claims count against the discovery budget
  (120 a minute by default, shared with pings), not the layer budget
  (6,000 a minute). So a peer can bring at most about 2% more layers
  through claims, and each one is checked in full. This is no consensus
  issue. One optional change for Grey is below.
- **The condition (D199, RED-115).** An offer over 1,024 bytes is now
  malformed. That costs the sender an `InvalidMessage` flag and the
  connection, and two such flags time the peer out. But the bound isn't
  in wire.md, and W-29 tells later releases to add trailing fields to an
  offer at bounty version 1. A later release whose offers grow past
  1,024 bytes would then lose its peerings with every older node.
  - That is the case Round 33's term 4 exists to prevent: it costs
    information, never a verdict, but it can keep a node from hearing a
    heavier ledger.
  - Write the bound into wire.md as frozen with bounty version 1, or end
    an over-size offer's connection with no offence. Either one meets the
    condition.

| Field | Value |
| --- | --- |
| Round | Source Round 37 |
| Consensus | None (D198, D199) |
| Other consensus suggestions | None |
| Base reviewed | `main` @ `7eb16ab` (PR #311: D198; GUI #315 on top) |
| Previous base | `main` @ `d25f777` |
| Blue branches | PR #317 @ `39e7cb2` (BLUE-038, D199: RED-115 to RED-117; carries GREEN-062 #314) |
| Red | PR #312 @ `1ad1671` (RED-115 to RED-117) |
| Proposals | ORANGE-047 (PR #313 @ `47ccf43`); GREEN-062 (PR #314 @ `eeba881`, carried in #317) |
| Result | 9 clear, 1 with a condition, 0 blocked |

## Claimed layers go to the engine (D198)

`handleBountyClaim` now does this:
1. It works out the claim's strength with the series' hash, and the
   series' floor (`MinLayerStrength`).
2. It asks `holdsLayer` whether the pool holds that layer already. This
   is a read under `Engine.Ledger`.
3. It asks `Book.Claim` for an outcome and sends the ack.
4. If the strength is at least the floor and the pool doesn't hold the
   layer, it calls `HandleLayerFrom(k, l, engine.Pushed, p.id)`.
5. Only a strength under the floor returns `invalidItem`.

What this means for consensus:
- **No rule is added.** `HandleLayerFrom` with `Pushed` is the path every
  pushed layer takes: the seen list, `AddLayer` with the series' floor and
  the entry held, then weight, selection and forwarding. A layer that
  fails a rule fails there, as any layer does. The market's floor check
  only decides whether the layer is offered to the engine. It is the
  same floor `AddLayer` applies, so it can't keep out a layer the ledger
  would take.
- **Valid weight is kept.** A layer the market won't pay for (too weak,
  filled, closed, no lease, a repeat) is still weight for the ledger, and
  now reaches it. A layer the engine refuses, for example because the
  pool doesn't hold the entry, is logged at debug level with no offence.
  That is lack of information at most.
- **No valid layer is an offence.** A layer at or above the floor is never
  an `InvalidItem`, whatever the offer asked. A layer under the floor
  breaks a rule every node applies, so the flag fits.
- **The Round 36 wording note is resolved.** D198 now says every claimed
  layer at or above the floor goes to the engine "unless the pool holds
  it", which is what the code does.

### Budgets: the claim path is a separate lane

D198 asks Black to confirm "that the claim path can carry no more layers
than the ordinary push path's budget allows". Strictly, it doesn't, but
that doesn't matter for consensus:

- `budgetKind` puts all bounty types (70 to 76) under
  `tracking.KindDiscovery`, with a default of 120 a minute. Pings and the
  other discovery messages share it. A pushed layer counts against
  `KindLayer`, 6,000 a minute.
- So a peer can bring up to about 120 more layers a minute through claims
  than through pushes. Each claim must name an offer that this node
  posted and still holds, so a claim can only add layers to entries this
  node asked for. Every layer that gets through is checked in full.
- A claimed layer also isn't added to the pool's cost record
  (`tracking.Cost`), as a pushed layer is.
- **Optional, for Grey.** To put claimed layers strictly inside the push
  budget, spend one `KindLayer` unit before `HandleLayerFrom`. A claim
  over that budget would still get its ack, and only the hand-off to the
  engine would be skipped. That is node policy, either way.

## The condition: the offer's size bound (D199, RED-115)

- **What changed.** `MaxOfferBytes` is 1,024. `DecodeOffer` refuses a
  longer body, and a list item has the same bound. `handleBounty` wraps
  the error as `wire.ErrMalformed`, so `misbehave` gives an
  `InvalidMessage` flag (severity 5, kept 24 hours) and ends the
  connection. Two flags reach `TimeOut` (10). Black checked the bound on
  `39e7cb2`: an offer is 96 bytes today, an offer padded to 1,024 bytes is
  taken, and one at 1,025 bytes is refused as "offer: 1025 bytes, over
  1024".
- **Why it needs a condition.** wire.md and W-29 say a field a later
  release adds after `salt` stays at bounty version 1, and older decoders
  ignore trailing bytes. Neither says the offer must stay within 1,024
  bytes. A later release that follows W-29 and adds about 930 bytes, for
  example a post-quantum signature for phase 2's signed offers, would
  send offers every older node calls malformed. Each such offer would
  cost that release a flag and its connection to every older node that
  takes part in the market.
- **Why that matters to Black.** It decides no verdict, so it isn't a hard
  fork under the owner's definition. But the connection it ends also
  carries ledger data. Round 32's terms say nothing may stop a node
  hearing about a heavier ledger because of what a peer's release adds,
  and Round 33's term 4 says a compliant later build must never trip an
  offence. Round 34 cleared the remaining `InvalidMessage` cases only
  because "those parts of the layout never change". The offer's size can
  change under W-29 as written.
- **What meets it.** Either of these, before the tag:
  1. Write the bound into wire.md as part of bounty version 1, frozen with
     it: the row for type 70 and the list in type 72 say "at most 1,024
     bytes, trailing fields included", and W-29 says a trailing field
     must keep the offer within it, while a larger offer needs bounty
     version 2. This is a few lines of documentation, and
     `TestBlue038OfferBytesBounded` already tests the bound.
  2. Or treat an over-size offer as `ErrBadExtras` does a bad extension
     list: drop it and end the connection with no offence. Better still,
     skip that one offer and keep the connection.

  Black recommends the first option. It costs no code, and it makes the
  room for later fields as clear as the hello's limits.

## Market state, files and settings (D198, D199)

- **Offers named by their bytes (W-27).** The id is SHA-256 of the bytes
  received, and a node passes on those bytes. That is market naming only.
  No entry, layer or transaction carries an offer or its id.
- **Later fields and the bounty version rule (W-28, W-29).**
  `bounties.json` keeps unknown fields within `keepjson`'s limits. The
  W-29 rule matches Round 32 and Round 33's terms: an optional extra
  between peers that both list the feature, with a new version sent only
  to peers that list it, and a new outcome read as "not paid".
- **The done file and saves (ORANGE-046 P1 to P4).** These are a node's
  own files and how often it writes them. An offer found in both files
  is left out, so a payment isn't owed twice. That is a market choice
  with no effect on any ledger. `TestOrange046ScanMatchesTheMap` checks
  that the `keepjson` scan writes the same bytes as before.
- **Settable bounty limits (GUI-GREEN-062, GREEN-062).** These are node
  policy, applied on reload. A `node.json` with a bounty amount over
  2^53 - 1 is now refused. That is the owner's own file and is White's
  call before the tag. It can't change which ledger is valid.
- **Per-address bounds and the done file (RED-116, RED-117).** `hostOf`
  (the IPv4 address or IPv6 /64) is read only by `Book.ReceiveFrom` and
  `Book.TakeFrom`. Nothing in the peer book, the tracker or the chase
  uses it. The done file is read in bounded pieces. These are memory
  limits on market state.
- **`poon status` and the CLI (W-30, GREEN-062).** Display and messages.

## Clear

| Item | Why it is clear |
| --- | --- |
| D198 (W-27): an offer named by the SHA-256 of its bytes, passed on as received | Market naming. Nothing in the ledger carries an offer |
| D198 (W-28, W-29): `bounties.json` keeps later fields; the bounty version rule in wire.md | A node's own file. The rule follows Round 32 and Round 33's terms |
| D198 (W-30): `bounty` in `poon status` | Display |
| D198 (ORANGE-046 P1 to P4): done file, timed lease saves, per-peer held counts, `keepjson` scan | Market files and memory. The `keepjson` output is byte for byte the same |
| D198: claimed layers at or above the floor go to the engine; only a layer under the floor is an `InvalidItem` (Black's Round 36 recommendations) | The ordinary pushed-layer path with every check. No valid layer is an offence. A separate budget lane, at most about 2% of the push budget |
| D198 (GUI-GREEN-062): settable bounty limits; amounts up to 2^53 - 1 | Node policy. A refused `node.json` is the owner's file |
| D199 (RED-116, RED-117): held offers and leases per address and /64; done file read in bounded pieces | Market memory. The address key is read only by the market |
| GREEN-062 (carried in #317): on and off in `poon policy limits -set`, `ErrNoTake`, notes | CLI and messages |
| Red (PR #312): RED-115 to RED-117; ORANGE-047 (PR #313) proposals 1 to 3 | Findings, fixed in #317. The proposals change how a node reads its own files, not verdicts |

## With a condition

| Item | Condition |
| --- | --- |
| D199 (RED-115): `MaxOfferBytes` 1,024, over it malformed (`InvalidMessage` and the connection ended) | Before the tag: write the 1,024-byte bound into wire.md as frozen with bounty version 1, with W-29 saying trailing fields must fit within it. Or end an over-size offer's connection with no offence (or skip the offer). Otherwise a later release that follows W-29 could be timed out by every older node |

## Items for the owner

- **Nothing needs your approval this round.**
- **A layer a grinder hands in is now always kept when it is valid,**
  even when it isn't paid, and a valid layer never counts against the
  grinder. The market still can't change which ledger any node accepts.
- **One small condition for Grey:** the new size limit on bounty offers
  should be written into the protocol document before the first tag, so
  that a later release can't trip it and lose its peers.

How this was checked:
- Read D198 and D199, `p2p/bounty.go` (`handleBountyClaim`, `handleBounty`,
  `hostOf`, `holdsLayer`), `p2p/peer.go` (`budgetKind`, `dispatch`,
  `misbehave`), `tracking.DefaultConfig`, `ledger.Pool.HasLayer`,
  `bounty.DecodeOffer` and `DecodeList`, docs/protocol/wire.md, and the
  diffs of PRs #311, #312, #313, #314 and #317.
- Black's check of the offer bound on `39e7cb2`: 1,024 bytes taken, 1,025
  refused as malformed.
- The Round 24 and Round 25 cross-version tests and `TestByteFormV1Frozen`:
  identical on `7eb16ab` and `39e7cb2`.
- `go test ./...` passes on `7eb16ab`, #312, #313, #314 and #317. The
  bounty, p2p, keepjson, cli, nodepolicy and rpc packages pass under
  `-race` on `39e7cb2`.
