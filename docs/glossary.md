# Glossary

## Summary

The terms used in the code and documents, with their meaning in this
implementation. They follow the design documents; contributors should use
them rather than inventing synonyms. Where a term needed a precise reading
the design left open, the matching decision is linked.

| Term | Meaning |
| --- | --- |
| **Series** | A published, fixed set of rules: number of mints, supply, pool size, hash, minimum strengths, allowed policy types and key algorithms, witness ceiling, optional issuer. Named by its series_id ([D4](decisions.md#d4-series_id)). |
| **Mint** | One unit of a series, named by (series_id, serial). Its supply S is split into portions. |
| **Serial** | A mint's number within its series, 0 to N−1. |
| **Pool** | P consecutive mints that share one ledger. Pool k holds serials kP to (k+1)P−1. |
| **pool_id** | `H("PoO-pool" ∥ series_id ∥ k)`, the root that the first entry of a pool points to. |
| **Portion** | One unspent output: an amount of one mint held by one address. Spent whole. |
| **Claim** | The first transaction of a mint: no inputs, outputs summing to the supply. |
| **Transaction / tid** | A transfer within one mint, spending whole portions and creating new ones of the same total. The tid is the hash of its body, without witnesses. |
| **Policy** | The rule that controls an address, such as "2 of these 3 keys". |
| **Address** | The hash of a policy. |
| **Witness** | The policy plus the data that satisfies it (signatures). Outside the tid. |
| **Entry** | A bundle of transactions of one pool plus the hash of its parent entry. |
| **Entry hash** | `H("PoO-entry" ∥ parent ∥ tids [∥ nonce])`. |
| **Layer** | An entry hash plus a nonce; unsigned work attached to an entry. |
| **Strength** | Leading zero bits of a hash. Always computed, never declared. |
| **Credit** | 2^strength, what one layer (or a counted entry hash) adds. |
| **Weight** | The total credit of a ledger: every layer on every entry in it. |
| **Ledger** | A chain of entries from the pool_id to a tip. |
| **Selected ledger** | The heaviest valid ledger a node holds for a pool ([D11](decisions.md#d11-selection-and-ties)). |
| **Switch** | Changing the selected ledger to a strictly heavier one. |
| **Fork point** | The last entry shared by two ledgers. |
| **Displaced** | Transactions of the old segment after a switch: unchanged, carried or void ([D14](decisions.md#d14-sorting-displaced-transactions)). |
| **Carried** | A displaced transaction that is still valid and goes back to pending. |
| **Void** | A displaced transaction that conflicts with the new ledger. |
| **Transfer layer** | A layer added to an entry already buried under other entries, adding weight to every ledger containing it. |
| **Top K** | The K heaviest branches besides the selected ledger a node keeps ([D12](decisions.md#d12-pruning-and-provisional-defaults)). |
| **Pending pool (mempool)** | Valid transactions not yet in the selected ledger; first come, first served. |
| **Seen list** | Hashes of valid items already handled, so they are not processed or forwarded twice. |
| **Pushed / Requested** | Whether an item arrived unasked (forwarded on if valid) or because the node asked for it (never forwarded) ([D15](decisions.md#d15-requested-data-bypasses-the-seen-list)). |
| **Stated weight** | The weight a peer says its ledger has; a hint for what to request, never trusted for selection. |
| **Consensus / series / node policy** | The three tiers of rules. See [architecture](architecture.md#the-three-tiers). |
| **Issuer** | An optional address named by a series; its witness proves who published the series. |
| **Following** | Which series, pools and mints a node keeps ledgers for. Node policy. |
