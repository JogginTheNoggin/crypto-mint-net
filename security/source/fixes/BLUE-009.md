# BLUE-009: Layers are counted per page and sorted outside the lock; large parts wait for disk space

| Field | Value |
| --- | --- |
| Fixes | RED-020 (medium), RED-021 (high), RED-022 (medium) |
| Branch | `claude/project-thread-ggo8dt` |
| Base | `claude/m10-watch` @ `ae8a103` (Source Round 8 Part C) |
| Decisions | D123 |
| Review | Black, Part D: all three touch the chase (G3, D115, D121) |

## Summary

These are the three fixes from Source Round 8. None adds a depth limit, a
size cap or a layer cap, and no heavier ledger is refused (D115).

- **RED-021 (high).** A sender that claims a huge number of layers on one
  entry no longer makes the node hold them all. Each page is counted as it
  arrives, and paging stops once the work counted beats the node's, the
  same rule `checkWork` had before D121. An entry whose work needs every
  page still gets every page.
- **RED-020 (medium).** A layers-from or parts reply no longer hashes and
  sorts every layer of an entry under the engine lock. It counts or copies
  the nonces under the lock and sorts outside it, and the order is cached
  by layer count, so one fetch sorts once.
- **RED-022 (medium).** The parts of a large entry wait when their disk is
  down to its reserve (the larger of 1 GiB and 2%), and go on from their
  offset once there is room. Nothing is refused.

There is no change to byte forms, the wire or consensus.

## What changed

- `pkg/p2p/chase.go`:
  - `askLayers(pi, p, hashes, take)` gives each answered entry, and each
    page of a many-layers entry, to `take` as it arrives; `take` returns
    enough to stop. It returns how many hashes were answered.
  - `layersFrom(pi, p, eh, take)` keeps only the count read between pages
    and stops on enough or when the layers end.
  - `checkWork` counts each layer in the callback and stops once
    `st.total` beats `beat`. `topUpLayers` takes each page as it comes.
  - `partsRoom` and `errPartsSpace`: `fetchParts` yields before a part that
    would go into the reserve. `chaseErr` maps it to a yield.
  - `diskSpace` is a seam over `statDisk` for tests.
- `pkg/p2p/space_unix.go`, `space_windows.go`, `space_other.go`:
  `statDisk(dir)` (free and total bytes) by `statfs`,
  `GetDiskFreeSpaceEx`, or an error elsewhere (then the fetch goes on).
- `pkg/p2p/host.go`: new `Config.PartsReserve` (1 GiB when zero).
- `pkg/p2p/serve.go`:
  - `partsKey` gains `order` and `layers`; `partsCache` gains `peek` and
    `put` and holds layer orders as well as encoded items.
  - `parts` peeks the cache first and, on a miss, copies only the entry
    and its unsorted nonces under the lock.
  - `layersFrom` counts under the lock, copies the nonces only on a miss,
    and sorts them with `strongestNonces` outside it.
  - `layers` counts an entry's layers before copying them.
- `pkg/ledger/pool.go`: `LayerCount` and `LayerNonces`.
- `go.mod`: `golang.org/x/sys` is now a direct requirement (`go mod tidy`
  also moved `klauspost/cpuid/v2`, already imported directly).
- Docs: D123, `docs/modules/p2p.md` and `ledger.md`.

## Tests added

- `pkg/p2p`:
  - `TestManyLayersArePagedOnlyUntilEnough` (RED-021): a raw sender claims
    2^32-1 layers on one entry and answers every layers-from request with
    more; the node asks one page and moves on to the ledger.
  - `TestLayerOrderIsCachedByCount` (RED-020, internal): the order is
    strongest first, lower nonce among equals, cached by layer count.
  - `TestLargeEntryWaitsForDiskSpace` (RED-022): with a fake free-space
    reading at the reserve no part is asked for; once room is made the
    fetch goes on from its offset.
  - `largeSender` keeps its entry hash across `serve` calls, so a test can
    serve in phases.
  - The heavier-ledger tests still pass, including `TestHeavierLedgerHuge`
    (run without `-race`).

## How Grey can check it

```sh
go test -race -run 'ManyLayers|LayerOrder|PartsCache|LargeClaim|LargeEntry|ServeManyLayers|Heavier' -v ./pkg/p2p
go test -count=1 -run TestHeavierLedgerHuge ./pkg/p2p
go test -race ./...
GOOS=windows GOARCH=amd64 go build ./... && GOOS=darwin GOARCH=arm64 go build ./... && GOOS=freebsd GOARCH=amd64 go build ./...
```

staticcheck and govulncheck are not installed here, so please run them.

## Merging with Green and Orange

- GREEN-033 (`claude/source-green` @ 512026c) sets the daemon's
  `PartsDir` to `<data>/parts` and clears leftovers at start. This branch
  does not touch `pkg/daemon` or the `PartsDir` default.
- ORANGE-018 (`claude/source-orange-r8` @ d328a01) adds `h.paced(p)` checks
  in `checkWork`, `fetchBranch`, `fetchAll` and `topUpLayers`, and config
  fields before `PartsDir`. This branch changes the body of the layers
  loops in `checkWork` and `topUpLayers`; the pace checks sit at the top of
  those loops, so expect small textual conflicts there, not logical ones.
  `PartsReserve` comes after `PartsDir`.

## Open

- **For Black.** The stop in `checkWork` is the decision it made before
  D121 (the work counted beats `beat`), now taken inside a page. The disk
  reserve makes a fetch wait, never fail. Neither adds a rule of validity
  or selection.
- **Per-sender disk budget (RED-022's third point).** Not done: the
  reserve keeps the store's room whatever the number of senders, but one
  sender can still hold disk up to the reserve with many progress slots.
- **Streaming decode (D117's "not yet").** Still open.
