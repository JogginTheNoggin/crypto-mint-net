# RED-063: `seen_size`, `max_pending` and `ties` have no ceiling, so one setting crashes the node or eats its memory

| Field | Value |
| --- | --- |
| Severity | Medium |
| Status | fixed |
| Found | Source Round 22 Part B, on `main` @ `b997eb0` (D159 added bounds to the other limits, not to these three) |
| Area | `pkg/nodepolicy` (`Validate`), `pkg/engine` (`NewSeen`, `Config`), `pkg/mempool` (`New`), `pkg/ledger` (`Config.Ties`) |
| Raised by | Source Green saw the panic; Red verified it |
| For Black | No consensus rule is touched. These are local limits. A ceiling only has to stay far above any ledger a node must hold; none of the three limits what the node accepts as valid. |
| Fixed by | `claude/project-thread-ggo8dt` @ f307480 (BLUE-023) |

## What is wrong

`Validate` only checks that `seen_size`, `max_pending` and `ties` are not
negative (`pkg/nodepolicy/nodepolicy.go`, the `p.Ties < 0 ||
p.MaxPending < 0 || p.SeenSize < 0` line). D159 put every other limit
behind a `Bound` (floor, ceiling, default), but these three kept the old
check.

`engine.NewSeen(size)` allocates the whole ring and a map sized for it at
once: `make([]hash.Hash, size)` and `make(map[hash.Hash]int, size)`.

## How I checked it

1. A fresh `poo init -example -dir d`, then `seen_size` set to
   `1000000000000000` in `d/node.json`:

   ```
   $ poo doctor -dir d        # every line ok or skip, no complaint
   $ poo node -dir d
   panic: runtime error: makeslice: len out of range
   goroutine 1 [running]:
   github.com/JogginTheNoggin/crypto-mint-net/pkg/engine.NewSeen(...)
   	pkg/engine/seen.go:25
   ```

   A panic, not an error message, and it comes from `node.Open`, so the
   node never starts and the file has to be mended by hand.

2. The same value through the settings import. A bundle exported from the
   same node with `seen_size` 1e15, `max_pending` and `ties` 2e9:

   ```
   $ poo settings import hs.json -dir e
     read first: these change how this node runs ... : max_pending, seen_size, ties
     + max_pending: 2000000000
     + seen_size: 1000000000000000
     + ties: 2000000000
   $ poo settings import hs.json -dir e -apply
     saved
   ```

   The preview does warn ("read first"), but it prints a number, not
   "this node will crash". With a node running, `-apply` reloads it; the
   reload calls `node.Open` again, so the panic takes down the daemon.

3. A value that is not absurd. `seen_size` 300,000,000 (3e8): the node
   starts, and after 12 seconds the process holds 11 GB resident and
   23 GB virtual, still growing, because the ring (32 bytes an item) and
   the map are sized for it before one item is seen. 1e9 would be tens of
   gigabytes. On a small machine it is the OOM killer.

`max_pending` and `ties` are only caps, so a huge value allocates
nothing at once. It removes the cap instead: the pending pool of every
pool, and the off-chain entries kept for a tie, then grow with whatever a
peer sends. That is the protection the limit was for.

## What could happen

- A shared settings file (the import exists for that) carries one of the
  three. The user previews, sees a number, applies. The node restarts into
  a panic or into swap, and cannot start again until node.json is edited
  by hand.
- The same through `poo init`/`doctor` paths: `doctor` says everything is
  fine for a value the node cannot run with.
- A mild file that sets `max_pending` or `ties` very high turns off two
  limits that bound memory under a flood, and the "read first" line shows
  only the setting names.

## Suggested fix (Blue's call)

Use the D159 pattern: a `Bound` for each (`engine.seen_size`,
`max_pending`, `ties`) with a floor, a ceiling and a default that is what
0 means now, checked in `Validate`, listed in `Bounds()`, so `doctor`, the
import's "does not fit" check and the GUI form take it with the rest.
Suggested starting points: `seen_size` 1024 to 4,194,304 (default 65,536;
the ceiling is about 128 MB of ring plus the map), `max_pending` 16 to 1,048,576, `ties` 1 to 4096. Allocate the seen
list's map without a size hint (or cap the hint), so the memory follows
the items and not the setting.

Also worth a test: every integer field of a node policy, set to
`math.MaxInt64`, must be refused by `Validate` or the node must open
without a panic. Other fields I read for the same shape and did not find:
`rpc` and `p2p` limits that D159 and earlier bounds already cover.

## Not tested

`ties` at a large value against a real flood of tying entries; I read the
cap (`len(ties) > p.cfg.Ties`) but did not build the flood.

## Resolution

Fixed (D160), with the D159 pattern Red suggested and Red's ranges.

- `nodepolicy.BoundSeenSize` (65,536; 1,024 to 4,194,304),
  `BoundMaxPending` (10,000; 16 to 1,048,576) and `BoundTies` (64; 1 to
  4,096). They are in `Bounds()` and checked by `Validate`, so `poo
  doctor`, `poo settings import` ("does not fit") and the GUI's form
  refuse an out-of-range value and name the setting and its range. The
  old "must not be negative" check is gone; a negative value is out of
  range.
- `engine.NewSeen` no longer allocates the ring or the set for the whole
  size: both grow as items arrive, up to the size. So even the ceiling
  costs nothing at start.
- Red's extra test is in: `TestEveryIntegerSettingAtItsMaximum`
  (pkg/node) sets every integer field of a node policy, one at a time, to
  its type's maximum. Each must be refused by `Validate`, or the node, its
  peer book and its tracker must open without a panic. It fails on the old
  code for `SeenSize` with Red's `makeslice` panic. All 36 fields pass now.
- Black's `TestBlackBundleSettingsCannotRefuseHeavier` now runs the three
  limits at their floors and at their ceilings, and still reaches the
  heavier ledger.
- Tests: `TestLimitsBounds` (the new bounds), `TestSeenGrowsWithItems`
  (pkg/engine).
