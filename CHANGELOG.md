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
- **Grind jobs, refined (D112, D113):** a `tx:TID` target grinds for one
  payment until it is safe; a job can carry a lease, and `poo grind run`
  keeps one, so a job whose follower dies stops; a hangup stops a `poo`
  command as an interrupt does (but reloads `poo node`); a device that
  cannot grind is refused at start; progress comes at most once a second
  with a steadier rate; one `start` per job, then a `plan` per pool;
  waiting jobs say what would help; job-wide events reach watchers of
  their series. `poo grind schedule` (list, add, remove, set) and
  `/v1/grind/schedule` change the jobs `node.json` starts, applied at once
  with no reload. A native BLAKE3 grinder keeps chaining values (5 times
  faster entry tries), and `-o table` spreads nested objects into columns.
- **For front ends:** `poo doctor -o json|table` and a `fix_args` per
  check, and `cli.ErrorCode` and `cli.SecretOutput` so the GUI handles
  errors and secret output the way `poo` does (D110).
- **Library:** the commands are package `pkg/cli`, so the GUI runs the
  same code as `poo` (D84).
- **Fixed:** a node catching up a heavier ledger one entry at a time could
  prune each entry before its work arrived and never switch (D79).
- **A heavier ledger is always reachable (D115 to D117):** no node policy
  can stop a node reaching a heavier valid ledger. A chase finds a split
  at any depth by halving, takes in any run of entries without layers,
  and yields and resumes past its time slice instead of giving up; only
  hashes shown false are bad-listed. Entries and transactions have no
  size or count cap: an entry larger than a frame travels in parts (new
  wire types: parts request, large, part), and the store splits large
  records. `p2p.depth_limit` and `fast_catch_up` are ignored.
- **Grinding (D118, D119):** `grind.layer_floor` keeps every layer above a
  floor (off by default; several times the credit for the same work);
  SIMD lane grinders in a `GOEXPERIMENT=simd` build; jobs stop at once
  and report progress each second; `poo grind schedule remove -target`
  guards against a stale row number.
- **Held branches and many layers (D121, D122):** a node that holds a
  heavier branch's entries without their layers now switches to it; layers
  added deep in a shared ledger are fetched; an entry with any number of
  layers is counted (wire types 27, 52, 53); budgets count only what they
  let through. Witnesses of large transactions are checked on every core,
  events no longer walk every branch, and `grind.layer_rate` bounds what
  `layer_floor` sends (`poo grind schedule set -layer-rate`).
- **Long chases always go on (D124 to D126):** a chase paces its requests
  under the sender's budget and keeps its progress through a timeout;
  paging one entry's layers yields each page and checks their order; step
  4 moves on only past what it counted; a chase waiting for disk space
  shows in `poo chases`, `poo doctor` and the metrics. Grind schedule
  changes are made on the node (`POST /v1/grind/schedule/edit`, `If-Match`
  on `PUT`), so two at once both apply. Large items are encoded once, and
  large-entry parts live in the data directory; `grind.layer_rate` is at most 50
  a second, so a node's own layers stay inside peers' default budget
  (D127).
