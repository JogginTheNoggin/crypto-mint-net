# pkg/peers

## Summary

The peer book and the ranking rules. The book holds every node a full
node knows of, by contact address, with its state for each mint policy
(candidate, priority, backup or archived), where it was heard of, and when
it was last tried and heard from. The ranking rules (`rank.go`) decide
which verified peers fill the priority and backup slots. It follows the
"Peer states" and "Candidates and connections" parts of the design's "Peer
Discovery and Management", and "Peer Categories and Ranking". The book
only records and the rules only compute; package `p2p` applies them, and
every value is node policy. The book is saved as `peers.json` in the data
directory.

## States

| State | Means | Set by |
| --- | --- | --- |
| Candidate | Known, not checked by this node | `Add` (registry, peer list, follow reply, node policy), `Unverify`, `Unarchive` |
| Priority | Verified and relied on: asked, forwarded to, pinged often | `Verify(addr, s, Priority, now)` |
| Backup | Verified and kept ready: forwarded to, pinged less, asked as a fallback | `Verify(addr, s, Backup, now)` |
| Archived | Not in use; kept as a starting point | `Archive` (silence, a dropped policy, its own address) |
| Unlisted | Known only from a peer file the owner imported; in no category and not a candidate | `AddUnlisted` (`poo peers import`, D157); `PromoteImported` makes it a candidate |

`Delete` removes a node from the book. It may come back as a new
candidate; keeping an address out is a block in the tracker, which the
book honours through `SetRefuse` (decision D57). Verification never
survives a restart: on load a priority or backup policy is a candidate
again (decision D46). A book written before categories existed reads
"verified" as backup.

## Categories and ranking

`Category` is how the node treats a connection for one mint policy, in
the order its messages are handled: priority, backup, candidate,
unlisted. `Slots` are the rules for one policy (defaults in
`DefaultSlots`):

| Field | Default | Means |
| --- | --- | --- |
| `Priority`, `Backup` | 4, 4 | slots in each category |
| `PriorityPerRange` | 1 | priority peers from one network range |
| `PerRange` | 4 | priority and backup peers from one range |
| `InboundPriority` | 2 | priority peers that connected to this node |
| `Margin` | 1 | points a backup must lead by to swap |
| `PartialPenalty` | 1 | points taken from a peer that follows only part of the policy |

`Place(others, r)` places a newly verified peer. `Review(ranked)` returns
the peers whose category changes, in this order: priority peers whose
flags bar priority move to backup; free priority slots fill from backups,
then candidates, most points first; a backup whose **fair** points (the
measures both categories have) beat the lowest priority peer's by the
margin swaps with it; free backup slots fill from candidates; backups
beyond the slots become candidates. Peers this node dialled win ties.

## Safeguards

- **Network ranges** (`Range`): the /16 of an IPv4 address, the /32 of an
  IPv6 address, or the host name. `PerRange` bounds the records from one
  range and `VerifiedPerRange` the priority and backup peers for one
  policy. Each
  loopback address is its own range, so local test networks are not
  limited. That is safe because `pkg/p2p` keeps loopback and other
  non-public addresses another party names out of the book unless the
  node allows local peers (D96).
- **Spacing**: `Pick` returns candidates oldest attempt first, one per
  range before a second from any range, and skips a candidate that failed
  until `Retry` doubled per failure has passed.
- `MaxRecords` bounds the book. A node takes every limit from its node
  policy (`nodepolicy.Policy.BookLimits`: `p2p.max_book`,
  `p2p.book_per_range`, `p2p.verified_per_range`, `p2p.retry_seconds`,
  `p2p.candidate_seconds`, D159, D160); the defaults are 4096, 16, 4, one
  minute and one day.
- `MaxUnlisted` (a quarter of `MaxRecords` by default) bounds the
  records known only from an imported file: from `import`, unlisted for
  every policy. `AddUnlisted` refuses one more with `ErrUnlistedFull`.
  `ForgetImported` takes them all out (`poo peer remove -imported`), and
  `Unlisted` counts them (RED-058, D158).
- **Giving way** (RED-064, D160). When the book or the new address's range
  is full, one record gives way to an address added any way but an
  import, so neither a file nor a peer can fill the book for good. In
  order, oldest first in each:
  1. a record known only from an imported file;
  2. a *learned* record whose last try failed;
  3. a learned record older than `CandidateAge` (`p2p.candidate_seconds`,
     one day by default);
  4. for an address the owner gave (`hand`, `config`), and for Fit, any
     learned record;
  5. for an address the owner gave only, a record from the network (no
     label) that the node reached, unless it is a priority or backup peer
     now (RED-067, D162).

  A record is learned when it came from a peer list, a reply, a registry
  or an inbound connection, has no label, and the node has never reached
  it (`Reached`: a connection that worked, or a verification). A record
  the owner added, configured or labelled never gives way, and one the
  node reached gives way only to the owner. So the network can never shut
  the owner out of their own book, whatever it made the node reach; a
  configured peer that still does not fit is logged (`configured peer
  not in the peer book`). When the range is full the record must be in that range;
  otherwise any will do. `add` decides before it deletes, so a refused add
  changes nothing (RED-065). The book keeps the learned, failed and
  imported records on heaps, oldest first, so an address learned into a
  full book at the `max_book` ceiling costs no scan of the book
  (`BenchmarkAddFullBookLearned`: about 3.5 µs at 65,536 records).
- **Fit** brings a book over its limits (`max_book` or `book_per_range`
  lowered, or more imported records than `MaxUnlisted`) down to them, in
  the same order as an owner's add. Records that never give way stay; the
  book then takes no new address until it is under the bound, and Fit
  says how many it holds over. The node fits its book when it starts and
  logs `peer book over its limits: policy value lowered`. `Open` does not
  fit, since a command run offline opens the book with the default limits
  and must not trim one kept under larger ones.
- `Range` of a host name is the name in lower case without the root's
  trailing dot, as the tracker keys it (RED-060).
- **Names for one machine** (RED-067, D162). When the node reaches a host
  name, `Resolved` records the range of the address it led to
  (`Record.Via`, kept in the file). The name then counts in that range
  too: for `book_per_range`, `verified_per_range` and `Pick`'s spread. If
  that range is now over `book_per_range`, a record of it gives way as to
  a learned address, or else the name itself does (`ErrSameMachine`,
  logged), unless the owner gave or labelled it. So a thousand names for
  one server hold at most `book_per_range` places.

## Labels and imported addresses

A record may carry the owner's `Label` (`SetLabel`, at most `MaxLabel`
bytes, and no control, format or separator character: `CheckLabel`, the
same check as `plain.Text`, RED-061). It is local: it changes
nothing about how the node treats the address and is never sent to a
peer. An address added by hand has source `hand`, one from a peer file
`import` (D157).

`AddUnlisted` records an imported address as `Unlisted` for the mint
policies it is not known for yet, and leaves a known policy's state as it
is. `Pick` never returns it. `PromoteImported(s, n)` makes up to `n`
unlisted nodes for policies overlapping `s` candidates, the longest known
first; `n <= 0` promotes all. The connection manager does this when a
policy is short of peers, unless the node policy sets `no_dial_imported`,
and `poo peer connect -imported` does it for all of them.

## API

`Open(path, Limits)`, `Save`, `SetRefuse`, `Add`, `AddUnlisted`,
`ForgetImported`, `Unlisted`,
`SetLabel`, `CheckLabel`, `PromoteImported`, `Limits`, `Verify`, `Heard`,
`Unverify`, `Archive`, `Unarchive`, `Delete`, `Tried`, `Pick`,
`VerifiedFor(section, full, within)` (for peer list replies), `Count`,
`Records`, `Record`, `State`, `Range`, `DefaultLimits`; `Slots`,
`DefaultSlots`, `Place`, `Review`, `Ranked`, `Category`.

## Tests

`peers_test.go`: state changes, peer lists for the full and all variants,
deletion and refusal, range limits for records and listed peers,
spreading and backoff in `Pick`, and a save and load that keeps archived
entries and drops verification. `rank_test.go`: placement with the range
and inbound limits, swaps only by the margin and on fair points, flags
barring priority, free slots filling, partial and outbound peers.
