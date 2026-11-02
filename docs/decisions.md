# Decisions

## Summary

This is the record of every design decision the code makes where the design
documents leave a choice open, and every place the code departs from them.
Each entry says what was decided, why, and which tier it belongs to
(consensus, series or node policy). The biggest choices: Go with only the
standard library (D1); a fixed binary byte form instead of the JSON used in
the documents' examples (D2); domain-separated hashes for every object
(D3, D4); arbitrary-precision weights (D10); and provisional node-policy
defaults (D12) that milestone 7's simulator is meant to replace with
measured values. Where the documents disagree with each other, the code
follows the consolidated main document (D18). Milestone 2 adds storage and
the node daemon: a per-pool log that is replayed through the engine so
nothing on disk is trusted (D21), synced writes by default (D22),
compaction that cannot flip a tie (D23), accepting any issuer until a
project issuer exists (D24), the save, archive and delete operations for
node policies (D25), and a local RPC limited to loopback with a token
(D26).

Add a new entry for every new decision. Never renumber; mark a replaced
entry "Superseded by Dn".

## Entries

### D1. Go, standard library only

- **Decision:** The node is written in Go 1.24 and imports nothing outside
  the standard library.
- **Why:** Go builds a single static binary for every platform, has
  first-class concurrency and networking for later milestones, and ships
  SHA-256 and Ed25519. No third-party code means nothing to audit or pin in
  a consensus implementation.
- **Tier:** Implementation.

### D2. Binary byte form v1

- **Decision:** Every hashed or transmitted object has one canonical binary
  form: big-endian fixed-width integers, variable-length bytes as a U32
  length then the bytes, lists as a U32 count then the items, booleans as
  one byte 0 or 1. Decoders refuse trailing bytes and non-canonical values.
  The exact forms are in [protocol/encoding.md](protocol/encoding.md).
- **Why:** The documents show objects as JSON for readability, but hashing
  needs exactly one byte string per object. JSON has many encodings of the
  same value.
- **Tier:** Consensus.

### D3. Domain tags on every hash

- **Decision:** Each kind of hash starts with an ASCII tag: `PoO-address`,
  `PoO-tx`, `PoO-entry`, `PoO-pool`, `PoO-series`, `PoO-witness-v1`,
  `PoO-issuer-v1`. The tid is `H("PoO-tx" ∥ body)`. Layers have no tag
  (D6).
- **Why:** The documents tag addresses, entries and pools. Tagging the tid
  and series_id too means no byte string can be read as two kinds of
  object.
- **Tier:** Consensus.

### D4. series_id

- **Decision:** `series_id = H("PoO-series" ∥ encode(series params))`, with
  the hash named in the params. The issuer witness is not part of it.
- **Why:** The id must commit to every rule. Leaving the issuer witness out
  lets the issuer sign the id itself.
- **Tier:** Consensus.

### D5. Entry nonce only when a minimum entry strength is set

- **Decision:** The entry hash includes an 8-byte nonce only when the
  series' minimum entry strength is above 0. The wire form always carries a
  nonce field, which must be 0 when the series has none.
- **Why:** This is the main document's rule. A fixed wire form keeps
  decoding independent of the series.
- **Tier:** Consensus.

### D6. Layer hash

- **Decision:** `layer hash = H(entry hash ∥ nonce)` with an 8-byte
  big-endian nonce and no tag.
- **Why:** A layer is the smallest object in the system and the one hashed
  most often. The entry hash inside it is already tagged.
- **Tier:** Consensus.

### D7. Threshold policy rules

- **Decision:** Policy type 1 (threshold) lists 1 to 16 distinct keys and a
  K with 1 ≤ K ≤ n. Its witness data carries exactly K signatures with
  strictly increasing key indices. Key algorithm 1 is Ed25519. Type and
  algorithm numbers are never reused.
- **Why:** Exactly K and increasing indices give each valid witness one
  form, so witnesses cannot be padded or reordered. The key bound declares
  the type's verification cost, as the design requires of every type.
- **Tier:** Consensus (the numbers); series (which ones are allowed).

### D8. What the issuer witness signs

- **Decision:** The issuer's witness signs the message for
  `H("PoO-issuer-v1" ∥ series_id)`, built the same way as a transaction's
  witness message.
- **Why:** Reusing the witness machinery means any policy type can be an
  issuer, and the tag keeps an issuer signature from being reusable as a
  payment signature.
- **Tier:** Series.

### D9. Witness message

- **Decision:** A witness signs `"PoO-witness-v1" ∥ tid`.
- **Why:** The tag stops a signature over a tid from being valid in any
  other context, and the version lets a later witness form coexist.
- **Tier:** Consensus.

### D10. Arbitrary-precision weight

- **Decision:** Credit and weight use `math/big`. A layer of strength s
  credits 2^s, and strength can reach 256.
- **Why:** Lucky layers count in full, so a uint64 could overflow. Weights
  are compared exactly; floating point is never used.
- **Tier:** Consensus.

### D11. Selection and ties

- **Decision:** A node switches only to a strictly heavier valid ledger. At
  equal weight it keeps the one selected first; among new candidates of
  equal weight, the one seen first wins. There is no depth limit on
  switching.
- **Why:** These are the design's rules. "Seen first" is decided by an
  arrival counter, not a clock, so tests are deterministic.
- **Tier:** Consensus (strictly heavier); node policy (which equal ledger).

### D12. Pruning and provisional defaults

- **Decision:** Each pool keeps the selected ledger, the top K other
  branches by weight (ties broken by arrival), and up to Ties branches whose
  weight equals the selected weight. Defaults: K = 8, Ties = 64, at most
  10,000 pending transactions per pool, a seen list of 65,536 items.
- **Why:** The design leaves K to node policy. The Ties slots fix a problem
  found in testing: with K = 0, a fresh entry on the tip with no credit yet
  would be pruned at once and could never gain the layers to be selected.
  All numbers are provisional until the simulator (M7) measures them.
- **Tier:** Node policy.

### D13. Pending transactions that depend on pending ones

- **Decision:** Normally a transaction spending an output that is only
  pending is refused. Transactions carried back after a switch may depend
  on each other; they wait in the pool and become ready once what they
  spend is in the selected ledger. A carried transaction evicts pending
  transactions that conflict with it.
- **Why:** The forwarding document refuses dependents; the main document
  says carried transactions come back one entry per link. Both hold this
  way: no new dependency chains, but the history that was already agreed is
  not lost. Carried transactions were selected before, so they take
  precedence over newer pending ones.
- **Tier:** Node policy.

### D14. Sorting displaced transactions

- **Decision:** After a switch, each transaction of the old segment is
  checked, in its old order, against the new state plus the carried ones
  before it. It is unchanged if the new segment contains it, carried if it
  is still valid, and void otherwise.
- **Why:** This reproduces the design's worked example exactly (Appendix
  A), including chains of carried payments.
- **Tier:** Consensus (void); node policy (carrying is a choice to retry).

### D15. Requested data bypasses the seen list

- **Decision:** Items a node asked for are processed even if their hash is
  in the seen list, and are never forwarded.
- **Why:** A node may have pruned an entry it saw and later need it back to
  follow a heavier ledger.
- **Tier:** Node policy.

### D16. Empty entries are valid

- **Decision:** An entry may hold no transactions.
- **Why:** The design does not forbid it, and an empty entry on the tip
  gives builders something to add layers to when nothing is pending.
- **Tier:** Consensus.

### D17. Fenwick tree and relative weights

- **Decision:** The ledger keeps prefix weights of the selected chain in a
  Fenwick tree and stores off-chain weights relative to their fork point.
- **Why:** A layer on an old entry adds weight to every ledger above it.
  Doing that naively costs O(chain length) per layer.
- **Tier:** Implementation.

### D18. Disagreements between design documents

- **Decision:** Where the documents disagree, the code follows the
  consolidated main document. Instances: the entry nonce (D5), the order of
  hashes inside an entry (by serial, then tid), and one claim witness per
  output.
- **Why:** The main document is the latest and most complete statement of
  the design.
- **Tier:** Consensus.

### D19. Witness size

- **Decision:** The series' witness ceiling limits the encoded size of each
  witness (policy plus data, with length prefixes).
- **Why:** That is what a node stores and sends, so it is the cost the
  ceiling is meant to bound.
- **Tier:** Series.

### D20. Following a mint follows its pool

- **Decision:** Asking the engine to follow one mint makes it follow the
  whole pool of that mint.
- **Why:** All mints of a pool share one ledger. Checking an entry needs
  every transaction in it, whichever mint it belongs to.
- **Tier:** Node policy.

### D21. The store is a log replayed through the engine

- **Decision:** Each followed pool has an append-only log of the entries,
  layers and pending transactions the engine kept, in their canonical byte
  forms, framed with a length and a CRC-32C. On start-up the node replays
  the log through the engine as `Stored` items, so every rule is checked
  again. A torn or corrupt tail is cut off and reported.
- **Why:** Nothing read from disk is trusted, so a damaged or edited file
  can lose data but never make the node accept something invalid. Replaying
  in the original order rebuilds the same tree, selected ledger and pruning
  as before the restart. One file per pool matches the design: a pool is
  the unit a node follows, holds and asks for.
- **Tier:** Node policy (storage is local).

### D22. Write durability

- **Decision:** By default every append is synced to disk before the call
  returns. `no_sync` in the node policy skips that.
- **Why:** Safe by default. What a power loss can lose with `no_sync` is
  only what peers can send again, except a wallet's own pending
  transactions, which the wallet still has. Milestone 7 benchmarks will show
  whether the default costs too much.
- **Tier:** Node policy.

### D23. Compaction keeps the selected ledger first

- **Decision:** A log is rewritten, through a temporary file and a rename,
  when it holds at least 1024 records and more than twice what the node
  still needs. The rewrite lists the selected ledger from the pool_id up,
  then every other held entry by first sighting, each followed by its
  layers, then the ready pending transactions.
- **Why:** Pruned branches and transactions now in entries would otherwise
  grow the log without bound. Writing the selected ledger first means a
  branch of exactly equal weight cannot take its place on replay, so a
  restart never changes which side of a tie the node holds.
- **Tier:** Node policy.

### D24. Accept any issuer by default

- **Decision:** A new node policy accepts every series it lists
  (`issuers.any`), including series that name no issuer. A listed issuer
  witness must still be valid. `issuers.accept` restricts acceptance to
  named issuers, each with a valid witness.
- **Why:** The design expects the project's own issuer to be the default.
  There is no project issuer yet, and a node only knows the series written
  into its node policy, so accepting those is the useful default. Revisit
  when a project issuer exists.
- **Tier:** Node policy.

### D25. Saving, archiving and deleting node policies

- **Decision:** Node policies live under one root directory
  (`$POO_HOME`, default `~/.poo`). A saved policy is
  `policies/<name>/node.json` with its data beside it, so using it again
  picks up where it left off. Archiving moves the directory, data included,
  to `archive/<name>`. `poo policy use` on an archived policy starts from
  scratch (its data is deleted and its settings are saved again).
  `poo policy unarchive` takes it out by hand and picks up where it left
  off (`-fresh` drops the data). Delete removes it for good. A running node
  holds a lock on its directory, so it cannot be archived or deleted.
- **Why:** These are the design's expected meanings. What archiving is for
  is still open in the design; keeping the data costs disk but keeps both
  choices available.
- **Tier:** Node policy.

### D26. Local RPC: loopback and a token

- **Decision:** The node's control interface is JSON over HTTP, listening on
  `127.0.0.1:8645` by default. A non-loopback address is refused unless the
  node policy sets `allow_remote`. Every request needs the bearer token in
  `rpc.token` (32 random bytes, file mode 0600). Bodies are capped at 8 MiB
  and a work request at 2^24 nonces.
- **Why:** The interface controls the node (it submits, builds and works),
  so it should be reachable only by the node's owner. HTTP and JSON need
  nothing outside the standard library and are easy to use from any
  language. It is not the peer protocol, which is milestone 3.
- **Tier:** Node policy.

### D27. The Stored origin

- **Decision:** The engine has a fourth origin, `Stored`, for items read
  back from the node's own store. Like `Requested`, they skip the seen list
  and are never forwarded.
- **Why:** On start-up the seen list is empty anyway, but a stored entry may
  have been pruned and pushed again later; it must be taken in. Forwarding
  everything on every restart would flood peers with old data.
- **Tier:** Node policy.

### D28. What is not stored

- **Decision:** The seen list and first-seen times are not stored; after a
  restart, first-seen times are the replay time. Compaction keeps the ready
  pending transactions but not carried transactions still waiting on
  another. (Without compaction, replay reproduces them, since it repeats
  the switch that carried them.)
- **Why:** None of them changes which ledger is selected. First-seen times
  are local and selection never reads them. Waiting transactions are rare
  (a switch that displaced a chain of payments) and can be sent again.
- **Tier:** Node policy.

### D29. No expiry yet

- **Decision:** Entries and layers are kept while they are in the selected
  ledger or a top K branch; nothing expires by age.
- **Why:** The design makes expiry node policy and leaves defaults open.
  K already bounds what a node holds per pool. Expiry needs the entry-age
  questions the design lists as open; it can be added as a node policy
  setting once they are settled.
- **Tier:** Node policy.
