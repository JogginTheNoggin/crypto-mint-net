# GREEN-047: poo policy limits, and which other limits could be settable

| Field | Value |
| --- | --- |
| Status | adopted (D161): part 1; part 2 is a survey for Black |
| Based on | `main` @ `b997eb0` |
| Area | `pkg/nodepolicy` (`Policy.Limits`), `pkg/cli` (`poo policy limits`, view-only scope), `gui/e2e/parity.mjs` (one `LEFT_OUT_SUBS` line), docs |
| Effort | small (part 1); part 2 is a list for Grey and Black |
| Consensus | none for part 1. Part 2 lists limits to keep fixed where a setting could touch the heavier-ledger rule (Black first) |
| Decided by | Grey; Black for anything in part 2 |

## Summary

D159 made nine peer book and tracker limits settable. Nothing a person
runs shows them. `nodepolicy.Bounds` says it is "for docs, the CLI and
the GUI", but only tests call it. To learn a limit's name, range or
default, a person has to read `nodepolicy.md` or `decisions.md`, and
then edit node.json blind.

**Part 1, prototyped:** `poo policy limits [-o FORMAT]` prints each
settable limit with:

- its place in node.json
- its range
- its default
- the value node.json sets
- the value the node uses

It only reads, so it works in view-only mode. The list comes from a new
`Policy.Limits`, which is also the list `Validate` now checks, so a
future bound added there is checked and shown together.

**Part 2, a survey for jerry's principle that node policy lets people
tailor their settings:** which other fixed limits could be settable, and
which should stay fixed.

## Part 1: Idea

- `nodepolicy.Limit{Name, Min, Max, Default, Set, Used}` and `Policy.Limits()`. `checkBounds` loops over `Limits()` instead of its own copy of the list, so the list is kept in one place, not two.
- `poo policy limits` takes `-o json|table`, `-policy` and `-dir`. It is in the view-only read list (`scope.go`). It is in `poo help` and in completion, through the usage line.
- `gui/e2e/parity.mjs`: one `LEFT_OUT_SUBS` line for `policy limits`, as was done for `settings` in D155. The terminal runs it, and a GUI view is GUI Grey's call. That line is the only change outside Source's files. Without it, the parity check fails on any new subcommand.
- Docs: a `poo.md` row, and one sentence in `nodepolicy.md` under the limits table.

```
$ poo policy limits -o table
NAME                           MIN   MAX       DEFAULT  SET  USED
p2p.max_book                   256   65536     4096     0    4096
p2p.book_per_range             1     4096      16       0    16
...
tracking.log_size              16    4096      256      0    256
```

## Part 1: Testing

- `TestLimitsListsEveryBound`: `Limits` has every bound of `Bounds`, in order, with `Set` and `Used`, and `Validate` refuses a value it shows out of range.
- `TestPolicyLimitsCmd`: JSON values, and the table in view-only mode.
- `gui/e2e/parity.mjs` passes locally ("68 commands").
- gofmt, vet, staticcheck, and `go test -race ./...` pass.

## Part 2: other fixed limits

Green read the constants in `pkg/p2p`, `pkg/rpc`, `pkg/engine`,
`pkg/mempool`, `pkg/node` and `pkg/watch`.

**Already settable, but without bounds (sent to Red):**

- `max_pending`, `seen_size` and `ties` are top-level node.json fields. They are checked only for being non-negative.
- A huge `seen_size` (10^15) passes `Validate` and `poo doctor`. `poo node` then panics at start ("makeslice: len out of range", `engine.NewSeen`). A shared settings file can carry such a value: `poo settings import` lists it under "read first" but takes it.
- Green passed this to Red through the coordinator, and did not fix it. D159's `Bound` (a floor, a ceiling, 0 = default) is the obvious shape. Once bounded, these would also show in `poo policy limits`.

**Could be settable (node policy, local resource use only):**

| Constant | Where | Now | Why a person might change it |
| --- | --- | --- | --- |
| `partsCacheBytes` | `p2p/serve.go` | 256 MiB, 1 min | memory used to serve ledger parts to peers; a small machine may want less, a server more |
| `ownListLimit`, `ownPeerListLimit` | `p2p/discovery.go` | 256, 64 | how many addresses this node asks a peer for |
| `DefaultCompactMin` (`node.Options.CompactMin`) | `node/node.go` | 1024 records | when the log is compacted; already an option, not in node.json |
| `watch.MaxLimit` | `watch/query.go` | 1000 | the most items one query returns; a local tool may want more |

Each of these changes only what this node spends or asks for. None
changes what it accepts. Black should still confirm that a small parts
cache cannot slow a peer's fetch of a heavier ledger in a way that
matters. A smaller cache means re-encoding, not refusing.

**Should stay fixed:**

- The chase queue and its pacing, `QueueLimit`, `ChaseSlice`, `WeightlessPages` and `RequestPace`, already left fixed by D159.
- `MaxKeptLayers` and `KeptPerSecond` (`node/node.go`). They are set at half of what peers accept, so an honest grinder is never flagged. Raising them could get this node flagged.
- `helloTimeout`, `writeTimeout` and `sweepInFlight`. These are protocol pacing between peers.
- Wire and size caps: `MaxBody`, `MaxRecordSize`, `MaxBundleSize`, `peerport.MaxSize`, `registry.MaxSize`, and the `policy` sizes. These define what is a valid message or file, and the `policy` sizes are part of series validation.
- `engine.ListAll` and `MaxCountedCost`. These are tied to D152 and to how weight is counted.

## Risks

None for part 1: it is a new read-only command. For part 2, each new
setting is one more thing for import plans and Black to weigh. Green
suggests taking them one at a time, with a measurement from Orange where
the default is in question.

## Also noticed

Still open from GREEN-020: errors name `until_from` where the person
typed `-until-from`, and no command renews a lease.
