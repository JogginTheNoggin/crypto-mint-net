# RED-064: Peers and replies fill the book with candidates that never expire, and the new `max_book` floor makes it 16 ranges

| Field | Value |
| --- | --- |
| Severity | Medium |
| Status | fixed |
| Found | Source Round 22 Part B, on `main` @ `b997eb0` (D158, D159) |
| Area | `pkg/peers` (`Book.add`, `Open`), `pkg/p2p` (`learn`, follow replies, peer lists) |
| Related | RED-058 (the same effect through an import; fixed by letting imported records give way) |
| For Black | No consensus rule is touched. The peer book is node policy; nothing here stops a node taking a heavier ledger it can already reach. |
| Fixed by | `claude/project-thread-ggo8dt` @ HASH (BLUE-023) |

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

## Resolution

Fixed (D160). I followed Red's first, second and fourth points.

- Learned candidates give way. A record from a peer list, a reply, a
  registry or an inbound connection, with no label, that the node has
  never reached, gives way to a new address when the book or its range is
  full. `Record.Reached` is new: it is set by a connection that worked
  (`Tried` ok) or a verification. The order is:
  1. records known only from an import (as in D158);
  2. learned records whose last try failed;
  3. learned records older than `p2p.candidate_seconds`, a new D159-style
     setting (one day by default; 60 s to 365 days);
  4. for an address the owner gave (`hand`, `config`), any learned record.

  In each class the oldest goes first. A record the owner added,
  configured or labelled never gives way, and neither does one the node
  reached.
- So the owner's `poo peer add` always finds a place in a book full of
  junk. A junk address the node tries and fails gives way to the next
  learned one at once, and an untried one gives way after a day.
- A lowered `max_book` is applied. `Book.Fit` trims a book over its
  limits in the same order (imported records beyond `MaxUnlisted` first,
  then ranges over `book_per_range`, then the book). The node calls it on
  start and logs `peer book over its limits: policy value lowered`, with
  how many it deleted and how many it still holds over. It is not in
  `Open`, because a command run offline opens the book with the default
  limits and must not trim a book kept under larger ones. A book saved
  before D160 counts its listed records as reached.
- Not built: a share of the book per source (point 3). The book does not
  record which peer sent an address. The range limit already bounds one
  sender per range, and the owner's add can always take a learned place.
  Worth another look if Red finds a flood this order does not drain.
- Speed: the give-way classes are kept on heaps, oldest first, and are
  checked when read. An add into a full book at the 65,536 ceiling costs
  about 3.5 µs (`BenchmarkAddFullBookLearned`); my first version scanned
  the book and took 7.9 ms, which would have undone ORANGE-032.
- Tests: `TestLearnedCandidatesGiveWay` (Red's 16-range fill, then fresh
  and owner adds, failed and stale candidates, a full range, and the
  records that never give way), `TestFitLoweredBook` (4096 lowered to
  256, an offline open trims nothing, reached records stay over the
  bound), and `TestGiveWayQueuesStayBounded` (20,000 mixed changes; the
  heaps stay within twice the book and hold every record they should).
