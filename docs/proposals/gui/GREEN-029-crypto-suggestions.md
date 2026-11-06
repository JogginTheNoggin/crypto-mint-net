# GREEN-029: Key and hash fields suggest what poon crypto lists

| Field | Value |
| --- | --- |
| Status | adopted (Round 7) |
| Based on | claude/gui-4-watch @ e7df74b |
| Area | gui/web/static/views.js (cryptoChoices; Series and keys, Wallet create) |
| Effort | small |
| Decided by | (Grey) |

## Summary

GREEN-024 did this for grind devices. This does the same for the key
algorithm and hash fields ("Make a key", "New series parameters",
"Create or restore a wallet"): they suggest the names `poon crypto`
prints (sha256 … blake3, ed25519 … ecdsa-p521-sha512). Before, they were
blank text fields with one example as a placeholder, and the wallet's
said "(see Crypto)".

## Idea

- The Series and keys view's `poon crypto` panel fills two datalists,
  `crypto-hashes` and `crypto-algs`, through `onResult`. Fields name the
  list with a new `list` field option.
- A key field focused before that view was ever opened runs `poon crypto`
  once (it reads no node and no files) and fills the lists. That run
  shows in Jobs like any other.
- Suggestions only: any text still goes to `poon`, which decides.
- The comma-list fields (key algorithms, policy types for a new series)
  keep text input, because a datalist suggests one value per field.
  Their placeholders now point at the list above.

## Cost and risk

About 25 lines. No list of names lives in the GUI.

## Prototype

On the branch. `gui/e2e/qol.mjs` checks that the lists hold blake3 and
ed25519 after opening Series and keys. The first-focus path, from the
Wallet view on a fresh page, was checked separately with a one-off
browser script.
