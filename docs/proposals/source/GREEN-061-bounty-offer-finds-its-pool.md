# GREEN-061: A bounty offer goes to the pool that holds its entry

| Field | Value |
| --- | --- |
| Status | proposed (prototyped) |
| Based on | `main` @ `6b56136` |
| Area | `pkg/p2p` (`PostBounty` checks the pool holds the entry; `BountyPools`), `pkg/rpc` (`find_pool` in `/v1/bounty/post`), `pkg/cli` (`poo bounty post` with no `-pool`), docs (running-a-node, modules/bounty, modules/poo) |
| Effort | small |
| Consensus | none: posting is node policy. What a node accepts into a ledger does not change, and a bounty layer still enters only through `HandleLayerFrom` |
| Decided by | Grey; Black to confirm no rule changed; White may note the local RPC field below |

## Summary

BASE.md asked Green to try the bounty steps in docs/running-a-node.md
as a new user. Green ran two nodes on one machine, turned the market on
in each node.json as the docs show, and followed the commands as
written. The market works: the grinder found the offer, leased it,
ground a 28-bit layer in about 18 s and was accepted. One step went
wrong without any warning, and the docs skipped one step.

1. **The offer went to the wrong pool, and it was still paid for.** The
   docs' command is `poo bounty post -entry ENTRY -strength 28 ...` with
   no `-pool`. The flag defaults to 0, so the offer was made for pool 0
   even though the entry (the one `poo build -pool 1` had just printed)
   is in pool 1. Nothing checked that the pool holds the entry. The
   grinder worked the layer and the payer accepted the claim, which now
   counts as owed against `budget`. Then `HandleLayerFrom` dropped the
   layer with `ErrUnknownEntry`, logged only at debug level. So the
   payer paid for a layer no ledger holds. The same thing happens with
   a typo in the entry hash, and with the "tip" of an empty pool, which
   is not an entry.
2. **`poo wallet address` failed on the grinder.** The docs run it
   before the grinder has a wallet, and a new node has none.

## The change

- `Host.PostBounty` refuses an offer whose pool does not hold the
  entry, using the same `ledger.Pool.Has` check that `AddLayerHashed`
  makes before it keeps a layer. If another pool of the series does
  hold the entry, the error names it: "pool 0 of series d0b927f7 does
  not hold entry 229eb6ee, so a layer on it would be paid for and not
  kept; it is in pool 1".
- When neither `-pool` nor `-serial` is given, `poo bounty post` sends
  `find_pool: true`, and the node offers on the followed pool that holds
  the entry. If no pool holds it, the error says where entry hashes come
  from (`poo build`, `poo pools`). If two pools hold it, which needs a
  hash collision, the error asks for `-pool`. A named `-pool` or
  `-serial` is checked, not replaced.
- Docs: running-a-node.md says where `ENTRY` comes from and adds
  `poo wallet create` to the grinder's steps. modules/bounty.md and
  modules/poo.md describe the pool rule.

## How it was tested

- `TestBountyPostFindsTheEntrysPool` (pkg/cli/green061_test.go) fails on
  main ("an offer on pool 1's entry went to pool 0") and passes here. It
  covers no pool named, the wrong `-pool`, the wrong `-serial`, the right
  `-serial`, and an unknown entry.
- `TestBountyMarket`, which names `-pool 1`, passes unchanged.
- By hand, with two nodes: the docs' steps run as written, the old
  wrong-pool post is refused with the pool it is in, and an empty pool's
  tip is refused.
- gofmt, go vet, staticcheck and `go test -race ./...` pass.

## For Black

The check runs only on the payer, when it posts. It reads the pool and
changes nothing. Claims, leases and `HandleLayerFrom` are untouched.

## For White

`/v1/bounty/post` gains an optional `find_pool` field on the local RPC.
An older node ignores it and uses `pool` as before, so a new `poo`
talking to an old node falls back to today's behaviour. No file or wire
format changes.

## Not done (ideas for later)

- The payer can still owe for a layer the engine drops for another
  reason, for example the entry being pruned between post and claim.
  Logging "bounty layer not kept" at warning level, or showing it on the
  claim in `poo bounty list`, would make that visible.
- `budget` is not compared with the wallet's balance. In the docs'
  example a node holding 100 units posted offers worth 4,020.
- `poo bounty list -ask` could show each held offer's expected seconds
  at the grinder's last measured rate, to help a grinder choose.
