# pkg/peers

## Summary

The peer book: every node a full node knows of, by contact address, with
its state for each mint policy (candidate, verified or archived), where it
was heard of, and when it was last tried and heard from. It follows the
"Peer states" and "Candidates and connections" parts of the design's "Peer
Discovery and Management". The book only records; package `p2p` decides
when a state changes, and every change is node policy. It is saved as
`peers.json` in the data directory.

## States

| State | Means | Set by |
| --- | --- | --- |
| Candidate | Known, not checked by this node | `Add` (registry, peer list, follow reply, node policy), `Unverify`, `Unarchive` |
| Verified | Answered this node's own follow request or ping | `Verify` |
| Archived | Not in use; kept as a starting point | `Archive` (silence, a dropped policy, its own address) |

`Delete` removes a node for good and keeps its address on a deleted list,
so no source adds it again. Verification never survives a restart: on load
a verified policy is a candidate again (decision D46).

## Safeguards

- **Network ranges** (`Range`): the /16 of an IPv4 address, the /32 of an
  IPv6 address, or the host name. `PerRange` bounds the records from one
  range and `VerifiedPerRange` the verified peers for one policy. Each
  loopback address is its own range, so local test networks are not
  limited.
- **Spacing**: `Pick` returns candidates oldest attempt first, one per
  range before a second from any range, and skips a candidate that failed
  until `Retry` doubled per failure has passed.
- `MaxRecords` bounds the book.

## API

`Open(path, Limits)`, `Save`, `Add`, `Verify`, `Heard`, `Unverify`,
`Archive`, `Unarchive`, `Delete`, `Deleted`, `Tried`, `Pick`,
`VerifiedFor(section, full, within)` (for peer list replies), `Count`,
`Records`, `Range`, `DefaultLimits`.

## Tests

`peers_test.go`: state changes, peer lists for the full and all variants,
deletion, range limits for records and verified peers, spreading and
backoff in `Pick`, and a save and load that keeps archived and deleted
entries and drops verification.
