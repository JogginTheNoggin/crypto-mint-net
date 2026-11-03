# Module guides

## Summary

One page per package, in dependency order from the bottom up. Each page says
what the package implements from the design, its main types and calls, the
rules it guarantees, how it is tested, and how to extend it. Read
[../architecture.md](../architecture.md) first for how the packages fit
together. When you change a package, update its page.

| Package | Page | Tier |
| --- | --- | --- |
| `pkg/encoding` | [encoding.md](encoding.md) | Primitive |
| `pkg/hash` | [hash.md](hash.md) | Primitive |
| `pkg/weight` | [weight.md](weight.md) | Primitive |
| `pkg/policy` | [policy.md](policy.md) | Series |
| `pkg/series` | [series.md](series.md) | Series |
| `pkg/tx` | [tx.md](tx.md) | Consensus |
| `pkg/state` | [state.md](state.md) | Consensus |
| `pkg/entry` | [entry.md](entry.md) | Consensus |
| `pkg/layer` | [layer.md](layer.md) | Consensus |
| `pkg/ledger` | [ledger.md](ledger.md) | Consensus |
| `pkg/mempool` | [mempool.md](mempool.md) | Node policy |
| `pkg/engine` | [engine.md](engine.md) | Node |
| `pkg/store` | [store.md](store.md) | Node (storage) |
| `pkg/logging` | [logging.md](logging.md) | Node (logs) |
| `pkg/fsutil` | [fsutil.md](fsutil.md) | Node (storage) |
| `pkg/nodepolicy` | [nodepolicy.md](nodepolicy.md) | Node policy |
| `pkg/node` | [node.md](node.md) | Node |
| `pkg/wire` | [wire.md](wire.md) | Node (network) |
| `pkg/peers` | [peers.md](peers.md) | Node policy (network) |
| `pkg/registry` | [registry.md](registry.md) | Node policy (network) |
| `pkg/tracking` | [tracking.md](tracking.md) | Node policy (network) |
| `pkg/chase` | [chase.md](chase.md) | Node policy (network) |
| `pkg/p2p` | [p2p.md](p2p.md) | Node (network) |
| `pkg/peerport` | [peerport.md](peerport.md) | Node policy (network) |
| `pkg/filter` | [filter.md](filter.md) | Node (viewing) |
| `pkg/watch` | [watch.md](watch.md) | Node (viewing) |
| `pkg/grind` | [grind.md](grind.md) | Node (hash work) |
| `pkg/rpc` | [rpc.md](rpc.md) | Node (local control) |
| `pkg/daemon` | [daemon.md](daemon.md) | Node (running a node) |
| `pkg/wallet` | [wallet.md](wallet.md) | Wallet (holder's habits) |
| `pkg/sim` | [sim.md](sim.md) | Tooling (network simulator) |
| `pkg/vectors`, `pkg/testutil` | [vectors-and-testutil.md](vectors-and-testutil.md) | Tooling |
| `pkg/cli`, `cmd/poo` | [poo.md](poo.md) | Tooling |
