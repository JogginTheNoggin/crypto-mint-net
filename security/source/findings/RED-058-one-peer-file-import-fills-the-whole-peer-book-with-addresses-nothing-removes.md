# RED-058: One peer file import fills the whole peer book with addresses that nothing removes

| Field | Value |
| --- | --- |
| Severity | Medium |
| Status | fixed |
| Found | Source Round 21 Part B, on `main` @ `8662766` (D157) |
| Area | `pkg/peerport` (`Import`), `pkg/peers` (`AddUnlisted`, `Limits`, `PromoteImported`), `pkg/cli` (`poo peers import`) |
| For Black | No consensus rule is touched. The peer book is node policy. |
| Fixed by | `claude/project-thread-ggo8dt` @ 282571b (BLUE-022) |

## What is wrong

The peer book holds at most 4096 records (`MaxRecords`) and 16 from one
network range (`PerRange`). Nothing ever removes a record: not a failed
dial, not age, not a long wait as `unlisted`. Once the book is full,
`Book.Add` refuses every new address with `peers: book full`, whoever
learns it: a peer list, a reply, an inbound connection, a registry.

A peer file import adds up to 4096 `unlisted` records at once, in a
second, and the plan counts them as `+ ... (unlisted, 1 mint policy)`.
Unlisted records are tried only when the node is short of peers, so a
node that has enough peers never tries, never fails and never archives
them: they stay.

## How I checked it

A 4.8 MB peer file of 25,088 addresses in 49 x 256 different /16 ranges
(two a range):

```
$ poo peers import big.json -apply        # 2.4 s
  refused 59.254.1.5:8646: the peer book is full (4096)
  ...
$ poo peers import big.json | grep -c refused
20992
```

Then, in a unit test (`peers.Book`), the book at 4096 `AddUnlisted`
records:

```
Add("93.184.216.34:8646", ..., FromPeerList): peers: book full
Add("93.184.216.35:8646", ..., FromInbound):  peers: book full
```

An import of fewer addresses can aim: 16 junk addresses in the /16 of an
honest operator make `ErrRangeFull` for that whole network. 256 ranges
fill the book (16 each), so a file of 4096 chosen addresses blocks
discovery for the 256 /16 ranges where honest nodes sit (a cloud
provider's, say). Host names are their own range each (`host:NAME`), so
4096 names fill the book with no range limit at all.

There is no command that takes out the unlisted addresses together: `poo
peer delete` and `poo peer remove` take one address each (4096 commands),
or the owner deletes `peers.json` and loses everything else in it
(verified peers, archive, labels).

## What could happen

The owner imports a "public peer list" from a forum or a friend, which
is the feature's purpose. The file is larger than the book. From then on
the node can learn nothing new, however well its other peers behave, and
the only addresses it dials when short are the file's. That is an
eclipse made by the file's author, with no consensus rule involved and
no sign in the preview beyond the refused lines.

## Suggested fix (Blue's call)

- Give unlisted records a share of the book (a quarter, say), so an
  import cannot take the rest, and say so in the plan ("takes 1024 of
  the book's 4096 places").
- Let an unlisted record go when it has been in the book a set time and
  no manager pass has promoted it (a plain expiry, not a cap on what can
  be followed).
- Add one command that forgets by category or source (`poo peers forget
  -category unlisted`, or `-source import`), and have the preview name it.
- Let the learning paths replace the oldest unlisted record when the book
  is full, so a full book never blocks discovery.

## Not tested

A running node taking the same file through `POST /v1/peers/import`. It
calls the same `Import`, so I expect the same.

## Resolution

Fixed (D158). I followed three of Red's suggestions: a share of the
book, letting the learning paths take the place of imported records, and
one command to forget them.

- `peers.Limits.MaxUnlisted` defaults to a quarter of `MaxRecords`
  (1024 of 4096). It bounds the records known only from an imported file:
  those from `import` that are unlisted for every mint policy.
  `AddUnlisted` refuses one more (`ErrUnlistedFull`). The import's plan
  refuses the rest with "the book keeps at most N imported addresses (of
  its M places)", and its note says how many it takes.
- `Add` from any other source (a peer list, a reply, inbound, a registry,
  by hand) deletes the oldest imported-only record when the book is full,
  and does the same within a range when that range is full. So 16 junk
  addresses in an honest /16, or 4096 host names, never block discovery.
- `poo peer remove -imported` takes them all out (`Book.ForgetImported`, `POST /v1/peers/forget-imported`).
  Promoted, learned or verified records stay, and so do the tracker's
  blocks. The import preview names the command.
- Not built: an expiry for unlisted records. With the share and the
  eviction it is not needed for the book to stay open, and it would drop
  a file the owner meant to keep.
- Tests: `TestImportedAddressesGiveWay` (pkg/peers),
  `TestImportTakesAShareOfTheBook` (pkg/peerport) and
  `TestPeersForgetImported` (pkg/cli).
