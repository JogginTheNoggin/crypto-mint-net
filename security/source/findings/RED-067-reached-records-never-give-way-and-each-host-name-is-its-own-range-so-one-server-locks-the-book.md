# RED-067: Reached records never give way, and each host name is its own range, so one server can lock the peer book

| Field | Value |
| --- | --- |
| Severity | Medium |
| Status | fixed |
| Found | Source Round 23 Part B, on `main` @ `215d428` (D160, the fix of RED-064) |
| Area | `pkg/peers` (`Range`, `giveClass`, `victimLocked`, `Tried`, `makeRoomLocked`), `pkg/p2p` (`learn`, `Manage`, `Pick`, `New`) |
| Related | RED-058, RED-064 (this is what is left of it) |
| For Black | No consensus rule is touched. A node whose book is full lacks peers (information); it is not a different view of which ledger is valid. |
| Fixed by | `claude/project-thread-ggo8dt` @ e8f71ff (BLUE-024) |

## What is wrong

D160 lets a learned record that the node never reached give way to a new
address. A record the node *reached* never gives way, to anyone, the
owner's own `poo peer add` and `p2p.peers` included
(`strongSource` may take only the place of a learned record that was never
reached). `Reached` is set when a dial completes its handshake (`Tried(ok)`)
or when a record is verified, and it is saved in the book file.

The per-range limit (`p2p.book_per_range`) is what was meant to stop one
party from holding many records. But `Range` of a host name is
`"host:" + name`, so each name is a range of its own and the limit counts
one. The node does not resolve a name to count it by address. A party with
one server and a wildcard DNS name (`n1.evil.example`, `n2.evil.example`, ...)
can offer as many names as the book holds, all leading to the same machine.

`Pick` chooses oldest-tried first and "spreads over ranges", which with one
range per name changes nothing.

So the order is:

1. A peer (the first contact of a new node, or one of its peers) puts
   `max_book` host names in peer lists or follow replies. `learn` takes a
   name without resolving it.
2. The node dials them as it needs peers. The server speaks the protocol, so
   each handshake completes and `Tried(ok)` sets `Reached`.
3. Every record now has `Reached` set. None gives way. The book is full.

## How I checked it

At book level (`peers.Open("", Limits{MaxRecords: n, PerRange: 16})`),
`Add(name, ..., FromPeerList)` then `Tried(name, true, ...)` for `n`
distinct names, then a year later:

```
max_book 256:   reply add: book full   hand add: book full   config add: book full   import: book full
max_book 4096:  reply add: book full   hand add: book full   config add: book full   import: book full
```

`p2p.New` ignores the result of `book.Add(a, ..., FromConfig, ...)`
(`pkg/p2p/host.go`, the `for _, a := range cfg.Peers` loop), and the manager
dials only what is in the book, so a configured peer that does not fit is
never dialled and nothing is logged.

I did not run two real nodes over the wire; the book-level calls are the ones
`learn`, `Verify` and `Tried` make.

## What could happen

- An eclipse that survives the owner's repair. The book file keeps the
  reached records across restarts. The owner who notices adds an honest peer
  to `p2p.peers`, restarts, and the node still dials only the attacker's
  names, because the honest entry is refused without a message. `poo peer add`
  says "book full". The only way out is deleting records one at a time with
  `poo peer remove` or deleting the book file.
- It is cheap: one machine, one wildcard record. Without names the same
  needs one reachable host in each of up to `max_book / book_per_range`
  network ranges (256 /16 ranges by default, 16 at `max_book` 256).
- It makes the settable limits worse for a node that sets a small book: at the
  floor of 256 it takes 256 names.

## Suggested fix (Blue's call)

- Let the owner's add win: a hand or config address takes the place of the
  oldest *reached learned* record when nothing else gives way (never of
  another owner-given record). The owner is then never shut out of their own
  book, whatever the network did. Say so in the book's log when a configured
  peer cannot be added (log the `ErrFull` that `New` drops now).
- Count a host name by where it leads: when a name is dialled, record the
  address it resolved to and count it in that address's range (`v4:a.b`,
  `v6:xxxx`) as well, so 4096 names on one machine are one range for the
  limit and for `Pick`'s spread. A name that resolves into a range already
  full gives way like any record of that range.
- Cap the share of host-name records (a quarter of the book, a setting, as
  `MaxUnlisted` is for imports) so names cannot take it all.
- Optional: let a reached record give way to an owner address after a long
  idle time (no connection for `candidate_seconds`), so the book cannot be
  held by records that stopped being used.

## Not tested

The same over the wire; the effect of `PromoteImported` and the archive on
this order; IPv6 ranges (`v6:` /32, so one provider with a /32 holds
`book_per_range` records, the same as one /16).

## Resolution

Fixed (D162). I followed Red's first two points.

- The owner's add wins. When nothing else gives way, an address by hand or
  from `p2p.peers` takes the place of the oldest record from the network
  (no label) that the node reached. A record the owner added, configured
  or labelled is never taken, and neither is a priority or backup peer in
  use now. A learned address still never takes a reached record's place.
  These records are on their own heap (`reachedQ`), so this costs no scan.
- `p2p.New` logs `configured peer not in the peer book` instead of
  dropping the error.
- Host names count by where they lead. After a dial that works, the host
  calls `Book.Resolved(name, remote)`, which records the range of the
  connection's address (`Record.Via`, saved in the file).
  - The name counts in that range as well as its own, for
    `book_per_range`, `verified_per_range` and `Pick`'s spread.
  - When the range is then over its limit, a record of it gives way as to
    a learned address. Otherwise the name itself does (`ErrSameMachine`,
    logged), unless the owner gave or labelled it.
  - Red's 256 wildcard names for one server leave 16 records, and an
    address of that server's range is refused as the range is full.
- Not done:
  - A cap on the share of names (point 3). Names never reached give way
    (D160), and reached ones share their machine's range.
  - Idle expiry for reached records (point 4). The owner's add already
    takes their place.
- Tests: `TestNamesForOneMachineShareARange` and
  `TestOwnerAddTakesAReachedPlace` (pkg/peers). `TestLearnedCandidatesGiveWay`
  and `TestRefusedAddChangesNothing` are updated for the owner's place.
