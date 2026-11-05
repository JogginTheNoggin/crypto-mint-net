# pkg/peerport

## Summary

A node's peer book in one shareable file, and back (decision D157). An
export holds every address the book knows, or the ones in a chosen set of
categories, with the mint policies each serves, the owner's labels and
the blocks the node holds. An import adds the addresses to another
node's book as unlisted, and takes the blocks only when asked. A user can
give a new node somewhere to start, keep a backup, or share a block
list. `poo peers export` and `poo peers import` are the commands; a
running node imports through `POST /v1/peers/import`.

Nothing here is consensus: the peer book and the tracker are node policy,
local to one node.

## The file

```json
{
  "kind": "poo-peers",
  "format": 1,
  "exported": "2026-11-03T05:00:00Z",
  "categories": ["priority", "backup"],
  "peers": [{"addr": "203.0.113.5:7733", "policies": [{"section": "ISSUER/SERIES/*/*", "state": "priority"}], "label": "fast one"}],
  "blocks": [{"addr": "198.51.100.7", "until": "2126-11-03T05:00:00Z"}, {"range": "v4:192.0", "until": "2026-11-04T05:00:00Z"}]
}
```

`categories` is empty for a whole export. The categories are the book
states (`priority`, `backup`, `candidate`, `archived`, `unlisted`) and
the tracker's `blocked` and `timed-out`. An address is exported when one
of its mint policies is in a chosen state, or the tracker has it blocked
or timed out and that category is chosen. Blocks go in a whole export and
a `blocked` one. Points, flags, measures and node ids are never in the
file.

`Read` takes at most `MaxSize` (16 MiB), `MaxPeers` addresses,
`MaxBlocks` blocks and `MaxPolicies` mint policies an address. It
refuses a field it does not know, a newer format, an address or block
with a space or control character (RED-053), a bad address, mint policy,
state, label or range, and a block naming both or neither of an address
and a range. A bad entry refuses the whole file.

## Import

`Import(file, Options, Node, apply, now)` returns a `Plan` and, with
`apply`, makes it:

- **Addresses arrive unlisted** (`peers.Book.AddUnlisted`), whatever
  state the file names, so a file never makes a priority or backup peer.
  A known address keeps its states and gains only the mint policies it
  lacks. A label is set where the book has none; a different one is a
  conflict, and the book's is kept.
- **The book's guards hold.** An address blocked here is refused, a
  local one unless the node allows local peers or lists it, and so are
  addresses past the book's size or a network range's share. Imported
  addresses take at most the book's `MaxUnlisted` places, and the plan
  refuses the rest (RED-058, D158).
- **Blocks only when asked.** Without `Options.Blocks` the plan counts
  them and leaves them. With it, a block is taken only when it is longer
  than what the node holds, so a file never unblocks or shortens one; one
  more than 50 years away is taken as until unblocked; an ended one is
  skipped. An address the node policy lists (`p2p.peers`, the node's
  own) is never blocked. A range block needs `Options.Ranges` too, and is
  skipped when a listed address is in the range. A listed host name is
  compared in lower case without its trailing dot (`peers.Range`), and by
  name only: a block on the address it resolves to is not seen
  (RED-060).
- **Notes** (`Plan.Notes`) say what the lines do not:
  - that the node will try the imported addresses when it is short of
    peers, before registries, unless `p2p.no_dial_imported`;
  - how many of the book's places for imported addresses they take, and
    that `poo peer remove -imported` takes them out;
  - how many blocks and ranges there are, and how much of IPv4 the
    ranges cover;
  - that a listed host name is protected by name only.

  (RED-058, RED-059, RED-060, RED-062, D158.)

`Digest(file, Options, Plan)` is what `-if-digest` checks, as for policy
import (D151, D154). `ImportIf(file, Options, Node, digest, now)` plans
and applies in one call, and only when the plan it just made has that
digest (`ErrNotPreviewed` otherwise). The CLI and the node's
`if_digest` use it, so the plan checked is the plan applied (RED-062).

## Tests

`TestExportCategories`, `TestReadRefuses`, `TestImportRules` and
`TestImportGuards`; `TestImportTakesAShareOfTheBook`,
`TestListedHostNameInCapitalsIsProtected`, `TestPlanCountsRangeBlocks`
and `TestImportIfAppliesOnlyThePreviewedPlan` (D158);
`TestPeersExportImport`, `TestPeersForgetImported`, `TestPeerActionsOffline`
and `TestPeersRunning` in `pkg/cli`; `TestManagerDialsImported` and
`TestAddPeerByHand` in `pkg/p2p`; `TestUnlistedAndLabels` in
`pkg/peers`.

## Extending

A new field is optional and read by `Read` only once this code knows it.
A change that older nodes would misread bumps `Format`. A new category is
added to `Categories` and to `Export`.
