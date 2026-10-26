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
| `pkg/vectors`, `pkg/testutil` | [vectors-and-testutil.md](vectors-and-testutil.md) | Tooling |
| `cmd/poo` | [poo.md](poo.md) | Tooling |
