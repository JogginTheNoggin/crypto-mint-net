# RED-064: Peers and replies fill the book with candidates that never expire, and the new `max_book` floor makes it 16 ranges

| Field | Value |
| --- | --- |
| Severity | Medium |
| Status | open |
| Found | Source Round 22 Part B, on `main` @ `b997eb0` (D158, D159) |
| Area | `pkg/peers` (`Book.add`, `Open`), `pkg/p2p` (`learn`, follow replies, peer lists) |
| Related | RED-058 (the same effect through an import; fixed by letting imported records give way) |
| For Black | No consensus rule is touched. The peer book is node policy; nothing here stops a node taking a heavier ledger it can already reach. |

## What is wrong

RED-058 made an *imported* record give way to an address learned any
other way. Candidates learned from the network have no such rule and
nothing removes them:

- A peer list entry (`FromPeerList`), the contact in a follow request or
  reply (`FromReply`), and registries all go in through `Book.Add`.
- `Book.add` refuses once `len(recs) >= MaxRecords` (`ErrFull`) or the
  network range holds `PerRange` (`ErrRangeFull`). Only imported records
  are ever evicted.
- A failed candidate only waits longer to be tried again (`Failures`, at
  most 16 doublings); it is never dropped. The only `Delete` is by hand
  (`poo peer remove`).
- The book is saved and loaded as it is, so a full book is full after a
  restart.

So the book holds whatever came first, and a peer that sends many public
addresses of ranges the node does not know fills it. D159 lets the limits
go lower: at `p2p.max_book` 256 with the default 16 per range, **16 ranges
fill the book**; at the default 4096, 256 ranges. A literal public IP is
all `learnable` asks for, and it is not dialled to be learned.

## How I checked it

At book level (`peers.Open("", Limits{MaxRecords: n, PerRange: 16})`),
filling it with `Add(..., FromPeerList)` from enough ranges, then, a year
later:

```
max_book 256  (16 ranges):  reply add: peers: book full   hand add: peers: book full   import: peers: book full
max_book 4096 (256 ranges): reply add: peers: book full   hand add: peers: book full   import: peers: book full
```

Even `poo peer add` by the owner (`FromHand`) is refused, so the owner must
first find and delete the junk one address at a time.

The path to `Add` from a connected peer is a peer list reply or a follow
request, both of which the host answers or asks for as part of normal
discovery (`discovery.go`, `learn`). I did not run two real nodes for this
round; the book-level result is the same code.

Lowering `max_book` under the size of an existing book (4096 to 256) is
not enforced: `Open` loads every record, so the book stays at 4096 and
refuses every new address (`len(recs) >= MaxRecords`) until the owner
deletes down to the new bound. Nothing says so.

## What could happen

- A hostile peer (or a few) makes the node unable to learn any honest
  peer, and unable to take a new one by hand, for as long as the file
  lives. The node keeps its current connections; it cannot replace them
  when they drop. That is an eclipse by starvation of the book.
- A user who lowers `max_book` to save memory ends with a book that is
  both over its bound and closed.

## Suggested fix (Blue's call)

- Give network-learned candidates the same treatment RED-058 gave imports:
  an address that has never been reached (`Failures` at the cap, or never
  verified and older than a setting) gives way to a new one, oldest first,
  both when the book is full and when the range is full. Keep verified,
  hand-added, configured and labelled records, and any with a block.
- Let a hand add (`FromHand`) and a configured peer always replace the
  oldest unverified candidate, so the owner is never locked out of their own
  book.
- Reserve a share (a setting, like `MaxUnlisted`) for records learned from
  one source, so a single source cannot fill the whole book.
- On `Open`, with a lowered `MaxRecords`, drop down to the bound in the same
  order, and log it as a "policy value lowered" note.

## Not tested

Two real nodes over the wire. A restart with a book that was over its
bound (read, not run).
