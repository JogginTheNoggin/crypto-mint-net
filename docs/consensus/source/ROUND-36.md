# Source Round 36: consensus review (Black, Part D)

## Summary

This round adds the bounty market, phase 1 (D196, jerry's plan). A node
can pay its direct peers to grind layers on its entries. The flow is:
1. it posts offers;
2. peers take leases;
3. grinders send claims;
4. the payer pays with an ordinary wallet payment.

D195 makes payment and wallet files keep what a later release adds
(W-23, W-24, `pkg/keepjson`). Blue's D197 fixes Red's RED-109 to RED-114
in the market, and carries Green's GREEN-061.

**Result: 10 clear, 0 with conditions, 0 blocked.** Black also makes two
non-blocking recommendations for Grey.

- **No consensus rule or depth limit is added.**
  - A bounty layer is an ordinary layer. An accepted claim reaches the
    ledger only through `Node.HandleLayerFrom(k, layer, engine.Pushed,
    peer)`, the same path as any pushed layer, with every check.
  - The payment is an ordinary transaction.
  - `TestByteFormV1Frozen` passes on `main` and on PR #304, and the
    Round 24 and Round 25 cross-version tests give the same bytes on
    both.
- **Nothing in the market changes which ledger a node accepts.**
  - The engine is untouched.
  - The market reads the pool only through `Has` and `HasLayer`.
  - The offer's strength target decides payment, never validity. An
    offer can't ask for less than the series' layer floor.
- **Wire and features follow the Round 32 and Round 33 terms.**
  - The feature `bounty` only switches on optional messages between two
    peers that both list it.
  - Message types 70 to 76 are new types, which older nodes and nodes
    with the market off ignore.
  - Nothing ranks, refuses or stops chasing a peer for its features.
- **Two recommendations.** These are about information and peer standing,
  never about verdicts:
  1. Hand every claimed layer that meets the series' floor to the
     engine, whatever the market outcome.
  2. Don't count a claim that is valid but weaker than its offer as an
     `InvalidItem`.

  Both are below.

| Field | Value |
| --- | --- |
| Round | Source Round 36 |
| Consensus | None (D195 to D197) |
| Other consensus suggestions | None |
| Base reviewed | `main` @ `d25f777` (PR #297: D195, D196; GUI #298 on top) |
| Previous base | `main` @ `58273f7` |
| Blue branches | PR #304 @ `ecc4cac` (BLUE-037, D197: RED-109 to RED-114; carries GREEN-061 #299) |
| Red | PR #300 @ `357a58a` (RED-109 to RED-114) |
| Proposals | ORANGE-046 (PR #301 @ `5509093`); GREEN-061 (PR #299 @ `383b16b`, carried in #304) |
| Result | 10 clear, 0 with conditions, 0 blocked |

## The bounty market and the ledger (D196, D197)

### A claimed layer takes the ordinary path

In `handleBountyClaim`, the payer:
1. works out the claim's strength with the series' hash, for the market
   only;
2. asks `Book.Claim` for an outcome;
3. sends the ack.

Only on `Accepted` does it build `layer.Layer{Entry, Nonce}` and call
`HandleLayerFrom(k, l, engine.Pushed, p.id)`.

- **Every check runs.** That is the path a pushed layer takes:
  - the seen list;
  - `AddLayer`, with the series' floor and the entry held;
  - weight and selection;
  - forwarding.

  A claim that would fail any rule fails there, as any layer does.
- **The market can't make a layer valid.** The offer's target (`least
  strength`) is at least the series' layer floor, which `post` checks.
  Strength is a fact of the bytes, the same on every node, so the payer
  and the grinder agree on it.
- **Payment is outside the ledger.** `poo bounty pay` makes an ordinary
  wallet payment, and the node tells the grinder its tids. Nothing about
  a bounty is in an entry, a layer or a transaction's rules.
- **The market reads, never writes.** `holdsEntry` and `holdsLayer`
  (RED-110, GREEN-061) read `ledger.Pool.Has` and `HasLayer` under
  `Engine.Ledger`.

### Wire and features

- **New types.** Messages 70 to 76 go through `dispatch` with the
  discovery budget. A node with the market off, or one that doesn't know
  them, ignores them. That is the rule older builds already keep for
  unknown types.
- **The feature `bounty`.** It is listed in the hello only when the
  market is on, and bounty messages go only to a peer that lists it. That
  is an optional extra between two peers that both list it (Round 33,
  term 2). Nothing in the chase, the peer book or the tracker reads it.

### Recommendations, not conditions

Neither of these touches a verdict. Under the owner's definition,
dropping a valid layer or penalising a peer is lack of information, not
a hard fork. Both still go against the principle that a node should keep
what is valid and not punish what is valid.

1. **Keep every valid claimed layer.** Today only an `Accepted` claim
   reaches the engine. A claimed layer that meets the series' floor but
   has a market outcome of too weak, filled, closed, reserved, no lease,
   mismatch or repeat is dropped. That layer is valid weight for the
   ledger, and the market only decides whether it is paid. Black
   suggests handing every claimed layer with strength at least the
   series' floor to `HandleLayerFrom`, whatever the outcome. A known
   layer can be skipped, since the pool already holds it.
2. **Don't flag a valid layer as invalid.** A claim below the offer's
   target returns `invalidItem`. That is a `tracking.InvalidItem` flag
   (severity 5, kept 24 hours), and two of them time the peer out. But
   `InvalidItem` is defined as an entry that breaks a rule or a layer
   below the floor, and a too-weak claim above the floor is neither.
   Black suggests a market offence of its own, or the overuse budget,
   rather than `InvalidItem`, so a buggy grinder can't get itself timed
   out on every connection for a valid layer.

**Wording note for D197.** It says "a known layer still goes to the
engine like any layer". In the code a `Known` claim isn't handed to the
engine. That costs nothing, since the pool holds it already, but the text
should say "is not passed on, since the pool holds it".

## Files keep what a later release adds (D195)

`pkg/keepjson` keeps unknown keys in payment and wallet files, within the
same limits as node.json (256 keys, 64 KiB, depth 64). A payment file now
carries `kind` and `format`.
- These are tool files. A payment's transaction bytes are untouched, so
  `poo sign` still signs the tid every node computes.
- A newer format is refused with a message.

## Clear

| Item | Why it is clear |
| --- | --- |
| D195 (W-23, W-24): payment file kind and format; unknown keys kept in payment and wallet files | Tool files. Transaction bytes and tids are unchanged |
| D196: bounty offers, leases and claims; `bounty` node policy, off by default | Market state outside the ledger. Payment decides nothing about validity |
| D196: an accepted claim goes to `HandleLayerFrom` as `Pushed` | The ordinary layer path with every check. The offer's target is at least the series' floor |
| D196: wire types 70 to 76; the feature `bounty` | New types that older nodes ignore. An optional extra between peers that both list it (Round 33 terms) |
| D196: `poo bounty pay` | An ordinary wallet payment |
| D197 (RED-109, RED-110): random start nonce; a claim needs a lease; a known layer not owed (`HasLayer`, a read) | Market rules. The pool is only read |
| D197 (RED-111, RED-112, RED-113): payer fixed to the first sender; leases per address; bounded acks and records | Market and memory limits |
| D197 (RED-114): `bounties.json` re-checked at start | A node's own file |
| GREEN-061 (carried in #304); ORANGE-046 proposals 1 to 3 | An offer goes to the pool that holds its entry. Orange's proposals change market memory and work, not verdicts |
| Red (PR #300): RED-109 to RED-114 | Findings, fixed in #304. None touches a rule |

## Items for the owner

- **Nothing needs your approval this round.**
- **The bounty market can't fork anything.** A paid layer is an ordinary
  layer, checked by the same rules as every other. A payment is an
  ordinary payment. No market setting changes which ledger any node
  accepts.
- **Two small suggestions for Grey:** keep every valid layer a grinder
  hands in, even when it isn't paid, and don't treat a valid but too-weak
  layer as an offence.

How this was checked:
- Read D195 to D197, `p2p/bounty.go` (`handleBountyClaim`, `holdsEntry`,
  `holdsLayer`, the post check against `MinLayerStrength`), `pkg/bounty`
  (`Book.Claim`, `TakeFrom`, `Receive`, `Open`), `tracking` (the
  `InvalidItem` rule), `pkg/keepjson`, `wallet.Unsigned`, and the diffs
  of PRs #299, #300, #301 and #304.
- The Round 24 and Round 25 cross-version tests and `TestByteFormV1Frozen`:
  identical on `d25f777` and `ecc4cac`.
- `go test ./...` passes on `d25f777`, #299, #300, #301 and #304. The
  bounty, p2p, wallet, keepjson and cli packages pass under `-race` on
  `ecc4cac`.
