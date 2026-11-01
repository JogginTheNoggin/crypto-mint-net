# BLUE-010: Departed senders' parts are let go, a stall keeps its parts, older node.json files open

| Field | Value |
| --- | --- |
| Fixes | RED-023 (medium), RED-024 (medium), RED-025 (low) |
| Branch | `claude/project-thread-ggo8dt` |
| Base | `claude/m10-watch` @ `1ae4d46` (Source Round 9 Part C) |
| Decisions | D128 |
| Review | Black, Part D: RED-023 and RED-024 touch the chase (G3, D115) |

## Summary

These are the three fixes from Source Round 9. None adds a size or depth
limit, and no heavier ledger is refused (D115).

- **RED-023 (medium).** A sender that leaves no longer keeps its progress
  and parts file until the node restarts. Both go once the sender has been
  gone for 2 minutes, or nothing has picked them up for an hour. A fetch
  about to wait for disk space first removes the parts of senders that are
  not connected, so departed senders cannot hold an honest fetch.
- **RED-024 (medium).** A timeout no longer throws away a large entry's
  parts; the next chase goes on from the offset it reached.
- **RED-025 (low).** A node.json with a `layer_rate` over the new most
  (50) opens, at 50, with a warning in the log and in `poo doctor`. Writes
  still refuse it.

There is no change to byte forms, the wire or consensus.

## What changed

- `pkg/p2p/chase.go`:
  - `progress` gains `saved` and `away`.
  - `saveProgress` stamps `saved` and sweeps. `sweepProgress` and
    `sweepLocked` forget progress whose sender has been away longer than
    `ProgressGrace`, or saved longer ago than `ProgressTTL`, dropping its
    parts. `connectedIDs` snapshots the peers before `progMu` (no nested
    locks).
  - `dropAwayParts` removes the parts files of progress whose sender is not
    connected. `fetchParts` calls it, then checks the room again, before it
    yields with `errPartsSpace`.
  - `fetchParts` has a named error and keeps its parts whenever
    `chaseErr(err).out.Keeps()` (yield or stall).
- `pkg/p2p/host.go`: `Config.ProgressGrace` (2 min) and `ProgressTTL`
  (1 h); `checkLoop` sweeps each tick.
- `pkg/p2p/peer.go`: a disconnect sweeps, starting the sender's grace.
- `pkg/nodepolicy/nodepolicy.go`: `Policy.Lowered` (`json:"-"`) and
  `lower`, which `Load` runs before `Validate`.
- `pkg/node/node.go`: logs each `Lowered` note as a warning.
- `pkg/cli/doctor.go`: a warn line for each `Lowered` note.
- Docs: D128, `docs/modules/p2p.md` and `nodepolicy.md`.

## Tests added

- `pkg/p2p`:
  - `TestDepartedSenderLeavesNothing` (RED-023): after the sender closes,
    the progress and the parts file go within the grace.
  - `TestDiskWaitTakesRoomFromDepartedSenders` (RED-023): with a fake disk
    that is full while two parts files exist, a new sender's fetch removes
    the departed one's file and goes on.
  - `TestStalledFetchKeepsItsParts` (RED-024): one parts request is left
    unanswered; after the stall the fetch asks from the same offset.
  - `largeSender.skip` leaves chosen parts requests unanswered;
    `ProgressCount` is exported for tests. `TestProgressForgetsLeastShown`
    now sets a long grace, since its senders are never connected.
  - Each new test fails with its fix taken out.
- `pkg/nodepolicy`: `TestLoadLowersAValueOverALoweredBound` (RED-025).

## How Grey can check it

```sh
go test -race -run 'Stalled|Departed|DiskWait|LargeEntry|LargeClaim|Progress|Heavier' -v ./pkg/p2p
go test -count=1 -run TestHeavierLedgerHuge ./pkg/p2p
go test -race -run 'Lowered|SaveLoad|Validate' ./pkg/nodepolicy
go test -race ./...
GOOS=windows GOARCH=amd64 go build ./... && GOOS=darwin GOARCH=arm64 go build ./... && GOOS=freebsd GOARCH=amd64 go build ./...
```

staticcheck and govulncheck are not installed here, so please run them.

## Open

- **For Black.** The grace and the time to live forget only kept
  progress. A later chase rebuilds it from the sender, so a heavier ledger
  is delayed at most, never refused. Removing departed senders' parts
  before a disk wait cannot cost a connected sender anything.
- **Per-sender disk budget** (RED-022's third point) and the **streaming
  decode** (D117) are still open.
