# Changelog

## Unreleased (0.1.0)

The first release: a full node for Proof of Ownership, with its wallet and
simulator. See [docs/roadmap.md](docs/roadmap.md) for what each milestone
built.

- **Consensus core:** series, pools, threshold policies,
  transactions, entries, layers and credit, pool state, the per-pool ledger
  tree with selection, switching and top-K pruning, and the pending pool.
- **Node:** an append-only, checksummed store replayed on start-up, node
  policies, and a local RPC on loopback with a token.
- **Network:** a framed binary wire protocol, forwarding, catch-up, peer
  discovery and relaying, address registries, peer categories, ranking,
  tracking and chasing.
- **Wallet:** keys from one sealed seed, claims, payments with coin
  selection, payment requests, co-signing and light mode.
- **Simulator:** many real engines over a modelled network, with load,
  partition, attack and sizing scenarios (`poo sim`).
- **Hardening:** fuzzing of every decoder, benchmarks, a security review
  ([docs/security.md](docs/security.md)), a separate cap on inbound
  connections, byte form v1 and wire version 1 declared stable, a
  supported Go toolchain, and reproducible release builds.
- **GUI:** `poo-gui`, a local web app over the same `poo` commands
  (`pkg/cli`), with a terminal that is `poo` itself, a job list, and a
  locked-down loopback server ([docs/gui.md](docs/gui.md)). `poo-gui
  -view-only` runs every command in poo's view-only mode, and the GUI
  masks a passphrase or seed exactly when poo reads one (D89).
- **Cryptographic choice:** a series can use SHA-256, SHA-512/256,
  SHA3-256, BLAKE2b-256 or BLAKE3; allow Ed25519, ECDSA (P-256, P-384,
  P-521, secp256k1), BIP-340 Schnorr and ML-DSA-44/65/87 keys; and allow
  hashlock and nested k-of-policies addresses
  ([docs/cryptography.md](docs/cryptography.md)). Wallets choose their key
  algorithm (`poo wallet create -alg`); `poo crypto`, `poo keygen -alg
  -hash` and `poo series new` are new. Requires Go 1.27.
- **Viewing and watching:** one filter (series, pool, mint, address,
  address policy, transaction, event kind) for views of pools, ledger
  pages, pending transactions, unspent portions, a transaction and an
  address, and for a live event stream (`poo pools`, `ledger`, `pending`,
  `unspent`, `tx`, `address`, `watch`; `GET /v1/watch` as NDJSON).
- **Live following:** `poo follow` and `poo unfollow` (and
  `POST /v1/follow`) change what a running node follows and apply it at
  once by reloading the node in place; `poo reload` and SIGHUP do the
  same after a hand edit. `poo wallet follow` applies at once too.
- **Wallet:** new wallets seal the seed with Argon2id (D95); older
  PBKDF2 wallets still open, and `poo wallet passwd` upgrades them.
- **Read-only use:** `rpc.view-token` reads and nothing else (D100), and
  `poo -view-only` / `POO_VIEW_ONLY=1` runs only the commands that read
  (D101). `poo help`, typo suggestions, `poo doctor`, `-o table`, shell
  completion (`poo completion`) and `GET /v1/metrics` are new. A
  passphrase typed at a terminal is no longer echoed.
- **Logs and diagnostics:** levelled, structured node logs (`poo node
  -log-level -log-format`, plain, text or JSON; D104), and `poo version -v`
  with the Go version, platform, commit and dependencies.
- **Performance:** signatures are verified once per witness and before
  the node's lock, items share fsyncs, and branch ranking and hashing
  are cheaper (D105): several times the transactions a second, with no
  rule change.
- **Safer operation:** a reload onto a broken `node.json` is refused and
  the node keeps running (D106); `poo wallet create -restore -` reads
  the seed hidden.
- **Grinding:** `pkg/grind` detects the CPU (and its hashing features)
  and GPUs, picks a grinder for the series' hash on a chosen device or
  auto, and runs jobs in the node on the tip, pending transactions or an
  entry, by hand, within hours or from `node.json` (D109). `poo grind`
  (devices, bench, start, run, jobs, stop), `/v1/grind`, watch kind
  `grind`, and `-device`, `-threads`, `-for` on `poo build` and `poo work`
  are new; grinding uses every core but one, with the same result.
- **For front ends:** `poo doctor -o json|table` and a `fix_args` per
  check, and `cli.ErrorCode` and `cli.SecretOutput` so the GUI handles
  errors and secret output the way `poo` does (D110).
- **Library:** the commands are package `pkg/cli`, so the GUI runs the
  same code as `poo` (D84).
- **Fixed:** a node catching up a heavier ledger one entry at a time could
  prune each entry before its work arrived and never switch (D79).
