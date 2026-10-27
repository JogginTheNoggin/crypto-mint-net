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
| **Wallet seed** | The 32 bytes every key of a wallet is derived from; sealed in the wallet file with the passphrase ([D63](decisions.md#d63-keys-from-one-seed-with-a-gap-limit), [D64](decisions.md#d64-the-seed-is-sealed-with-pbkdf2-and-aes-gcm)). |
| **Gap limit** | How many unused derived addresses a wallet watches past the last one it gave out, so a restored wallet finds its payments. |
| **Leg** | One transaction of a payment: the part paid from one mint. A payment across mints has one leg per mint, each settling under its own pool's weight. |
| **Leg state** | `unsigned`, `pending`, `selected`, `dropped` (not held, inputs unspent: can be resent) or `void` (another transaction spent an input) ([D66](decisions.md#d66-leg-states-dropped-and-void)). |
| **Rival** | An entry on an inactive branch holding a transaction that spends an input of a payment. Its branch's weight after the fork is what from(e) must stay ahead of ([D67](decisions.md#d67-rivals-and-settled)). |
| **Payment request** | What a receiver hands a payer: series, fresh address, amount, memo and the policy behind the address, so the payer can check it ([D68](decisions.md#d68-payment-requests-carry-the-policy)). |
| **Simulator** | `pkg/sim`: real engines over a simulated network in virtual time, driven by one seed ([D71](decisions.md#d71-the-simulator-runs-real-engines-over-a-modelled-network)). |
| **Sibling share** | The share of layer credit made on entries that end outside the selected ledger: work lost to competing entries. |
| **Light mode** | A full node whose node policy follows only the pools of a wallet's watched mints ([D70](decisions.md#d70-light-mode-is-a-full-node-following-less)). |
| **Peer category** | How a node treats a connection for one mint policy: **priority** (relied on: asked, forwarded to, pinged often), **backup** (kept ready), **candidate** (not yet verified, or no slot free) or **unlisted** (none of these, such as a wallet). Messages are handled in that order ([D58](decisions.md#d58-slots-replace-the-peer-count)). |
| **Slot** | A place for a priority or backup peer of one mint policy; node policy sets how many. |
| **Review** | The regular comparison of peers' points that moves them between categories. |
| **Points** | A peer's score for one mint policy from what it was seen to do ([D61](decisions.md#d61-points-and-fair-comparisons)). **Fair points** use only the measures both categories being compared have. |
| **Flag** | A record of an offence by an address, with a severity and an expiry. The live total decides loss of priority, timeouts and blocks. |
| **Timeout / block** | Refusing an address for a while: a timeout keeps its categories for its return; a block does not, and can cover a network range. |
| **Tracking template** | What a node records about a peer: basic, standard, detailed or custom. |
| **Chase** | Following up a stated weight above the node's own: confirm, find the common entry, check the work, fetch the entries. |
| **Evidence** | How far a chase group has shown its branch is real: word-only, corroborated or work-checked. The chase queue orders by evidence, never by stated weight. |
| **Provisional layers** | Layers fetched during a chase to check a branch's work before any entry; never kept or counted. |
| **Known-bad list** | Entry hashes shown to be false, ignored for a while. |
