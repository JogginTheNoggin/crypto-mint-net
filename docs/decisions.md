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
