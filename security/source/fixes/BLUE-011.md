# BLUE-011: Kept progress holds nothing per layer; idle parts give way at the disk reserve

| Field | Value |
| --- | --- |
| Fixes | RED-026 (high), RED-027 (medium) |
| Branch | `claude/project-thread-ggo8dt` |
| Base | `claude/m10-watch` @ `0a6cc94` (Source Round 10 Part C) |
| Decisions | D131 |
| Review | Black, Part D: both touch the chase (G3, D115, C-6) |

## Summary

These are the two fixes from Source Round 10. They add no size or depth
limit and refuse no heavier ledger (D115). Black's C-6 holds: kept
progress still lasts to `ProgressTTL`, counted from the end of the
sender's backoff.

- **RED-026 (high).** Kept progress no longer keeps a hash for each layer
  counted. It remembers only which entries had their layers counted, so
  2,000 layers cost one entry. A sender gone past the grace has its
  progress cut down to its common entry, and counts again if it comes
  back.
- **RED-027 (medium).** A fetch waiting for disk space takes the room held
  by kept chases that rank below it, connected or not: a departed
  sender's first, then one that asked and got nothing, then the least
  shown, then the smallest. A chase that ranks higher keeps its parts.

## What changed

- `pkg/p2p/chase.go`:
  - In `checkWork`, `counted` is keyed by entry, and an entry's nonces are
    deduplicated within their reply. A many-layers entry being paged is
    exempt from the entry check.
  - `layersFrom` passes `take` only the layers not read before. Repeats
    that D124 allows under `manySlack` are checked as before but not
    passed on.
  - `sweepLocked` cuts a departed sender's progress down to
    `newProgress(common)` at the grace (`compacted`). A topping-up
    progress keeps the rest.
  - `partial` gains `tried`, `got` and `idle`. `saveProgress` sets `idle`
    when the chase asked for parts and took none.
  - `reclaimParts` and `partsRank` replace `dropAwayParts`.
- Docs: D131 and `docs/modules/p2p.md`.

## Tests added

- `pkg/p2p`:
  - `TestKeptProgressHoldsNothingPerLayer` (RED-026).
  - `TestIdleConnectedSenderGivesUpItsParts` (RED-027).
  - `ProgressCounted` is exported for tests.
  - Each new test fails with its fix taken out.
  - The earlier RED-023, D129 and B3/C-5 tests still pass, and so does
    `TestHeavierLedgerHuge`.

## How Grey can check it

```sh
go test -race -run 'KeptProgress|IdleConnected|Departed|DiskWait|Stalled|Outlast|ManyLayers|LargeEntry|LargeClaim|Heavier' -v ./pkg/p2p
go test -count=1 -run TestHeavierLedgerHuge ./pkg/p2p
go test -race ./...
GOOS=windows GOARCH=amd64 go build ./... && GOOS=darwin GOARCH=arm64 go build ./... && GOOS=freebsd GOARCH=amd64 go build ./...
```

staticcheck and govulncheck are not installed here, so please run them.

## Known flake, not from this change

`TestSlowSenderIsReachedOverManyChases` (D129) failed 2 of 3 times under
`-race` here when other tests ran alongside it. It failed in the same way
on the base `0a6cc94` (b flagged for "address timed out"). Grey plans to
fix the test in Round 11 Part A. In a full `go test -race ./...` it
passed.

## Open

- **For Black.**
  - The cut at the grace forgets only what the sender can show again.
  - A parts file is reclaimed only for a waiting chase that ranks above
    it, so the order is strict and two fetches cannot take turns.
- **Not done.**
  - Red's host-wide byte budget. It would let cheap worked progress make
    an honest chase that is still counting forget its count again and
    again (D115).
  - Showing kept bytes in `poon chases`.
- **Trickling sender.** A connected sender that sends one part each turn
  is not idle, and keeps its parts if it ranks above the waiting chase.
- **Still open from earlier.** The per-sender disk budget and the
  streaming decode.
