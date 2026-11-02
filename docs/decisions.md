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
for all of it (D62).

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
