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
  limited.
- **Spacing**: `Pick` returns candidates oldest attempt first, one per
  range before a second from any range, and skips a candidate that failed
  until `Retry` doubled per failure has passed.
- `MaxRecords` bounds the book.

## API

`Open(path, Limits)`, `Save`, `SetRefuse`, `Add`, `Verify`, `Heard`,
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
