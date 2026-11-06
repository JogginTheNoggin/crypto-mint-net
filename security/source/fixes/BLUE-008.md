# BLUE-008: Work is asked for and listings are paced; large parts go to disk

| Field | Value |
| --- | --- |
| Fixes | RED-015 (high), RED-016 (high), RED-017 (medium), RED-018 (low), RED-019 (low) |
| Branch | `claude/project-thread-ggo8dt` |
| Base | `claude/m10-watch` @ `0fd59fa` (Source Round 7 Part C) |
| Decisions | D120 |
| Review | Black, Part D: RED-015 and RED-016 touch G3 (D115, D116) |

## Summary

These are five fixes from Source Round 7. None adds a depth limit or a size cap, and no heavier ledger is refused (D115).

- **RED-015 (high).** A sender that lists hashes with no work now costs the node a page of memory and a paced share of chase time. Before, Red measured +948 MB in 15 s. The chase keeps only hashes that carried layers. After 64 pages in a row with no work it yields, then goes on later from where it stopped.
- **RED-016 (high).** Claiming a huge entry now costs the node disk only as the sender actually sends the bytes. Parts go to a temporary file, and the first part must start with the right parent. A fetch is spread over several time slices. A node that holds nothing asks for work before it fetches a whole ledger.
- **RED-017 (medium).** A parts reply no longer encodes the item under the engine lock, and each item is encoded once per fetch.
- **RED-018 (low).** A layer floor sends at most 16 extra layers per search and 50 a second.
- **RED-019 (low).** `poon grind run` retries a failed lease renewal.

There is no change to byte forms, the wire or consensus.

## What changed

- `pkg/p2p/chase.go`:
  - `progress` replaces `after`, `listed` and `checked` with `pending` (one page), `listed` (a count), `last` (the anchor) and `marks` (layered hashes by index).
  - `checkWork` lets go of each hash once it is checked. It yields after `WeightlessPages` pages in a row that add no weight. Its first page after the pool_id asks for the sender's first hashes, and a zero `HashLimit` means one request for the whole page.
  - `fetch` takes an `expect` function, which compares only marked indexes. It also takes the progress and the deadline, for parts.
  - `fetchParts` writes to a `partial` temporary file. Each part must be non-empty and stay within the stated size, and the first part must start with the parent. The fetch yields with `errPartsYield` past the deadline and resumes from its offset.
  - `runChase` runs the work check before `fetchAll` when the node holds nothing. `progress.drop` removes a parts file when its progress ends, is evicted, or the host stops.
- `pkg/p2p/host.go`: new `Config.WeightlessPages` (64) and `Config.PartsDir`. `Run` drops the kept progress when it stops.
- `pkg/p2p/serve.go`: `replier.parts` copies the item under the lock and encodes it through `partsCache`.
- `pkg/node/node.go`: `MaxKeptLayers` (16), `KeptPerSecond` (50) and a `keptBucket` token bucket in `WorkWith`.
- `pkg/cli/grind.go`: `renew` takes a `renewer`, the tick and a warning writer. `errOf` (in `main.go`) gives an invocation's error output.
- Docs: D120, plus `docs/modules/p2p.md`, `grind.md` and `poon.md`.

## Tests added

- `pkg/p2p`:
  - `TestWeightlessListingIsPaced` (RED-015).
  - `TestLargeClaimIsPacedOnDisk` and `TestLargeClaimWithWrongParentIsFalse` (RED-016). The first checks that the work is asked for before the ledger, that parts reach disk, and that the fetch resumes from its offset across slices.
  - `TestPartsCacheEncodesOnce` (RED-017).
  - The heavier-ledger tests still pass, including `TestHeavierLedgerHuge` (an entry over 16 MiB, 58 s, run without `-race` as CI does).
- `pkg/node`: `TestWorkWithFloorIsCapped` (RED-018).
- `pkg/cli`: `TestRenewSurvivesAFailure` (RED-019).

## How Grey can check it

```sh
go test -race -run 'Weightless|LargeClaim|PartsCache|EmptyBranch|FalseBranch|LongBareRun|DeepSplit|CatchesUp|Heavier' -v ./pkg/p2p
go test -count=1 -run TestHeavierLedgerHuge ./pkg/p2p
go test -race -run 'Floor' -v ./pkg/node
go test -race -run 'Renew|Grind' ./pkg/cli
go test -race ./...
GOOS=windows GOARCH=amd64 go build ./... && GOOS=darwin GOARCH=arm64 go build ./...
```

staticcheck and govulncheck are not installed here, so please run them.

## Open

- **For Black.** These changes touch G3. Pacing works by pages in a row without work and by the time slice; nothing ends a chase for its length. The only new gate is the work check before a whole-ledger fetch, which any heavier ledger passes.
- **Memory when decoding (D117's "not yet").** Decoding a large entry still needs it whole in memory. A sender that really sends a 1 TiB entry still costs that memory once all of it has arrived.
- **Orange's tracker note.** Orange's note that the tracker counting dropped messages starves a steady heavy sender is not changed here. It is Black's to check.
