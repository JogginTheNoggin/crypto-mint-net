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
(D26). Milestone 3 connects nodes: a small framed binary protocol over
plain TCP (D30), SHA-256 request ids (D31), a hello with coverage and a
static peer list until discovery exists (D33), a notice after a switch that
forwarding does not show (D36), hash chains to find a split (D35), and
disconnecting misbehaving peers until ranking exists (D38). Milestone 4
finds peers: a connection is a peer only for what the node verified with
its own follow request (D40), broadcasts are marked in the request body
(D41), registries have a provisional format and no default list yet
(D44), and verification is never carried across a restart (D46).
Milestone 5 ranks and tracks peers and chases heavier ledgers: flags and
measures kept by address (D50), bad data costing flags rather than
connections (D55), deleting that does not ban (D57), priority and backup
slots in place of a peer count (D58), regular checks that ask for stated
weight (D59), points with fair comparisons (D61) and provisional defaults
for all of it (D62). Milestone 6 adds the wallet: keys derived from one
seed with a gap limit (D63), the seed sealed with PBKDF2 and AES-GCM
(D64), coin selection that keeps a payment in one mint or pool (D65), leg
states that tell a dropped payment from a void one (D66), rivals and the
settled rule (D67), payment requests that carry the policy (D68),
co-signing through a file (D69), and light mode as a full node following
less (D70). Milestone 7 adds a simulator of real engines over a modelled
network (D71), fixes a pending-pool bug it found (D72), fuzzes every
decoder for a single encoding per value (D73), and keeps the provisional
defaults on the evidence so far (D74). Milestone 8 hardens for release:
inbound connections cannot take the outbound slots (D75), byte form v1 and
wire version 1 are frozen (D76), releases build with a supported, patched
Go toolchain (D77), and release archives are reproducible and published as
drafts the owner approves (D78). Re-running the simulator at larger sizes
found that pruning could drop a heavier ledger while it was being fetched,
fixed by keeping the newest entry (D79). Milestone 9 lets each mint
choose its own cryptography: five hashes (D80), nine signature algorithms
with one way to derive their keys from a seed (D81), hashlock and nested
k-of-policies address types (D82), and the few maintained dependencies
that needs (D83). Round 7 records the owner's rule that no node policy may
make a heavier ledger unacceptable (D115) and removes every limit the
consensus audit found against it: depth, length and budget limits in the
chase (D116) and size and count caps on items (D117). Round 8 closes the
hard fork Black found, a held branch without its layers, and the narrow
cases around it (D121). Round 9 closes the last two Black found: a long
chase now paces itself and keeps its progress through a timeout, and
paging one entry's layers yields and checks their order (D124).

Add a new entry for every new decision. Never renumber; mark a replaced
entry "Superseded by Dn".

## Entries

### D1. Go, standard library only

- **Decision:** The node is written in Go (1.24 at first, see D77 for the
  current version) and imports nothing outside the standard library.
  Superseded in part by D83, which allows a few cryptographic modules.
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

### D30. A framed binary protocol over plain TCP

- **Decision:** Nodes talk over TCP in frames (a `U32` length, at most 16
  MiB) holding a versioned header and a body, using the primitives of the
  byte form (D2). Items inside messages use their byte forms unchanged. The
  connection is not encrypted or authenticated. See
  [protocol/wire.md](protocol/wire.md).
- **Why:** The design fixes what a message carries, not its encoding. A
  binary form like the item forms is compact, exact and easy to
  reimplement. Everything sent is public and every item is checked on
  arrival, so encryption protects only privacy; it can be added under the
  frames later (milestone 8) without changing any message.
- **Tier:** Node policy (the protocol nodes agree on to talk; not
  consensus: it never changes which ledger is selected).

### D31. SHA-256 for request ids

- **Decision:** A request id is SHA-256 of a tag, the type, the section
  and the body, for every series. The hop count is left out.
- **Why:** The id is checked before the node knows whether it follows the
  series, and it only needs to be unique, not to match the series' hash.
  The hop count changes at each relay (milestone 4).
- **Tier:** Node policy.

### D32. The mint policy section

- **Decision:** The section is a depth (0–4) and the issuer, series id,
  pool_id and serial up to that depth, wildcards from the right; a series
  with no issuer uses the zero hash as issuer. Pushed items, notices and
  ledger messages carry their pool section. A node advertises a whole
  series as a series section and otherwise each followed pool, at most 4096
  sections.
- **Why:** This is the design's path written as bytes. A pool is the unit
  that has a ledger, so pool sections are the finest messages need until
  mint-level following is used for filtering.
- **Tier:** Node policy.

### D33. A hello, and listed peers only, until discovery

*Partly superseded by D40: the hello no longer carries an address or
coverage, and peers also come from discovery.*

- **Decision:** Each side sends a hello (protocol, a random node id per
  run, an advertised address, an agent string, its coverage) and nothing
  else until it has the other's, within 10 seconds. A node drops
  connections to itself, to a node it is already connected to (both keep
  the connection dialled by the lower id), with no coverage in common, or
  above its peer limit. Peers come only from the node policy's list and
  from inbound connections; listed peers are redialled with backoff from 2
  seconds to 5 minutes.
- **Why:** The hello is what both sides need before any other message
  makes sense. Discovery, relaying and registries are milestone 4; a static
  list is enough to run and test a network until then.
- **Tier:** Node policy.

### D34. Layers travel as nonces in replies

- **Decision:** A reply with layers sends, for each entry, only the nonces
  of its layers. Pushed layers keep their full byte form.
- **Why:** A layer is an entry hash and a nonce, and in a reply the entry
  hash is already known, so the hash would repeat for every layer. The
  receiver rebuilds each layer and checks it as usual.
- **Tier:** Node policy.

### D35. Finding the split with hash chains

- **Decision:** After "heavier, not from this entry", the node asks for
  the latest hashes, then pages back with the oldest one as anchor, until
  it finds the newest hash it holds (on any branch); if the chain reaches
  the pool's first entry, the split is the pool_id. It does not step back
  one entry at a time. An extension that brings no entries while the peer
  still states more weight means more layers on shared entries, so the node
  then asks for the layers on its latest entries.
- **Why:** The design makes the choice node policy. Hash chains take a few
  requests for a split of any depth and cost little for a shallow one. The
  layers case is not spelled out in the design but follows from it: the
  extension request answers only with entries.
- **Tier:** Node policy.

### D36. When to send a notice

- **Decision:** After a switch, a node sends a notice (its new tip, with
  its new stated weight) to covering peers other than the sender when the
  item that caused the switch is not forwarded (requested data) or the
  switch displaced entries. Notices can be turned off.
- **Why:** Requested data is never forwarded, so without a notice peers
  would learn of the switch only at their next check. A forwarded item
  that extends the ledger already tells them.
- **Tier:** Node policy.

### D37. Provisional p2p defaults

- **Decision:** 32 peers; asker limits 1 MiB chunks, 512 entries, 256
  hashes, 65536 layers per reply; a server sends at most 1024 entries or
  4096 hashes per reply; a regular check every minute; 30 seconds for each
  reply chunk; request ids kept 10 minutes, 2 minutes of clock skew allowed;
  4096 messages queued per peer; port 8646 on every interface. A zero limit
  in a request means the asker sets none and the server applies its cap.
- **Why:** The design leaves all of them to node policy. They keep one
  reply well under the frame limit and keep a slow peer from holding
  memory. Milestone 7's simulator should replace them with measured
  values.
- **Tier:** Node policy.

### D38. Disconnecting misbehaving peers, until ranking exists

Superseded in part by D55 (invalid entries and weak layers are flags).

- **Decision:** A peer is disconnected for a malformed message, a request
  id that does not match, an invalid entry, a layer below the minimum
  strength, a reply that does not fit its request or is out of chain
  order, or a send queue that stays full. A refused transaction is not a
  reason.
- **Why:** The design leaves what to do about bad peers to node policy and
  milestone 5 brings ranking and tracking. Until then, disconnecting is the
  simplest response that keeps a node safe. Transactions can be refused for
  honest reasons (a race between two spends).
- **Tier:** Node policy.

### D39. Every reply is chunked

- **Decision:** Every reply, even one without a body, is sent as chunks
  numbered from 1 with the last flag on the final one. An item larger than
  the chunk size is sent alone in its chunk. Positions must arrive in
  order; a node continues after a more marker with a new request from the
  last whole item, never by position.
- **Why:** One shape for every reply keeps the receiver simple. The design
  cuts chunks only between whole items; an item that does not fit any
  chunk must still be sendable.
- **Tier:** Node policy.

### D40. Peers are verified by follow requests, not by the hello

- **Decision:** The hello carries only the protocol, a node id and an
  agent string. After it, each side sends the other a direct follow
  request for each mint policy it follows; the policies answered with a
  yes or partial yes make the connection a peer for them. A connection
  verified for nothing within the follow timeout is closed. Forwarding,
  ledger requests and catching up use only verified policies.
- **Why:** The design defines a peer as a node this node verified with its
  own follow request or ping, and treats anything a node says about itself
  as a hint. The M3 hello carried coverage that was trusted; that was a
  shortcut until discovery existed. It supersedes the coverage part of
  D33.
- **Tier:** Node policy.

### D41. Broadcast is a flag in the follow request body

- **Decision:** A follow request carries a broadcast flag in its body. Only
  broadcast requests are relayed; a direct one is answered only by the node
  it was sent to. Replies to a broadcast only add candidates; verification
  always comes from a direct request.
- **Why:** The design has broadcast and direct follow requests but does not
  say how a receiver tells them apart. In the body the flag is covered by
  the request id, so no relay can turn a direct request into a broadcast.
  A reply to a broadcast may come from a node the asker has never
  connected to, so it cannot verify it.
- **Tier:** Node policy.

### D42. Hop counts on replies

- **Decision:** A relaying node adds 1 to the hop count of a reply it
  passes back, so a reply's hop count tells the asker how far it came. A
  replier sends a single message without peers whenever the request's hop
  count is above 0.
- **Why:** The design gives replies to relayed requests a different shape
  but puts the hop count only on requests. Counting on the way back costs
  nothing and lets both ends tell a relayed exchange from a direct one.
- **Tier:** Node policy.

### D43. Chunks of a follow reply with peers

- **Decision:** A direct follow reply whose peers do not fit one chunk
  sends the reply (contact, list, more marker, first peers) in chunk 1 and
  only more peers in later chunks. Peer list replies are kept 2 minutes for
  missing chunk requests; follow replies are not kept.
- **Why:** The design says such replies come in chunks but not how they
  split. Keeping peer list replies, the larger ones, is enough for the
  missing chunk request to be useful; how long to keep them is node
  policy.
- **Tier:** Node policy.

### D44. A provisional registry format, and no default registry yet

- **Decision:** A registry is a JSON document (`format`, then `nodes` with
  `addr` and `follows` in the text form of a section), fetched over http or
  https or read from a file, at most 1 MiB. A node has no default registry
  list until a project registry exists.
- **Why:** The design leaves the format, signing and how the default list
  is updated open, and the project has no public site yet (the repository
  stays private until the owner decides). JSON is easy to publish by hand
  or from a script. A signed format can replace it with a new `format`
  number.
- **Tier:** Node policy.

### D45. Network ranges

- **Decision:** A network range is the /16 of an IPv4 address, the /32 of
  an IPv6 address, or the host name of an unresolved address. The book
  keeps at most 16 records and the host at most 4 verified peers per mint
  policy from one range. Each loopback address is its own range.
- **Why:** The design asks for spreading by network range and leaves the
  size to node policy. These are the usual sizes of one provider's block.
  Loopback cannot be an outside attacker's range, and local test networks
  need many nodes on one machine.
- **Tier:** Node policy.

### D46. What the peer book keeps across a restart

- **Decision:** The peer book (`peers.json`) keeps candidates, archived
  nodes, attempt times and the deleted list. A verified policy is a
  candidate again after a restart. An inbound peer's offered contact is
  only ever a candidate; only an address this node dialled itself is
  marked verified in the book.
- **Why:** Verification is this node's own check of a live node; after a
  restart it must be done again. An inbound peer can claim any address,
  including a victim's, so trusting it would let one node point others at
  an address it does not own.
- **Tier:** Node policy.

### D47. Provisional discovery defaults

- **Decision:** 4 verified peers per mint policy, 8 outbound connections,
  the connection manager every 30 seconds, a broadcast and peer list
  request at most every 5 minutes per policy, registries reloaded at most
  hourly, 5 seconds to wait for a follow reply, a hop limit of 8, 120
  discovery requests per peer per minute, a ping every 2 minutes with 3
  silences ending a peering, return paths kept 30 seconds.
- **Why:** The design leaves all of them to node policy. Milestone 7's
  simulator should replace them with measured values.
- **Tier:** Node policy.

### D48. Dropped policies

- **Decision:** A dropped policy update ends the peering for every
  verified policy inside the one named; a wider verified policy that holds
  it stays (a section cannot say "all but this part"), and the next ping
  shows what the peer still follows. A node sends updates through
  `Host.DropPolicy`; since following changes only with a new node policy,
  which needs a restart, a running node does not send them yet.
- **Why:** The design makes the update optional and leaves what peers do
  with it to node policy. Pings already catch a node that stopped
  following something.
- **Tier:** Node policy.

### D49. Answering and relaying discovery requests

- **Decision:** A node answers a follow request or ping for a mint policy
  it follows all of with a yes or pong, one it follows part of with a
  partial yes or partial pong listing its policies inside it, and stays
  silent otherwise. It drops a discovery request whose id does not match,
  that it has seen, that is outside the time window, or over the peer's
  rate. It relays only as a follower of the policy, only to its peers for
  it, and only below its hop limit. Contact details in a follow request or
  reply make the sender a candidate, never a connection.
- **Why:** These are the design's rules, in the cheap-checks-first order it
  gives. Turning contact details into candidates only, dialled later by
  the connection manager with backoff and range limits, is the design's
  protection against aiming nodes at a victim.
- **Tier:** Node policy.

### D50. Tracker keys

- **Decision:** The tracker keeps flags and measures by IP address, so
  one machine is one record whatever port it connects from. A host name
  is its own key, lowercased. Loopback keeps the full address and port,
  unless `LoopbackAsOne` is set.
- **Why:** The design tracks offences by address so a peer cannot shed
  its flags by reconnecting. Ports are free to change; addresses are not.
  Loopback cannot be an outside attacker, and local test networks run
  many nodes on one machine.
- **Tier:** Node policy.

### D51. Windows are node-wide

- **Decision:** The windows for budgets, repeats, activity, chases and
  connections are set once for the node, not per template. A template
  says only what is recorded.
- **Why:** The design lets templates choose what to record. Counting the
  same measure over different windows for different peers would make
  their points incomparable, and ranking compares points.
- **Tier:** Node policy.

### D52. Backoff and the known-bad list live in the chase queue

- **Decision:** A sender's chase backoff and the list of entry hashes
  shown to be false are kept by the chase queue (`pkg/chase`), whatever
  template the sender is tracked with, and are not saved across a
  restart.
- **Why:** They are safety rules, not telemetry: a template that records
  little must not let a sender be chased again at once or a false branch
  be fetched twice. Losing them on restart costs at most one more cheap
  check per sender.
- **Tier:** Node policy.

### D53. Too little work is not an offence

- **Decision:** A chase whose provisional layers do not beat the weight to
  beat ends with the sender backed off and a "nowhere" chase outcome, not
  a flag. Flags are kept for branches that are provably false: a hash
  listed twice, an entry that does not match its listed hash or does not
  link, an invalid entry, a layer below the minimum strength.
- **Why:** Honest peers can be wrong about weight: a ledger may have
  changed between the stated weight and the chase, or layers may be
  pruned. Only rule-breaking proves bad faith. The backoff and the
  points already make a peer that is often wrong cost it.
- **Tier:** Node policy.

### D54. A node holding nothing fetches the whole ledger

- **Decision:** When a node holds no entry of a pool, a chase fetches the
  sender's whole ledger with a full ledger request, without finding a
  common entry or checking work first. When the common entry is beyond
  the depth limit (16 hash chain pages), the chase stops, unless
  `FastCatchUp` is set and the sender is a priority peer, in which case
  its whole ledger is fetched.
- **Why:** With nothing held there is nothing to beat, so any valid
  ledger is progress and the engine still checks every entry. A split
  deeper than the limit is rare and costly to check; fetching a whole
  ledger is the design's fast catch-up, which it leaves to node policy and
  which is only reasonable from a peer already relied on.
- **Tier:** Node policy.

### D55. What each kind of bad data costs

- **Decision:** An invalid pushed entry or layer is an `invalid-item` flag
  (severity 5), and the connection stays. A message that cannot be
  decoded or whose request id does not match is an `invalid-message` flag
  and also closes the connection. The flag limits then time out and block
  the address. A refused transaction is still not a reason. This
  supersedes D38's disconnect for invalid entries and weak layers.
- **Why:** The design moves from disconnecting to flags with limits, so
  one mistake does not cost a peering and repeated ones do. A connection
  that sends undecodable frames cannot be resynchronised, so it closes.
- **Tier:** Node policy.

### D56. Two limits on discovery requests

- **Decision:** Milestone 4's token bucket on follow, peer list and ping
  requests (`RequestRate`) stays, alongside the tracker's discovery
  budget. The bucket drops quietly; the budget flags.
- **Why:** The bucket protects relaying at once, before any tracking, and
  works with tracking turned down to nothing. The budget is what makes
  persistent excess cost the sender.
- **Tier:** Node policy.

### D57. Deleting does not ban

- **Decision:** `Delete` takes a node out of the peer book with no list of
  deleted addresses; it may return as a new candidate. Keeping an address
  out is a block in the tracker, which the book honours (`SetRefuse`): a
  blocked address is not added, dialled or accepted.
- **Why:** The design separates deleting (forgetting) from blocking
  (refusing, with an expiry). One mechanism for refusal, with expiries
  and ranges, is simpler than two.
- **Tier:** Node policy.

### D58. Slots replace the peer count

- **Decision:** `peers_per_policy` is replaced by priority and backup
  slots per mint policy (4 and 4). The connection manager dials while a
  policy has fewer listed peers than its slots. An outbound connection
  verified with no slot free is closed and retried later with backoff; an
  inbound one stays a candidate. An inbound connection verified for none
  of the node's policies stays unlisted, within `unlisted_conns` (8). A
  peer that loses its last policy is closed.
- **Why:** The design's categories are what a node relies on; the count
  of connections follows from them. Dialling a peer and keeping it with
  no use wastes a slot of the remote node's, while inbound connections
  cost this node nothing to keep as candidates. Wallets and light nodes
  are unlisted by nature and must be served.
- **Tier:** Node policy.

### D59. Regular checks ask for stated weight

- **Decision:** The regular check (`check_seconds`) sends a weight request
  to each priority peer of each pool, instead of milestone 3's extension
  request. A heavier stated weight goes to the chase queue.
- **Why:** The design's chasing rules start every catch-up from a stated
  weight and check work before data. A weight request is the cheapest
  way to learn one; the chase decides whether anything is fetched.
- **Tier:** Node policy.

### D60. When a pushed item starts a chase

- **Decision:** A pushed entry or layer whose parent or entry is unknown,
  and a notice, start a chase only through their stated weight, and only
  if it is above this node's own; the item's hash is the group's hash.
  Items naming a hash on the known-bad list are dropped.
- **Why:** An unknown parent is the usual sign of a branch this node does
  not hold, but on its own it is not evidence of weight. Following the
  stated weight through the queue keeps one path for every chase.
- **Tier:** Node policy.

### D61. Points and fair comparisons

- **Decision:** Points are a weighted sum of the recorded measures over
  the activity window (one hour): answered pings, round trip, first
  deliveries, answered requests, reply timing, chase outcomes, minus
  denials and cost (weights in `tracking.DefaultConfig`). A backup and a
  priority peer are compared only on pings, round trip and first
  deliveries, which both categories have. A peer that follows only part
  of a policy loses one point.
- **Why:** The design asks for points from observed behaviour and for
  fair comparisons between categories that are measured differently
  (backups are not asked for data). The weights are a starting point for
  milestone 7's simulator to tune.
- **Tier:** Node policy.

### D62. Provisional peer management defaults

- **Decision:** 4 priority and 4 backup slots per policy, at most 1
  priority and 4 listed peers per network range, at most 2 inbound
  priority peers, a swap margin of 1 point, a review every 10 minutes,
  archiving after 30 minutes unheard, priority barred after 3 wrong
  denials in an hour, backups pinged every 6 minutes. Flags: priority lost
  at 5, a 10-minute timeout at 10, a 24-hour block at 30 or on a fourth
  timeout within 7 days. Budgets per minute: 1200 transactions, 1200
  entries, 6000 layers, 600 requests, 120 discovery messages; 600 repeats;
  30 new stated weights per 10 minutes. Inbox shares 8/4/2/1, 1024 queued
  per category. The chase queue: 4 chases, 2 per pool, 1 with an unlisted
  sender, 64 groups per pool, maximum delays 5 s / 30 s / 1 min / 5 min,
  backoff 30 s doubling to 1 h, known-bad hashes kept 24 h, 16 hash chain
  pages, 65536 provisional layers. Default template basic, escalating to
  standard at 5 flags.
- **Why:** The design leaves every one of these to node policy. They are
  sized so a single misbehaving peer is noticed within minutes and a
  burst from honest peers is not punished. Milestone 7's simulator should
  replace them with measured values.
- **Tier:** Node policy.

### D63. Keys from one seed, with a gap limit

- **Decision:** A wallet holds one random 32-byte seed. Key *i* is the
  Ed25519 key with seed HMAC-SHA256(seed, `PoO-wallet-ed25519-v1` ∥
  U32(i)), and its address is the 1-of-1 threshold policy of that key.
  Each payment, request and change output takes the next unused key. The
  wallet derives and watches 20 unused keys past the last one used (the
  gap), so a wallet restored from the seed finds payments to every address
  it gave out unless more than 20 in a row went unused.
- **Why:** The design's section 17.6 asks for one address per payment so
  payments are not linked by address, and for a backup that does not
  change as addresses are made. One seed and a deterministic derivation
  give both. HMAC-SHA256 with a domain label is a standard KDF from the
  standard library; a hierarchical scheme (BIP32-style) adds public
  derivation the wallet does not need yet. The derivation is a wallet
  convention, not a protocol rule; another wallet may derive differently.
- **Tier:** Wallet (none of the three tiers: invisible to the network).

### D64. The seed is sealed with PBKDF2 and AES-GCM

- **Decision:** The wallet file holds the seed encrypted with AES-256-GCM
  under a key from PBKDF2-HMAC-SHA256 of the passphrase with a random
  16-byte salt and 600,000 iterations, with the additional data
  `PoO-wallet-seed-v1`. The work factor and salt are stored in the file.
  Public keys, labels, watched mints and payments are kept in the clear,
  so a locked wallet lists addresses and follows payments. The file is
  written atomically with mode 0600.
- **Why:** Standard library only (decision D1) rules out scrypt and
  Argon2; PBKDF2-SHA256 at 600,000 iterations is the current OWASP
  guidance for it. GCM authenticates, so a wrong passphrase or a damaged
  file is detected rather than producing a wrong seed. Keeping the public
  parts clear lets watching run without the passphrase in memory.
- **Tier:** Wallet.

### D65. Coin selection: one mint, else one pool, else fewest pools

- **Decision:** A payment is paid from one mint when one mint can cover
  it, choosing the mint needing the fewest inputs, then the least change.
  Otherwise from the one pool needing the fewest mints. Otherwise across
  pools, largest totals first. Within a mint, the smallest single portion
  that covers the amount, else portions largest first. Each mint used is
  one transaction (a leg) with change to a fresh address.
- **Why:** A transaction spends only one mint (the design's transaction rules), so a
  payment across mints must be several transactions. Legs in one pool
  settle under one ledger's weight; legs across pools settle separately
  (section 7.6), so fewer pools means fewer things to wait for. The design
  also advises keeping value in few mints; preferring one mint and the
  least change does that.
- **Tier:** Wallet.

### D66. Leg states: dropped and void

- **Decision:** Each leg is `unsigned`, `pending`, `selected`, `dropped`
  or `void`. When the node holds neither the leg nor an entry with it, the
  wallet reads the inputs' mint: if every input is still unspent the leg
  is `dropped` and can be sent again (`Resend`); otherwise another
  transaction spent an input and the leg is `void`. For a claim, a mint
  that already has portions means void. The wallet reserves the inputs of
  its own unsigned and pending legs so it does not spend them twice.
- **Why:** The design's section 6.6 distinguishes a carried transaction
  (still valid after a switch) from a void one (it conflicts). A node may
  also simply drop a pending transaction (the pending limits of decision D12).
  The holder needs to know which, because only a dropped one can be
  resent. The node reports holdings at its selected tip only, so the
  wallet must remember what it already spent.
- **Tier:** Wallet.

### D67. Rivals and settled

- **Decision:** The ledger reports, for a transaction, every inactive
  branch with an entry holding another transaction that spends one of its
  inputs, with that branch's weight after its fork (`Ledger.Rivals`); the
  engine and the RPC return it with the payment. A leg is ahead when its
  from(e) is greater than the heaviest rival's weight after its fork. A
  payment is settled when every leg is selected, ahead, and has from(e)
  at least the receiver's threshold (default 0).
- **Why:** The design's section 7.3 measures a payment's safety by the
  margin a rival must overcome, and 7.4 leaves the threshold to the
  receiver by the payment's value. A node that keeps the top K branches
  (decision D12) already knows the rivals that matter; showing them turns
  "how much weight is behind it" into "how far ahead is it". A rival the
  node does not hold cannot be shown, so the threshold still matters.
- **Tier:** Node policy (what is reported); the rule itself is the
  receiver's.

### D68. Payment requests carry the policy

- **Decision:** A payment request is JSON: series, a fresh address, the
  amount, a memo and the encoded policy behind the address. The payer's
  wallet checks the policy hashes to the address, that the series allows
  its type and key algorithms, and declines unknown policy types, before
  paying. The receiver records the request and the tids the payer names,
  and finds legs both by those tids and by portions at the address.
- **Why:** The design's section 17.6 asks that a payer can check an
  address is spendable under the series' rules before paying, which needs
  the policy, not just its hash. Only the payer learns it; the network
  sees the policy only when the receiver spends.
- **Tier:** Wallet.

### D69. Co-signing through a file

- **Decision:** A payment from a shared policy that this wallet cannot
  complete is written as an `Unsigned` JSON file: each leg's transaction
  and, per input, the policy and the signatures gathered. Co-signers add
  theirs (`poo wallet cosign`) and the payer submits it (`poo wallet
  submit`); submission only accepts a payment the wallet recorded, so its
  inputs stay reserved and followed.
- **Why:** The tid leaves out witnesses (the design's transaction id), so every
  co-signer signs the same tid independently and signatures can be
  gathered in any order and by any channel. A plain file keeps the wallet
  free of a network protocol of its own; an external or hardware signer
  can use the same hand-off.
- **Tier:** Wallet.

### D70. Light mode is a full node following less

- **Decision:** Light mode is not a separate client: it is a full node
  whose node policy follows only the pools of the mints the wallet
  watches (`poo wallet follow`). It checks every rule for those pools.
  Verifying only some mints of a pool, or trusting another node's
  answers, is not done.
- **Why:** The design's following model already lets a node keep less,
  and a node that follows a pool verifies it without trusting anyone. A
  pool's entries carry every mint of the pool, so a pool is the smallest
  unit a node can check on its own; anything lighter means trusting a
  peer's word, which the design avoids.
- **Tier:** Node policy.

### D71. The simulator runs real engines over a modelled network

- **Decision:** `pkg/sim` runs one real `engine.Engine` per node and
  models only the network: fixed link latencies, partitions, flooding of
  what the engine says to forward, fetching missing parents and entries,
  a notice and a layer request after a switch on requested data, and
  stated weights after requested work. Time is virtual, and every random
  choice comes from the run's seed, including each builder's starting
  nonce. Peer ranking, chasing and the byte form are not modelled.
- **Why:** The design's tests are about the rules (selection, switching,
  displaced transactions, weight) under network conditions. Running the
  real engine means a simulated result is a result about this code. A
  model of `pkg/p2p` on top would be slower and would test the model.
  `pkg/p2p` is tested over real connections instead. Seeding the nonces
  matters: with fixed nonce ranges every seed drew the same layers, and
  the attack results did not vary.
- **Tier:** Tooling.

### D72. Revalidate releases chains whatever their order in the pool

- **Decision:** `mempool.Revalidate` first drops every transaction the
  selected ledger now holds, then rechecks the rest oldest first, again
  and again until nothing changes. A waiting transaction becomes pending
  as soon as nothing it spends is still pending.
- **Why:** The simulator found a payment lost after a partition healed.
  It was pending, spending a parent that the switch displaced and carried
  back, so it waited on the parent. When the parent was bundled and
  selected again, Revalidate checked the child first (it was older in the
  pool) while the parent was still held, then removed the parent. No
  later call came, so the child waited forever and was never bundled. The
  design asks that chains of displaced payments come back one entry per
  link. `TestOlderChildOfCarriedParent` pins the fix.
- **Tier:** Node policy (what a node keeps pending). No validity rule
  changed.

### D73. Fuzzing checks one encoding per value

- **Decision:** Every decoder has a fuzz target: transactions, entries,
  layers, series params, witnesses, threshold params and data, store
  logs, registry documents, wire messages and bodies. Each checks that
  decoding never panics and that whatever decodes re-encodes to exactly
  the same bytes. Entries are also checked, witnesses verified, and store
  logs reopened after their cut. CI runs each target for 5 seconds;
  `scripts/fuzz.sh` runs them for longer.
- **Why:** Hashes are taken over the byte form (D2). Two encodings of one
  value would let two nodes hash the same transaction differently, and a
  panic on hostile input would let any peer stop a node.
- **Tier:** Tooling (it guards consensus byte forms).

### D74. Provisional defaults kept on the evidence so far

- **Decision:** K stays 8 and the other defaults of D12 and D62 stay as
  they are.
- **Why:** In 20 simulated partitions with payments on both sides, K = 8
  kept every node in agreement and lost no uncontested payment. K = 1 or
  2 left 3 of 20 networks split ([simulation.md](simulation.md)). No run
  hit the pending limit or the seen list size. The peer management
  defaults of D62 are not exercised, since the simulator does not model
  ranking. They stay provisional until a ranking model or a test network
  measures them.
- **Revised by D79:** the splits with K = 1 or 2 were D79's pruning bug,
  not a property of small K. With it fixed, K = 1, 2 and 8 each kept all
  20 networks in agreement. K stays 8: the measurements no longer argue
  for it, but nothing argues against it, and it costs little.
- **Tier:** Node policy.

### D75. Inbound connections cannot take the outbound slots

- **Decision:** A new `MaxInbound` caps connections other nodes open. By
  default it is `MaxPeers` less `MaxOutbound` (24 of 32), so the slots for
  peers the node dials itself are always free. Node policy
  `p2p.max_inbound` sets it; -1 leaves only `MaxPeers`.
- **Why:** Before this, anyone could open `MaxPeers` connections to a
  node, answer its follow request, and leave no room for the peers it
  chose: an eclipse. The security review (docs/security.md) found it.
  Outbound peers come from the node's own discovery and ranking, which an
  attacker cannot fill by connecting.
- **Tier:** Node policy.

### D76. Byte form v1 and wire version 1 are frozen

- **Decision:** From release 0.1.0 the byte forms of
  [protocol/encoding.md](protocol/encoding.md) never change in place. A
  change is a new format version, chosen by a series through its
  parameters' format version byte. `TestByteFormV1Frozen` checks the
  digest of the published vectors, which `-update` cannot get past. Wire
  version 1 keeps its message types and bodies; new behaviour comes as
  new types (ignored by older nodes) or a new wire version in the hello.
- **Why:** Every id in a series is a hash over these bytes. An in-place
  change would split every network running the old code, and independent
  implementations need a fixed target.
- **Tier:** Consensus (byte form) and protocol (wire).

### D77. A supported, patched toolchain

- **Decision:** `go.mod` says `go 1.26.0` (the oldest supported Go
  release) and `toolchain go1.27.3` (the newest patch), and CI and release
  builds use the toolchain line. Since milestone 9 it says `go 1.27.0`,
  the first release with ML-DSA (D81). The security review runs `govulncheck`
  and `staticcheck` before each release.
- **Why:** On Go 1.24.7, `govulncheck` reported 37 reachable standard
  library vulnerabilities (in `net/http`, `crypto/tls`, `crypto/x509` and
  others), and Go 1.24 no longer gets fixes. With 1.27.3 it reports none.
  The code needed no change.
- **Tier:** Implementation.

### D78. Reproducible release archives, published as drafts

- **Decision:** `scripts/release.sh VERSION` builds `poo` for Linux,
  macOS, Windows (amd64 and arm64) and FreeBSD (amd64) with
  `CGO_ENABLED=0 -trimpath -buildvcs=false -buildid=`, packs each with
  fixed times and owners, and writes `SHA256SUMS`. Pushing a tag `vX.Y.Z`
  runs it in CI and opens a draft release. Only the owner tags and
  publishes.
- **Why:** Anyone can rebuild an archive from the tag and compare the
  digest, so a published binary need not be trusted. A draft keeps
  publishing, like the license and the repository's visibility, the
  owner's call.
- **Tier:** Tooling.

### D79. The newest entry is not pruned while it is a leaf

- **Decision:** Pruning (D12) never drops the entry added last while it has
  no children. It is ranked again, and dropped if it is still beyond the
  limits, as soon as another entry arrives. This keeps at most one branch
  beyond K and Ties.
- **Why:** A node catching up takes a rival ledger one entry at a time, from
  the fork, each followed by its layers. Its first entries alone are light,
  and once a node had K stale siblings heavier than that (more builders, longer
  partitions), each one was pruned the moment it arrived, before its layers
  could count. The next entry then had no parent, and the node stayed on
  the lighter ledger for good. The simulator found it at milestone 8: 24
  nodes split for 60 seconds left half the network on the losing side
  after the heal, with every node asking for the same entry forever.
  `pkg/p2p`'s chase adds entries the same way, so real nodes had the bug
  too. `TestRivalFetchedEntryByEntryIsKept` and
  `TestLongPartitionLargerNetwork` pin the fix, and
  `TestHeavierRivalBeyondKSiblings` shows it over real connections: before
  the fix, two nodes never agreed.
- **Tier:** Node policy (what a node keeps). Selection and validity are
  unchanged.

### D80. Five hashes, all with 32-byte output

- **Decision:** A series may name hash 1 SHA-256, 2 SHA-512/256, 3
  SHA3-256, 4 BLAKE2b-256 or 5 BLAKE3 (32-byte output). Every hash the
  node offers has a 32-byte output. `pkg/hash` keeps one table of number,
  name and constructor, with a known-answer test for each.
- **Why:** The owner asked that mints be able to pick their own. These are
  the standard, widely reviewed 256-bit hashes of three unrelated designs
  (SHA-2, Keccak, BLAKE), so a series can avoid any one family. Keeping
  the output at 32 bytes leaves ids, strength (at most 256) and the byte
  form unchanged, so no other package needs to know which hash a series
  chose.
- **Tier:** Consensus (a series' hash fixes all its ids). Existing series
  are unchanged.

### D81. Nine signature algorithms, keys from one seed

- **Decision:** Key algorithms 2 to 9 are ECDSA over P-256, P-384 and
  P-521 (with SHA-256, SHA-384, SHA-512), ECDSA over secp256k1
  (SHA-256), BIP-340 Schnorr over secp256k1 (over SHA-256 of the message),
  and ML-DSA-44, -65 and -87 (FIPS 204, empty context), beside Ed25519.
  ECDSA keys are SEC 1 points (uncompressed for NIST curves, compressed
  for secp256k1) and signatures r ∥ s with s in the lower half; the upper
  half is refused. `policy.NewSigner(alg, seed)` derives any algorithm's
  key from a 32-byte seed: Ed25519 and ML-DSA use it as their standard
  seed, the others take the first valid scalar from HKDF-SHA512 with info
  `"PoO-key-v1 <name> <attempt>"`. A wallet's keys all use one algorithm,
  fixed at creation; key *i* comes from `HMAC-SHA256(seed,
  "PoO-wallet-<name>-v1" ∥ U32 i)`, which for Ed25519 is D63's derivation
  unchanged. Public keys are at most 4096 bytes and signatures 8192.
  *(Replaced by D170, 2026-11-04: there is no network-wide size limit;
  each key and signature is read at its algorithm's exact size, and the
  series' witness ceiling is the only overall limit.)*
- **Why:** These cover the keys holders already have: hardware tokens and
  cloud key services (P-256), Bitcoin tooling (secp256k1), and resistance
  to quantum computers (ML-DSA, the NIST standard). One encoding per
  signature keeps witnesses from being re-encoded. Deriving every
  algorithm from a seed keeps backup to one secret. Fixing one algorithm
  per wallet keeps the wallet's gap of addresses one list. The size
  limits fit ML-DSA-87 with room.
- **Tier:** Consensus (verification). Wallet derivation is implementation.

### D82. Hashlock and k-of-policies address types

- **Decision:** Policy type 2, hashlock: params are a hash number (U32)
  and a 32-byte digest; data is a preimage of at most 1024 bytes. Type 3,
  k-of-policies: params are K (U16) and 1 to 16 distinct policies; data
  is exactly K parts, each an index (strictly increasing) and that
  policy's witness data. Nesting is at most 4 deep, counting the outermost
  as 1. A series must allow every type and key algorithm inside, at every
  depth. Timelocks are not added.
- **Why:** With these a series can allow "key and preimage" (the claim side
  of an atomic swap), "either of two keys of different algorithms" (a
  post-quantum key beside a classical one), and groups of signers. A
  hashlock names its own hash so it can match a lock made elsewhere. The
  bounds keep a witness's cost proportional to its params. A timelock
  needs a clock, and nothing in this design's consensus has one: validity
  depends only on the transaction and the pool state, and each pool grows
  on its own. That is a design question for later, not a policy type.
- **Tier:** Consensus. Series that do not list the new types are
  unaffected.

### D83. A few maintained cryptographic dependencies

- **Decision:** Beside the standard library, the module may import
  `golang.org/x/crypto` (for BLAKE2b), `github.com/zeebo/blake3`,
  `github.com/decred/dcrd/dcrec/secp256k1/v4` and
  `github.com/btcsuite/btcd/btcec/v2` (for BIP-340), and what they
  require. Each is pinned in `go.sum`. The project still implements no
  primitive itself. Advised against, and not offered: RSA, SLH-DSA,
  Ed448, Falcon, and hashes of other sizes ([cryptography.md](cryptography.md)).
- **Why:** The standard library has no BLAKE2b, BLAKE3 or secp256k1.
  `x/crypto` is the Go team's own. The secp256k1 packages run in
  production in the `dcrd` and `btcd` full nodes. `zeebo/blake3` is the
  most used Go BLAKE3. Writing any of these ourselves would be far riskier
  than depending on them. `govulncheck` covers them at every release.
- **Tier:** Implementation. Supersedes D1 in part.

### D84. The commands are a library; the GUI runs the same code

- **Decision:** Every `poo` command lives in package `pkg/cli`, behind
  `cli.Main(ctx, args, Stdio{In, Out, Err})`. `cmd/poo` is only the process
  around it: arguments, the standard streams, and an interrupt or SIGTERM
  that cancels the context. Commands get input and cancellation from the
  invocation, never from process globals, so one process can run several
  invocations at once. The GUI (docs/gui.md) runs commands only through
  `cli.Main`.
- **Why:** The owner's hard requirement is that the headless tool and the
  GUI always share the same code, the GUI being only a way to interact
  with it. A `main` package cannot be imported; a library can, and then
  the GUI's terminal and its buttons run the very code the headless
  binary runs, with no second implementation that could drift.
- **Tier:** Implementation.

### D85. The GUI is a local web app in Go

- **Decision:** The GUI is the program `poo-gui`. It serves a page on a
  loopback address and opens it in the system browser, or in a browser
  app window with `-app`. The page is plain HTML, CSS and JavaScript,
  embedded in the binary, with no framework, npm or CDN. A native webview
  shell may wrap the same server later.
- **Why:** It is cross-platform wherever Go builds, needs no CGO or native
  toolkit, keeps release builds reproducible with only the Go toolchain
  (D78), and can be tested end to end in a headless browser. Fyne and
  Wails need CGO and native libraries. Electron and Tauri add a second
  toolchain. A TUI is not the visual layer that was asked for. The
  options are compared in [gui.md](gui.md).
- **Tier:** Implementation.

### D86. The GUI is its own module and may import only pkg/cli

- **Decision:** The GUI is the Go module `.../crypto-mint-net/gui` in
  `gui/`. It requires the core module through `replace ../`. Its packages
  may import only the standard library, their own packages and
  `pkg/cli`. `gui/imports_test.go` enforces this and checks that nothing
  in the core imports the GUI.
- **Why:** The headless tool is the source, and the GUI is a layer on
  top. A module boundary means the headless binary can never contain GUI
  code. Allowing only `pkg/cli` means the GUI cannot compute, sign or
  read node state except by running the commands. To move the GUI to its
  own repository, the `replace` line becomes a version requirement, and
  the GUI still builds on the same source.
- **Tier:** Implementation.

### D87. Every GUI action is a `poo` command line

- **Decision:** The GUI does everything by running a command line through
  `cli.Main` as a job, with its own context, input and output. A view
  runs a command and renders what it prints. A form builds an argv, and
  the command validates it. The terminal splits a typed line as a shell
  would and runs it unchanged. Each view and job shows its command line.
  Data that no command prints is added to `pkg/cli`, never computed in
  the GUI.
- **Why:** This makes it true by construction that the GUI and the
  headless tool behave the same. Tests compare the GUI's output with
  `cli.Main`'s byte for byte. Showing the command teaches the headless
  tool, and anything the views do not cover is one line away in the
  terminal.
- **Tier:** Implementation.

### D88. The GUI server is locked to this machine and this browser

- **Decision:** `poo-gui` listens only on loopback and refuses any other
  address. A random secret, made at each launch, is in the address it
  opens. The first visit trades the secret for an HttpOnly, SameSite=Strict
  cookie, which every API call needs. Requests with a different Host are
  refused. Changes must be POSTs with the `X-Poo-GUI` header and no
  foreign Origin. A strict Content-Security-Policy is sent. Job input is
  dropped when the job ends, and the terminal masks a passphrase prompt.
  Release archives ship `poo` alone and `poo-gui` separately (G5).
- **Why:** Whoever can reach the GUI can run any command, including
  spending from a wallet. Loopback stops other machines. The secret and
  cookie stop other local users and other pages. The Host check stops DNS
  rebinding. The header and Origin checks stop cross-site requests. The
  CSP stops injected script.
- **Tier:** Implementation.

### D89. The GUI takes secrets and view-only mode from the core

- **Decision:** Every job's `cli.Stdio` carries a `Secret` reader. It
  writes the prompt to standard error, reads one line that is never
  written to the output, and marks the job's state `secret` while it
  waits; the page masks that line and only that line. `poo-gui
  -view-only` sets `cli.Stdio.ViewOnly` on every job and shows a "view
  only" badge; `GET /api/info` reports it.
- **Why:** The core now says when it reads a secret and decides what a
  view-only invocation may run (D101). Taking both from it keeps the GUI
  free of its own rules, as D86 and D87 require: what the GUI masks and
  refuses is exactly what headless `poo` hides and refuses. It replaces
  masking every input line (GUI-003), which hid ordinary answers too.
- **Tier:** Implementation.

### D90. One filter for every view, the stream and every front end

- **Decision:** Package `pkg/filter` defines the only way to select
  items: by series, pool, mint serial, address, address policy,
  transaction id and event kind. Fields are joined with "and" and values
  within a field with "or"; an empty field matches everything. The same
  value is used by the views, the watch stream, the RPC (as query
  parameters) and the command line (as flags). An address policy matches
  through the address it makes under each series' own hash.
- **Why:** The owner asked for comprehensive filtering per series or
  policy, and for the GUI to reuse the headless code exactly. One value,
  with one meaning, keeps the command line, the RPC and the GUI from
  each growing their own slightly different selection. "And" across
  fields and "or" within them is the rule people expect from search
  filters, and it covers every combination asked for without a query
  language.
- **Tier:** Implementation. Changes nothing on the wire or in consensus.

### D91. A live event stream that drops, never blocks, and does not replay

- **Decision:** `pkg/watch`'s hub turns every item the node keeps into
  typed events (`tx`, `entry`, `layer`, `switch`, `selected`,
  `reverted`, `dropped`) and delivers each to the subscribers whose
  filter matches it. Each subscriber has a bounded buffer (1024 events);
  when it is full, events for that subscriber are dropped and counted,
  and the stream says so with a `missed` notice. Over the RPC the stream
  is newline-delimited JSON on `GET /v1/watch`, with `hello`, `ping`
  (every 15 seconds of quiet), `missed` and `end` notices. Events are
  not stored: a client that reconnects, after a reload for instance,
  reads the views for where things stand.
- **Why:** The hub runs under the node's lock, so a slow or stuck
  watcher must never hold up the node; dropping with a count is the only
  choice that keeps the node's pace independent of its viewers. Storing
  events for replay would add a second history beside the store, which
  is already the record and which the views read. NDJSON over plain HTTP
  needs nothing but the existing interface and its token, and every
  language reads it.
- **Tier:** Implementation.

### D92. A running node reloads its node policy in place

- **Decision:** `pkg/daemon` is the one place that runs a node. On a
  reload (`POST /v1/reload`, `POST /v1/follow`, SIGHUP to `poo node`)
  it ends the watch streams, stops the host and the interface, closes
  the node, and opens it again with the node policy on disk, in the same
  process. Stores of pools no longer followed stay on disk.
- **Why:** The owner asked that following be set and forget. Before,
  changing what a node follows needed a restart by hand. Reopening is
  the same path as a start-up, which is already tested and checks every
  record, so a reload cannot leave the node in a state a restart would
  not. Changing the engine's pools while it runs would be faster but
  would need new locking in every package that holds a pool. Keeping the
  stores means following a pool again replays it rather than fetching
  it from peers.
- **Tier:** Implementation.

### D93. Follow edits merge into the follow list

- **Decision:** `nodepolicy.AddFollow` and `RemoveFollow` change the
  follow list by merging: an issuer once, a whole series in place of its
  parts, pools and mints of a series in one entry. Removing pools from a
  series followed whole lists the rest. A mint followed only through its
  pool cannot be removed alone. Both validate the series and ranges, and
  the policy is validated before it is saved. Every front end (`poo
  follow`, `poo unfollow`, `poo wallet follow`, `POST /v1/follow`) goes
  through them.
- **Why:** Edits from several tools must leave one clean list, not a
  growing pile of overlapping entries, and must never save a policy the
  node would refuse at start-up. A mint is checked with its whole pool
  (the pool is the unit of verification), so taking one mint off a
  followed pool would promise something the node cannot do.
- **Tier:** Node policy.

### D94. Views are bounded per request

- **Decision:** Every view walks only the pools its filter's scope can
  match (`MatchScope`), and a ledger view walks only as far as its page
  needs: at most `limit` entries (default 50, at most 1000), newest
  first, with `next` to continue. A filtered page looks at no more than
  50 × `limit` entries, stepping over those that do not match, so `next`
  always moves back and a filter that matches nothing costs a bounded
  walk.
- **Why:** The interface is local and trusted, but a GUI polling a large
  ledger should cost each request the same, not the whole ledger's
  length. Pages that always move keep paging simple and stop a filter
  that matches nothing from being an unbounded walk.
- **Tier:** Implementation.

### D95. New wallets seal the seed with Argon2id

- **Decision:** A new wallet's passphrase key is Argon2id (RFC 9106) with
  3 passes over 64 MiB in 4 lanes, a random 16-byte salt and a 32-byte
  key, from `golang.org/x/crypto/argon2`. The wallet file names the
  derivation and its settings (`name`, `iterations`, `memory`,
  `threads`), so wallets sealed with PBKDF2 under D64 still open.
  `Rekey` (`poo wallet passwd`) moves a PBKDF2 wallet to Argon2id
  with the defaults and keeps an Argon2id wallet's own settings. Opening
  refuses settings outside fixed limits (at most 4 GiB and 100 passes for
  Argon2id, 100,000,000 iterations for PBKDF2, at least 8 KiB per lane)
  before deriving anything, so a damaged or crafted wallet file cannot
  make an unlock exhaust memory or run for hours.
- **Why:** PBKDF2 costs an attacker who has stolen the wallet file only
  computation, which GPUs and ASICs make cheap; Argon2id also costs
  memory per guess. D1 kept the standard library only, but D83 already
  depends on `x/crypto` (the Go team's), so Argon2id adds no new module.
  64 MiB and 3 passes is RFC 9106's recommendation for memory-constrained
  machines and takes a fraction of a second to unlock. This closes the
  "Key derivation" open item of the M8 security review.
- **Tier:** Wallet. Amends D64 for new wallets.

### D96. Addresses learned from others must be public

- **Decision:** An address another node gives (a follow request's or
  reply's contact, a peer list entry) or a registry lists goes into the
  peer book only if it is a host name or a public unicast IP: not
  loopback, unspecified, private (RFC 1918, fc00::/7), link-local,
  multicast, shared (100.64/10), documentation, benchmarking, 0/8, 240/4
  or broadcast. When the connection manager dials a book candidate, the
  dialer checks the resolved IP of each attempt the same way before
  connecting, so a host name resolving to a local address, or a record
  from an older peer file, is refused too. Addresses in the policy's
  `peers` and `Host.Connect` from the owner are not checked. Policy
  `p2p.allow_local_peers` (off by default) turns the checks off, for
  networks on one machine or one LAN; the `pkg/p2p` tests turn it on.
  `peers.Range` still gives each loopback address its own range.
- **Why:** Red finding RED-001: any peer that had only sent a hello could
  name an address in a follow request, and the node would later connect
  to it, letting a remote party probe the node's own machine and network
  (`127.0.0.1:N`, `169.254.169.254`, RFC 1918 hosts) through it. Checking
  the resolved address at dial time closes the DNS route as well as the
  literal one. The per-address loopback range (D45) assumed loopback
  could not be an outside attacker's; with this check that holds again,
  and collapsing it would cap local test networks for no gain. Asking a
  second peer to corroborate a contact was not added: a public address
  one peer names is an ordinary candidate, verified when dialled, and
  dialling is paced by the manager.
- **Tier:** Node policy.

### D97. Tighter limits on a wallet file's key derivation

- **Decision:** A wallet file's Argon2id settings may name at most 1 GiB
  of memory and at most 8 GiB in passes times memory (so 8 passes at
  1 GiB, or 100 at the default 64 MiB); PBKDF2 at most 10,000,000
  iterations. `wallet.CheckKDF` checks settings without deriving. This
  replaces the 4 GiB and 100,000,000 limits of D95.
- **Why:** Red finding RED-004: with D95's limits a crafted or imported
  wallet file could still make opening it allocate 4 GiB, or run 100
  passes over it, before the passphrase is checked, which can exhaust a
  modest node host. The defaults (192 MiB-passes) sit far inside the new
  limits, and so does any setting a person would choose on purpose.
- **Tier:** Wallet. Amends D95.

### D98. A seed is never a command-line argument

- **Decision:** `poo claim` and `poo pay` read the key's seed from
  `POO_SEED` or `-seed-file FILE` (`-` for standard input). `-seed` is
  refused with an error that names the alternatives. `poo keygen` takes
  `-seed-file` too and keeps `-seed HEX` for reproducible test vectors,
  documented as visible to other users; with neither it makes a random
  seed, and it does not read `POO_SEED`, so an exported seed is never
  reused for a new key by accident.
- **Why:** Red finding RED-005: a seed in an argument is readable by every
  user of the machine through the process list and `/proc/<pid>/cmdline`,
  and stays in the shell's history and the GUI terminal's. `poo wallet`
  already reads its passphrase this way. `keygen -seed` stays because the
  GUI's tests and the design's vectors use fixed seeds, and a seed made
  for a vector holds nothing.
- **Tier:** Implementation.

### D99. Building an entry is bounded, cancellable and lock-free

- **Decision:** `/v1/build` accepts `max_tries` up to `MaxBuildTries`
  (2^24, the same as `MaxWorkTries`); 0 or no value means that limit, and
  more is a bad request. The engine splits `Build` into `BuildEntry`
  (choose the ready transactions and the tip under the engine's lock,
  then search for the nonce without it) and `HandleEntry`, and `Work`
  into `GrindLayer` and `HandleLayer`. `Node.Build` and `Node.Work` hold
  the node's lock only to take the result in and store it, never while
  searching. `entry.BuildContext` and `Node.BuildContext` stop the search
  with the context's error; the RPC passes the request's context, so a
  client that goes away, or a shutdown, ends the search. `entry.Build`
  and `engine.Build` keep their signatures, with no limit, for callers in
  process. `poo build -max-tries 0` now means the node's limit.
- **Why:** Red finding RED-006: a build with no limit on a series with a
  high minimum entry strength searched for up to 2^64 nonces holding both
  the node's and the engine's locks, so one request froze the RPC and all
  peer processing until the process was killed. If the tip moves during
  a search, the entry is still valid on its old parent and is kept as a
  branch, the same as one received from a peer.

### D100. A second, read-only token for the local interface

- **Decision:** Beside `rpc.token`, the node creates `rpc.view-token`
  (32 random bytes in hex, mode 0600) in its data directory. A request
  carrying it is served for `GET` and `HEAD` only; any other method is
  refused with `403 forbidden`. `rpc.token` keeps full control.
  `rpc.DialView` dials with it. (GREEN-007, part 1.)
- **Why:** A monitor, a metrics scraper (`GET /v1/metrics`) or a viewer
  needs to read the node but should never be able to pay, build, change
  follows or block peers. Every endpoint that changes something is a
  `POST`, so the method is the scope, with no per-endpoint table to keep
  in step. A second file means the full token never has to leave the
  owner's account.
- **Tier:** Implementation.

### D101. View-only mode, and secrets read through the caller

- **Decision:** An invocation of `pkg/cli` is **view-only** when
  `Stdio.ViewOnly` is set, when `$POO_VIEW_ONLY` is `1`, or when its
  arguments start with `-view-only`. Then every command that talks to the
  node dials with `rpc.view-token` (D100), and a fixed table in
  `pkg/cli/scope.go` lets only the commands that read run: anything not
  listed, a new command included, is refused with `view-only: poo X
  changes things` before it runs. A test fails until every command is
  classified. Separately, `Stdio.Secret`, when set, reads every
  passphrase and seed in place of `In`; `cmd/poo` sets it when standard
  input is a terminal, reading with echo off through `golang.org/x/term`
  and prompting on standard error.
- **Why:** The owner wants a GUI view mode that is the headless code,
  not a second implementation: `poo-gui -view-only` only sets the
  variable. Two layers keep it honest: the node refuses writes from the
  view token whatever the client does, and the table stops local writes
  (node policy, wallet file) that the node never sees. Refusing by default
  means a forgotten command is safe. The secret callback lets the GUI
  mask exactly the input that is a secret instead of guessing, and lets
  the terminal hide typed passphrases, which `bufio` on standard input
  could not. `x/term` is the Go team's, beside `x/crypto` (D83).
- **Tier:** Implementation. Supersedes nothing; GREEN-007 part 2.

### D102. IPv6 addresses that carry an IPv4 address are not public

- **Decision:** The public-address check of D96 also refuses NAT64
  (`64:ff9b::/96` and the local-use `64:ff9b:1::/48`), 6to4 (`2002::/16`),
  Teredo (`2001::/32`), IPv4-compatible (`::/96`) and IPv4-translated
  (`::ffff:0:0:0/96`) addresses, whatever IPv4 address they carry. Listed
  `peers` and `allow_local_peers` work as before.
- **Why:** Red finding RED-007: on a host with NAT64, `64:ff9b::7f00:1`
  reaches 127.0.0.1, so a peer could name loopback or a private host in
  IPv6 form and get past D96. Refusing the ranges outright is simpler and
  safer than decoding each form (Teredo obfuscates its address, and a
  network may use its own NAT64 prefix, which no check can know): a peer
  reachable through one of them has a native IPv4 address to give.
- **Tier:** Node policy. Amends D96.

### D103. The address view is bounded per request

- **Decision:** `watch.AddressPage` (behind `GET /v1/address` and `poo
  address`) looks at no more than 50×`limit` entries per request in all
  followed pools, at most `limit` matching items per pool, from height
  `below` down (0: the tips). The reply's `next` lists each pool it did
  not finish with the `below` to ask for the rest. It checks the
  request's context before each pool and while walking. `AddressOf` keeps
  its signature, from the tips.
- **Why:** Red finding RED-008: the view walked every followed pool's
  ledger to height 1 under the engine's lock, and the read-only view
  token (D100) could ask for it, so a reader could stall the node on a
  tall ledger. `LedgerPage` already had this bound (D94).
- **Tier:** Implementation.

### D104. Node logs are structured, and a stopping interface always stops

- **Decision:** The node logs through `log/slog` (`pkg/logging`, formats
  `plain`, `text` and `json`, levels from the node policy's `log` section
  or `poo node -log-level` and `-log-format`). Text from a peer goes only
  into attributes, which are quoted, never into the message, and the
  debug line for each received message is capped at 200 a second per
  host, with the number dropped logged after. When the local interface
  stops, the event hub refuses new subscribers once closed (a late one
  gets a closed stream at once), and a request still running after
  `rpc.ShutdownTimeout` (5 seconds) is cut off; neither makes the stop
  fail.
- **Why:** GREEN-006, with Red's review of the debug level: a peer must
  not be able to forge log lines or fill a disk. The stop: GUI GREEN-015
  item 3 saw `poo node` exit 1 with "context deadline exceeded"; a watch
  that subscribed after the hub closed during a stop or reload waited
  forever and held the shutdown past its deadline.
- **Tier:** Implementation.

### D105. Faster checks and stores that change no result

- **Decision:** Four of Orange's Round 3 proposals are adopted, none of
  them a rule change. Each series remembers up to 16384 witnesses that
  satisfied their policy for a tid, under a key covering the policy, the
  witness data and the tid, and skips only the satisfaction function for
  them. A node verifies an item's signatures before taking its own lock
  (`Engine.PreverifyTx`, `PreverifyEntry`), meant to be only for what
  `Handle*` would verify too: not for an item seen or held, and not for an
  entry that fails `entry.CheckShape` (series, pool, order, nonce,
  strength). That fell short, and D107 corrects it.
  The store's group commit lets items wait for one shared fsync after the
  node's lock is released; a node event now fires after the write and
  before the fsync, while the `Handle` call returns only after it. The
  ledger keeps its off-chain set, and hash functions reuse digests.
- **Why:** Orange measured each (ORANGE-001 to 005): a 64-node simulation
  from 41 s to 16.7 s, a no-sync node from 12.9k to 35k transactions a
  second on 4 cores, and a syncing node from 2.9k to 17.4k. The guard on
  entries keeps `entry.CheckStructure`'s order, strength before signatures, so a
  peer cannot make a node verify signatures without doing an entry's
  work. An event before the fsync can show or forward an item that a
  power loss then takes from this node's disk; peers still hold it and
  the node fetches it again, as it would any item it missed, so nothing
  depends on the event coming after the fsync.
- **Tier:** Implementation.

### D106. A reload onto a broken policy is refused, and seeds are read hidden

- **Decision:** `POST /v1/reload` (and `poo reload`, and `poo follow`
  through it) loads the node policy first, and answers 422 with the
  reason if it does not load; SIGHUP does the same check, logs "reload
  refused" and keeps running on the policy it started with. `poo wallet
  create -restore -` reads the seed like a passphrase, through
  `Stdio.Secret` when a front end gives one, and `cli.SecretFlags()` names
  the flags whose values can be secrets (`seed`, `restore`) for front ends
  that keep command lines.
- **Why:** GREEN-011: a mistake in `node.json` followed by a reload
  stopped the node for good while `poo reload` printed "reloaded". GUI-006:
  a seed on the command line can be read by other users (as with D98),
  and the GUI's terminal masks secrets in its history. The hex form of
  `-restore` still works, for scripts.
- **Tier:** Implementation.

### D107. Preverifying runs the cheap checks first and stops at a bad witness

- **Decision:** `Engine.PreverifyTx` and `PreverifyEntry` hold the engine's
  lock for every check `HandleTx` or `HandleEntry` runs before a
  signature, and find the address each witness must satisfy:
  `mempool.Pool.Owners` (not seen, pending or selected, no conflict or
  pending dependency, room, `tx.CheckForm`, then `tx.Owners`: mint
  claimed, inputs unspent at the tip and of the mint, amounts) and
  `ledger.Pool.EntryOwners` (shape and strength, not held, parent held,
  then `entry.Owners` against the state after the parent). Outside the
  lock `tx.Preverify` checks witness *i* against its owner with
  `CheckCached`, the same call `Check` makes, in order, and stops at the
  first failure; an entry stops at its first bad transaction. An item
  that fails a cheap check costs no signature check, and one with a bad
  witness costs one ahead and one under the lock. `ledger.Pool.AddEntry`
  now checks the parent before any claim's witness. The mempool wraps a
  check's error with `%w`, so a bad witness shows as `tx.ErrWitness`, and
  `pkg/p2p` charges a pushed transaction with one as an invalid item.
  `policy.Registry.Preverify` and `series.Series.Preverify`, which ran a
  witness without its address or any state, are removed.
- **Why:** Red finding RED-009: D105's preverify verified every witness of
  a peer's frame before the cheap checks, did not stop at a failure,
  remembered passes for inputs that do not exist, and a refused
  transaction was never an offence. A peer could make a node spend about
  390 times the CPU of the old path per byte, repeatedly, and fill the
  witness list with its own keys. With the order restored, a peer must
  own real unspent portions to get a signature checked, and a bad one
  costs it standing. No honest node forwards a transaction with a bad
  witness, since it checks before forwarding.
- **Tier:** Implementation. Corrects D105.

### D108. A failed fsync stops the pool's store

- **Decision:** A `store.Log` latches its first failed write or fsync
  (`ErrFailed`). After it the log takes no appends, `Durable` returns the
  error for every record not already synced and never runs the sync
  again, and `Rewrite` and `Close` return it. The node logs one error per
  pool and every later item of that pool fails with the error and is
  neither stored nor forwarded; a restart replays what is on disk.
- **Why:** Red finding RED-010: after a failed `fsync` Linux may drop the
  unwritten pages and report success on the next one, so the group commit
  (D105) let a waiter behind the failed sync retry it and report the same
  records durable. Others (PostgreSQL among them) learned to treat a
  failed fsync as fatal. Stopping the pool's store, rather than the whole
  node, keeps the other pools running; the lost tail is held by peers and
  fetched again.
- **Tier:** Implementation.

### D109. Grinders: devices, a registry per hash, plans and jobs

- **Decision:** Hash work goes through `pkg/grind` (M11). A `Registry`
  holds device probes and grinders keyed by kind of device and hash ID; a
  grinder for hash 0 is generic and grinds any hash. `Choose` makes a
  `Plan` for a hash on a named device, or on `auto`, which prefers a
  device other than the CPU with a fast grinder for the hash, then the
  CPU's fast grinder, then a generic one (`fallback`), and says why.
  Threads default to every logical core but one and are capped at the
  device's. The search loops of `pkg/entry` and `pkg/layer` take any
  `hash.NewHasher`, a ctx and a tries counter through `engine.Grind`, and
  find what the one-goroutine search finds. A `Manager` in the daemon runs
  jobs on a target (`tip`, `pending` or an entry hash), never a hash: the
  target's series gives the hash. `tip` uses the selected tip, or the
  best branch while none is selected. Jobs work in 2-second slices from a
  random nonce and may be limited by `for`, `hours` and `until_from`;
  at most 16 run at once and the last 32 ended are kept. Jobs come from
  `poo grind`, `/v1/grind` or `node.json`'s `grind` section. A build or
  work request may grind for at most 4 minutes (`MaxGrindFor`, under the
  client's timeout); longer work is a job. Starting and stopping jobs are
  writes, which the view token cannot do.
- **Why:** The owner asked (2026-10-28) for fast grinding that detects the
  hardware, lets the user pick or leave it on auto, assigns work by hand,
  on a schedule or automatically to specific pools and entries, picks up
  the series' hash by itself, and has one modular grinder per hash type.
  A registry keyed by kind and hash makes a GPU or a new hash one
  `Register` call; the generic grinder means a hash added to `pkg/hash`
  can be ground at once. Leaving one core free keeps the node responsive
  to peers and the RPC. GREEN-016, ORANGE-006, ORANGE-007.
- **Tier:** Implementation. No consensus change: a grinder only finds
  nonces faster.

### D110. Front ends get secret output, error codes and doctor fixes from pkg/cli

- **Decision:** `cli.SecretOutput(args)` says whether a command line
  prints a secret (`keygen`, `wallet seed`), beside `SecretFlags`.
  `cli.ErrorCode(err)` maps an error to a stable code: the node's own
  `rpc.Code*`, or `no_node` (wrapped `rpc.ErrNotRunning`, which `Dial` now
  returns, or a refused dial), `view_only` (`cli.ErrViewOnly`), `no_input`
  or `error`. `poo doctor` takes `-o json|table` (`-json` stays), and a
  failed check carries `fix_args`, the `poo` arguments that fix it. An
  interrupted `poo grind run` stops its job, prints an `end` line and
  exits 0.
- **Why:** The GUI must treat secrets and errors exactly as `poo` does
  without copying lists or matching messages (GUI-009, GREEN-021,
  GREEN-022, the owner's rule that the two never diverge).
- **Tier:** Implementation.

### D111. A deep parent is checked once, and its revert is the sender's cost

Partly superseded by D116: an entry pushed over the revert budget goes to
the chase queue instead of being dropped.

- **Decision:** `ledger.Pool.EntryOwners` (the engine's preverify) refuses
  with `ErrDeepParent` when building the state after an entry's parent
  would revert and apply more than `MaxPreviewCost` (64) entries;
  `AddEntry` still takes the entry, under the lock, with its signatures
  verified there as before ORANGE-004. `Engine.EntryCost` reports that
  number of entries, and `pkg/p2p` draws it for every pushed entry from
  the sender's new `revert` budget in `tracking` (`Spend`, 200,000 entries
  a minute per address by default; requested entries are not charged).
  Over it the entry is dropped and the address flagged once per window.
  `POST /v1/grind` takes only the source `user` (or none), so
  `DELETE /v1/grind?source=user` stops every job made over the RPC.
- **Why:** RED-011: an entry on the pool id costs a revert of the whole
  selected chain (about 60 ms at 40,000 entries with transactions), and
  BLUE-005 had made the node pay it twice under the engine lock, for an
  entry that needs no work or valid signature. An honest entry builds on
  the tip or near it, so the bound costs it nothing; an honest deep reorg
  arrives through the chase, as requested entries. At the default budget
  one address may make the node revert about 0.3 s of chain a minute at
  40,000 entries. A cached view and a work gate on the fork depth (Red's
  other options) are left open: the cache costs memory as large as the
  chain, and the gate is a consensus change for the owner.
- **Tier:** Implementation. No consensus change: what is valid and what is
  selected are unchanged.

### D112. Grind jobs: leases, payment targets, refusals at start, steady events

- **Decision:** A `grind.Spec` may carry a `lease` (1s to 10m). The job
  stops, with that reason, unless it is renewed (`Manager.Renew`,
  `POST /v1/grind/{id}/renew`) within it. `poo grind run` asks for a
  15-second lease and renews it every 5 seconds, and `poo` (but not
  `poo node`, which reloads) stops on SIGHUP as on an interrupt,
  unless SIGHUP is ignored already, as under `nohup` (GUI-011). A target
  `tx:TID`, with `until_from`, follows one payment through
  `Engine.Payment`, the call `poo tx` reads: entries while it is pending,
  then layers on its entry until from(e) reaches the goal; it fails if
  the payment was held and is gone. A job that would work now is refused
  at `Start` if a device cannot grind one of its hashes; one with `hours`
  is not. Events: one job-wide `start` with the spec, a per-pool `plan`,
  `progress` at most once a second with the rate over that second, and a
  `hint` on `waiting`. Job-wide events pass a watch filter on series or
  pools when the job's spec can work on one of them. A job's own context
  and its `for` timer are both cancelled when it ends. `cpu-blake3-cv`
  becomes BLAKE3's CPU grinder; the SIMD lanes grinder stays a prototype
  behind `GOEXPERIMENT=simd` until Go's `simd` package is stable.
- **Why:** GUI-010: a followed job outlived a dead follower and ground on
  for nobody. The `for` timeout replaced the job's cancel, so each such
  job left a context on the manager's until the node closed. GREEN-017:
  a start that succeeded on a missing device failed only later; a
  series watcher never saw a job end; `run` printed two starts. GREEN-019:
  people grind to make a payment safe and hold a TID, not an entry hash.
  GUI/Orange: thousands of progress lines a second on easy targets, with
  a rate measured over a few milliseconds. GREEN-027: a waiting job gave
  no next step. ORANGE-009 and ORANGE-011 measured the grinders.
- **Tier:** Implementation. No consensus change.

### D113. The grind schedule changes live, without a reload

- **Decision:** `GET /v1/grind/schedule` returns `node.json`'s `grind`
  section, and `PUT /v1/grind/schedule` replaces it: the node validates
  and saves it, then `Manager.Schedule` stops the jobs the policy started,
  takes the new defaults and starts the new jobs, reporting any that did
  not start. `poo grind schedule` lists, adds (the job flags without
  `-for`, which a job that runs within its hours has no use for), removes
  (by number from 1, or `-all`) and sets the defaults, through the node
  when it runs and in `node.json` when it does not. Without `add`,
  `remove` or `set` it only reads, so it runs in view-only mode.
- **Why:** GREEN-026: the GUI may not write `node.json`, and editing the
  schedule by hand needs a reload, which would stop every user job.
  Applying the section in the manager keeps user jobs running.
- **Tier:** Implementation.

### D114. A branch costs what its entries hold, once, whoever sent it

Partly superseded by D116: the `MaxBareRun` limit is removed, and entries
a chase takes in after checking their work are not charged.

- **Decision:** `ledger.Pool.AddEntry` keeps the state after the entry it
  added (unless that entry switched the ledger) and starts the next entry
  on it from there; a switch or the prune of that entry drops it.
  Off-chain nodes keep their anchor height and the pool its set of
  off-chain leaves, so ranking and weighing a branch no longer walk it.
  View cost (`ViewCost`, `EntryCost`, the `revert` budget, D111) is one
  unit per entry reverted or applied plus one per transaction in it;
  `MaxPreviewCost` is 256 units. `pkg/p2p` charges entries a chase takes in
  to the sender's `revert` budget as it does pushed ones, and ends a fetch
  at more than `MaxBareRun` (4,096) entries in a row without layers in a
  series without entry hash credit. `nodepolicy.Save` writes a temporary
  file of its own, syncs it and renames it; the RPC serialises its edits of
  `node.json` (follow, unfollow, the grind schedule, which it applies under
  the same lock), `grind.Manager.Schedule` runs one call at a time, and
  `Manager.Start` holds its slot from the `MaxJobs` count to the insert.
- **Why:** RED-012: a branch of L entries without layers cost O(L^2)
  (32,000 entries took 32.5 s) in re-applied state, anchor walks and leaf
  scans, and a chase took them in uncharged and without a work check; now
  the cost per entry is flat (1.5 us at 16,000 and 64,000 entries). Entries
  with transactions cost up to 45 times more to revert than empty ones, so
  counting entries alone let a busy chain cost far more lock per budget.
  An honest catch-up or switch taken in order costs only its first revert.
  RED-013 and RED-014: concurrent writers of `node.json` tore the file
  and lost updates, and concurrent starts passed `MaxJobs`.
- **Tier:** Implementation. No consensus change: what is valid and what is
  selected are unchanged.

### D115. No consensus rule outside the design documents; node policy may never refuse a heavier ledger

- **Decision:** The owner's rule, recorded in `AGENTS.md`: no consensus
  rule is added outside the design documents without the owner's
  approval, and every suggested consensus change goes to Black first.
  Work targets node policy, on one condition: no node policy, alone or
  with others, may make a heavier valid ledger unacceptable for good, so
  a hard fork is impossible whatever the settings. Node policy may decide
  when, how fast, from whom and in what order, and may ask for work before
  it spends anything; it may not set a depth limit, nor a size or count
  limit that every peer would hit. The consensus audit
  (`docs/consensus-audit.md`) lists what the code did that broke this; its
  plan is D116 and D117, its test P0 is
  `pkg/p2p/heavier_test.go`, run in CI with the default and the tightest
  settings and, for items over 16 MiB, in a step of its own.
- **Why:** The owner, 2026-10-30 and 2026-10-31, after the audit found
  two caps that acted as consensus (C1, C2) and four node limits that
  could stop a node reaching a heavier ledger (G1 to G4).
- **Tier:** Process. It decides how every later change is judged.

### D116. A chase never refuses a heavier ledger: it halves, rechecks and yields

- **Decision:** For audit items P2 to P5. `pkg/p2p` drops `DepthLimit`,
  `ProvisionalLayers`, `MaxBareRun` and `FastCatchUp` (the node policy's
  `depth_limit` and `fast_catch_up` still load and are ignored). The
  common entry is found by halving over the node's own selected ledger,
  anchored only on entries it holds, about log₂(height) requests at any
  depth. The work check pages the sender's hashes forward with no cap on
  layers. Entries fetched after the work check are not charged to the
  revert budget; a whole-ledger fetch and pushed entries still are, and
  `tracking.Spend` lets one spend larger than the whole budget pass in an
  empty window. Over the budget, or past `ChaseSlice` (2 minutes), a chase
  yields (`chase.Yield`): its sender waits `YieldWait` (5 s) with no
  backoff, and what it learned is kept (`progress`, up to 256 chases) so
  the next one goes on from there; every slice makes at least one
  request. A pushed entry over the budget is offered to the chase queue.
  Before each extension request the chase checks that no ledger off the
  branch and heavier than its work was selected meanwhile. Only hashes
  shown false go on the known-bad list: an entry hash below the minimum,
  an invalid entry, a listed hash another entry was sent for; a sender
  that repeats a hash, sends entries out of order or weak layers is
  flagged and backed off, and the hashes it named are not listed (G4).
- **Why:** G1 (the revert budget ended a fetch), G2 (4,096 bare entries
  ended a fetch), G3 (16 hash chain pages ended the search) and G4 (a valid
  entry could be bad-listed for a day) each let node policy stop a node
  reaching a heavier valid ledger, which D115 forbids. Halving keeps the
  search cheap without a limit, and yielding with kept progress keeps any
  one chase from holding the queue.
- **Tier:** Implementation (node policy). What is valid and what is
  selected are unchanged.

### D117. No size or count cap on items; large entries travel in parts

- **Decision:** For audit item P1. `entry.MaxTxs`, `tx.MaxInputs`,
  `tx.MaxOutputs`, the witness count cap and the per-item layer cap in
  `pkg/wire` are removed; `encoding.Reader.Count` already refuses a count
  the bytes left could not hold. Wire version 1 gains three types (D76
  allows new types): parts request (26), large (50) and part (51). An
  entry item larger than `p2p.Config.PartsOver` (by default what one frame
  holds) ends an entries reply before it, or is named by a large reply
  when it comes first; the asker fetches it with parts requests, at most
  64 MiB per reply, checks each part starts where the last ended and names
  the stated size, and decodes and checks the hash once whole. Large
  entries are announced with a notice, never pushed; a pending
  transaction too large to push travels only inside an entry. The store
  writes a record over `MaxRecordSize` as `KindPart` records and a last
  record of its kind.
- **Why:** C1 and C2: the owner refused both caps. A valid entry larger
  than a frame, or a transaction with more than 65,536 outputs, could not
  be decoded or sent at all, so every node would refuse a ledger holding
  one: a hard fork.
- **Tier:** Implementation and wire (new message types; byte forms of
  entries and transactions unchanged, so no test vector changes).
  A layers reply for one entry with more layers than a chunk holds
  (about two million) sends the strongest that fit; the extension that
  follows carries them all, in parts.
- **Not yet:** an entry is put together in memory, not on disk, and the
  RPC body limit (8 MiB) applies to items submitted locally. Both are
  noted for Round 8.

### D118. The node's own grinder may keep every layer above a floor (ORANGE-010)

- **Decision:** Node policy `grind.layer_floor`, 0 (off) by default. Above
  0, each search also hands the node the strongest layer of every range of
  4,096 nonces that reaches the floor, raised to the series' minimum layer
  strength; the node takes each in as its own layer and forwards it. The
  strongest layer of the search is still taken in first.
- **Why:** ORANGE-010 measured 3 to 7 times the credit for the same work.
  Black's condition: the grinder only submits more layers and drops
  nothing counted, so what any node accepts and selects is unchanged.
- **Tier:** Implementation (node policy). Red checks the higher honest
  layer rate against the flood limits in Round 7 Part B.

### D119. Round 7 adoptions: BLUE-007, GREEN-020, ORANGE-012, ORANGE-013, GREEN-031

- **Decision:** BLUE-007 (RED-012 to RED-014, D114) is merged, with its
  per-peer budget reworked by D116. GREEN-020 (the start state outside a
  job's hours, shorter tables), ORANGE-012 (SIMD lane grinders behind
  `GOEXPERIMENT=simd`, `layer.Search`) and ORANGE-013 (the ended channel
  and progress ticker) are merged. GREEN-031 and GUI-013: `poo grind
  schedule remove N -target T` removes nothing unless each numbered job
  grinds T.
- **Why:** Each was reviewed in Round 6 and measured or tested; the guard
  stops a stale row number from removing another job.
- **Tier:** Implementation.

### D120. Work is asked for and listings are paced; large parts go to disk

- **Decision:**
  - **RED-015.** A chase's progress holds only what work backs: the page of
    hashes being checked, the listed hashes that carried layers (by index,
    so step 5 still checks that each is the entry sent there), and the
    entries fetched. A hash with no layers is let go once its layers are
    counted. After `p2p.Config.WeightlessPages` (64) pages in a row that
    bring no work, the chase yields and goes on later from its progress.
    The first page after the pool_id is the sender's first hashes.
  - **RED-016.** A node that holds nothing checks the work from the
    pool_id before a whole-ledger fetch. Every part of a large entry must
    hold at least a byte and stay within the stated size, and the first
    must start with the expected parent. Parts go to a temporary file
    (`p2p.Config.PartsDir`). A fetch past its slice yields, keeping the
    file and its offset, and the file is removed when the fetch ends, its
    progress is forgotten, or the host stops.
  - **RED-017.** A parts reply copies the item under the engine lock and
    encodes it outside, in a cache of items (one minute after last use,
    256 MiB in all).
  - **RED-018.** A layer floor sends at most 16 extra layers per search,
    the strongest first, and 50 a second across searches.
  - **RED-019.** `poo grind run` retries a failed renewal while the lease
    may still hold, and stops renewing only for a job the node does not
    know or that is done.
- **Why:** Red's Round 7 findings. Under D115, nothing here refuses a
  heavier ledger for good. A real heavier ledger has work, and the node
  waits for it. A listing with no work costs the node a page of memory
  and a paced share of its time, not 948 MB in 15 s. A claimed size costs
  disk only as the sender sends the bytes, and no size is refused. Work
  on a heavier ledger may sit on any of its entries, so weightless pages
  are paced, not ended. The extra layers a floor keeps add credit and
  remove none, so dropping some changes only how much of its own work a
  node sends (D118's condition holds).
- **Tier:** Implementation (node policy and the CLI). No consensus change:
  what is valid and what is selected are unchanged.
- **For Black:** the RED-015 and RED-016 changes touch G3. Pacing is by
  pages in a row without work and by the time slice. The only new gate is
  the work check before a whole-ledger fetch, which any heavier ledger
  passes. Decoding a large entry still needs it whole in memory (D117's
  "not yet").

### D121. Held branches, deep layers and many layers are always reachable (Black's Round 7 review)

- **Decision:** Fixes for Black's B1 and conditions C-1 to C-3 in
  `docs/consensus/source/ROUND-7.md`, all node policy.
  - **B1.** When the sender's newest held hash is off the selected
    ledger, `findCommon` walks back through entries held locally (no
    request, no limit) to where that branch leaves the selected ledger;
    the walked hashes go first in the listing, and steps 4 and 5 run as
    for any branch. When the newest held hash is on the selected ledger,
    `topUpLayers` asks for the layers on the sender's own hashes, page by
    page back from its tip, until the node is as heavy as the sender
    stated or the sender's chain ends; it yields past its slice and goes
    on from saved progress. It used to ask about this node's latest
    entries, which a sender on another branch does not hold.
  - **C-1.** When the node's own ledger outgrows the work counted so far,
    `fetchBranch` hands back to step 4 to count more of the listing; only
    a branch counted to its end ends as not heavier.
  - **C-2.** Past 256 remembered chases the one with the least shown is
    forgotten: not worked before worked, then less work, then oldest. A
    worked chase is never forgotten for one that is not.
  - **C-3.** Wire types 27 (layers-from request), 52 (many layers) and 53
    (layers from), as D76 allows. A layers reply ends before an entry with
    more than `p2p.Config.LayersWhole` layers (by default what one frame
    holds) and names it in a many-layers reply when it comes first; the
    asker pages its nonces strongest first from any index until all have
    come (`askLayers`, `layersFrom`). The strongest-first order means
    layers that arrive meanwhile can only move ones already read later,
    so paging misses none. The old truncation to the strongest that fit
    is gone.
  - **G4 note.** A sender that sends another entry where it listed hash W
    is flagged; W is no longer bad-listed, since it may be a valid
    entry's hash the sender copied.
  - **Budgets.** `tracking.Spend` counts only what it lets through, so a
    message dropped over a budget adds nothing and a steady sender gets
    the budget's rate (ORANGE-016's point; Black's B1 note).
  - **C-4** (ORANGE-014): the set key's coverage is recorded at `setKey`.
  - `encoding.Reader.Count` refuses a count past `int` on a 32-bit build.
- **Why:** B1 was a reproduced hard fork: a node holding a heavier
  branch's entries without their layers never switched (D115). C-1 to C-3
  could, by reading, keep a heavier ledger out of reach in narrow cases.
- **Tier:** Implementation and wire (new message types; no byte form of
  an entry, transaction or layer changes, and no rule of validity or
  selection).

### D122. Round 8 adoptions: GREEN-032, ORANGE-014, ORANGE-015, ORANGE-016

- **Decision:** GREEN-032 (`nodepolicy.Grind.IsZero` counts every field,
  so a section of only `layer_floor` or `layer_rate` is saved and
  checked; the `-layer-floor` flag of the schedule's `set`, and `-layer-rate`
  added here; `poo CMD -h` exits 0). ORANGE-014 (`policy.Registry.CheckAll`
  checks many witnesses on every core and remembers the set as one key,
  on Black's conditions C-4). ORANGE-015 (`Engine.Tip` for events and
  stated weight, no branch walk). ORANGE-016 (`grind.layer_rate`, 10 a
  second by default, strongest first), combined with BLUE-008's per-search
  and per-second caps (D120): the stricter of the two applies.
- **Why:** Measured in Round 7 Part B and cleared by Black.
- **Tier:** Implementation (node policy).

### D123. Round 8 fixes: RED-020, RED-021, RED-022 (BLUE-009)

- **Decision:**
  - **RED-021.** `askLayers` gives each answer, and each page of an
    entry's many layers, to the caller as it arrives. `checkWork` counts
    every layer as it comes and stops paging once the work counted beats
    the node's; `topUpLayers` takes each page in as it comes. Only the
    count of nonces read is kept between pages. Nothing caps the layers:
    an entry whose work needs every page gets every page (D115, D121).
  - **RED-020.** Under the engine's lock a layers-from or parts reply
    only counts the entry's layers (`Pool.LayerCount`) or copies its
    nonces unsorted (`Pool.LayerNonces`). The hashing and the
    strongest-first sort run in the reply closure, outside the lock, and
    the order is cached for one minute under the entry and its layer
    count, so the pages of one fetch sort once and a new layer makes a
    new order. A layers reply counts before it copies.
  - **RED-022.** Before each part of a large entry is asked for, the free
    space of the parts directory's disk is read. When the part would go
    into the reserve, the larger of `p2p.Config.PartsReserve` (1 GiB by
    default) and 2% of the disk, the chase yields with the parts it has
    and goes on from their offset once there is room. It is never
    refused, and no size is capped. When the space cannot be read, the
    fetch goes on as before.
- **Why:** RED-021: a sender's claimed layer count could make a node hold
  every nonce of one entry before it counted any. RED-020: sorting
  millions of layers under the engine's lock stalled every peer. RED-022:
  a claimed size could fill the disk the store needs.
- **Tier:** Implementation (node policy). No consensus change: no rule
  of validity or selection, and no byte form, changes. For Black: every
  change paces or stops reading once the answer is known; none refuses a
  heavier ledger, and a fetch at the disk reserve waits rather than fails.

### D124. Long chases and paged layers always go on (Black's B2, B3 and C-5)

- **Decision:**
  - **B2.** ORANGE-018 is adopted: a chase counts the requests it sent
    each sender over `p2p.Config.RequestPaceWindow` (a minute) and
    yields, with its progress kept, at `RequestPace` (480, under the 600
    a minute peers serve by default). A request that times out ends the
    chase as the new outcome `chase.Stalled`: the sender is backed off
    as after Nowhere, but it stays in the group and the chase's progress
    is kept, so the next chase with it goes on from where this one
    stopped. A sender with a budget below the pace, or a slow link, only
    delays the chase.
  - **B3.** Paging one entry's many layers (`layersFrom`) checks the time
    slice and the pace before each page after the first, and yields with
    the entry and its offset kept in the chase's progress. The order is
    checked across pages in constant memory: within a page each layer
    comes after the one before it (stronger first, the lower nonce first
    among equals); a page may start with layers read already, as layers
    that arrive meanwhile push read ones later, but no more of them than
    the stated total grew by, and the run must reach the furthest layer
    read before any layer past it. A total that shrinks, a page past the
    total, or any other order is false (the sender is flagged, nothing is
    bad-listed).
  - **C-5.** `askLayers` reports how many hashes, from the first, it
    counted whole. Step 4 moves on only past those: an entry it stopped
    in the middle of keeps its offset, and the entries after it are asked
    for again when the chase comes back to step 4 (C-1). The early stop of
    RED-021 stays.
  - **RED-022's condition.** A chase that waits for disk space logs a
    warning, its group's new `waiting` and `waiting_since` fields show it
    in `poo chases`, `poo doctor` warns about it, and the metric
    `poo_chases_waiting_disk` counts it. Every yield or stall records its
    reason in `waiting`.
- **Why:** Black's Round 8 review (docs/consensus/source/ROUND-8.md).
  Under the default budgets a long heavier branch overran the sender's
  budget, timed out and started again for ever (B2, measured by Orange),
  and a sender could hold a chase slot by paging one entry for ever (B3).
  The early stop could skip uncounted entries of a one-page branch, which
  brought back Round 7's C-1 (C-5). All three break D115.
- **Tier:** Implementation (node policy). No rule of validity or
  selection, and no byte form, changes. Tests:
  `TestChaseBelowSenderBudgetIsReached`, `TestHeavierLedgerLongBareRun`
  (100,000 bare entries with the default budgets, in CI's long step),
  `TestManyLayersPagingYieldsAndGoesOn`, `TestManyLayersRepeatedAreFalse`
  and `TestEarlyStopCountsTheRestLater` (the last three fail on the Round
  8 code), and `TestStalledKeepsTheSender`.

### D125. Grind schedule changes are made on the node (GUI-014)

- **Decision:** `POST /v1/grind/schedule/edit` takes a
  `nodepolicy.GrindEdit` (remove, remove all, add, set) and applies it to
  node.json's grind section under the interface's policy lock, so two
  changes made at once both take effect. A numbered removal names the
  whole job, which must still be at that number, so a stale number never
  removes another job, even one with the same target. `GET` and the
  replies carry the section's `version`; a `PUT` with `If-Match` replaces
  only that version, and an edit may name one too. A stale change fails
  with code `conflict`. `poo grind schedule add|remove|set` sends edits;
  without a running node it applies the same edit to node.json.
  `rpc.Client` gains `GrindEditSchedule` and `GrindReplaceSchedule`.
- **Why:** GUI-014 (GUI Red, PR #39): a client read the section and wrote
  it back whole, and RED-013's lock only ordered the writes, so a removal
  and an addition made at once (the GUI and a shell) lost one of them.
- **Tier:** Implementation (node policy, the RPC). Grinding schedule only.

### D126. Round 9 adoptions: ORANGE-017, GREEN-033

- **Decision:** ORANGE-017 (a counting writer gives `Entry.Size` and
  `Tx.Size`, encodings are presized, and a kept entry's size is checked
  before it is encoded, so a large item is encoded once). GREEN-033 (the
  daemon puts large-entry parts in `<data dir>/parts`, mode 0700, and
  removes `poo-parts-*` leftovers when it starts; `layer_rate` documented
  in nodepolicy.md).
- **Why:** Measured or found in Round 8 Part B and cleared by Black.
- **Tier:** Implementation (node policy). The bytes are unchanged
  (`pkg/vectors` and the round-trip tests pass).

### D127. `grind.layer_rate` is at most 50 (GUI-015)

- **Decision:** `nodepolicy.MaxLayerRate` is 50 layers a second, down
  from 1,000. A section asking for more does not validate, so `poo grind
  schedule set -layer-rate`, the RPC and the GUI's form refuse it.
- **Why:** GUI-015 (GUI Red, PR #42): peers take 6,000 layers a minute
  from one node by default (100 a second) and flag a node that keeps
  sending more once a minute, which times it out after about 10 minutes
  and blocks it for 24 hours after about 30. Half the budget leaves room
  for the node's other layers and the ones it forwards.
- **Tier:** Implementation (node policy). It bounds only what this node's
  own grinding adds with `layer_floor`; nothing it accepts or fetches
  changes, and the chase still fetches any layers it needs.

### D128. Round 9 fixes: RED-023, RED-024, RED-025 (BLUE-010)

- **Decision:**
  - **RED-023.** A yielded chase's progress, its parts file included, is
    forgotten once its sender has been disconnected for
    `p2p.Config.ProgressGrace` (2 minutes) or no chase has taken it up for
    `ProgressTTL` (an hour). The sweep runs at each save, at each
    disconnect and every `CheckEvery`. A sender back within the grace goes
    on from what it left. A fetch that would wait for disk space first
    removes the parts files of senders that are not connected, whatever
    their grace, and checks again.
  - **RED-024.** `fetchParts` keeps its parts whenever the chase keeps its
    progress (`Outcome.Keeps`: a yield, or a stall such as a timeout), so
    the next chase goes on from the offset it reached. A false entry, a
    finished one, or a chase that ends otherwise lets them go.
  - **RED-025.** `nodepolicy.Load` brings a `grind.layer_rate` over
    `MaxLayerRate` down to it and notes it in `Policy.Lowered`; the node
    logs a warning and `poo doctor` shows one. Writes stay strict. A bound
    is only ever lowered this way.
- **Why:** RED-023: a sender that left kept disk and memory until the
  node restarted, and with D123's disk reserve could keep honest
  large-entry fetches waiting for ever. RED-024: one timeout threw away a
  large entry's parts. RED-025: lowering a bound made older valid files
  stop the node from starting.
- **Tier:** Implementation (node policy). No consensus change: no rule of
  validity or selection, and no byte form, changes. For Black: the grace
  and the time to live forget only kept progress, which a later chase
  rebuilds from the sender; nothing refuses a heavier ledger, and no
  limit is added to what a ledger may hold.

### D129. Kept progress outlasts the backoff (Black's C-6)

- **Decision:**
  - `ProgressTTL` is counted from when the progress was saved or, if
    later, from when its sender's backoff ends, so a stalled chase's
    progress is never forgotten while it waits for its next turn,
    however long the backoff has grown.
  - A stall after the chase took something in (entries, layers, part
    bytes or counted work, compared by a snapshot of the progress) is the
    new outcome `chase.Slowed`. The sender waits the first backoff step,
    its doubling starts again, it stays in the group, and the progress
    is kept. A stall that took nothing in still doubles the backoff.
  - `ProgressGrace` now removes only a departed sender's parts file. The
    rest of its progress, which costs little memory and is capped at 256
    chases, goes at the time to live.
- **Why:** Black's Round 9 review (docs/consensus/source/ROUND-9.md): with
  BLUE-010's time to live of an hour against a backoff doubling to an
  hour, a sender that stalls every time lost its progress just as its
  next chase came due, so a branch needing more than one chase's worth of
  transfer was never reached from it (B2 again, for slow senders).
- **Tier:** Implementation (node policy). It changes only when progress is
  forgotten and how long a sender waits. Tests:
  `TestSlowSenderIsReachedOverManyChases` (fails on the BLUE-010 code),
  `TestProgressOutlastsTheBackoff`, and `TestDepartedSenderLeavesNothing`
  (now: the parts file goes at the grace, the progress stays).

### D130. Round 10 adoptions: ORANGE-019, GREEN-034, the grind limits

- **Decision:** ORANGE-019: the default `p2p` `HashLimit` is 4,096, what
  peers already serve, so a long listing takes 16 times fewer requests
  under the pace (a 100,000-entry bare run in about a second instead of
  two minutes); `WeightlessPages` stays at 64 pages, as Black prefers.
  GREEN-034: `poo grind schedule` prints the section's version, and
  `add`, `remove` and `set` take `-if-version V`. The schedule replies
  carry `limits` (`nodepolicy.Limits()`: the most jobs, threads,
  `layer_floor` and `layer_rate`, and the default rate), so the GUI
  offers exactly what validates (GUI/Blue's ask). `remove -target` is
  now described as a check on what was meant, since the removal names
  the whole job (D125).
- **Why:** Measured or found in Round 9 Part B and cleared by Black.
- **Tier:** Implementation (node policy, the RPC and the CLI).

### D131. Round 10 fixes: RED-026, RED-027 (BLUE-011)

- **Decision:**
  - **RED-026.** `progress.counted` holds the entries whose layers were
    counted, not a hash for each layer. An entry listed again is not
    counted again. Within one reply an entry's layers are told apart by
    nonce, and `layersFrom` passes on only layers not read before, since
    D124's order check already tells which ones in a page repeat. So kept
    progress holds nothing for each layer.
  - At `ProgressGrace` a departed sender's progress loses its parts file
    and is cut down to its common entry (a topping-up progress, which
    holds a page at most, keeps the rest). It is kept to `ProgressTTL` as
    D129 says, so C-6 holds. A sender that comes back lists and counts
    again from the common entry.
  - **RED-027.** A fetch about to wait for disk space removes, one at a
    time and lowest first, the parts files of kept chases that rank below
    it, connected or not, until there is room. The order is:
    - a departed sender's;
    - then one whose last chase asked for parts and took none;
    - then less shown (C-2's order);
    - then fewer bytes held.
    A chase that ranks at or above the waiting one keeps its parts, so the
    first large entry to fetch always gets its room, and two fetches never
    take turns throwing each other's away.
- **Why:** RED-026: a sender could leave behind about 53 bytes of kept
  progress for each 8-byte layer it sent, for an hour. RED-027: a
  connected sender that stopped sending held the disk reserve, so no
  other large entry was fetched.
- **Tier:** Implementation (node policy). No consensus change: no rule of
  validity or selection, and no byte form, changes. No new limit.
- **For Black:**
  - Nothing here refuses a heavier ledger. What is forgotten is asked
    for again from the sender.
  - The host-wide byte budget Red suggested is not added. It would let
    cheap worked progress make an honest chase that is still counting
    forget its count over and over (D115).

### D132. Black's B4: a trickled claim never holds the disk reserve

- **Decision:** `reclaimParts`' last tie-break (after a departed sender,
  an idle one, and less shown) is now *more bytes still to come ranks
  below*, not *fewer bytes held*. A sender that lists the heavier ledger
  (so it has shown as much as an honest one), claims a huge size for one
  of its large entries and trickles the bytes always has more to come
  than an entry being fetched whole, so its parts give way to the honest
  fetch at the reserve, and its own chase can no longer reclaim the
  honest file. To rank higher it must claim a size no larger than the
  honest one's remainder, and then send all of it, after which the hash
  check decides. Between two honest fetches the one nearer its end keeps
  the room, and it only gets nearer, so they never take turns.
- **Why:** Black's Round 10 review: with the old order the trickler held
  the reserve against every honest fetch of that entry for as long as
  the trickle lasted, so the heavier ledger was never taken (D115). Of
  Black's three shapes, this is the smallest: one parts file per entry
  needs shared state across senders, and turns let the trickler keep
  half the room.
- **Tier:** Implementation (node policy). No rule of validity or
  selection, no byte form and no limit changes. Test (P0):
  `TestTricklerCannotHoldTheReserve` (fails on `31bdf06`, the code
  before it).

### D133. The asker learns each sender's request budget

- **Decision:** When a second request in a row to a peer goes
  unanswered, with no reply from that peer since either was sent, the
  pace to that peer becomes three quarters of what was sent to it in the
  last `RequestPaceWindow`, for 30 windows from the last such drop
  (`reqWindow.dropped`, `paced`). A lower learned pace replaces a higher
  one, and it never rises while it lasts; it applies even when
  `RequestPace` is off. One unanswered request alone, or one sent before
  a later reply, teaches nothing, since it is as likely lost or slow.
- **Why:** Black's Round 10 condition on
  `TestSlowSenderIsReachedOverManyChases`, which guards C-6 and flaked
  under `-race` and load. The cause was real node behaviour, not the
  test: a sender whose request budget is below the asker's pace drops
  the excess and flags the asker once per budget window; ten flags time
  the asker out, the sender then refuses it for the time-out's length,
  and a branch longer than ten windows was not reached from it (D115).
  With the learned pace the asker stays under the budget after two
  drops, so the flags stop at one or two per 30 windows, well below the
  time-out. The test now matches b's pace window to a's budget window,
  counts chases that went on later (slowed or paced), and checks that a
  never timed b out; 30 of 30 runs passed ten at a time under `-race`.
- **Tier:** Implementation (node policy, the asker only). It only slows
  this node's requests. Tests: `TestReqWindowLearnsThePace`,
  `TestSlowSenderIsReachedOverManyChases`,
  `TestChaseBelowSenderBudgetIsReached`.

### D134. GUI-017: offline schedule edits take the data directory's lock

- **Decision:** With no node answering, `poo grind schedule add`,
  `remove` and `set` change `node.json` under `nodepolicy.Lock(dir)`,
  taken without waiting. A running node holds that lock, so an edit is
  refused, and `node.json` left as it was, when a node answered the dial
  but its schedule could not be read, or when a node is starting or
  another command is writing. A leftover `rpc.addr` from a crash holds no
  lock, so the edit goes on. Viewing takes no lock.
- **Why:** GUI/Red's GUI-017: the CLI fell back to writing `node.json`
  whenever reading the schedule from the node failed, which could write
  behind a running node (its next write undoes the change, or the change
  undoes one of its own), and two offline writers raced.
- **Tier:** Implementation (the CLI). Test:
  `TestOfflineScheduleEditTakesTheLock`.

### D135. Round 11 adoptions: ORANGE-020, GREEN-035

- **Decision:**
  - ORANGE-020 as written: a catch-up reply's signatures are checked on
    every core before the entries are applied in order.
  - GREEN-035:
    - A node runs `grind.UserJobs` (4) jobs past a full schedule
      (`grind.MaxJobs` is 20, the schedule still at most 16), so a
      schedule of 16 jobs, even all waiting, never blocks a job a user or
      follower starts. Green's option (b), a schedule cap of 12, was not
      taken: it would have refused files that hold 13 to 16 jobs.
    - A schedule edit that changes only the jobs applies the difference:
      a running policy job still scheduled keeps its ID, tries and rate,
      and only jobs taken out stop, with the reason "the schedule
      changed". A changed default restarts them all. The schedule
      reply's `started` lists the running job of each scheduled job,
      kept or new, in the schedule's order.
    - `Policy.Lowered` is a list of `nodepolicy.Lowered` (the field, the
      value written, the value used and `FixArgs`), and `poo doctor`
      offers `grind schedule set -layer-rate 50` as the fix_args of a
      lowered `layer_rate` (GUI/Grey's ask).
    - The prototyped bench reason and the `node` log component, as
      written.
- **Why:** Measured or found in Round 10 Part B and cleared by Black.
- **Tier:** Implementation (node policy, the grinder and the CLI).

### D136. Round 11 fixes: RED-028, RED-029, RED-030 (BLUE-012)

- **Decision:**
  - **RED-028.** The first pace learned from a sender is a probe. It
    doubles once in each `RequestPaceWindow` in which the sender replies,
    and ends when it reaches `RequestPace`. The pace holds as D133 says,
    and is remembered as the sender's known budget, when either:
    - two more requests in a row are dropped while it is still probing; or
    - after a pace lapses, a drop comes with as much sent as the known
      budget.
    A held pace is lowered only by drops that came with at least half of
    it sent. So a sender that was only slow, or lost two requests, is soon
    served at the pace it allows, and a sender with a smaller budget
    costs at most one more round of drops when it is first learned.
  - **RED-029.** At the disk reserve, a kept parts file ranks as slow, just
    above a departed sender's, when the last chase that asked it for parts
    took in less than `partsMoving` (1 MiB, a sixty-fourth of a parts
    reply) or a parts request timed out. This is kept on the progress, so
    a waiting chase is ranked by its own last chase too. A sender that
    trickles a part a chase gives way to a fetch that is moving, whatever
    size it claims. Among fetches that are moving, D132's order stands.
  - **RED-030.** `poo follow`, `poo unfollow` and `poo init -force` write
    `node.json` only under `nodepolicy.Lock`, through the helper
    `lockPolicy` that D134's schedule edit now uses too. When a node is
    holding the lock, the write is refused and nothing is changed.
    `poo wallet follow` changes the follow list through the node when it
    answers (the series' rule is removed, then the wallet's pools are
    added and applied), and otherwise writes under the same lock.
- **Why:**
  - RED-028: two lost requests pinned an asker to one request a window
    for 30 windows.
  - RED-029: claiming just under the honest size defeated D132's
    tie-break.
  - RED-030: these commands could write `node.json` behind a running
    node.
- **Tier:** Implementation (node policy and the CLI). No consensus
  change: no rule of validity or selection, and no byte form, changes.
  No limit is added: `partsMoving` only ranks, and the pace only spaces
  requests.
- **Tests:**
  - `TestLearnedPaceRisesUnlessShown`;
  - `TestReqWindowLearnsThePace`, `TestSlowSenderIsReachedOverManyChases`
    and `TestChaseBelowSenderBudgetIsReached`, all still passing;
  - `TestTricklerJustUnderCannotHoldTheReserve`;
  - `TestPolicyWritesTakeTheLock`.

### D137. Black's B5: a slow fetch waiting for disk gets its turn

- **Decision:**
  - A slow chase (`progress.partsSlow`) that has waited `partsTurn` (two
    of its sender's first backoff steps) for room on the parts' disk has
    its turn in `reclaimParts`. It ranks as moving against a moving file,
    and above a slow file unless that file's last chase took in more than
    its own last chase did (`progress.partsLast`). A chase whose only
    parts request was lost (it took in nothing yet) has its turn against
    any slow file.
  - With its turn it asks for parts. If it takes in `partsMoving` (1 MiB)
    it is moving and keeps the room against slow files. If it is slow
    again, the next slow chase's turn may take the room back. So two slow
    fetches take turns, and between them the one that takes in more each
    turn keeps the room.
  - A parts request that timed out after `partsMoving` had arrived no
    longer makes the chase slow. The bytes came, so an honest link too
    slow for a whole 64 MiB parts reply within `RequestTimeout` (under
    about 2 MB/s) is moving (Black's third shape).
  - `partial.timedOut` is gone. `progress.partsWait` is set when a chase
    first waits for room, and cleared once it asks.
- **Why:** Black's Round 11 review (B5, reproduced): a waiting chase
  cannot ask for parts, so a slow flag set by one lost request never
  cleared. A slow trickler with less still to come then held the reserve
  for good, and the heavier ledger's large entry was never fetched
  (D115). Black's appendix test, `TestBlackSlowHonestCannotReclaimFromTrickler`,
  failed on the merged code before this change (the honest sender was
  asked for 1 part and the trickler for 22) and passes now.
- **What is left:** the turns compare what each fetch took in per chase.
  A trickler can take the room from an honest fetch slower than 1 MiB a
  chase only by sending as much each chase as the honest sender does,
  and a fresh sender gets one turn before it has a measure. Each turn
  costs it real bandwidth and a wait of two backoff steps. No case is
  left in which a fetch waits for good. For Red in Part B: an honest
  sender under about 35 KB/s against a trickler, and a stream of fresh
  senders.
- **Tier:** Implementation (node policy). It changes only which kept
  parts file gives way at the disk reserve. No rule of validity or
  selection, no byte form and no limit changes. Tests (P0):
  `TestBlackSlowHonestCannotReclaimFromTrickler`, `TestPartsRankTurns`,
  `TestTricklerCannotHoldTheReserve` and
  `TestTricklerJustUnderCannotHoldTheReserve`.

### D138. Round 12 adoptions: ORANGE-021, GREEN-036

- **Decision:**
  - ORANGE-021 as written: on restart, `node.load` decodes the store
    1,024 records at a time, and `Engine.WarmStored` checks their
    witnesses' signatures on every core before the window is replayed
    in order. It is used only for the node's own store. The address and
    every other check still run during the replay, so a tampered store
    is refused as before.
  - GREEN-036 as written: when `-root`, `-policy` or `-dir` chose the
    policy, `poo doctor` adds `-dir DIR` to every `fix_args`, so a fix
    changes the policy that was checked. A schedule change to
    `layer_floor` or `layer_rate` restarts no job, since the grinder
    reads both at each search. Only `device` and `threads` restart jobs
    (this refines D135).
- **Why:** Measured or found in Round 11 Part B and cleared by Black.
  ORANGE-021: restart with 20,000 entries went from 1.7–2.2 s to
  0.86–0.95 s on 4 cores.
- **Tier:** Implementation (the node's start-up, `poo doctor` and the
  grinder).

### D139. Round 12 fixes: RED-031 to RED-034, GUI-018 (BLUE-013)

- **Decision:**
  - **The disk reserve is shared by paid cuts, not by ranks.** This
    replaces the reclaim order of D131, D132, D136 and D137
    (`reclaimParts`, `partsRank`, `partsSlow`, `partsTurn`,
    `partsMoving` and ORANGE-022's `finishesFirst` are gone).
    - A chase that finds no room for parts asks anyway if another kept
      parts file may give way (`mayTake`). Room is then taken only for
      bytes that have arrived: each part that comes in cuts that many
      bytes off the end of another kept file (`takeRoom`,
      `partial.cut`). The cut file asks for them again on its next
      chase. A part that arrives with no file to cut is dropped and
      asked for again later.
    - Which file gives way (`takeFrom`): first one whose sender has
      left; then one whose last chase took in nothing or that has not
      been chased for two backoff steps (`partsStale`); then one that
      would finish later than the waiting fetch. "Finish" is bytes still
      to come over a rate in bytes a second: the file's from its last
      chase (`progress.partsRate`, measured at each save), the waiting
      fetch's from what it is receiving now. Between equal times, the
      file nearer its end keeps the room.
  - **RED-034.** Drops while a learned pace is probing show the budget
    only when at least seven eighths of what was sent at the first drops
    (`reqWindow.peak`) was sent again. With less, the probe was holding
    the asker low: the probe halves, not below where it began
    (`reqWindow.start`), and goes on rising.
  - **GUI-018.** `POST /v1/follow` takes `replace`, which puts the given
    pools and mints in place of the series' rule in one change under
    `policyMu` (`nodepolicy.ReplaceFollow`). `poo wallet follow` sends one
    such request; with no node it uses the same function under the lock.
- **Why:** Red's Round 12 findings showed that each rank of D137 had a
  shape an attacker could hold: a trickler that takes 1 MiB a chase and
  claims just under the honest size (RED-031), a stream of fresh senders
  each ranked as moving (RED-032), a trickler one part faster a chase
  than a slow honest link (RED-033). ORANGE-022 showed two honest slow
  fetches throwing each other's files away. With paid cuts a sender
  takes only as much room as it delivers, and keeps it only by
  delivering faster than the fetch it holds back would finish. A claimed
  size never decides. The fetch that would finish first keeps the room,
  as ORANGE-022 asked, and the others resume where they were cut.
- **What is left:** a sender that delivers as fast as the honest fetch
  can hold it back; that is a bandwidth contest, which costs the
  attacker the honest sender's bandwidth for as long as it lasts. A
  short burst takes only the bytes it sends. Two outages at the same low
  load still hold the pace at about three quarters of that load, as
  D133 says of a real budget. No case is left in which a fetch waits for
  good, and nothing is refused for its size (D115). For Black in Part D:
  the takeRoom redesign replaces B4's and B5's ranks.
- **Tier:** Implementation (node policy). It changes only which kept
  parts file gives way at the disk reserve, the learned pace and how
  the follow list is edited. No rule of validity or selection, no byte
  form and no limit changes. Tests (P0): `TestTakeFrom`,
  `TestMovingTricklerJustUnderCannotHoldTheReserve`,
  `TestSecondOutageWhileProbingKeepsProbing`, `TestReplaceFollow`, the
  replace case in `pkg/rpc`'s view test, and the existing B4, B5,
  ORANGE-022, trickler and slow-sender tests.

### D140. Black's B6: the disk reserve goes by rates over whole chases

- **Decision:** BLUE-013's paid cuts (D139) stay: room is taken only for
  bytes that arrive, cut from the end of another file, so a cut file
  resumes rather than starting over. What decides a cut changes, as
  Black's Round 12 review set out:
  - **Rates, not times to finish.** A waiting fetch may cut a file whose
    sender delivers at least `partsMargin` (1.5) times more slowly than
    it does. Within that margin, the file with more still to come gives
    way. So a claim, and how much is still to come, never decide against
    a much faster sender. Departed, idle and stale files give way first,
    as in D139.
  - **Whole chases, never bursts.** A rate is the bytes of parts
    received, kept or not, over the time the requests took, lost ones
    included. It counts, is saved and decides a cut only once a request
    was lost, a full parts reply (64 MiB, or the rest of the entry)
    arrived, or the chase's slice ended. Until then a waiting fetch with
    no measure keeps asking for the part at its offset and throws it
    away. A waiting fetch whose measure is older than `partsStale` asks
    again to measure afresh, so a slow flag from one lost request never
    lasts (B5).
  - **One byte of room is not room.** A chase asks without a measure
    that could take room only when a whole part fits.
  - The measure is kept on the progress by `fetchParts` itself.
    `partial.tried`, `seen` and `dur` and `finishIn` are gone.
  - Black's appendix test is added as
    `TestBlackColludingTricklersHoldTheReserve`. Only its teardown
    changed: the raw senders now stop before their connections close, so
    the test no longer fails while it closes down.
- **Why:** Black's B6, reproduced: two colluding senders at about
  4 KB/s each claimed a little more than they held and cut each other in
  bursts. Each burst's instant rate "finished first", so no file ever
  completed, and an honest fetch was locked out. That broke D115.
- **What is left:** a holder keeps the room against an honest fetch
  only by delivering at least two thirds as fast, for as long as it
  holds it. That is the bandwidth contest D139 names. A fresh fetch
  spends up to one parts reply, or one slice, of the sender's bandwidth
  measuring before it can cut. For Red in Part B: three or more
  colluders, and a fresh sender at every chase.
- **Tier:** Implementation (node policy). It changes only which kept
  parts file gives way at the disk reserve. No rule of validity or
  selection, no byte form and no limit changes. Tests (P0):
  `TestBlackColludingTricklersHoldTheReserve`,
  `TestBlackSlowHonestCannotReclaimFromTrickler`,
  `TestTwoSlowFetchesBothFinish`, `TestTricklerCannotHoldTheReserve`,
  `TestTricklerJustUnderCannotHoldTheReserve`,
  `TestMovingTricklerJustUnderCannotHoldTheReserve` and `TestTakeFrom`.

### D141. Round 13 adoptions: GREEN-037; ORANGE-022 superseded

- **Decision:**
  - GREEN-037 as written: `/v1/status` reports `policy_changed` when
    `node.json` differs from the policy the node runs, ignoring the
    grind section (it applies live; `nodepolicy.Policy.SameApplied`).
    `poo doctor` warns with the fix `poo reload`.
  - ORANGE-022 is superseded, as Black ruled. Its finding (two slow
    fetches taking turns deleting each other's files) is closed by
    D139's cuts by bytes and D140's margin. Its test,
    `TestTwoSlowFetchesBothFinish`, is kept, and it runs in CI's long
    step.
- **Why:** Found in Round 12 Part B and cleared by Black.
- **Tier:** Implementation (status, `poo doctor` and node policy).

### D142. Round 13 fixes: RED-035, RED-036, RED-037; ORANGE-023 (BLUE-014)

- **Decision:**
  - **RED-035, with ORANGE-023 as written.** A probe at the disk
    reserve (D140) throws away the whole reply it is in, not one
    chunk. Each parts request checks every part against the offset it
    expects next (`want`), which moves with every part received, kept
    or not, so the chunks after one thrown away are never taken for
    parts out of order and an honest sender is never flagged false.
  - **RED-036.** Drops count toward a held pace only if the sender
    answered something sent in the window before the drops began (more
    requests in the window than misses in a row). A sender over its
    budget has served the budget first; one that answered nothing in the
    window is out. Such drops never hold, lower or confirm a known pace:
    a probe steps back and goes on rising.
  - **RED-037.** A waiting chase whose measured chase wrote nothing asks
    again only once its measure lapses: `partsStale` at first, doubled
    for each such chase in a row (`progress.partsMiss`), up to eight
    times (`measureLasts`). A chase that writes resets it.
- **Why:** Red's Round 13 findings and Orange's ORANGE-023. RED-035 was
  high: at the reserve every honest sender of a large entry whose reply
  held more than one chunk was flagged false and in a few chases blocked
  for a day, so the heavier ledger's large entry could not be fetched
  (D115). Not taken from RED-036: a hold only above `RequestPace/4`,
  since a sender whose budget is below that would then be overshot every
  few windows and time this node out (D133). Not taken from RED-037: a
  record of a cut file's old offset. A cut file already resumes where it
  was cut, and the bytes cut are gone.
- **What is left:** a sender that answers one request in a window and
  then loses two still looks like a budget of about that load, as D133
  says of a real budget. A waiting sender that has sped up waits up to
  eight times `partsStale` (32 minutes at the defaults) to show it,
  unless the holder slows, stalls or leaves. A fast fresh sender still
  cuts a slow file at its own rate: the bandwidth contest D140 names. No
  case is left in which a fetch waits for good, and nothing is refused
  for its size.
- **Tier:** Implementation (node policy). It changes only how parts are
  checked while probing, the learned pace and when a waiting chase asks.
  No rule of validity or selection, no byte form and no limit changes.
  Tests (P0): `TestProbeNeverFlagsAMultiChunkSender` (fails on 97456c1),
  `TestProbeKeepsAnHonestSender` (ORANGE-023), `TestOutageAtLowLoadNeverHolds`,
  `TestMeasureLastsLongerAfterMisses`, and the B5, B6, ORANGE-022,
  trickler and slow-sender tests.

### D143. Round 14: GREEN-038 adopted; three commands for parity; notes on D140

- **Decision:**
  - **GREEN-038 as written.** `poo follow -series S ... -replace`
    follows exactly the given pools and mints in place of the series'
    rule, in one change, through the node or under the lock offline,
    as the GUI's `POST /v1/follow` with `replace` does (GUI-018, D139).
    `TestEveryClientCallHasACommand` fails when an `rpc.Client` method
    has no `poo` command and is not listed with a reason.
  - **Its three proposals, built.** `poo submit entry` and
    `poo submit layer` hand the node an entry or a layer made elsewhere,
    in the hex its API prints. Each is decoded before the node is asked
    and then checked as one from a peer is: the command adds no rule and
    skips none. `poo grind jobs ID` prints one job, as the GUI's job view
    reads it. `submit` is a write command, refused under a view-only
    token. Three calls remain listed in `noCommandYet`, each with its
    reason.
  - **The accepted residual of D140, stated in `docs/modules/p2p.md`.**
    A holder that delivers at two thirds of the honest rate is inside
    the 1.5 margin. It keeps the room only by sending at that rate for
    as long as it holds it. That is a bandwidth contest it pays for, not
    a hold. The honest node pays for probing: at most one parts reply
    (64 MiB) or one slice per measure, once per lapse of that measure.
  - **Black's note on Round 13.** `RequestTimeout` (30 seconds) stays
    below `ChaseSlice` (2 minutes), so one lost request never outlasts
    the slice that its chase's measure is taken in. The rule is stated
    on both fields and checked by `TestRequestTimeoutBelowChaseSlice`.
    It must hold if either one ever becomes a setting.
- **Why:** jerry's hard rule is that the headless node and the GUI never
  diverge. Green's Round 13 sweep found the GUI-only calls, and Black
  cleared GREEN-038 and asked for the timeout note.
- **Tier:** Implementation (the command line and docs). No rule of
  validity or selection, no byte form and no limit changes. Tests:
  `TestSubmitEntryAndLayer` (an entry and a layer from one node selected
  on another through `poo submit`), `TestGrindCommands` (`grind jobs
  ID`), the view-only scope test, `TestFollowReplace` and
  `TestEveryClientCallHasACommand`.

### D144. Round 14 fix: RED-038 (BLUE-015)

- **Decision:**
  - `poo submit entry|layer` takes the hex as its one argument, as
    before, or from a file (`-f FILE`) or standard input (`-`, or
    `-f -`), whole and trimmed (`submitHex`).
  - The RPC's `MaxBody` (8 MiB) no longer bounds `POST /v1/entry` with
    the full token. Every other request, and every request with the view
    token, is bounded as before; the view token is refused before the
    body is read.
- **Why:** Red's Round 14 finding. Linux limits one argument to 128 KiB,
  so an entry over 64 KiB could not be submitted with `poo`, while the
  GUI could. Testing a large body through the real RPC, as the
  coordinator asked, showed a second bound: `MaxBody` refused any entry
  over about 4 MiB from both `poo submit` and the GUI. The node takes an
  entry of any size from a peer, in parts (D117), so its owner must be
  able to hand it one too (jerry's headless-and-GUI rule; D115's spirit:
  no size cap on what the node can take). Consistent with GREEN-039's
  `-bytes`, whose output is what `poo submit` reads.
- **What is left:** the node decodes a submitted entry whole in memory,
  as it does one from a peer (D117), and only the full token can send
  one. Not run with a valid entry over 64 KiB: the tests use the real
  path with built entries through a file and standard input, 300 KB of
  hex on standard input, and an entry body over `MaxBody` through the
  real RPC.
- **Tier:** Implementation (the CLI and the local RPC). No rule of
  validity or selection, no byte form and no consensus change. Tests:
  `TestSubmitEntryAndLayer` (file, standard input, 300 KB, `-f` with an
  argument), `TestSubmitEntryBodySize` (full token over `MaxBody`
  decoded; view token 403; a layer over `MaxBody` refused; no token 401).

### D145. Round 15: wallet saves never undo another change; new passphrases confirmed; GREEN-039 adopted

- **Decision:**
  - **GUI-019.** `wallet.Save` is a compare-and-write under a short
    lock. It locks `wallet.json.lock` beside the file (flock on Unix,
    LockFileEx on Windows, and no lock elsewhere), reads the file again,
    and writes only if the bytes are those `Open` read or the last
    `Save` wrote. Otherwise it writes nothing and returns `ErrChanged`,
    which tells the user to run the command again. A new wallet whose
    file appeared meanwhile gets `ErrExists`. So a `poo wallet` command
    left waiting at its prompt can no longer undo another command's
    change, a new passphrase included. Refusing was chosen over merging:
    the wallet file has no merge rule, and a merge that guessed could
    bring back the old sealed seed.
  - **GUI-020.** `poo wallet create` and `poo wallet passwd` ask for a
    new passphrase twice when it is typed through `Stdio.Secret` (a
    hidden terminal, or the GUI's masked field), and change nothing if
    the two differ. A passphrase piped on standard input, or given by an
    environment variable or a file, is read once, so scripts work as
    before.
  - **GREEN-039 as written,** merged with BLUE-015 (D144). `build` and
    `work` take `-bytes`, which keeps the hex that `poo submit` reads. A
    missing parent is named in the error, with what to do. `grind jobs
    ID` reads its flags after the id.
  - **Black's Round 14 note on memory.** `POST /v1/entry` decodes the hex
    straight from the body into bytes (`hexBytes`), so a large entry is
    no longer also held as a string twice its size. The JSON is
    unchanged.
  - **Not taken: Black's optional note that a waiting file's staleness
    should follow `measureLasts`.** A file whose chase writes nothing
    would then keep its measure up to eight times longer against a fetch
    that pays for every byte. Black and ORANGE-024 showed the current
    rule is a delay paid for in bytes, never a hold. Lengthening it would
    favour the file that is not moving, so the two clocks stay apart.
- **Why:** GUI-019 (medium) and GUI-020 (low) came from GUI/Red, PR #63,
  routed by GUI/Blue. GREEN-039 and the memory note came from Round 14,
  and Black cleared both.
- **Tier:** Implementation (the wallet file, the CLI and the local RPC).
  No rule of validity or selection, no byte form and no limit changes.
  Tests: `TestSaveNeverUndoesAnotherChange` and
  `TestConcurrentSavesOneWins` (pkg/wallet), `TestNewPassphraseIsConfirmed`
  (pkg/cli), and the hex cases in `TestSubmitEntryBodySize` (pkg/rpc).

### D146. Round 15 fix: RED-039 (BLUE-016)

- **Decision:**
  - `Wallet.Hold` takes the lock beside the wallet file until it is
    released and reads the file again, keeping the open seed. While a
    wallet is held, its `Save` does not take the lock again. A file that
    is no longer this seed's (its first key differs) or that changed its
    format or key algorithm is `ErrChanged`.
  - `poo wallet pay`, `claim`, `submit` and `resend` hold the wallet,
    after the passphrase prompt, from before they act on the node until
    after they save. Other commands' saves wait for the release; one that
    read the file before the hold is refused (D145), having sent nothing.
  - If the save after a send still fails, the command prints the record
    and says the payment or claim was sent and must not be sent again,
    in place of `ErrChanged`'s "run the command again".
- **Why:** Red's Round 15 finding. D145's compare-and-write refused the
  save of a payment that was already on the node when another command
  (the GUI, a second terminal) had saved in between. The record and the
  change address were lost, and the message said to run the command
  again, which paid a second time.
- **Not taken:** a merge of the new records into the changed file after
  the fact. With the hold there is nothing to merge, and a merge would
  have to decide between two commands that took the same key for
  different uses. A refusal to pay a request already paid was not taken
  either: the hold removes the cause, and with a record lost it would
  not fire.
- **What is left:** the lock is held while the node answers, so another
  command's save waits that long (seconds at most; never at a prompt).
  `lock_other.go` (systems with neither flock nor LockFileEx) has no
  lock, as before.
- **Tier:** Implementation (the wallet and the CLI). No consensus change.
  Tests: `TestPaymentHoldsTheWallet`, `TestSaveSentSaysDoNotRepeat`.

### D147. Round 16: the entry body's buffer grows; submit sends the hex as read; Windows locks node.json; waiting wallet commands keep other changes

- **Decision:**
  - **GREEN-040 as written.** `poo submit` reads a `-bytes` reply as it
    was printed (`entry`; `layer` or `layer.layer`), and bare hex as
    before. A missing wallet says how to make one.
  - **ORANGE-025, with the size hint changed.** `POST /v1/entry` reads
    the body once into one buffer and decodes it there (`readEntryBody`,
    with the same JSON rules as `decodeBody`). The buffer starts with
    room for the stated `Content-Length` up to 64 MiB (`entryHint`). It
    doubles as bytes arrive, and never grows past the stated length.
    ORANGE-025 reserved up to 1 GiB from the stated length. Blue asked
    for growth toward it and Black for a 64 MiB ceiling, and this does
    both. A false length now sets aside at most 64 MiB, or twice what was
    sent. An honest large entry costs a copy or two on the way in. There
    is still no bound on the size read.
  - **GUI ORANGE-012, taken into Source.** `poo submit entry` checks that
    the entry decodes, lets the decoded form go, and sends the hex as it
    was read (`rpc.Client.SubmitEntryHex`). It no longer encodes the
    entry again. The node gets the same JSON. GUI/Orange measured a 128
    MiB entry: 12 s became 5 to 7 s, and the client's peak heap went from
    1 GB to 625 MB.
  - **GUI-017's Windows residual.** `nodepolicy.Lock` takes a
    LockFileEx lock on Windows. It is refused at once when the lock is
    held, as on Unix, so an offline `node.json` edit is never written
    behind a running node there either. It is built and vetted for
    Windows. CI builds Windows but runs no tests there.
  - **Key-only wallet commands hold the wallet after their prompts.**
    This was the coordinator's question: retry once on `ErrChanged`.
    `passwd`, `address`, `watch`, `request`, `receive`, `status` and
    `policy` call `Wallet.Hold` (D146) just before they change the
    wallet, so the file is read again under the lock. A change saved
    while they waited at a prompt is kept, and nothing needs to be run
    again. A retry would have redone the command's work on a fresh read.
    Hold gives the same result with no second pass, and it is the path
    payments already use.
  - **Black's Round 15 review is merged** (7 clear, 0 blocked).
- **What is left:** GUI/Red measured the node's peak at about 1.3 GB on a
  128 MiB entry, before this round. The body (twice the entry, as hex),
  its decoded bytes and the decoded entry are each still held once.
  Orange can measure what remains after D147.
- **Why:** the Round 16 queue: BLUE-016, GREEN-040, ORANGE-025 with its
  open hint question, GUI/Orange's ORANGE-012 and GUI/Red's Windows
  residual (PR #67).
- **Tier:** Implementation (the wallet file, the CLI, the local RPC and
  the data-directory lock). No rule of validity or selection, no byte
  form and no limit changes. Tests: `TestReadSized` (pkg/rpc),
  `TestWaitingCommandKeepsAnotherChange` (pkg/cli, fails without the
  hold), `TestSubmitFromReply`, `TestEntryBodyReadOnce`, and the
  existing submit, wallet and lock tests.

### D148. Round 16 fixes: RED-040, RED-041, RED-042 (BLUE-017)

- **Decision:**
  - **RED-041.** One durable writer, `fsutil.WriteFile`: a temporary
    file of its own, the mode, `Sync`, `Close`, `Rename`, then the
    directory synced (skipped where it cannot be, as on Windows). The
    wallet, the peer book, the tracker, the policy manager and
    `nodepolicy.Save` (whose own code it was, D114) all use it.
  - **RED-040.** `rpc.Client.SubmitEntryHex` sets the body's
    `Content-Length`, so the node sizes its buffer by it (ORANGE-025,
    D147). ORANGE-026's change to `readSized` is on the node's side and
    independent of this.
  - **RED-042.**
    - `status` and `receive` ask the node without the wallet held.
      `status` holds it only to save the legs whose state changed, each
      only where the file still has the state it read, so a change made
      meanwhile (a resend) stands. `receive` holds it only to note what
      `-tid` names.
    - The lock is tried first without waiting. A command that must wait
      says so on its error output (`Wallet.OnBusy`, `holdWallet`), then
      waits.
- **Why:** Red's Round 16 findings. RED-041: the wallet file is the one
  record of sent payments (D146), and without syncs a crash could empty
  it after `Save` had returned. RED-040: a chunked body made the node's
  buffer double past the entry's need (748 against 492 MiB for a 100 MiB
  entry, Red's measure). RED-042: a read-only `status` waited silently
  behind a command waiting for the node.
- **Not taken:** a time limit on the wait (RED-042's second point). The
  holder is sending a payment and saving its record, and each of its
  node requests is bounded by the client's timeout. Giving up would
  leave the waiter's change unsaved, so the waiter says why it waits and
  goes on waiting. The send-then-save commands (`pay`, `claim`, `submit`,
  `resend`) still hold across their node calls. Their select-then-spend
  order needs it (D146).
- **What is left:** a `status` that has a leg to update still waits for
  a holder to save it, and says so. Not tested: a real crash or power
  loss, and Windows (built and vetted only).
- **Tier:** Implementation (storage, the wallet, the CLI and the local
  RPC client). No consensus change and no limit. Tests: `TestWriteFile`,
  `TestSubmitEntryHexSendsItsLength` (fails on b502960),
  `TestHoldSaysWhenBusy`, `TestHeldWalletWaitersSayWhy`.

### D149. Round 17: node policy export and import; GUI-022 fixed; GREEN-041 and ORANGE-026 adopted

- **Decision:**
  - **Bundles.** One JSON file, a bundle (`nodepolicy.Bundle`, kind
    `poo-node-policies`, format 1), carries node policies between nodes.
    Its scope is a series (one series, how it is followed, its issuer),
    a policy (one node policy) or a node (every saved node policy, each
    by name, with the current one marked). A `settings` member is
    reserved for the node's settings, which a later format adds. The
    owner asked for this on 2026-11-02, for sharing, several machines and
    backups.
  - **Export** (`poo policy export`) leaves out the machine-local
    settings (`nodepolicy.MachineLocal`: `rpc`, `no_sync`, `p2p.listen`,
    `p2p.advertise`, the grind device and threads) unless `-full`, and
    always the unused `fast_catch_up` and `depth_limit`. No secret is in
    a bundle, since none is in node.json.
  - **Import** (`poo policy import FILE`) prints a plan and changes
    nothing without `-apply`. FILE `-` reads standard input, so the GUI
    can hand over a file picked in the browser. The default merge only adds: new series,
    follows merged as `AddFollow` does, accepted issuers, peers,
    registries and grind jobs as sets, and every single setting the
    node leaves unset. Where both set a value and they differ, the node
    keeps its own and the plan lists the conflict. `-prefer-file` takes
    the file's, `-replace` takes the file's node policy whole, and
    `-full` takes machine-local settings too. A series bundle touches
    only series, follows and accepted issuers. A node bundle goes into
    each saved node policy by name, creating the missing ones, and is
    refused before any change if one of those names is archived. The
    result must validate, or nothing is written.
  - **Reading** refuses unknown fields rather than dropping them, a
    newer format, more than 16 MiB or 256 node policies, an invalid
    node policy or series, and bad or repeated names. A plain node.json
    imports as a bundle of one.
  - **A running node** takes an import through `POST /v1/policy/import`
    under the lock its follow edits use, and reloads. It never takes an
    `rpc` section, even with `-full`. Where the interface listens and
    who may reach it changes only in node.json with the node stopped,
    so a token holder cannot open the interface to the network. With no
    node running, the CLI edits node.json under the data directory's
    lock.
  - **GUI-022 (GUI/Red, medium).** `Wallet.Hold` no longer adopts a
    file whose seal changed since the command was unlocked: it returns
    `ErrPassphraseChanged` and the command does nothing. A passphrase
    rotation then revokes commands already waiting at a prompt, and a
    stale `passwd` never re-seals over the owner's change, as under
    D145. Other changes saved meanwhile are still taken in (D146).
  - **A feature list, docs/FEATURES.md (owner, 2026-11-02).** One plain
    file of what the project offers, one feature per row, with its
    command, its package and its decision. It has a Source part, which
    Source/Grey updates each Part A, and a GUI part, which GUI/Grey
    updates. It feeds the later docs site. Each feature stays in its own
    package behind a small API, with thin CLI and RPC layers over it, so
    it can be extended. Orange may challenge that for one piece where
    measurements show it really hurts performance, in a Part B proposal
    (AGENTS.md, Orange proposals; the owner, 2026-11-02).
  - **Merged with this round:** BLUE-017 (D148), GREEN-041 (help lists
    only the flags a command takes), ORANGE-026 (the entry body's buffer
    ends at its stated size) and Black's Round 16 review (10 clear, 0
    blocked).
- **Why:** the owner's feature request, and the Round 16 queue.
- **Consensus:** none. A bundle holds only node policy and is checked as
  node.json is. No import can add a depth limit (node.json has none) or
  make a heavier ledger unacceptable. Black reviews this in Part D.
- **What is left:** the GUI form (GUI Part A, after this merges); the
  settings export, when the owner asks; signing a bundle, so that a
  shared file can be checked against its publisher (not done: there is
  no key to sign with yet, and the plan already shows every series,
  peer and registry a file adds).
- **Tier:** Implementation (node policy, the wallet, the CLI and the
  local RPC). Tests: `TestExportStripsMachineLocal`, `TestBundleRoundTrip`,
  `TestReadBundleRefuses`, `TestMergeAddsAndKeepsLocal`,
  `TestMergeReplace`, `TestMergeSeriesOnly`,
  `TestMergeWitnessConflictAndRefused`,
  `TestMergeRefusesWhatWouldNotValidate` (pkg/nodepolicy);
  `TestPolicyExportImport`, `TestPolicyExportAllImportsEachByName`,
  `TestPolicyImportIntoRunningNode`, `TestPolicyImportKeepsRunningRPC`
  (pkg/cli); `TestHoldRefusesAStaleSeal` (pkg/wallet, GUI-022).

### D150. Round 17 fixes: RED-043, RED-044, RED-045 (BLUE-018)

- **Decision:**
  - **RED-043. An import takes what to follow, and node settings only
    when asked.**
    - `nodepolicy.Merge` takes from a file, by default, only `Followed`:
      series, follows and accepted issuers.
    - Every other setting is a node setting. That covers peers,
      registries, grind jobs, tracking, the p2p switches and limits, `k`
      and `issuers.any`. The plan lists them in `skipped_settings` and
      leaves them alone, whether merging or replacing.
    - `MergeOptions.Settings` (`poo policy import -settings`, the
      endpoint's `settings`, implied by `full`) takes them. The plan's
      `defences` then names each changed line that touches `Defences`,
      and the CLI prints those first.
    - A registry that is a local file comes in only with `full`.
    - This is an allow-list, so a setting added to node.json later stays
      out of a default import unless it is added to `Followed`.
    - The node's registry fetch refuses, after name resolution and on
      redirects, an address that is not public unless
      `allow_local_peers`, as a book candidate's dial does (D96). Through
      an environment proxy, a literal address is checked.
    - Export is unchanged.
  - **RED-044.** The file's follow entries are merged in one pass
    (`addFollows`) against the series built once. The result is the same
    as `AddFollow` one by one.
  - **RED-045.**
    - `poo policy import` plans every node policy first and applies only
      if all plans succeeded. A later failure names what was saved.
    - A full-token `POST /v1/policy/import` takes up to `MaxImportBody`,
      so any bundle the CLI reads fits.
    - New node policy names may not be Windows device names or end in
      `.` (`checkNewName`). Existing ones still load.
- **Why:** Red's Round 17 findings on D149. A shared file could turn off
  peer-steering protection (D96) or punishment, make the node fetch an
  internal address on every start, or start grind jobs. The only guard
  was a person reading a long plan in which those lines looked like any
  other. Taking only what to follow by default keeps the feature's main
  use (sharing what to follow, backups with `-full`) and makes every
  other change something the operator asks for.
- **Consensus:** none. Only node policy changes. No limit is added, and
  no import can make a heavier ledger unacceptable. Fewer settings come
  in from a file by default than before.
- **Tier:** Implementation (node policy, the CLI, the local RPC and the
  registry fetch). Tests are listed in BLUE-018.

### D151. Round 18: GUI-022 closed for every command; imports tied to the previewed file; GREEN-042 adopted

- **Decision:**
  - **GUI-022, the rest (GUI/Red, PR #86).** `Wallet.Unlock` reads the
    file again before it checks the passphrase. If the seal changed
    since `Open` read it, Unlock returns `ErrPassphraseChanged`, and the
    old passphrase opens nothing. This covers the commands that never
    hold the wallet: `wallet seed` and `wallet cosign`. A stale "Show
    seed" no longer prints the seed after a rotation. `Hold` keeps its
    own check (D149) for a rotation made between the unlock and the
    hold.
  - **The previewed file (GUI-023's Source half).** `ReadBundle` sets
    the file's SHA-256 as `Bundle.Digest`. `poo policy import` prints
    it, and with `-json` gives it as `digest` on every row. The new flag
    `-if-digest D` refuses, before any change, a file whose digest is
    not D. The GUI passes the digest it previewed with Apply, and the
    terminal can do the same. The GUI half, Apply sending the previewed
    text, is GUI/Blue's.
  - **Black's extreme-settings test is adopted** as
    `pkg/p2p/settings_extremes_test.go` (Black, Round 17 appendix). It
    sets `k`, `ties`, `max_pending`, `seen_size` and the four reply
    limits to their smallest, then their largest, values. In both cases
    a node still reaches a heavier ledger.
  - **Merged with this round:**
    - BLUE-018 (D150: RED-043 to RED-045), with Red's findings (PR #83).
    - GREEN-042 (grind `-series` help, plans that show values).
    - ORANGE-027 (the modular seams measured; no exception needed).
    - Black's Round 17 review (9 clear, 0 blocked).
  - **FEATURES.md:** imports gain `-settings`, `-if-digest` and `-`.
    `holdings` and `payment` are listed. The wallet row lists `seed`
    and `cosign`.
- **Why:** the Round 18 queue.
- **Consensus:** none. This round touches the wallet, the CLI and a
  test.
- **What is left:** the GUI form's half of GUI-023 (GUI/Blue). Two
  points from GREEN-020 are still open: errors name `until_from` where
  `-until-from` was typed, and no command renews a lease.
- **Tier:** Implementation. Tests: `TestUnlockRefusesAStaleSeal`
  (pkg/wallet; `TestSaveNeverUndoesAnotherChange` now expects the
  refusal at unlock), `TestPolicyImportIfDigest` (pkg/cli), and the
  extreme-settings test (pkg/p2p).

### D152. Round 18 fixes: RED-046 (in part), RED-047, RED-048 (BLUE-019)

- **Decision:**
  - **RED-046. A huge whole-series follow costs what is in use.**
    - The node finds its stores by reading `pools/`.
    - Status, metrics, queries and grind targets go through
      `Engine.ListedPools`. That lists every pool of a whole series under
      `ListAll` (65,536), and of a larger one only the pools with state.
    - The p2p host builds its pool index without a list of keys first,
      with maps sized once and no per-pool section text.
    - The plan states each touched series' pool count. A whole-series
      follow of `LargeFollow` (65,536) pools or more goes in `defences`.
    - No series parameter is capped and no follow is refused.
  - **RED-047.** `-if-digest`, when given, must be 64 hex digits and
    needs `-apply`.
  - **RED-048.** `Defences` is gone. Under `Settings` every node-setting
    line is named first except `Cosmetic` (`log.format`), the same
    inversion as `Followed` (D150).
- **Why:** Red's Round 18 findings. A default import could bring in a
  series of 2^32 or more pools followed whole, and the node then paid per
  pool at every start and every status call. An empty variable turned the
  digest check off. Settings nobody had thought of went unnamed.
- **Consensus:** none. Every pool stays followed, and a pool's state is
  still made when its first item or request arrives. No limit is added.
  Nothing changes what a node accepts.
- **What is left:** the p2p host's index is still one entry per followed
  pool. The wire names a pool only by its id hash, which cannot be turned
  back into an index. A pool index carried next to the id (a protocol
  change) is put to Black. It is not built.
- **Tier:** Implementation (node start-up, engine listings, the p2p index,
  node policy import, the CLI). Tests are listed in BLUE-019.

### D153. Round 19: the weight check cannot be turned off; GREEN-043 adopted

- **Decision:**
  - **The weight check is never off (Black's B7, as node policy).**
    - The problem: `check_seconds: -1` turned off the regular check of
      priority peers' stated weight. With peers that send no notices, a
      node in a quiet pool never learned of a heavier ledger they held.
    - Under the owner's definition (2026-11-03) this is a lack of
      information, not a hard fork. The node takes the heavier ledger as
      soon as it hears of it. The change is an improvement, not a rule.
    - Now `nodepolicy.MaxCheckSeconds` is 3600. A negative value, or one
      over that, is read as one hour. `Load` notes it in `Lowered`, as it
      does for `grind.layer_rate`, so `poo doctor` shows it. `p2p.FromPolicy`
      reads it the same way.
    - `ping_seconds: -1` stays as it is. No consensus rule changes.
  - **Merged with this round:**
    - BLUE-019 (D152: RED-046 in part, RED-047, RED-048), with Red's
      findings (PR #94).
    - GREEN-043: plan notes read like the plan's lines, and help names
      `-settings` and `-if-digest`.
    - ORANGE-028: imports are linear, and Unlock's check costs nothing.
      No cap is needed.
    - Black's Round 18 review: 8 clear, and B7 cleared as an information
      gap. It also writes the owner's hard-fork definition into
      AGENTS.md.
  - **FEATURES.md:**
    - It now says a node bundle is planned whole before anything is
      saved. A failed save names what was already saved, which is weaker
      than "all or nothing" (Green's note).
    - The weight check has its own row.
- **Why:** the Round 19 queue, and Black's optional B7 improvement.
- **Not taken:** Black's B7 test as written. With `-1` the first check
  now comes after an hour, so a 60 s test cannot wait for it. Instead:
  - `TestCheckSecondsNeverOff` checks that -1 and every value over the
    most give the hour, through both `FromPolicy` and `Load`.
  - Black's `TestBlackCheckFindsLaterHeavier` shows that the check finds
    a heavier ledger a no-notices peer holds.
  - `TestBlackNoCheckCaughtByNextEntry` shows that a new entry is
    enough.
  Wire protocol v2 is not built: the owner has not decided on it.
- **Tier:** Node policy and p2p configuration. Tests are in
  `pkg/p2p/check_never_off_test.go`.

### D154. Round 19 fixes: RED-049, RED-050, RED-051 (BLUE-020)

- **Decision:**
  - **RED-049.**
    - A pool's state is kept only once an item leaves something in it.
      Reads use a fresh state that is not kept.
    - The p2p host announces a series of `engine.ListAll` (65,536) pools
      or more followed whole as one section. It indexes that series'
      pools as they come into use: with state at start, an item kept, a
      branch offered. A pushed transaction or entry finds its pool by its
      first mint serial, checked against the section's pool id.
    - The sorted pool list is built only when the index changes.
  - **RED-050.**
    - `Follow.Except` means a whole series but those pools.
      `RemoveFollow` on a whole series writes `except`, or the pools left
      when they are fewer.
    - Joining follows follows what either follows.
    - The engine follows such a rule with `FollowSeriesExcept`, and p2p
      announces it as the whole series.
    - A follow change needing more than `MaxSections` (4096, the wire's
      coverage limit) separate sections is refused when made.
  - **RED-051.** The import digest is the SHA-256 of the file's digest,
    the merge options and the plans. `-apply -if-digest` recomputes it
    from a fresh plan before any change.
- **Why:** Red's Round 19 findings, on top of D152:
  - p2p still kept and sorted every pool.
  - A refused item left a pool's state behind.
  - All but one pool of a large series made the node refuse to start.
  - The digest did not bind the options.
- **Consensus:** none.
  - A fresh pool state is decided by its series and index alone, so every
    item meets the same state as before.
  - Messages for a pool of a huge whole series that is not in use yet go
    unanswered. Under the owner's definition that is lack of information,
    not a hard fork.
  - `MaxSections` limits how a follow list is written, not what can be
    followed.
  - No series is capped.
- **With ORANGE-029 (PR #104):** it changes how a peer's weights are
  asked. This changes which pools: the indexed ones. They touch the same
  loops and compose; whichever merges second takes a small merge.
- **What is left:** a pool index on the wire, for messages other than a
  pushed transaction or entry. Black ruled it a protocol change under
  D76, which is jerry's call as a feature, so it is not built.
- **Tier:** Implementation (engine pool state, the p2p index, node policy
  follows, the CLI). Tests are listed in BLUE-020.

### D155. Round 20: settings export and import; ORANGE-029 and GREEN-044 adopted

- **Decision:**
  - **Settings export and import (the owner's feature, 2026-11-03).**
    - The ask: a way to save a node's settings and carry them to another
      machine, as node policies can be exported and imported. Nothing
      personal goes in.
    - A node's settings are everything in its node policy except what it
      follows (`nodepolicy.Followed`: series, follows, accepted issuers):
      peers, registries, limits, the p2p switches, tracking, logs, the
      grind defaults and jobs, and `issuers.any`. node.json holds no key,
      seed, passphrase or RPC token, and the wallet is a separate file,
      so none of those can be in the file.
    - `poo settings export [-full] [-out FILE]` writes a bundle of scope
      `settings` (`nodepolicy.ScopeSettings`, made by `ExportSettings`).
      It uses the same file format as a policy bundle, format 1.
      Machine-local settings (`MachineLocal`) are left out unless
      `-full`.
    - `poo settings import FILE|- [-apply] [-prefer-file] [-replace]
      [-full] [-map-path OLD=NEW]... [-if-digest D]` is `poo policy
      import` with `MergeOptions.SettingsOnly`. It reuses the same
      reading, plan, conflicts, defences-first lines, digest, apply,
      running-node path (`POST /v1/policy/import`) and view-only refusal.
      It never takes what to follow, even from a whole node.json. With
      `-replace`, the file's settings replace the node's, and what the
      node follows stays its own.
    - The two commands refuse each other's files with a hint:
      - `poo policy import` refuses a settings file.
      - `poo settings import` refuses a series file or an `-all` file.
      - `poo settings import` takes the settings part of a policy file
        or a node.json.
  - **Moving to another machine.**
    - `-map-path OLD=NEW` (repeatable, on both imports) rewrites a
      registry file path that starts with OLD, at a path boundary. Those
      files are the only local paths in node.json. It is part of
      `MergeOptions`, so the digest covers it.
    - `Plan.Fit` lists what the import brings in that may not fit this
      machine (`nodepolicy.Fit`, with `grind.ThisMachine`):
      - a grind device it lacks, or more threads than it has;
      - a registry file that is not there;
      - an RPC or p2p listen address that is none of its interfaces';
      - an advertised address that came from another machine;
      - a grind job for a series this node policy does not list.
      Only lines the import adds are listed. The preview prints them as
      "may not fit this machine". They refuse nothing. Both imports fill
      them, offline and through a running node.
  - **Merged with this round:**
    - BLUE-020 (D154: RED-049, RED-050, RED-051), with Red's findings
      (PR #103).
    - ORANGE-029: the weight check asks within the peer's request budget
      (`sweepWeights`). Black cleared it as an information gap.
    - GREEN-044: help names `grind schedule`, completion reads grind's
      usage, and doctor gives a fix for a lowered value with no command.
      Its text point is taken too: a whole follow now prints "follows
      every pool (N)".
    - Black's Round 19 review: 8 clear, 0 blocked.
- **Why:** the owner asked for it: "so it would allow them to carry it
  over to another machine". Reusing the bundle and `Merge` keeps one
  import path, so `-settings` on a policy import and a settings import
  plan the same lines the same way.
- **Consensus:** none. It writes node policy through the same `Merge` and
  `Validate` as any import. No setting can make a heavier ledger
  unacceptable, and the check that cannot be turned off (D153) stays
  clamped.
- **Not built:** settings kept outside node.json. The `settings` field of
  the bundle stays reserved for them. The GUI form comes next, from
  GUI/Grey.
- **Tier:** Node policy and the CLI. The tests are `TestExportSettings`,
  `TestMergeSettingsOnly`, `TestMapPaths` and `TestFit` in
  `pkg/nodepolicy`, and `TestSettingsExportImport` and
  `TestSettingsImportIntoRunningNode` in `pkg/cli`.

### D156. Round 20 fixes: RED-052 to RED-057 (BLUE-021)

- **Decision:**
  - **RED-052.** A plain export (settings or node policy) leaves out what
    is personal (`nodepolicy.Personal`): a registry that is a local
    file, a registry URL with a user, password, query or fragment, and a
    grind job on one payment or entry (`tx:TID` or an entry hash). `-full`
    keeps them. Peers and `tracking.peers` stay in, as a setup is made of
    them. The export prints what the file holds and left out
    (`ExportNotes`): after the "exported" line, or on stderr when the
    bundle is printed. The docs no longer say "nothing personal", and the
    `-full` help lists what it keeps.
  - **RED-053.**
    - `Validate` refuses a control character, a bidirectional override or
      bytes that are not UTF-8 in any text of a node policy (`checkText`).
      Its error quotes the value escaped.
    - The CLI prints every value from a file through `termSafe`.
    - `Fit` never opens a network or device path (`\\host\share`,
      `//host/share`, `\\?\`). It names it as not checked.
  - **RED-054.** `Plan.Empty` counts a conflict that took the file's
    value. A plan made of such conflicts alone is saved.
  - **RED-055.**
    - `-map-path` joins the rest of a path with one separator, `To`'s own.
    - A `From` written the Windows way matches in any case and with either
      separator.
    - A registry file a map rewrote is taken without `-full`.
  - **RED-056.** A series local lists but refuses that the result uses is
    named in `Plan.NowUsed` and under "read first".
  - **RED-057.** A branch offered in a pool of a large whole series that
    is not indexed no longer indexes it. The record is held in `chasing`
    for the chase alone. `sweepChasing` drops it at the next check once the
    chase queue (`Queue.Pools`) holds no group for it. A pool is indexed
    when something is kept in it.
- **Why:** Red's Round 20 findings, on D155 and D154:
  - A shared setup file carried user names, passwords and home
    addresses.
  - A file could erase the preview's warnings with escape sequences.
  - `-prefer-file` could report a change and save nothing.
  - `-map-path` glued names and needed `-full`.
  - A settings import could start using refused series unnamed.
  - A message with a stated weight added a pool to the index for good.
- **Consensus:** none.
  - Leaving a record out of the p2p index until something is kept limits
    no ledger and no follow. A pool offered again gets its record again,
    and the chase runs as before.
  - The text check refuses only text that no valid setting holds.
  - Nothing here touches which ledger a node selects.
- **Tier:** Node policy, the CLI and p2p.
  - `pkg/nodepolicy/blue021_test.go`: `TestExportLeavesOutPersonal`,
    `TestControlCharactersRefused`, `TestFitDoesNotOpenNetworkPaths`,
    `TestPreferFileConflictAloneIsAChange`, `TestMapPathJoins`,
    `TestMapPathWithoutFull`, `TestSettingsImportNamesSeriesNowUsed`.
  - `pkg/cli/blue021_test.go`: `TestImportRefusesTerminalEscapes`,
    `TestTermSafe`, `TestPreferFileAppliesALoneConflict`.
  - `pkg/p2p/blue021_test.go`: `TestStatedWeightIndexesNoPool`.

### D157. The peer book by hand, filters, and peer export and import

- **Decision:** jerry's Round 21 request.
  - **State `unlisted`** (`peers.Unlisted`): an address the book knows for a
    mint policy but has not picked. Imports land there. `Record.Label` is
    the owner's name for an address (at most 128 bytes of UTF-8, no
    control characters, `peers.CheckLabel`). Sources `hand` and `import`.
  - **By hand:** `poo peer add|remove|label|connect` (`POST /v1/peer`
    actions `add`, `remove`, `label`, `connect`). `add` puts the address in
    as a candidate for every mint policy the node covers, refusing a local
    one unless `p2p.allow_local_peers` or `-keep`. `-keep` and `remove`
    edit `p2p.peers` in node.json. With the node stopped they work on the
    files.
  - **Filters:** `poo peers -category -mint-policy -label -addr` and
    `poo tracking -blocked -timed-out`.
  - **Export and import** (`pkg/peerport`, kind `poo-peers`, format 1):
    the whole book or chosen categories (the book states, `blocked`,
    `timed-out`) with labels and blocks. An import:
    - adds every address unlisted, for the file's mint policies; sets a
      label only where the book has none;
    - takes blocks only with `-blocks`, adding or lengthening, never on an
      address the node policy lists or the node's own; ranges only with
      `-ranges`;
    - never removes, unblocks or shortens; refuses addresses blocked here,
      local ones (unless allowed) and anything past the book's limits;
    - shows a plan with a digest first, as policy import (D151, D154).
    A running node imports through `POST /v1/peers/import`.
  - **Dialling imported addresses:** when a mint policy is short of
    candidates after the archive step, the manager promotes imported
    addresses to candidates, oldest first (`Book.PromoteImported`), before
    peer lists and registries. `p2p.no_dial_imported` turns it off;
    `poo peer connect -imported` promotes them all and runs the manager.
- **Why:** jerry asked for filters, adding peers by hand, and exporting
  and importing the peer book (all or by category), with imports landing
  unlisted unless a blocked list is shared, and for a way to start
  dialling them.
- **Consensus:** none. The peer book, labels, blocks and dialling are node
  policy, local to one node. An import can only add addresses and lengthen
  blocks the user chose to take; it never blocks a listed address, so a
  node can always reach the peers its policy names. A node that blocks
  or never dials a peer lacks information, which is not a hard fork; any
  heavier ledger is still taken from any peer that offers it.
- **Tier:** Peers, p2p, RPC and the CLI.
  - `pkg/peers/unlisted_test.go`, `pkg/peerport/peerport_test.go`
    (`TestExportCategories`, `TestReadRefuses`, `TestImportRules`,
    `TestImportGuards`), `pkg/p2p/imported_test.go`
    (`TestManagerDialsImported`, `TestAddPeerByHand`),
    `pkg/cli/peersport_test.go` (`TestPeersExportImport`,
    `TestPeerActionsOffline`, `TestPeersRunning`).
  - Also adopts GREEN-045 and ORANGE-030 as merged this round.

### D158. Round 21 fixes: RED-058 to RED-062 (BLUE-022)

- **Decision:**
  - **RED-058.**
    - The peer book keeps at most `MaxUnlisted` records known only from
      an imported file: a quarter of `MaxRecords` (1024) by default. An
      import is refused past it, and the plan says so.
    - An address learned any other way takes the place of the oldest
      such record when the book or its range is full, so a file never
      blocks discovery.
    - `poo peer remove -imported` (`POST /v1/peers/forget-imported`,
      `Book.ForgetImported`) takes them all out. A record the node learned, promoted or verified
      stays, and so do blocks.
  - **RED-059.**
    - The plan's notes total the blocks and ranges, and say how much of
      IPv4 the ranges cover.
    - The trim that made 65,536 blocks take a minute is ORANGE-031's (PR
      #126), and is not repeated here.
  - **RED-060.** `peers.Range` of a host name is the name in lower case
    without the root's trailing dot, as the tracker keys it. A listed name
    in capitals meets a file's `host:` range block. The plan says a
    listed host name is protected by name only.
  - **RED-061.**
    - A new package, `plain`, is the one text check: no control, format
      (direction marks, zero-width characters) or separator character,
      and valid UTF-8.
    - `peers.CheckLabel`, `nodepolicy`'s `checkText` and the CLI's
      `termSafe` use it.
    - Table cells and keys are escaped (`termSafe`).
  - **RED-062.**
    - `peerport.ImportIf` plans and applies in one call, only when the plan
      has the given digest. `poo peers import -apply -if-digest` and the
      node's `if_digest` use it.
    - The preview says the addresses will be tried when the node is short
      of peers, and how `p2p.no_dial_imported` stops that.
    - Points 1 and 2, offline `peer add -keep -label`, are GREEN-046's (PR
      #124).
- **Why:** Red's Round 21 findings on D157:
  - One shared file could fill the peer book for good.
  - A large block list took a minute and its effect was not shown.
  - A listed name in capitals was not protected.
  - Labels could hold invisible and direction characters.
  - The peer import applied a plan other than the one checked.
- **Consensus:** none. The peer book and the tracker are node policy,
  local to one node.
  - `MaxUnlisted` limits what one file puts in the book, not what the node
    can learn or follow. Every limit of the book gives way to an address
    the node learns.
  - Nothing here touches which ledger a node selects.
- **Tier:** peers, peerport, plain, the CLI and RPC.
  - `pkg/peers/blue022_test.go`: `TestImportedAddressesGiveWay`,
    `TestRangeOfAHostName`, `TestLabelRefusesInvisibleText`.
  - `pkg/peerport/blue022_test.go`: `TestImportTakesAShareOfTheBook`,
    `TestListedHostNameInCapitalsIsProtected`,
    `TestPlanCountsRangeBlocks`,
    `TestImportIfAppliesOnlyThePreviewedPlan`.
  - `pkg/plain`: `TestTextAndSafe`.
  - `pkg/cli/blue022_test.go`: `TestPeersForgetImported`,
    `TestTableEscapesText`.

### D159. Settable peer book, tracker and discovery limits

- **Decision:** jerry's Round 22 request: the limits of the peer book and
  the tracker are node policy, each with a floor and a ceiling
  (`nodepolicy.Bounds`, `pkg/nodepolicy/limits.go`). Left out or 0, a
  setting is its default; any other value outside its range is refused by
  `Validate`, which names the setting and its range.
  - `p2p.max_book` (4096; 256 to 65536), `p2p.book_per_range` (16; 1 to
    4096 and at most `max_book`), `p2p.verified_per_range` (4; 1 to 64),
    `p2p.retry_seconds` (60; 1 to 86400), `p2p.registry_seconds` (3600; 60
    to 86400).
  - `tracking.max_addresses` (8192; 1024 to 131072),
    `tracking.flag_history_seconds` (7 days; 1 hour to 365 days),
    `tracking.log_seconds` (3600; 60 to 86400), `tracking.log_size` (256;
    16 to 4096).
  - The places for imported addresses stay a quarter of `max_book` (D158).
  - `Policy.BookLimits` gives the book's limits; the node and the CLI's
    offline import open the book with them. `TrackingConfig` carries the
    retention.
  - A book over a lowered `max_book` keeps its records and takes no new
    address until it is under the bound.
  - Also merges GREEN-046 (offline `peer add` checks before it writes;
    zero times read `-`), ORANGE-031 (tracker trim backoff, the book's
    range cache) and adds a `poo tracking` pointer to `poo peers -h`.
- **Left fixed, on purpose:** the chase queue and its pacing
  (`QueueLimit`, `ChaseSlice`, `WeightlessPages`, `RequestPace`), the
  manager's interval, and the peer file's own caps. A user setting on
  the chase could slow or starve the fetch of a heavier ledger, and no
  node policy may make a heavier ledger unacceptable (AGENTS.md). The
  weight check is already bounded by D153. The peer file caps (65536
  addresses, 16 MiB) hold any book the bounds allow.
- **Why:** jerry: "the whole point of node policy is to allow people to
  tailor their settings to their needs".
- **Consensus:** none. These bound how many peers a node knows and how long
  it remembers their offences. A smaller book or memory means the node
  has less information, which is not a hard fork; the floors keep a node
  able to find and keep peers, and nothing here touches selection,
  chasing or the weight check.
- **Tier:** Node policy.
  - `pkg/nodepolicy/limits_test.go`: `TestLimitsDefaults`,
    `TestLimitsSet`, `TestLimitsBounds`, `TestValidateRefusesOutOfBounds`.
  - `pkg/cli/limits_test.go`: `TestPeerImportUsesPolicyLimits`.

### D160. Round 22 fixes: RED-063 to RED-065 (BLUE-023)

- **Decision:**
  - **Engine memory limits are bounded (RED-063).** `seen_size` (65,536;
    1,024 to 4,194,304), `max_pending` (10,000; 16 to 1,048,576) and
    `ties` (64; 1 to 4,096) are D159 `Bound`s: listed in
    `nodepolicy.Bounds`, checked in `Validate`, so `poo doctor`, the
    settings import's "does not fit" check and the GUI form take them with
    the rest. The seen list's ring and set grow with the items it holds,
    not with the setting, so a large `seen_size` costs nothing at start.
    `seen_size` at 10^15 used to panic the node at start, and 3×10^8 took
    11 GB before one item arrived.
  - **Learned candidates give way (RED-064).** A record learned from the
    network (a peer list, a reply, a registry, an inbound connection)
    with no label that the node has never reached (`Record.Reached`, set
    by a connection that worked or a verification) gives way to a new
    address when the book or its range is full. The order is: records
    known only from an import, then learned ones whose last try failed,
    then learned ones older than `p2p.candidate_seconds` (new; one day;
    60 s to 365 days), then, for an address the owner gave (`hand`,
    `config`), any learned one; oldest first in each. A record the owner
    added, configured or labelled, and one the node reached, never gives
    way. The owner's own `poo peer add` therefore always finds a place
    unless the book is all such records.
  - **A lowered `max_book` is applied (RED-064).** `Book.Fit` trims a book
    over its limits in the same order; the node calls it on start and
    logs `peer book over its limits: policy value lowered`. `Open` does
    not, because a command run offline opens the book with the default
    limits. A book saved before D160 counts its listed records as
    reached.
  - **A refused add changes nothing (RED-065).** `Book.add` works out
    whether the book and the range are full and which record gives way,
    and deletes only when the add will succeed; when the range is full the
    record must be from that range.
  - The give-way classes are kept on heaps checked when read, so an add
    into a full book at the 65,536 ceiling costs about 3.5 µs, keeping
    ORANGE-032's speed-up (the first cut scanned the book: 7.9 ms).
  - Includes ORANGE-032 (#136), which this builds on.
- **Not done, on purpose:** a share of the book per source (Red's third
  point). The book does not record which peer sent an address, the range
  limit already bounds one sender's addresses per range, and the owner's
  add can always take a learned place. Left for a later round if Red
  shows a flood the order above does not drain.
- **Consensus:** none. The peer book is node policy and the three engine
  limits only bound memory: an entry or transaction dropped or forgotten
  can be sent again, and none of them bounds the weight, depth or length
  of a ledger a node takes. A node with a smaller book, seen list or tie
  set has less information, which is not a hard fork (AGENTS.md). `ties`
  existed and was already a cap; the new ceiling only stops a value that
  turned the cap off.
- **Tier:** Node policy.
  - `pkg/peers/blue023_test.go`: `TestRefusedAddChangesNothing`,
    `TestLearnedCandidatesGiveWay`, `TestFitLoweredBook`,
    `BenchmarkAddFullBookLearned`; `blue023_internal_test.go`:
    `TestGiveWayQueuesStayBounded`.
  - `pkg/node/blue023_test.go`: `TestEveryIntegerSettingAtItsMaximum`
    (every integer setting at its type's maximum is refused, or the node,
    its book and its tracker open without a panic).
  - `pkg/engine/blue023_test.go`: `TestSeenGrowsWithItems`.
  - `pkg/nodepolicy/limits_test.go`: the four new bounds;
    `pkg/p2p/settings_extremes_test.go` now runs Black's heavier-ledger
    check with the three engine limits at their floors and ceilings.

### D161. Rival claims, building on a chosen parent, and setting limits

- **Decision:** jerry's Round 23 request: a user can claim a mint the
  selected ledger has already claimed, and choose where the node builds.
  - `engine.BuildOn{Parent, Txs}` and `Engine.BuildEntryOn` build an entry
    on any held entry of the pool, or on the pool_id, from chosen
    transactions or the pending ones. `ledger.Pool.CheckOn` checks them
    against the state after the parent, as `AddEntry` will, and leaves
    out the ones that do not fit (one claim of a mint, no input twice, no
    output of the same entry spent), returning each with its error.
  - `Pool.ClaimEntry` and `Engine.ClaimParent` find the selected entry
    that holds a mint's claim and its parent: the place to build a rival
    claim. `Engine.PoolID` gives the pool_id.
  - `POST /v1/build` takes `parent` (an entry hash, `root` or
    `before-claim:N`) and `txs`, and replies with `parent` and `refused`.
  - `poo claim -parent HASH|root|before-claim [-max-tries N] [-work N]`
    builds the claim into an entry on that parent instead of submitting it
    as pending, and prints the command that adds weight to it. `poo build
    -parent` builds the pending transactions that fit there.
  - `poo policy limits -set NAME=VALUE` (repeatable) and `POST
    /v1/policy/limits` set the bounded settings (`Policy.SetLimits`),
    checked whole first: through the running node, which saves node.json
    under its lock and reloads, or in node.json with the node stopped.
    `Policy.Limits` is now built from `Bounds` and `limitField`, so a new
    bound is listed, checked and settable from one place.
  - Merges GREEN-047 (`poo policy limits`), ORANGE-032 (the book's kept
    counts, the tracker log re-slice; its ceiling costs are in
    nodepolicy.md), BLUE-023 (D160) and Black's Round 22 review.
- **Left out:** a grind target that names a mint's rival claim: a rival
  claim is an entry, and `poo grind start ENTRY` already grinds it, with
  `-until-from` for a goal. A pin that keeps a lighter local branch from
  pruning: `k: -1` keeps every branch.
- **Why:** jerry: "go back to an existing claim and work on making our
  own", "lets include that and anything else related that is missing".
  The GUI needs a command to set limits, since it only runs poo.
- **Consensus:** none. A rival entry is checked by `AddEntry` like any
  entry from a peer, and selection is by weight as before. Building on a
  parent other than the tip was always valid (a rival branch,
  `TestDoubleSpendResolvedByWeight`); this only lets the node build one.
  No rule, check or selection changes, and the bounds `-set` writes are
  D159's and D160's.
- **Tier:** Engine, ledger, RPC and the CLI.
  - `pkg/engine/rivalclaim_test.go`: `TestRivalClaimOnBranch` (a second
    claim refused as pending, built before the first claim, kept by every
    node, selected once heavier, the mint then the rival's),
    `TestBuildOnRefusals`.
  - `pkg/cli/rivalclaim_test.go`: `TestRivalClaimCLI`,
    `TestPolicyLimitsSet`.
  - `pkg/nodepolicy/limits_test.go`: `TestSetLimits`.

### D162. Round 23 fixes: RED-066 to RED-069 (BLUE-024)

- **Decision:**
  - **The node proves itself before a command sends the token
    (RED-066).** `rpc.Dial` and `DialView` first ask `GET /v1/hello` with
    a random nonce and send the token only if the reply holds
    HMAC-SHA256(token, `poo-rpc-hello`, the address in `rpc.addr`, the
    nonce). A process on a stale `rpc.addr`'s port knows neither token, and
    the node at another address signs that address, so neither gets the
    token. The failure wraps `ErrNotRunning`, so commands fall back as when
    no node runs. `poo doctor` names both cases. I chose the proof rather
    than Red's lock check: probing the data directory lock could make a
    node that is starting at the same moment fail to take it, and the lock
    does nothing on platforms without one.
  - **The owner's add always finds a place (RED-067).** After every class
    of D160, an address the owner gives (`hand`, `config`) takes the place
    of the oldest record from the network (no label) that the node
    reached, unless that record is a priority or backup peer now. A learned
    address still never takes a reached record's place. `p2p.New` now logs
    a configured peer the book refuses instead of dropping the error.
  - **Host names count by where they lead (RED-067).** When the node
    reaches a host name, `Book.Resolved` records the range of the address
    it led to (`Record.Via`, saved in the book). The name counts in that
    range too, for `book_per_range`, `verified_per_range` and `Pick`'s
    spread. If the range is then over `book_per_range`, a record of it
    gives way as to a learned address, or else the name does, unless the
    owner gave or labelled it. One server behind any number of names holds
    at most `book_per_range` places.
  - **A build on a chosen parent is linear and checks the pool
    (RED-068, RED-069).** ORANGE-033 (#143, merged into this branch) makes
    the conflicts set lookups. `ledger.Pool.CheckPendingOn` does not check
    shape or witnesses again for the pending pool's transactions: they were
    checked when the pool took them, and an input's owner is the address
    of the output it spends after any parent. 20,000 pending take about
    30 ms (1.3 s with the witnesses). `CheckOn` and `CheckPendingOn` leave
    out a transaction on another pool's mint with `entry.ErrWrongPool`,
    so the grind never starts.
- **Not done, on purpose:** Red's cap on the share of host-name records.
  Names the node has not reached are learned and give way (D160), and the
  ones it reached share their machine's range, so a cap would add a limit
  with nothing left to stop. Also not done: Red's "bound the pending path
  to a count". It would change which transactions an entry on a chosen
  parent holds, against a plain build, and with the witnesses gone the
  check is linear.
- **Consensus:** none. The RPC proof and the peer book are local. A build
  on a parent picks what this node puts in its own entry; `AddEntry`
  checks that entry in full as before, and `ErrWrongPool` is the refusal
  `AddEntry` would give, earlier. Nothing changes which ledger a node
  takes.
- **Tier:** Node policy.
  - `pkg/rpc/blue024_test.go`: `TestDialSendsNoTokenToAStranger`,
    `TestDialProvesTheNode`.
  - `pkg/peers/blue024_test.go`: `TestNamesForOneMachineShareARange`,
    `TestOwnerAddTakesAReachedPlace`; `blue023_test.go` updated for the
    owner's place.
  - `pkg/ledger/blue024_test.go`: `TestCheckPendingOnIsLinear`,
    `TestCheckPendingOnMatchesCheckOn`.
  - `pkg/engine/blue024_test.go`: `TestBuildOnRefusesAnotherPool`.

### D163. Variable supply series, phase 1: the definition, parts and releases

- **Decision:** jerry approved variable supply series as a consensus
  change. They are specified in *Mint Series: Fixed and Variable Supply*
  and Proof of Ownership section 16.8. They are built over several rounds,
  as [variable-supply.md](variable-supply.md) plans. Phase 1 builds what a
  series and its parts are:
  - **Params.** `series.Params` gains `SupplyType` (left out for a fixed
    series), `PartSize` (M) and `MaxParts` (0: no limit).
  - **Byte form.** A variable series is written in format version 2:
    version 1's fields, then supply type, part size and largest number of
    parts ([protocol/encoding-variable.md](protocol/encoding-variable.md)).
    A fixed series stays version 1, byte for byte.
  - **Validation.** A variable series names an issuer, has M ≥ 1, has N
    and M as multiples of P, and has a cap whose serials fit in 64 bits.
    A fixed series may not set M or a cap.
  - **Parts.** `PartRange`, `PartOf`, `MintsThrough` and `PartID`
    (`H("PoO-part" ∥ series_id ∥ U64(k) ∥ part_id(k − 1))`).
  - **Releases.** `Release` has a byte form and a JSON form.
    `CheckRelease` checks the issuer's witness over `H("PoO-part-v1" ∥
    part_id)` first, then the previous part id.
  - **Not followed yet.** `nodepolicy.ErrVariableNotFollowed` wraps
    `series.ErrUnreadable`. A node refuses to follow a variable series
    until phase 2 builds its transactions.
  - **Tools.** `poo series new -variable -part-size -max-parts -issuer`,
    `poo series sign`, `poo series release -part | -check` and
    `poo series parts`.
  - **Vectors.** `pkg/vectors/testdata/variable.json`.
- **Choices made here, within the documents:**
  - Format version 2 for the new fields, as encoding.md's stability rule
    asks for any extension. A node from before this change reads a
    variable series as unreadable, never as invalid.
  - The part number is written as U64, like a pool index.
  - The release message tag is "PoO-part-v1", from the documents. It is
    used the way the issuer message "PoO-issuer-v1" is, as a pseudo-tid
    under the witness message.
  - `MaxParts` counts part 1.
  - A serial whose part would hold serials past 64 bits is in no part.
- **Why:** jerry: "the next couple of rounds should include implementing
  this new concept", with "due diligence to ensure nothing gets broken".
- **Consensus:** this is the consensus change jerry approved. Phase 1
  changes nothing a node decides about any ledger:
  - A fixed series keeps its bytes, series_id, JSON, pools, tids and
    entries.
  - A variable series is refused as unreadable, so no transaction of it
    is checked, kept or selected.
  - The new checks decide only what a variable series and a release are.
    Nodes that hold the same series and releases reach the same answers,
    because nothing in a part can be chosen. Black reviews the change in
    Part D.
- **Tier:** Series, node policy, CLI and vectors.
  - `pkg/series/variable_test.go`:
    - `TestFixedSeriesUnchanged`: 2,000 random fixed series against a
      frozen copy of the version 1 encoder, their ids, and JSON with no
      new keys.
    - `TestFixedSeriesIDPinned`.
    - `TestVariableByteForm`, `TestVariableRules`.
    - `TestPartRanges`, which uses the document's table.
    - `TestPartIDs`.
    - `TestRelease`: a wrong previous id, another signer, a tid or the
      issuer message signed instead, part 1, part 0, past the cap, and a
      fixed series.
  - `pkg/nodepolicy`: `TestVariableNotFollowedYet`.
  - `pkg/cli/seriesparts_test.go`: `TestSeriesReleaseCLI`.
  - `pkg/vectors`: `TestGoldenVariable`. `TestByteFormV1Frozen` still
    passes, unchanged.

### D164. Round 24 tooling fixes: peer import on a running node, peer counts, an empty -parent

- **Decision:**
  - **Peer import (GUI/Orange's report).** `rpc.PeersImportRequest` carries
    `file_digest`. `peerport.File.Digest` is left out of JSON, so a running
    node computed the import digest with an empty file digest, and every
    `poo peers import -apply -if-digest` against it was refused as not
    previewed. The node now uses the caller's file digest, which is the one
    its preview covered.
  - **Peer counts (GUI ORANGE-017).** `poo peers -counts` and
    `GET /v1/peers?counts=1` reply with the connections and
    `counts{addresses, connected, states}`, without the book's records. A
    full book of 16,384 addresses is about 12.8 MB of JSON for each
    refresh.
  - **An empty -parent (GUI/Blue).** `poo claim` and `poo build` refuse an
    explicitly empty `-parent`. Before, it read as if `-parent` had been
    left out, so the claim was submitted.
- **Left out, for a later Part A:** the `-set` that arrives while the node
  is still reloading from an earlier one (a bare EOF), and ORANGE-033's
  in-place `-set`. Both need the reload path changed, and Black should
  see the in-place one.
- **Consensus:** none. These are local tools and the local RPC.
- **Tier:** CLI and RPC.
  - `pkg/cli/peersport_test.go`: `TestPeersRunning` applies a previewed
    non-empty plan with `-if-digest` to a running node, which failed
    before this change, and checks `-counts`.
  - `pkg/cli/rivalclaim_test.go`: `TestEmptyParentRefused`.

### D165. Round 24 fixes: RED-070 and RED-071 (BLUE-025)

- **Decision:**
  - **Part ids in bounded memory, with a short lock (RED-070).** This
    builds on ORANGE-034 (#157, merged into this branch), whose marks every
    1,024 parts gave the same ids in k/1,024 of the memory. That is still
    2 GiB at part 2³⁶. Now:
    - When the marks would pass 65,536, every other one is dropped and the
      stride doubles. The chain then holds at most 2 MiB whatever part is
      asked for.
    - Part k up to the newest costs at most one stride of hashes from its
      mark, and parts asked for in order cost one each (the last answer is
      kept).
    - Working far out runs 16,384 hashes at a time under the lock and lets
      other calls in between.
    - A release of part k still costs k hashes once. That is time, not
      memory.
  - **No cap on parts.** As Red and jerry's definition say, refusing a
    valid release because it is far out would make nodes with the same
    information disagree, which is a hard fork. No node policy may do it.
    A later limit may only make a node slower, never make it say "invalid".
  - **Phase 2 note (Red's point 4).** When phase 2 checks a transaction
    on a part, it must first check that a known release reaches that part,
    and answer "not released yet" with no hashing otherwise. Only then may
    it ask for `PartID`. This is for the phase 2 design and is not built
    here.
  - **A variable series' JSON is unreadable to an older poo (RED-071).**
    - `Params.MarshalJSON` writes a variable series' hash as
      `variable_hash` and leaves `hash` out. A node built before D163
      ignores unknown keys, so it finds hash 0, which names no hash, and
      calls the series unreadable instead of following a look-alike fixed
      series. I checked this with the 215d428 binary: `series info` fails,
      and `doctor` lists the series as not accepted.
    - A fixed series' JSON is unchanged.
    - `UnmarshalJSON` reads both forms and refuses a key it does not know
      (`ErrUnknownKey`), so the next extension is unreadable here instead
      of misread.
- **Consensus:** none. Part ids are the same ids, worked out with less
  memory. The JSON change affects how a series is written in files, not
  its byte form or its id.
- **Tier:** Node policy (memory), and the series' file form.
  - `pkg/series/blue025_internal_test.go`: `TestPartChainBoundedAndExact`
    (small strides; the marks thin out; every id matches the naive chain in
    any order and from 8 callers at once),
    `TestPartIDFarOutInBoundedMemory` (part 8 million under 16 MiB of heap
    growth; a known part is answered while a long call runs).
  - `pkg/series/blue025_test.go`: `TestVariableJSONIsUnreadableToOlderReaders`.

### D166. Variable supply phase 2: nodes follow variable series (Round 25)

- **Decision:** a node follows a variable series and its released parts,
  built to the six requirements in Black's Round 24 review
  ([ROUND-24.md](consensus/source/ROUND-24.md)) and to PoO 16.8. Fixed
  series are unchanged.
  - **A mint names its part.** `series.MintID` gains `Part`, set only for a
    variable series (0, left out, for a fixed one). A transaction's body is
    `series ∥ part ∥ serial ∥ …` for a variable mint and stays `series ∥
    serial ∥ …` for a fixed one, so the tid covers the part and a fixed
    transaction keeps its bytes and tid (Black's requirement 5). The
    decoder knows which form to read from the series id: `series.New`
    registers each variable series id in a process-wide set
    (`series.IsVariableID`). Only a followed series' pools are taken, and a
    followed series is always built before its transactions are read. A
    transaction of a variable series this process never built reads as a
    fixed one and then fails its checks against that series, so nothing is
    taken from it. The other choice in variable-supply.md, a marker byte,
    was not taken: it would add a form a fixed transaction never needs.
  - **One name per mint (requirement 4).** `Series.CheckMint` refuses a
    part that is not `PartOf(serial)` (`ErrWrongPart`), a serial past the
    largest number of parts (`ErrNoSerial`, shown as the serial out of
    range), and a fixed mint that names a part.
  - **Waiting (requirements 1 and 2).** A mint on a part past the newest
    release this node knows returns `series.ErrWaiting` before any hashing:
    the check compares the part with the released count, an atomic counter.
    The engine adds a transaction to its seen list only on success, so a
    waiting one is never cached and is taken when given again after the
    release. Pools of unreleased parts are not counted in `PoolCount`, so
    an entry for one is "not followed", the same as a pool this node does
    not follow: p2p ignores it with no penalty. The RPC answers a waiting
    transaction with 409 `waiting`. Nothing about a waiting transaction or entry is invalid.
  - **Keeping releases (requirement 6).** A node keeps the newest release
    of each variable series in node.json (`series[].release`). It proves
    every part before it, so nothing else is kept. `Series.AddRelease`
    answers a part it already has at once (no hashing), checks a newer one
    with `CheckRelease` (witness first), and only then moves the count up.
    The count only grows. Validity never depends on when a release arrived:
    with the same series and release, every node has the same parts.
  - **No size rules (requirement 3).** Nothing limits a release's part
    number or a part's size beyond the series' own cap. Working out a far
    part id costs time, in bounded memory (D165).
  - **Part 1 needs the issuer witness.** A variable series listed without
    its `issuer_witness` is kept and not followed (`nodepolicy.ErrNoPartOne`,
    unreadable, never invalid). `poo init -series` and `poo doctor` say so.
    `poo series sign` now prints `{series_id, params, issuer_witness}`, a
    file `poo init -series` and `poo series info` read, checking the id
    and the witness.
  - **Giving a node a release.** `poo series add-release RELEASE` and
    `POST /v1/series/release` check the release, save it in node.json and
    reload the node. With the node stopped, the command changes node.json
    under the policy lock. `GET /v1/series` adds `variable{released,
    next_part, mints, units, release}` for a variable series, which
    `poo series followed` prints. A client applies the node's release to
    its own copy of the series, so a claim it builds on a released part is
    checked against the same parts.
  - **Round 25 tooling.**
    - `poo policy limits -set` (and `add-release`) sent while the node is
      still reloading from an earlier change used to fail with a bare EOF.
      The client now dials again for up to 20 seconds while the connection
      is dropped. Both calls are idempotent, so a retry is safe.
    - `poo series info` reads a signed series file, prints the supply type,
      part size and largest number of parts (left out when uncapped), and
      `-h` prints its usage instead of opening a file named "-h"
      (GUI-GREEN-051).
    - `poo peers -counts` prints `node_id`, `listen`, the connections
      (`peers`, which the GUI's Peers panel shows) and `counts`, without
      the empty `book` list (GREEN-049's idea). `GET /v1/peers?counts=1` is unchanged.
    - `poo series parts -o table` lists one row per part (GREEN-049's idea).
    - `poo series release FILE -check -` reads the release from standard
      input, so the GUI can check one it has just signed (GUI-027).
  - **Left out:** ORANGE-033's in-place `-set` (a limit applied without a
    reload). The reload path is what makes a change take effect everywhere
    at once, and Black should see a design first. Passing releases between
    nodes is phase 3.
- **Consensus:** this is the consensus change jerry approved on
  2026-11-04 (PoO 16.8 and the variable supply document), phase 2 of 3.
  Nothing outside those documents is added. For fixed series nothing
  changes: `TestFixedTxUnchanged` checks 2,000 random fixed transactions
  against a frozen copy of the version 1 body, and the v1 vectors are
  untouched.
- **Tier:** Consensus (variable series only), node policy, RPC and CLI.
  - `pkg/tx/variable_test.go`: `TestFixedTxUnchanged`, `TestVariableMints`.
  - `pkg/engine/variable_test.go`: `TestVariableSeriesWaitsForRelease`.
  - `pkg/nodepolicy`: `TestVariableFollowed`.
  - `pkg/cli/seriesparts_test.go`: `TestVariableSeriesOnANode` (a claim on
    part 2 waits, `add-release`, the reload, the claim, `series followed`,
    node.json keeps the release), `TestSeriesPartsTable`.
  - `pkg/vectors/testdata/variable.json` gains a claim of the first mint of
    part 2 (body, tid, encoding).

### D167. Round 25 fixes: RED-072 to RED-076 (BLUE-026)

- **Decision:**
  - **One way to build a listed series (RED-072).** `SeriesInfo.Build`
    builds the series and adds the release node.json keeps. `follow`,
    `unfollow`, `wallet follow`, `ExportSeries`, the import merge and its
    plan counts, and `policySeries` in the CLI all use it, so they see the
    same parts as the running node. A pool or mint of a part this node
    does not know released yet is still refused by `follow`, and the
    message now says it is waiting for a release.
  - **A variable series stays whole on `unfollow` (RED-072).** A whole
    follow of a variable series takes the pools of later releases too, so
    `unfollow -pool` keeps it whole with exceptions and never rewrites it
    as a list of the pools left (the "shorter form" is kept for fixed
    series only).
  - **A bad kept release is left out, not the series (RED-073).** A
    release is information the node was given. One that does not check
    is left out: the series is followed with the parts proven without it
    (part 1 at least), the node logs a warning, `poo doctor` shows a
    `releases` warning, and `add-release` of a valid one replaces it. An
    import drops a release that does not check, and names it in the plan
    (`dropped_releases`).
  - **`add-release` checks every release (RED-074).** A release of a part
    already known is checked in full on the CLI and RPC path too, so a
    forged one is refused instead of answered with `added: false`. Only
    the peer path (phase 3) may take a known part without hashing. Release
    JSON refuses a key it does not know (`ErrUnknownKey`). The keys `poo
    series release` adds for people (`part_id`, the serial and pool
    ranges, `mints`, `units`) are read and ignored.
  - **The chain is not worked out again on each load (RED-075).**
    - ORANGE-035 (#167, merged into this branch) shares part ids within a
      process, so a reload no longer hashes the chain again.
    - Across processes, the node keeps its own cache,
      `releases-checked.json` beside node.json, holding the series, part
      and previous id of each kept release it checked in full. `Load` uses
      an entry only when it matches the kept release exactly, and seeds
      the one id `part_id(k − 1)`. The release's witness is still checked
      each time, which needs no chain. A missing, damaged or stale cache
      only costs time. The cache is never part of an export, so a release
      another node wrote is always checked in full.
    - The RPC's `add-release` checks the release before it takes the
      policy lock, so other policy commands are not held for the hashing.
      The add after it finds the ids already worked out.
    - Still no cap on part numbers (D165).
  - **The series_id text is quoted (RED-076).** The id-mismatch error
    prints the file's `series_id` with `%q`, so terminal codes in it are
    escaped.
- **Consensus:** none. What is valid is unchanged. The cache only seeds
  an id this node itself worked out and checked. A cache that does not
  match is not used, so it can make a node slower, never change a result.
  Leaving out a bad kept release means the node knows fewer parts
  (missing information, never a hard fork).
- **Tier:** Node policy, RPC and CLI.
  - `pkg/nodepolicy/blue026_test.go`: `TestFollowSeesReleasedParts`,
    `TestUnfollowKeepsAVariableSeriesWhole`, `TestBadKeptReleaseIsReplaced`,
    `TestAddReleaseChecksAKnownPart`, `TestCheckedReleasesCache`.
  - `pkg/nodepolicy`: `TestVariableFollowed` (a bad release and a fixed
    series' release are now left out, not the series).
  - `pkg/series/blue025_internal_test.go`: `TestTrustReleaseSeedsOnlyThatPart`.
  - `pkg/cli/blue026_test.go`: `TestSeriesIDTextIsQuoted`,
    `TestReleaseFileKeys`.

### D168. Round 26: releases between nodes, limits in place, SLH-DSA, work tries from the floor

- **Decision:** four pieces of Source Round 26 Part A, and Green's daemon
  change from Round 25 (GREEN-050, #168), which is accepted as merged: a
  node reloading onto a changed node.json waits up to 10 s for its own
  directory lock, held for a moment by a command, instead of stopping.
  - **Variable supply phase 3: releases pass between nodes.**
    - A new wire v1 message, `release` (type 14), carries a release's byte
      form in its series' section. A node that does not know it ignores
      it, as every unknown type is ignored, so the protocol version stays
      1 (wire protocol v2 is not built).
    - A node sends a peer the newest release of each variable series it
      follows when they connect, and passes each release it takes on to
      its connected peers that follow the series.
    - The receiver drops a release of a series it does not follow, or of
      a part it already knows, with no hashing (D165's point 4), and
      counts it as a repeat. A later one is checked in full, witness
      first, so only the issuer can make a node work out a long chain. It
      is then kept in node.json and added to the running series
      (`node.AddRelease`). A release that does not check is an offence:
      its validity depends only on the series, so no honest node passes
      one on.
    - The node's edits of node.json now share one lock, the node's own
      (`node.PolicyMu`), which the RPC and the p2p host both take, so a
      release from a peer and a command's edit never lose each other's
      change.
    - **In place, as a reload would (ORANGE-035's design, accepted by
      Black in Round 25).** The host indexes the part's pools as it does
      on start (`Host.AddPools`). Where it cannot promise what a start
      gives, the node reloads onto the saved node.json instead: a series
      followed by a list of pools (its coverage can change) and a series
      followed whole that the release makes large enough to be indexed by
      the pools in use (D154). `poo series add-release` and `POST
      /v1/series/release` take the same path and reply `in_place`, or
      `reloading`.
    - A series is built before its transactions are read (Black, Round
      25): a release only names a series the node built at start, and a
      release changes nothing about how a transaction is decoded.
  - **In-place `poo policy limits -set` (ORANGE-033, on Black's Round 25
    terms).**
    - `nodepolicy.InPlace` names the limits a running node applies with no
      reload: `p2p.max_book`, `p2p.book_per_range`, `p2p.retry_seconds`,
      `p2p.candidate_seconds`, the four `tracking.*` limits and
      `seen_size`. Each part ends as a reload onto the same node.json
      would leave it. The book takes the limits and runs the same `Fit` as
      on start (`Book.SetLimits`); the tracker takes its retention and
      trims nothing until its next record, as `New` does
      (`Tracker.SetRetention`); the engine gets a new, empty seen list, as
      a reload has (`Engine.SetSeenSize`).
    - The change is taken against the policy the node runs with
      (`node.Running`), not the node.json read before it. When node.json
      holds anything else the node does not run yet, such as a hand edit
      waiting for `poo reload`, the reload is used, so that edit is never
      left out (`nodepolicy.SameButInPlace`). `GET /v1/status`'s
      `policy_changed` compares with the running policy too.
    - `p2p.verified_per_range`, `p2p.registry_seconds`, `max_pending` and
      `ties` stay on the reload. A reload makes every verified peer a
      candidate again, so a lowered `verified_per_range` would hold after
      a reload but not in place; the other three need their own in-place
      path and test first. `k`, `check_seconds` and chase pacing are not
      settable limits and are untouched.
    - The tracker now trims records of the same age in address order, so
      the same records and bounds always trim alike: in place, after a
      reload, and between two reloads. The order before was the map's.
  - **SLH-DSA-SHA2-128s, key algorithm 10** (jerry, 2026-11-04 08:09). FIPS
    205, pure form, empty context, randomised signing, through
    `github.com/cloudflare/circl/sign/slhdsa` (v1.6.3). A 32-byte key and
    a 7,856-byte signature, under `MaxSigSize` (8,192). Keys come from a
    seed as every algorithm's do (HKDF-SHA512, info `"PoO-key-v1
    slh-dsa-sha2-128s 0"`, three 16-byte seeds). The larger sets (192s,
    256s and the "f" sets) do not fit `MaxSigSize`, a decode limit every
    node shares; adding them means raising it, which is for jerry and
    Black, so they are not added. A series allows the algorithm only by
    listing it in `key_algs`, so no existing series changes. *(D170
    removed `MaxSigSize` and added SLH-DSA-SHA2-256s as algorithm 11.)*
  - **Work tries follow the series' floor** (Orange's condition for the
    raised floors in the reserve series advice, jerry 08:16).
    `rpc.WorkTriesFor(series)` is the node's cap on one work call: 2^24,
    or 2^(floor + 4) when the layer floor (`min_layer_strength`) makes that
    larger, at most 2^40. `rpc.DefaultWorkTries(series)` is `poo work`'s
    default with no `-tries` or `-for`: 2^16, or 2^(floor + 2) when that
    is larger, within the cap. `poo build -work N` is capped the same way
    by the node.
  - **GUI Grey's asks (GUI-GREEN-052):** a command refused only because
    its part is not released yet exits 3 (`cli.ExitWaiting`, `cli.ExitCode`),
    as the RPC answers 409 `waiting`; `poo series parts` adds
    `mints_released` to each part, the mints that exist once it is
    released.
- **Consensus:** none new. Passing releases is information: a node that
  has not seen a release only waits for it (D166), so how soon it hears
  changes nothing it accepts. In-place limits are an optimisation of the
  reload and end in the same state, checked by comparison tests at the
  floors and ceilings, lowered and raised. SLH-DSA is the key type jerry
  asked for in this round; it is a new algorithm number that a series must
  list, and a series a node cannot build is unreadable there, never
  misread. The work cap and default are tooling.
- **Tier:** P2P, node, node policy, RPC and CLI.
  - `pkg/p2p/release_test.go`: `TestReleasesPassBetweenPeers`,
    `TestReleaseInPlaceMatchesReopen`.
  - `pkg/peers/inplace_test.go`: `TestSetLimitsMatchesReopen`.
  - `pkg/tracking/inplace_test.go`: `TestSetRetentionMatchesReopen`.
  - `pkg/rpc/inplace_test.go`: `TestLimitsSetInPlace` (floors, ceilings,
    back to the defaults, the selected tips and followed pools unchanged,
    the limits that reload, a waiting hand edit reloads).
  - `pkg/rpc/worktries_test.go`: `TestWorkTriesFollowTheFloor`.
  - `pkg/policy`: `TestEveryAlgorithm` and the known answers gain
    `slh-dsa-sha2-128s`.
  - `pkg/cli`: `TestRivalClaimCLI` (in place and reload),
    `TestVariableSeriesOnANode` (`add-release` in place, exit code 3),
    `TestExitCodeWaiting`, the parts test (`mints_released`).

### D169. Round 26 fixes: RED-077 to RED-081 (BLUE-027)

- **Decision:**
  - **No tool offers an address that can never be spent (RED-077,
    RED-079).**
    - `Registry.CheckSpendable` works out the least witness data a policy
      can be satisfied with:
      - a threshold needs K signatures of the smallest sizes among its
        keys;
      - k-of-policies needs its K cheapest parts;
      - a hashlock needs an empty preimage.
    - It refuses (`ErrUnspendable`) a policy for which even that data is
      over `MaxDataSize`, whose params are over `MaxParamsSize`, or whose
      witness is over the series' `max_witness`.
    - The error says how many of the keys' signatures fit: at most 14
      ML-DSA-87, or 8 SLH-DSA-SHA2-128s.
    - `CheckPolicy` runs it, so `poo wallet policy`, a payment request,
      and a payer's check before paying an address with its policy all
      refuse such a policy.
    - `poo crypto` prints `max_signatures_per_witness` for each algorithm.
      *(D170: the decode limits are gone. The check now uses the worst
      case and the series' ceiling only, and `poo crypto` counts against
      a ceiling, the reserve series' by default.)*
    - The decode limits stay as they are. The check is never part of
      checking a witness. Every witness that decodes and fits already
      passes it, since the sizes it adds up are the least a witness of
      the policy can have.
  - **Releases from peers are coalesced (RED-078).** This builds on
    ORANGE-036 (#178, merged into this branch), which credits and passes
    on only the copy that was taken.
    - A release has its witness checked on arrival
      (`Series.CheckReleaseSigned`: one signature, no chain). A forged one
      is an offence at once, so a forger cannot hold a place in the queue.
    - The first release of a series is taken at once. Any that arrive in
      the next 2 seconds (`releaseWindow`) wait, and only the highest is
      taken when the window ends; its sender is the one credited. A release
      proves every part before it, so nothing is lost.
    - A replay of parts 2 to 200, oldest first, now makes a node take
      about two of them, not 199. Each take is a save, a relay, a reload
      where one is needed, and a first-delivery credit.
    - A copy of the part the node holds is not counted as a repeat,
      because honest links cross. Older parts still are.
  - **Hand edits of grind jobs and manual peers wait for the reload
    (RED-080).** `SameButInPlace` no longer strips `grind` and `p2p.peers`.
    The RPC edits that apply them at once write them into the running
    policy (`node.UpdateRunning`): a peer kept with `peer add -keep`, and a
    grind schedule the grind manager started. Any other difference is a
    hand edit, so `limits -set` reloads.
  - **A large work request is bounded in time (RED-081).** A work request
    with more than `MaxWorkTries` (2^24) tries and no `for` stops at
    `MaxGrindFor` (4 minutes), as a `for` request does. The reply says
    `stopped_on_time`, or the error says where it stopped. `poo work -h`
    says so.
- **Consensus:** none.
  - `CheckSpendable` refuses only policies that no valid witness could
    satisfy, and only where addresses are made or paid.
  - Coalescing changes when a node takes a release, never whether it is
    valid. A release waiting in the window leaves its part waiting, which
    is missing information, and the highest one is still checked in full.
  - The in-place and work changes are local.
- **Tier:** Wallet and CLI checks, p2p, RPC.
  - `pkg/policy/blue027_test.go`: `TestCheckSpendable` (14 and 15 of 16
    ML-DSA-87, 16 of 16 ML-DSA-65, 8 and 9 of 10 SLH-DSA, mixes, nested
    policies, `max_witness`).
  - `pkg/p2p/blue027_test.go`: `TestReleaseReplayIsCoalesced`. With no
    window, the same replay takes 199 releases.
  - `pkg/rpc/inplace_test.go`: `TestInPlaceLeavesHandEditsToTheReload`.
  - `pkg/rpc/blue027_test.go`: `TestLargeWorkStopsOnTime`.

### D170. Round 27: per-series size limits, SLH-DSA-SHA2-256s, the default reserve series

jerry approved all three on 2026-11-04: the per-series limits ("Yes I'd
like this to be worked in and refactored"), Red's revised key list for the
default series, and Black's approach to Red's first condition ("Blacks
approach"). The plan is Black's `consensus/size-caps-trace.md`, with Red's
conditions from `consensus/red-size-caps-review.md` and the doc list from
`consensus/size-caps-doc-changes.md`.

- **Decision:**
  - **No network-wide witness size limit.** `MaxKeySize` (4,096),
    `MaxSigSize` (8,192), `MaxParamsSize` and `MaxDataSize` (64 KiB each)
    are removed. No design document named them (Black's trace): Proof of
    Ownership 6.2, 6.3 and 6.5 ask each type and algorithm to declare its
    bounds and each series to set its ceiling, and that is now all there
    is.
    - A witness is bounded by its **type's declared bounds** (16 keys,
      16 parts nested 4 deep, a 1,024-byte preimage; unchanged), by its
      **algorithms' exact key and signature sizes**, and by its
      **series' witness ceiling** (`max_witness`, D19), which `Check`
      applies first, before any hash or signature.
    - Decoding reads params and data bounded only by the bytes received
      (`Reader.Raw` refuses a length the input cannot hold), so what a
      node allocates stays in proportion to what it was sent (as D117).
      Decoding needs no series, so a node that has not built a series
      still frames its transactions and never judges them: an unknown
      series is unreadable, never invalid (Red's condition 3, PoO 16.7).
      `tx.DecodeFrom`, a series part's issuer witness, a node policy's
      issuer witness, the wallet and the fuzz targets all go through the
      same `policy.DecodeWitness`.
    - **Each signature is read at its key's exact size** (Red's condition
      2). Threshold data is decoded against its params: exactly K
      signatures, key indices increasing and in range, and each signature
      read with its algorithm's `SignatureSize` as the limit, all before
      a signature's bytes are copied. A known algorithm's public key is
      read at its exact size too. The verdict is the one the exact checks
      at verify time gave before; it only comes sooner.
    - Series with a ceiling at or below 64 KiB decide exactly as before.
      Series with no ceiling ("Not set", still valid under PoO 16.1) or a
      larger one become looser: ML-DSA-87 16-of-16 and SLH-DSA-128s
      16-of-16 now fit the reserve series' 128 KiB.
  - **The cross-version test** (Black's condition 2, Red's condition 4).
    `pkg/policy/testdata/crossversion.json` holds nine witnesses, each
    judged under ceilings 0, 64 KiB and 128 KiB. Its `old` verdicts were
    made once by a real build of main @ `439b369`, before this change
    (`testdata/crossversion-old/main.go`), and are pinned.
    `TestCrossVersion` checks that every witness the old build took this
    build takes, that the two decide the same under a ceiling at or below
    64 KiB, and that the witnesses over the old caps (16-of-16 ML-DSA-87,
    9-of-9 SLH-DSA-128s, params over 64 KiB) are refused by the old build
    and taken by this one under no ceiling or 128 KiB. The SLH-DSA-256s
    vector is marked unreadable to the old build: it cannot build a series
    that lists algorithm 11. The change is safe only because no build has
    been released (no v0.1.0, private repo). After a release, the same
    kind of change would need new algorithm or type numbers.
  - **Red's condition 1 as tool and node policy, not a rule** (jerry chose
    Black's approach). No design document changes, and `max_witness` 0
    stays a valid series.
    - `poo series new` writes a 128 KiB ceiling by default and refuses
      `-max-witness 0` or one above `CeilingLimit` (4 MiB) without
      `-any-ceiling`. `poo init -series FILE` refuses such a file the same
      way. `poo init -example` is exempt: it is the documents' worked
      example.
    - Node policy `series_ceiling` (0, the default: off): when set, a
      policy import declines a series it would add whose ceiling is not
      set or above it, with its follow rules, and the plan lists it under
      `declined`. It never drops a series the policy already lists.
      Declining is a lack of information, not a verdict on a ledger, so it
      cannot be a hard fork. Nodes are not required to set it; Orange and
      Red may recommend a default.
  - **Spendability per series, worst case** (Red's condition 5).
    `CheckSpendable` (D169) now refuses a policy whose largest witness
    does not fit the series' ceiling, so every set of signers the policy
    allows can spend: K of its largest signatures for a threshold, the K
    dearest parts for k-of-policies, a 1,024-byte preimage for a
    hashlock. With no ceiling every policy passes. The error says whether
    even the smallest witness is over, and how many of the keys'
    signatures surely fit. `policy.MaxK(alg, ceiling)` gives the largest
    k of a k-of-k policy of one algorithm under a ceiling, params
    included. `poo crypto [-max-witness N]` shows it for every algorithm,
    against the reserve ceiling by default.
  - **SLH-DSA-SHA2-256s, key algorithm 11** (Red's revised list, jerry
    20:21). FIPS 205 category 5: 64-byte keys, 29,792-byte signatures,
    through circl's `SHA2_256s`, derived from a seed as algorithm 10 is
    (HKDF-SHA512, info `"PoO-key-v1 slh-dsa-sha2-256s 0"`, three 32-byte
    seeds). A series allows it only by listing it. The 192-bit set is left
    out on purpose. The known-answer vectors gained its entry.
  - **The default reserve series** (adopted 2026-11-04).
    `series.Reserve()` returns it: fixed, 32,768 mints of 10^14 units,
    pools of 32 (1,024 pools), SHA-512/256, entry floor 20 and layer
    floor 24 with no entry credit, threshold and hashlock, Ed25519,
    ML-DSA-65, ML-DSA-87, SLH-DSA-SHA2-128s and SLH-DSA-SHA2-256s, a
    131,072-byte ceiling, no issuer.
    - Its id is pinned: `fe52c1ac4178cc691c871524e53c4d1d2614a06b9dc68da13a19376b6591f139`
      (`series.ReserveID`). `pkg/series/testdata/reserve.json` holds its
      params, their canonical byte form and the id;
      `TestReserveSeriesPinned` checks all three.
    - `poo init` with no `-series`, `-example` or `-reserve` follows it
      and prints "the built-in reserve series" with its full id.
      `-reserve` adds it beside others and `-no-series` follows none.
    - `poo series info`, `poo series followed` and the RPC's series list
      add `"builtin": "reserve"` only when the full id matches, never on a
      prefix, so a look-alike id is not named the reserve (Red's earlier
      advice).
    - It is node policy, not a rule: any node may follow any series.
  - **`in_place` on every limit** (GUI-GREEN-053). `poo policy limits`
    and the RPC's limits reply say, for each bounded setting, whether a
    running node applies a change without a reload, from the same
    `nodepolicy.InPlace` list the node uses.
  - **A far part id can be cancelled** (GUI-028). `Series.PartIDContext`,
    `UnsignedContext` and `CheckReleaseContext` stop when their context
    is done, between two chunks of hashes, and keep what they worked out.
    `poo series release`, `poo series release -check`, `poo series parts`,
    `poo series info` and `poo series add-release` with the node stopped
    (`nodepolicy.Policy.CheckReleaseContext`) use the command's context, so a GUI job's
    Cancel (or Ctrl-C) stops a part far out instead of hashing for hours.
    Stopping says nothing about validity. The plain methods are unchanged.
- **Why:** The documents ask for declared per-type and per-algorithm
  bounds and a per-series ceiling, and never for a network-wide number. A
  network-wide cap stopped custody the documents allow (RED-077, RED-079)
  and kept SLH-DSA-256s out, while a limit that lives in the series id and
  the algorithm list is the same on every node that reads the series, so
  two nodes with the same information cannot disagree. The reserve series
  is the one place where the ceiling and the key list are fixed for ever,
  so it had to be built with both.
- **Consensus:** jerry-approved consensus change, made before v0.1.0 as
  Black and Red required. It only loosens: no witness valid before
  becomes invalid (the cross-version test). New algorithm 11 is
  unreadable to older builds, never a disagreement. The tool refusals,
  `series_ceiling`, the reserve default and `in_place` are tools and node
  policy.
- **Tier:** Consensus (decoding and verification), CLI, node policy, RPC.
  - `pkg/policy/crossversion_test.go`: `TestCrossVersion`.
  - `pkg/policy/d170_test.go`: `TestDecodeWitnessNeedsNoSeries`,
    `TestThresholdSignatureAtExactSize`.
  - `pkg/policy/blue027_test.go`: `TestCheckSpendable` (now worst case
    and per series), `TestMaxK`.
  - `pkg/series/reserve_test.go`: `TestReserveSeriesPinned`,
    `TestReserveSeriesValues`.
  - `pkg/cli/d170_test.go`: `TestInitWritesTheReserveSeries`,
    `TestToolsRefuseAMissingOrHugeCeiling`, `TestCryptoMaxK`,
    `TestSeriesInfoNamesTheReserve`.
  - `pkg/nodepolicy/bundle_test.go`: `TestSeriesCeilingDeclinesNewSeries`,
    `TestLimitsSayInPlace`.
  - `pkg/series/gui028_internal_test.go`: `TestPartIDContextCancels`;
    `pkg/cli/d170_test.go`: `TestFarPartIsCancellable`;
    `pkg/nodepolicy/bundle_test.go`: `TestCheckReleaseContextCancels`.

### D171. Round 27 fixes: RED-082 and RED-083 (BLUE-028)

- **Decision:**
  - **RED-082.** A release from a peer is checked in full in one worker
    per series, never in the peer's read loop. The witness is still
    checked on arrival (one signature) and bursts still coalesce (D169):
    the worker takes the first release at once and, after it, the highest
    one held. The walk runs under the host's context
    (`node.AddReleaseContext`, `nodepolicy.CheckReleaseContext`), so
    stopping the node cancels it, and a cancelled check is not an offence.
    `POST /v1/series/release` and the RPC policy check use the request's
    context. A part above 2^20 is logged when its check starts.
  - **RED-083.** `series_ceiling` is now a bounded setting (0, or 1,024
    bytes to 1 GiB), settable with `poo policy limits -set`. An import
    plan lists in `unbounded` each series it adds, or whose follow it
    changes, that sets no witness ceiling or one above 4 MiB
    (`nodepolicy.CeilingWarning`, `CeilingLimit`). The text plan prints
    declined and unbounded series, each declined one with the number of
    follow rules left out, and says `nothing else to change` rather than
    `nothing to change` when it declined one. `poo series info`,
    `poo series followed`, `GET /v1/series` and `poo doctor` (`ceiling`)
    show the warning for followed series.
  - **Not done here.** A 4 MiB `series_ceiling` default for new nodes
    (Orange #188, proposal 1): it would decline the documents' example
    series (no ceiling) on import, so it is left to Grey and jerry. The
    open-file-limit warning (#188, proposal 2) and keygen addresses per
    series (Green #189) are separate PRs.
- **Why:** A far part is a long walk by design (D165: no cap on parts), so
  the fix is to keep it from blocking anything else and to let it stop,
  not to refuse it. A ceiling-less series is valid and may be wanted, so
  it gets a warning, not a refusal.
- **Consensus:** None. Where and when a release is checked changes, never
  whether it is valid. A cancelled check leaves the part waiting, which is
  missing information. Warnings change nothing a node accepts.
- **Tier:** Node, p2p, node policy, CLI, RPC.
  - `pkg/p2p/blue027_test.go`: `TestFarReleaseDoesNotHoldTheNode`.
  - `pkg/nodepolicy/blue028_test.go`: `TestImportWarnsOfUnboundedSeries`,
    `TestDeclinedSeriesCountsItsFollows`, `TestSeriesCeilingIsBounded`.
  - `pkg/cli/blue028_test.go`: `TestPlanTextNamesDeclinedAndUnbounded`.

### D172. Entry bodies on disk, behind a bounded cache (disk-backed state, step 1)

- **Decision:**
  - **Store locations.** Every log record has a location (`store.Loc`:
    the file's generation, offset and framed length). `Log.ReadAt` reads
    one record back, checking every frame's checksum; bytes that are not
    one whole record give `ErrBadRecord`, and a location from before a
    compaction gives `ErrStaleLoc`, so nothing is ever read at the wrong
    place. `OpenFunc` streams the records with their locations instead
    of returning them all, `AppendLaterAt` returns the new record's
    location, and `RewriteFunc` asks for the records one at a time and
    returns their new locations.
  - **Entry skeleton.** A pool keeps, for every held entry, what
    selection, switching and checking read: hash, parent, height, own and
    ledger weight, layers, items and undo. With a `ledger.BodySource`,
    an entry's body (transactions and witnesses) is dropped from memory
    once the source can give it back, and read through one
    `ledger.BodyCache` per node, a byte-bounded LRU, when something asks
    for it: serving a peer or a query, a rival search, a compaction. **No
    verdict reads a body that is not in memory**, so the cache's size
    changes what a node costs, never what it decides. A failed read is
    counted and hides the body from that query only.
  - **Node.** Each pool's locations are built while its log is replayed
    at start (one window of 1,024 records at a time, never the whole
    log) and as records are appended, and rebuilt by each compaction,
    which copies entry records from the old file by location rather than
    reading every body into memory. A read back is used only when the
    record is an entry whose hash is the one asked for.
  - **`entry_cache_mib`** (node policy, bounded 1 to 1,048,576, default
    512) bounds the cache across all pools, and is applied in place.
    `GET /v1/status` (`poo status`) shows `bodies` (bound, use, hits,
    loads, errors, held, in memory, located) and `/v1/metrics` the
    `poo_entry_cache_*` and `poo_entry_body_*` series.
  - **Open-file warning (ORANGE-037, proposal 2).** A node logs one
    warning at start, and `poo doctor` an `open-files` check, when the
    soft open file limit is below the followed pools plus 256.
    `GET /v1/status` shows `file_limit`.
  - **Deferred.** The sidecar index file of the plan moves to R30, beside
    snapshots: start-up reads the whole log anyway, so locations cost
    nothing to build then. Items and undo outside a recent window, and
    peer sync served from disk, are R29.
- **Why:** jerry's priority (2026-11-04): a node held every entry body in
  memory for its whole history, tens of GB a year for a busy series.
  Bodies are the largest part and are read only to serve, so they move
  first.
- **Consensus:** None. No rule, encoding or wire message changes. No
  depth limit: every entry is still held and checked as before; only where
  its bytes live changes. A node with a tiny cache selects, holds and
  serves exactly what a node with every body in memory does (the
  differential tests below). A body that cannot be read back is a lack of
  information for one query, never a verdict.
- **Tier:** Store, ledger, node, node policy, RPC, CLI.
  - `pkg/store/readat_test.go`: `TestReadAtLocations`.
  - `pkg/ledger/bodies_test.go`: `TestBodyCacheBound`; `runRandom`
    (`property_test.go`) now runs a twin pool with a 1 KiB cache beside
    every random history and compares tip, weight, state and bodies.
  - `pkg/node/bodies_test.go`: `TestBodiesFromDisk` (no cache, through a
    compaction and a restart), `TestEntryCacheLimit`;
    `bodies_internal_test.go`: `TestPoolBodiesChecks`.
  - `pkg/nodepolicy/filelimit_test.go`: `TestCheckFileLimit`,
    `TestFollowedPools`.

### D173. Round 28 fixes: RED-084, with ORANGE-038 and its cache sizing (BLUE-029)

jerry approved Orange's cache sizing on 2026-11-04 ("That's fine"):
count `entry_cache_mib` in real bytes, with a 256 MiB default.

- **Decision:**
  - **An unreadable body is not held, for the caller.** `Pool.Entry(h)`
    returns `(*entry.Entry, error)`: `ErrNoEntry` or `ErrUnreadable`, and
    never a nil entry without an error, so `e, _ :=` no longer hands a
    caller a nil it can use. Every caller was checked:
    - p2p serving: `entries` lists such an entry as not held, `parts`
      replies `not held`, and an extension ends before it (`not held`
      when it would be first). Nothing is encoded, and the asker fetches
      the entry from another peer.
    - `Rivals` searches each entry's skeleton items, which keep every
      input, so it reads no body, costs no disk read under the engine
      lock, and cannot miss a rival.
    - `Engine.Payment` and `GET /payment` say `unreadable` (no `tx`); the
      rivals are found from the skeleton (`Pool.Tx`).
    - `watch.LedgerPage` and `AddressPage` count `unreadable` entries, and
      a ledger page lists each with `unreadable: true`.
    - The watch hub leaves such an entry's transactions out of its
      events; compaction already stops with an error (D172).
  - **Defence in depth.** A panic while handling one peer message closes
    that peer, is logged with its stack, and is not counted against the
    peer. The process runs on.
  - **`poo doctor` warns** (`bodies`) when the running node counted
    failed body reads, and says to check the disk and restart: on start
    the node cuts a pool log at its first damaged record and fetches the
    rest again from peers. Refetching in place, with no restart, is left
    for later; it needs the log to take a second record of an entry.
  - **ORANGE-038 (#201) is carried:** bodies are released with their
    items' witnesses, and the chase walks parents from the skeleton.
  - **Cache sizing (Orange's proposal 2, jerry):** a body counts as its
    byte form plus 1 KiB, as measured, and `entry_cache_mib` defaults to
    256. Orange's proposal 1 (reading bodies outside the engine lock when
    serving) is left for Grey.
  - **GUI-029** needs nothing more from the source: D171 already lists a
    ceiling-less series in the plan's `unbounded`, as a warning only, and
    `series_ceiling` stays off by default. The GUI shows it.
- **Why:** A body read can fail since D172, so "held" no longer means
  "readable". Treating an unreadable body as missing information is what
  a node does with any entry it lacks: it is fetched again, and nothing is
  decided from it.
- **Consensus:** None. No verdict reads a body that is not in memory
  (D172). The cache's size changes cost only. A node that cannot serve an
  entry leaves its peers short of information, never in disagreement.
- **Tier:** Ledger, p2p, engine, watch, RPC, CLI, node policy.
  - `pkg/p2p/blue029_test.go`: `TestDamagedLogDoesNotCrashServing`.
  - `pkg/node/blue029_test.go`: `TestDamagedLogQueriesSayIncomplete`.
  - `pkg/ledger/bodies_test.go`: `TestBodyCacheBound` (a failed read
    returns `ErrUnreadable`).

### D174. A panic mid-change stops the node; bodies served outside the engine lock

- **Decision:**
  - **Black's Round 28 condition.** D173's per-peer recover stays, but a
    panic while the engine changes a pool (`HandleTx`, `HandleEntry`,
    `HandleLayer`, through their one `handling` guard) now fails the
    engine first: `engine.ErrFailed`, `Failed()` closed, `Err()` set. The
    engine then takes nothing more; reads still answer. The daemon stops
    the node on `Failed()`, logging why, and `Node.Close` and `Compact`
    leave the stores as written, never rewriting them from a possibly
    half-changed pool. A restart rebuilds from the stores, as before D173.
  - **Orange's proposal 1 (ORANGE-038).** A ledger reply takes, under
    the engine's lock, only each entry's `ledger.BodyRef` (the body if
    still in memory, or what to read it through) and its layer nonces;
    bodies are read back and encoded after the lock is released
    (`BodyRef.Get` needs no lock). Extension, whole-ledger, by-hash and
    parts replies all do this.
- **Why:** A pool left half changed by a panic and kept running could
  select on a state no full replay gives; stopping is what a crash did.
  A cold reply of many bodies read from disk held the engine's lock for
  tens of milliseconds (ORANGE-038).
- **Consensus:** None. What a reply holds is unchanged; only when bodies
  are read changes. Stopping on an internal fault is a lack of
  information until the restart.
- **Tier:** Engine, node, daemon, p2p.
  - `pkg/engine/failed_test.go`: `TestPanicMidChangeFailsTheEngine`.
  - The p2p serving tests run with bodies released (D172).

### D175. Items and undo outside a recent window (disk-backed state, step 2)

- **Decision:**
  - An entry of the selected ledger more than `recent_window` entries
    below the tip, whose body the store holds, is **cold**: its items
    (its transactions, without witnesses since D173) and its undo record
    are dropped from memory. It keeps its skeleton: hash, parent, height,
    weights, layers, its tids in the index, the count of its transactions
    and the mints it claims.
  - **No undo file.** Items come back from the body (`BodySource`). The
    undo record is recomputed: each input an entry spends is an output of
    the transaction that made it, which is on the selected ledger below
    it (an entry never spends what its own transactions make) and found
    through the tids index. So the plan's per-pool undo log is not
    needed, and a pool still keeps one open file.
  - **Switches.** A switch whose fork is below the window reads back the
    items and undo of every entry it reverts **before it changes
    anything**, and keeps them (those entries are now off the selected
    ledger, where items stay in memory). A view at an old anchor (an
    entry on a deep parent, a check of a branch) reverts cold entries on
    an overlay without keeping them. Nothing is refused for depth: a deep
    switch only reads the disk and is slower.
  - **A failed read** while the pool needs it panics with
    `ledger.ReadFault`; that fails the engine (D174), so the node stops
    and rebuilds from its stores. The panic can come after the pool
    already changed part way (Black, R29: `AddLayer` may have counted a
    layer, `AddEntry` may have inserted the entry), so the pool is not
    left untouched; what keeps the node safe is that a failed engine
    takes no more input, skips compaction and is rebuilt from the logs on
    restart (D174). A fault is never read as an invalid entry and the
    node never serves a selection made on partial data. In a read path
    (a query), the panic ends that query only. (Reworded in R30.)
  - **Node policy `recent_window`** (16 to 2^30, default 10,000), applied
    on reload. `bodies.cold` in `/v1/status` counts cold entries.
  - **Peer sync from disk** is D172 plus D174: replies read bodies back
    outside the engine's lock.
- **Why:** After D172 and ORANGE-038 the skeleton, about 0.9 KiB per
  transaction, was what still grew with history (ORANGE-038 proposal 3).
  Items and undo are most of it.
- **Consensus:** None. The same entries give the same selected ledger
  and state with any window: the recomputed undo is the record Apply
  made, and items are the body's transactions in its order. No depth
  limit. Checked by the random-history twin pool, now with a window of 2,
  where every seed exercises cold entries.
- **Tier:** Ledger, node policy, node.
  - `pkg/ledger/cold_test.go`: `TestDeepSwitchReadsBack`,
    `TestDeepSwitchReadFault`; `property_test.go` `runRandom`'s twin
    (window 2, required to go cold).
  - `pkg/node/bodies_test.go`: `TestColdEntries` (restart, compaction,
    an old payment from a cold entry).

### D176. Replace imports honour series_ceiling (GUI-030)

- **Decision:** A `-replace` import (`nodepolicy.Merge` with `Replace`)
  now keeps each series once, by id, and leaves out a series the node does
  not list yet, with its follow rules, when the node's own
  `series_ceiling` declines it, listing it in `declined` as a merge does.
  The ceiling is the node's, so a file cannot lift it by replacing it.
  (GUI Red's nit, leaving out `digest` from `-json` when nothing
  changes, is left for a round where the GUI changes with it: the GUI
  views e2e applies a no-op plan today.) A comment that said `poo init` writes
  `series_ceiling` is corrected: nothing writes it; the default stays off
  (jerry, 2026-11-04). GUI-029's ask is already met by the plan's
  `unbounded` list (D171), which the GUI shows; `defences` stays the list
  of setting lines.
- **Why:** Only the merge path checked the ceiling (GUI Red, GUI-030).
- **Consensus:** None. Which series a node adds is information.
- **Tier:** Node policy, CLI. `pkg/nodepolicy/replaceceiling_test.go`:
  `TestReplaceHonoursSeriesCeiling`.

### D177. Round 29 fixes: RED-085 and RED-086, and `poo status -counts` (BLUE-030)

- **Decision:**
  - **RED-085.** `PreverifyTx`, `PreverifyEntry`, `PreverifyPage` and
    `BuildEntryOn` took the engine lock with a bare `Lock`/`Unlock`. They
    now take it through `Engine.under`, which always releases it and, on a
    panic (a `ledger.ReadFault` on a cold entry), fails the engine as
    `handling` does (D174): `e.failed` is set, `Failed()` is closed, and
    the panic goes on to whoever recovers it (the p2p work loop closes
    that peer, D173; the HTTP server answers 500). The node then stops on
    `Failed()` without compacting, and `Node.Close` no longer hangs on
    `Err()`. Once the engine has failed they do nothing, and
    `BuildEntryOn` returns `ErrFailed`. A read fault on a path that changes
    nothing still means the log cannot be trusted, so stopping is right.
    These were every `e.mu.Lock()` in `engine.go` without a deferred
    unlock that reaches the ledger; the others read only the skeleton.
  - **RED-086.** An entry's view cost (`unitsOf`) now counts one unit for
    each distinct transaction its inputs spend from, besides one for the
    entry and one per transaction. A cold revert reads at most one body
    for each, so a merge of many inputs from many older entries costs what
    it reads. Nothing is refused: the revert budget only paces a sender
    and the entry still comes (D111, D116). An ordinary chain of one-input
    spends costs 3 units an entry where it cost 2, so the 200,000 a minute
    default paces such reverts about a third sooner; it is a setting, and
    a new default is for Grey and Orange to measure.
  - **`poo status -counts`** (GUI ORANGE-021): one row of totals per
    followed series (pools, entries, branches, pending, waiting, stored
    records) with the same notes (replay, bodies, file limit, policy
    changed), so a view that refreshes often does not print a row for
    every pool. The CLI adds the rows up, so the GUI does not (D84).
  - **Not carried:** Orange #212 (mempool only, no overlap) and Green
    #209 (no overlap).
- **Why:** D174 made a panic mid-change fail the engine, but only on the
  paths through `handling`; a panic on a read path left the lock held,
  which is worse than a crash. The view cost exists to charge what a
  revert costs, and D175 made a cold revert read maker bodies the cost
  did not count.
- **Consensus:** None. Failing the engine stops the node; it decides
  nothing. The view cost decides only when a peer's entry is checked,
  never whether it is valid.
- **Tier:** Engine, ledger, CLI.
  - `pkg/engine/blue030_test.go`: `TestColdReadFaultReleasesTheLock`
    (BuildEntryOn, PreverifyEntry, PreverifyPage; it hangs without the fix).
  - `pkg/ledger/blue030_test.go`: `TestColdRevertCostCountsMakers`.
  - `pkg/cli/blue030_test.go`: `TestStatusBySeries`.
  - `pkg/p2p/serve_test.go`: `TestDeepForkSpendsRevertBudget` counts three
    units a spend.

### D178. Pool snapshots and the sidecar index (disk-backed state, step 3)

- **Decision:**
  - **What a snapshot is.** Beside each pool log, `<index>.log.snap`
    holds the pool as the node runs it: every held entry's hash, parent,
    first-sighting order, counts, claimed mints, layers (hash and nonce)
    and, for the selected ledger, its tids; the unspent portions at the
    tip; the pending transactions; and the **sidecar index**, where in the
    log each held entry's record is. It holds no entry body. It ends with
    a SHA-256 of everything before it and is written to a temporary file,
    synced and renamed, so it is whole or absent.
  - **Start.** A node uses a snapshot only if all of these hold: the
    checksum (a torn or damaged file), the pool (a file swapped from
    another pool), the build that wrote it (an upgrade replays once, so
    it never trusts a pool an older build computed), and the log's tag
    (`store.Tag`: the offset it covers and the first and last 4 KiB
    before it, so a log compacted, cut short or replaced gives another
    tag). It then restores the skeleton (`ledger.Restore`), gives the
    pending transactions again through the normal checks, and replays
    only the records after the snapshot. Entries more than
    `recent_window` below the tip start cold (D175); the rest read their
    items back and recompute their undo. Every held entry is restored, at
    any depth. Anything else means a full replay, as before, and the
    reason is in `replay.snapshots_unused` and the log.
  - **Writing.** A clean stop writes one when the log grew since the
    last; each compaction removes the snapshot before it rewrites the log
    and writes a new one after, so a snapshot never outlives the log it
    describes; a running node writes one every `snapshot_every` records
    (node policy, 1,000 to 2^30, default 50,000, in place), gathered under
    the node's lock and written to disk outside it. `no_snapshots` turns
    them off. A pool whose newest body is not written yet is skipped
    until the next one.
  - **Trust and `-reverify`** (jerry, 2026-11-05: trust the node's own
    snapshot, with a flag to check everything).
    `poo node -reverify` ignores the snapshots on that start, replays
    every log whole, and where a snapshot covered the whole log compares
    the pool the replay built with the snapshot's fingerprint
    (`ledger.Pool.Fingerprint`), reporting `reverified` and
    `reverify_differed` in `/v1/status` and `poo doctor`. A difference
    would be a bug; the node runs on the replay.
  - **Disk errors.** A body that fails to read back marks its pool
    damaged: its snapshot is removed and none is written. An engine that
    failed (D174) removes every snapshot at Close. So the next start
    replays the logs whole and cuts a damaged log at its first bad record,
    which peers then refill, rather than skipping the damage inside a
    snapshot's span. A body that fails to read during a restore makes the
    snapshot unusable, not the node.
  - **Compaction renumbers first sightings.** After a compaction the
    running pool takes the first-sighting order a replay of the new log
    gives it (`Pool.Renumber`: the selected ledger first, then the rest as
    they were, which is the order the compaction writes). Before this, a
    restart after a compaction could break a later tie between an
    off-chain entry and a formerly selected one the other way from the
    running node. Ties are broken by first sighting, which is local
    information, so this changes no rule; it makes restart and running
    agree, and the snapshot differential found it.
  - **The sidecar index** is the snapshot's index section, so it is built
    now rather than deferred: a start from a snapshot reads no record the
    snapshot covers. The plan's separate index file is not needed.
  - **Status and doctor.** `/v1/status` and `poo status -counts` carry
    `snapshots` (pools, missing, the oldest's age, the most records a
    start would replay, bytes, `snapshot_every`); `replay` gains
    `snapshots`, `snapshot_records`, `snapshots_unused`. `poo doctor` adds
    a `snapshots` check.
- **Why:** jerry's priority (2026-11-04): disk-backed state, thinking
  long term. A start replayed and re-checked every record. Measured on 4
  cores, 20,000 entries with one transaction each (Ed25519): 0.60 s
  replaying, 0.10 s from the snapshot (about half of that reads back the
  10,000 recent bodies). With post-quantum witnesses, which cost more to
  check, the gap grows, since a snapshot start checks no witness the
  snapshot covers.
- **Consensus:** None. A snapshot is the node's own data, never sent or
  taken from a peer, and restores the pool the node had: the same
  entries, weights, selected ledger and state. Nothing is refused and no
  depth is limited. Checked by:
  - the random-history twin (window 2, bodies on disk), now restored from
    its snapshot every 37 steps and then run on against the in-memory
    pool, comparing switches, tips, weights and state after every step,
    with fingerprints equal at each restore;
  - node tests where a start from the snapshot, and from an older
    snapshot plus the records after it, build exactly the pool a full
    replay builds (fingerprint, status, holdings, pending), before and
    after a compaction.
- **Tier:** Ledger, engine, store, node, node policy, RPC, CLI.
  - `pkg/ledger/snapshot.go`; `snapshot_test.go`: `TestRestoreRefuses`,
    `TestRestoreAnyWindow`; `property_test.go` restores the twin.
  - `pkg/node/snapshot.go`; `snapshot_test.go`: `TestSnapshotStart`,
    `TestSnapshotAfterCompaction`, `TestSnapshotFaults` (torn, damaged,
    trailing bytes, another node's, not a snapshot),
    `TestSnapshotEvery`, `TestSnapshotOtherBuild`,
    `TestSnapshotDroppedOnDamage`.
  - `pkg/store`: `OpenFuncAt`, `Tag`, `Log.Size`; `snapshot_test.go`:
    `TestOpenFuncAtAndTag`.
- **Not built:** repairing a damaged body in place, by fetching it again
  from peers while the node runs. Today the node stops on a fault during
  a change, and serves without it on a read path, and the restart repairs
  it. In-place repair needs a fetch-by-hash path into the log and its own
  fault tests; it is proposed for a later round.

### D179. `builtin` in `poo status -counts` (GUI-GREEN-056)

- **Decision:** Each row of `poo status -counts` carries `builtin`, as
  `poo series followed` does (`"reserve"` for the default series), so the
  GUI can name the series rather than show only its hash.
- **Why:** GUI Green, R30B (PR #217).
- **Consensus:** None. Display only.
- **Tier:** CLI. `pkg/cli/blue030_test.go`: `TestStatusBySeriesBuiltin`.

### D180. No digest for an import that changes nothing (GUI Red's R29 nit)

- **Decision:** `poo policy import -json` and `poo settings import -json`
  leave out `digest` when no plan changes anything, as the text preview
  already did (GREEN-043). The GUI, since GUI R30 (#220), says "Nothing to
  apply" for such a preview and offers no Apply.
- **Why:** A digest pins an apply to what was previewed; with nothing to
  apply it only invited one. Held from D176 until the GUI changed with it.
- **Consensus:** None. Output only.
- **Tier:** CLI. `pkg/cli/policyport_test.go`:
  `TestPolicyImportNoOpHasNoDigest`.

### D181. Round 30 fixes: RED-087, RED-088 and RED-089 (BLUE-031)

- **Decision:**
  - **RED-087.** A snapshot's build id now names the release:
    the program name, `snap<N>`, the version and the commit, then the
    module version and VCS
    revision when the binary records them. The cli package hands its
    `Version` and `Commit` (set by `-X` in `scripts/release.sh`, which
    builds with `-buildvcs=false`) to the node with `node.SetRelease` at
    init. A dirty tree adds a hash of the executable, so two dirty builds
    of one commit differ. `snapshotSemantics` (now 1) is bumped by hand in
    the same commit as any change to what a pool is or what a snapshot
    holds, even one that leaves the file format alone, as a second guard.
  - **RED-088.** After `Engine.Restore` every start compares the restored
    pool's fingerprint with the header's. On a difference the snapshot is
    not used (`snapshots_unused` says why), and the log is replayed whole.
    So every start is a differential of `Restore` against `Snapshot`.
    With `-reverify` comparing a full replay with the same fingerprint,
    the two together compare a replay with what the snapshot restores.
    Under `-reverify`, a pool whose snapshot does not reach the end of the
    log is listed in `reverify_not_compared`, and `poo doctor` warns,
    never "ok". Red's other option, replaying up to the snapshot's offset,
    is not built.
  - **RED-089.** `<index>.log.snap.tmp` is removed when its log is
    opened, whatever the policy, and counted in `snapshot_tmp_removed`;
    `poo doctor`'s `snapshots` check names it.
  - **GREEN-055 (#224) is carried:** it changes the same `-reverify` code.
- **Why:** D178's guard against a pool an older build computed relied on
  a build id that every release build shared, and its full check compared
  against the writer, never the reader. A pool the new code would not
  build from the log is nodes with the same information disagreeing, so
  the guard has to hold for real.
- **Consensus:** None. A snapshot is a cache of what the log builds; not
  using one only costs a replay.
- **Tier:** Node, CLI.
  - `pkg/node/blue031_test.go`: `TestBuildIDNamesTheRelease`,
    `TestSnapshotRestoreIsChecked` (fails without the check),
    `TestReverifyBehindTheLogIsReported`, `TestSnapshotTmpRemoved`.
  - `pkg/cli/blue031_test.go`: `TestSnapshotsCheckNotComparedAndTmp`.

### D182. Round 31: ORANGE-040 adopted in part, and opt-in port mapping

- **Merged first:** Blue's #226 (D181, carrying Green's #224), Red's #223,
  Orange's #225 (ORANGE-040, measurements) and Black's #227 (Round 30
  review: everything clear).
- **Decision (ORANGE-040):**
  - **Proposal 1, in part.** Gathering a snapshot now walks the ledger
    nodes' transactions once (`Ledger.SnapshotPrint`) and computes the
    header fingerprint from that same pass, rather than two `tidsByNode`
    passes (Orange measured each at about 42% of the lock hold). The
    header fingerprint is kept: Blue's per-start check (RED-088, D181)
    compares a restored pool with it, and Black cleared keeping that
    check. Dropping the fingerprint, as the proposal asked, would have
    lost it.
  - **Proposal 2.** A pool's snapshot is due after
    `max(snapshot_every, records in its log / 10)` new records
    (`snapshotDue`), so a large pool is gathered less often while a small
    one keeps `snapshot_every`. `poo status` shows `snapshots.due`, and
    `poo doctor` measures "behind" against the larger of the two.
  - **Proposal 3** (gather outside the node lock) is not built yet.
- **Decision (port mapping, Source Red's recommendation, approved by
  jerry for Round 31):**
  - `p2p.port_map` is **off by default**. `auto` tries PCP (RFC 6887),
    then NAT-PMP (RFC 6886), then UPnP IGD. `pcp`, `natpmp` or `upnp`
    names one. `p2p.port_map_gateway` names the router's IP address. When
    it is empty, the default route's gateway is found on Linux only.
    `p2p.port_map_seconds` is the lease asked for, from 120 to 86,400
    seconds, 3,600 by default, applied on reload. `poo init -port-map`
    sets the mode.
  - **Only the gateway is spoken to.**
    - PCP and NAT-PMP use a UDP socket connected to the gateway's
      port 5351, so nothing else can answer. A PCP reply must carry the
      request's nonce.
    - A UPnP search answer must come from the gateway's address. Its
      description URL, `URLBase` and control URL must be plain `http`
      on the gateway's own IP; a host name is refused.
    - HTTP uses no proxy and follows no redirect.
    - Every reply is capped at 64 KiB. Each method waits at most
      3 seconds, resending at 250 ms, 500 ms and 1 s.
  - **Lease and renewal.** A mapping is renewed at half the lifetime the
    router granted, and removed at a clean stop.
    - A UPnP router that grants only permanent leases (error 725) gets
      a lease of 0. That mapping is removed at a clean stop and lasts
      past a crash.
  - **The advertised address comes from the router, never from peers.**
    1. The external address the router reports must be public, unless
       `allow_local_peers` is on. A carrier-grade NAT address is refused.
    2. The node then dials it and must get its own hello back (its own
       node id) before it offers the address to peers.
    3. If the dial does not come back, the mapping is removed and nothing
       is advertised. Many routers cannot loop a connection back;
       `poo doctor` then names the address to set by hand.
    4. A new external address on renewal is confirmed again before use.
    5. A `p2p.advertise` set by hand always wins, and then no mapping is
       asked for.
  - **Limits stay as they are.** Inbound connections keep every cap they
    had: `max_inbound`, the per-range book and verified limits, and the
    tracker.
  - **Machine-local.** `p2p.port_map` and `p2p.port_map_gateway` are
    machine-local (`MachineLocal`). A shared policy or settings file
    therefore never opens a port on another machine. `-full` carries
    them.
  - **Reporting.** `GET /v1/status` and `poo status` show `port_map`:
    its mode, state, method, the router's address, the advertised
    address, the lease and any error. `poo doctor` adds a `port-map`
    check when the policy asks for one.
- **Why:** Before this, a node behind a home router could make only
  outbound connections unless its owner forwarded a port by hand. Red's
  review found the risks: a router or LAN host lying about the address,
  UPnP URLs pointing elsewhere (SSRF), and peers steering what a node
  advertises. Mapping is opt-in because it has a privacy cost: peers
  learn the network's public address and that a node runs there, and the
  open port can be scanned.
- **Consensus:** None. Port mapping is node policy. It changes only
  whether other nodes can dial this one, which is information reaching
  a node, never which ledger is valid.
- **Tier:** p2p, node policy, CLI.
  - `pkg/portmap/portmap_test.go`: PCP map, renew and unmap, foreign
    nonce, refusal, NAT-PMP fallback, silent gateway, cancel, short
    replies, UPnP map and unmap, permanent-only leases, eleven ways off
    the gateway, oversized and redirected, the route table.
  - `pkg/p2p/portmap_test.go`: confirmed, advertised, renewed and removed;
    another node's address unconfirmed; not public; retry; a manual
    advertise wins; off by default.
  - `pkg/nodepolicy/portmap_test.go`; `pkg/cli/portmap_doctor_test.go`.

### D183. Round 31 fixes: RED-090, RED-091 and RED-092 (BLUE-032)

- **Decision:**
  - **RED-090.** The dial-back now proves it reached this node's own
    listener. `confirmSelf` puts a fresh random 128-bit token in the
    hello it sends (`Agent` is `poo-self-check/` and the token in hex, so
    the wire format is unchanged). The listener's handshake, on an
    inbound hello with this node's own id, records a token `confirmSelf`
    is waiting on (`sawSelfProbe`). Only that record confirms the
    address. The far end's answer is not trusted: it can only end the
    wait early with a failure (another node's hello, or a closed
    connection). An echo server or a copy of the node's hello therefore
    confirms nothing. This replaces D182's "must get its own hello back".
    One case stays: a host that relays the connection to this node's real
    listener passes. Connections to that address do reach this node, so
    the address works for peers, and a relay sees no more than the router
    already can.
  - **RED-091.** A failed request leaves nothing at the router:
    - UPnP asks for the external address before `AddPortMapping`, so that
      failing makes nothing.
    - A PCP reply that grants an address the node cannot use (not IPv4,
      or port 0) is removed with the same nonce.
    - A request whose answer was not read gets a short best-effort
      removal with its own context, at most 1 s. That covers the caller's
      context ending, and a UPnP call whose connection failed.
    - A renewal's mapping stays the caller's to remove, as before. Since
      each method cleans up after itself, `auto` moves to the next method
      with nothing left behind.
    - `exchange` now reports the caller's deadline as
      `context.DeadlineExceeded` rather than "no answer", so a cut-short
      wait is told apart from a silent router.
  - **RED-092.**
    - The renewal comes at half of the granted lifetime or of
      `p2p.port_map_seconds`, whichever is shorter (`renewAfter`), and
      never sooner than 30 s. A lifetime of 0 (until removed) counts as
      the lease.
    - A granted lifetime of 30 s or less is refused (`failed`, removed,
      not advertised), since it would lapse before the renewal.
    - A PCP reply with lifetime 0 is refused: in PCP that means no
      mapping.
    - The dial-back's dial, write and wait all end with the run's
      context, so a stop no longer waits up to 10 s.
  - **GREEN-056 (#234) is carried:** it changes the same `portmap.go`.
- **Why:** D182 relies on the dial-back to keep a router's claim from
  becoming the address peers are told. The old check compared the answer
  with a value the node had just sent, so a lying router or a forged
  NAT-PMP reply could steer peers to an address it controls. A mapping
  left after a failure is an open port the owner did not agree to. With
  a permanent UPnP lease, it stays until someone removes it.
- **Consensus:** None. Port mapping is node policy. It decides what a
  node tells peers about itself, never which ledger is valid.
- **Tier:** p2p, portmap.
  - `pkg/p2p/blue032_test.go`:
    - `TestBlue032DialBackNeedsOwnListener`, against an echo server and
      a copied hello.
    - `TestBlue032StopDoesNotWaitOnSilentDialBack`.
    - `TestBlue032ShortLifetimeRefused`.
    - `TestBlue032RenewalCappedByLease`.
  - `pkg/portmap/blue032_test.go`:
    - `TestBlue032UPnPExternalFailsLeavesNoMapping`, Red's router.
    - `TestBlue032UPnPCancelRemoves`.
    - `TestBlue032PCPUnusableRemoved`, Red's IPv6 answer.
    - `TestBlue032PCPLifetimeZeroRefused`.
    - `TestBlue032CancelSendsRemoval`, for PCP and NAT-PMP.
  - Each test fails on the code before the fix.

### D184. Round 32: a count of unused snapshots, unknown settings kept, the release in status

- **Merged first:** Blue's #238 (D183, carrying Green's #234), Red's #233,
  Orange's #235 (ORANGE-041, measurements), Black's #239 (Round 31
  review: all clear) and White's charter #241, which adds White to Part D
  beside Black (AGENTS.md "White reviews (Part D)") and lists WHITE-001
  (first public release plan) and WHITE-002 (version and features
  between peers) as proposed. Neither proposal is decided here.
- **Decision:**
  - **GUI ORANGE-023.** `replay.snapshots_unused` and
    `replay.reverify_not_compared` in `GET /v1/status` and `poo status`
    carry at most `node.MaxReasons` (8) reasons. New
    `snapshots_unused_count` and `reverify_not_compared_count` give the
    whole number. The node's start log names every pool with its reason,
    as before, and `poo doctor` counts all of them and says the rest are
    in the start log. Black's condition (doctor or the start log still
    lists each pool) holds.
  - **W-1.** `nodepolicy.Load` keeps the settings in `node.json` that
    this release has no field for, and `Save` writes them back. That
    covers the top level and inside objects at any depth, but not inside
    list entries (a series, a follow, a grind job). So going back a
    release and changing a setting no longer drops what the newer release
    set. The node warns once at start, naming them. Exports and bundles
    carry only what this release knows. A file holding unknown settings
    is written with its keys in sorted order.
  - **W-10.** `GET /v1/status` and `poo status` (with `-counts`) carry
    `release`: the `version` and `commit` that `node.SetRelease`
    records, the same ones `poo version` prints. GUI/White decides what
    the GUI does on a mismatch.
  - **Not taken this round:**
    - ORANGE-041 proposal 1 (a parallel fingerprint by sections). If it
      is taken later, it follows Black's terms in ROUND-31.md.
    - Proposal 2 (PCP and NAT-PMP sent together under `auto`).
    - WHITE-002 step 1 (`poo/VERSION` in the hello Agent). It tells every
      peer the exact release, so it waits for White's and Black's review
      of the whole proposal.
    - Green's idea of a command to set `p2p.port_map` or `p2p.advertise`
      on an existing node.
- **Why:**
  - After an upgrade, every pool's snapshot is unused once. Listing them
    all made a 5 s status answer 1.3 MB at 8,192 pools.
  - A rollback that saves should lose nothing. White's rule is that going
    back one release loses nothing that cannot be fetched again.
  - A front end needs to know which release it talks to.
- **Consensus:** None.
- **Tier:** Node, node policy, RPC, CLI.
  - `pkg/node/reasons_internal_test.go`: `TestReplayedCapsReasons`,
    `TestRunningRelease`.
  - `pkg/nodepolicy/unknown_test.go`: `TestUnknownSettingsSurviveSave`.
  - `pkg/cli/blue031_test.go`: `TestSnapshotsCheckCountsCutReasons`.
  - `pkg/rpc/rpc_test.go`: the status reply names the release.

### D185. Round 32 fixes: RED-093 to RED-096 (BLUE-033), with ORANGE-042, GREEN-057 and GUI ORANGE-024

- **Decision:**
  - **RED-093.** A key in node.json is known when `encoding/json` would
    fill a field from it: the exact name, or else a name equal under
    Unicode case folding (`strings.EqualFold`, the decoder's own rule;
    `fieldFor`). Such a key is never kept as unknown. `Load` applies it,
    as the decoder always did. `Save` writes the setting under its own
    name and drops the other spelling, so a setting the owner turned off
    stays off, and a lookalike key cannot override the real one.
    - `FoldedSettings` names these keys, and the node and `poo doctor`
      warn.
    - When kept settings go back in, none is written beside a key that
      folds to its name (`holdsFolded`).
    - Red's other option, refusing such a file, is not taken: it would
      stop a node from starting over a spelling.
  - **RED-094.** Limits on what `Load` keeps:
    - at most 256 settings (`MaxKeptSettings`);
    - each value at most 64 KiB compacted (`MaxKeptBytes`);
    - each value at most 64 levels deep (`MaxKeptDepth`).

    A setting over a limit is not kept. `UnknownDropped` names it with the
    limit, and the start warning and doctor say the next save drops it.
    `Save` writes kept values compacted on one line: it puts in marker
    strings with a random part, indents, then replaces the markers in one
    pass. Red's 9,990-deep file now stays its size. `GET /v1/status`
    caches node.json and loads it again only when the file is replaced or
    its size or modification time changes. A cap on node.json's size is
    not taken: variable series releases are kept in node.json, so a
    legitimate file can be large.
  - **RED-095.** ORANGE-042 (#249, merged in) already inserts every kept
    setting in one pass. Start warnings name the first ten settings with
    a count (`NameSome`); doctor names the first twenty.
  - **RED-096.** `Merge` keeps the local node's kept settings, so an
    import that saves loses none. `clone` copies the dropped and folded
    lists too.
  - **GUI ORANGE-024.** The set of variable series ids in `pkg/series` is
    a `sync.Map` grown in place, with an atomic flag for the empty fast
    path, in place of a map copied whole on each add. It holds the same
    ids, and reads still take no lock.
  - **GREEN-057 (#247) is carried:** it changes the same `unknown.go`.
    The folded keys are added to its doctor checks.
- **Why:** W-1 (D184) keeps what a newer release wrote, but deciding
  "unknown" by exact name made a key the decoder applied also count as
  unknown. A routine save then wrote it back and revived a setting the
  owner had turned off, `rpc.allow_remote` included. Keeping values
  without limits let a small crafted file grow without bound on save.
- **For White:** a newer release's settings over the limits are lost on
  a save by this release. The limits are far above what one release adds
  (W-1's case is a handful of settings), and the start warning names
  what will be dropped. A folded key is saved under the setting's own
  name, which every release reads the same way.
- **Consensus:** None. node.json is node policy.
- **Tier:** Node policy, RPC, series.
  - `pkg/nodepolicy/blue033_test.go`:
    - `TestBlue033FoldedKeyIsTheSetting`, Red's `ALLOW_REMOTE`.
    - `TestBlue033UnicodeFoldedKey`, Red's `liſten`.
    - `TestBlue033KeptCompacted`.
    - `TestBlue033KeptLimits`, Red's 9,990-deep value, 64 KiB and 266
      settings.
    - `TestBlue033ImportKeepsUnknown`.
  - `pkg/rpc/blue033_internal_test.go`: `TestBlue033StatusCachesPolicy`.
  - `pkg/series/blue033_internal_test.go`:
    `TestBlue033VariableSetGrowsInPlace` and a benchmark.
  - The nodepolicy tests fail on the code before the fix.
