# GREEN-046: Offline peer add checks its label first, and zero times read "-"

| Field | Value |
| --- | --- |
| Status | adopted (D159) |
| Based on | `main` @ `8662766` |
| Area | `pkg/cli` (`peerOffline`, `peerCmd`, the table writer), `docs/modules/poon.md` |
| Effort | small |
| Consensus | none: the command line and the peer book's local files only |
| Decided by | Grey (Black may skip it) |

## Summary

Green tried D157's peer book commands as a user and found three
problems.

1. **`poon peer add -keep -label BAD ADDR` with the node stopped changed node.json, then failed.** `peerOffline` saved `p2p.peers` first and checked the label after. A refused command left a change behind.
2. **With the node stopped, a good `-label` with `add -keep` was dropped without a word.** The address went into node.json, the label went nowhere, and the reply looked like success.
3. **`poon peer add ADDR` with the node stopped said only "is the node running?".** It did not say that `-keep` works with the node stopped.

Also, in `-o table`, every never-tried peer showed `LAST_TRIED` as
`0001-01-01T00:00:00Z`, Go's zero time.

All four are fixed on the branch.

## Idea

- `peerOffline` checks everything before it changes any file:
  - The label is checked first.
  - For `add -keep -label`, the peer book is opened. If the book holds the address, the label is set there and node.json takes the address. If it does not, the command refuses: "...cannot take a label yet: add it with -keep alone, then poon peer label ADDR -label TEXT once the node has it; nothing was changed".
  - `remove` and `label` work as before.
- A plain `add` with the node stopped appends "(with the node stopped, add -keep lists the address in node.json)" to the dial error.
- The table writer shows the zero time as `-`, as an empty list already is. `-wide` and JSON keep it as it is. This is one rule in `tabler.cell`, so every reply with a "never" time (`last_tried`, `last_heard`, `timed_out_until`, `blocked_until`) benefits.
- `poon.md`: the `peer add` row says what works with the node stopped, and the table paragraph names the zero time.

## Testing

- `TestPeerAddOfflineLabel`:
  - A bad label, and a good one for an address the book lacks, are both refused, and node.json is unchanged.
  - A label for an address the book holds is set, and the address is kept.
  - A plain offline `add` names `-keep`.
  - It fails on `main`, where the bad label had already changed node.json.
- `TestTableZeroTime`: `-` in a table, and the zero time kept with `-wide`.
- `TestPeerActionsOffline`, `TestPeersExportImport` and `TestPeersRunning` still pass.
- gofmt, vet, staticcheck, and `go test -race ./...` pass.

## Risks

A script that read `0001-01-01` from a text table now reads `-`. Scripts
should read JSON, which is unchanged.

## Also noticed (not changed)

- **The two `-category` flags take different lists.** `poon peers -category` takes the book states (candidate, priority, backup, archived, unlisted). `poon peers export -category` also takes `blocked` and `timed-out`, which `poon tracking -blocked -timed-out` shows instead. The error names the list each takes, so nothing is wrong. A note in `poon peers -h` saying blocked and timed-out addresses are under `poon tracking` would save a try.
- **A `peer add` reply prints the tracker status with zero times** (`timed_out_until`, `blocked_until`). In JSON that is correct. In a table it now reads `-`.
- Still open from GREEN-020: errors name `until_from` where the person typed `-until-from`, and no command renews a lease.
