# RED-072: `follow`, `unfollow` and policy import ignore the kept release: released pools are "out of range", and `unfollow` drops whole parts

| Field | Value |
| --- | --- |
| Severity | Medium |
| Status | fixed |
| Found | Source Round 25 Part B, on `main` @ `e93eb29` (D166) |
| Area | `pkg/nodepolicy` (`Policy.series`, `follow.go`, `bundle.go`), `pkg/cli` (`follow`, `unfollow`, `wallet follow`, `policy import`) |
| For Black | No consensus rule is touched. A node follows less than its owner asked; it is not a different view of which ledger is valid. |
| Fixed by | `claude/project-thread-ggo8dt` @ `15e8f34` (BLUE-026) |

## What is wrong

D166 keeps the newest release in `node.json` (`series[].release`). Two
places apply it: `Policy.Accepted()` and `Policy.AddRelease()`. The code
that edits what the node follows builds the series with
`series.New(si.Params)` and never adds the release, so it sees only part 1:

- `Policy.series()` (`follow.go:307`), which `look` uses to check `-pool`,
  `-mint` and `-except`, and to count pools in `unfollow`;
- `ExportSeries` and the import merge (`bundle.go:266`, `:640`, `:862-875`),
  which check the file's follow rules and print "follows every pool (N)".

The running node, `poon series followed` and the wallet use `Accepted()`, so
they know the released parts. The two sides disagree about how many pools
exist.

## How I checked it

Series of `docs/variable-supply.md`'s shape: 40 mints in part 1, parts of 8,
pools of 4, so part 1 is pools 0 to 9 and part 2 is pools 10 and 11. Node
`n2` has the release of part 2 in node.json and follows the whole series.

1. A pool or mint of a released part cannot be named:

```
$ poon follow   -dir n2 -series 8dfdf9a3.. -pool 10     poon: nodepolicy: invalid: pool 10 out of range
$ poon follow   -dir n2 -series 8dfdf9a3.. -mint 42     poon: nodepolicy: invalid: mint 42 out of range
$ poon unfollow -dir n2 -series 8dfdf9a3.. -pool 10     poon: nodepolicy: invalid: pool 10 out of range
$ poon wallet follow -dir n1 -wallet w.json              poon: nodepolicy: invalid: pool 10 out of range
```

   (`poon series followed` on the same node says 12 pools; the wallet had
   watched mint 43 of part 2.) So light mode cannot follow
   any mint past part 1.

2. `unfollow` drops released parts. Running node, 12 pools followed whole:

```
$ poon unfollow -dir n2 -series 8dfdf9a3.. -pool 0 -pool 1 -pool 2 -pool 3 -pool 4
$ poon pools -dir n2      ->  pools [5, 6, 7, 8, 9]       (10 and 11 are gone)
$ poon follow -dir n2 -series 8dfdf9a3..   ->  [0 .. 11]   (put back by hand)
```

   `unfollow` counts `PoolCount()` as 10, finds `left (5) <= len(except) (5)`,
   and writes the "shorter form": the pools left, listed. Pools 10 and 11,
   which the node follows, are not in the list. On a node that knows part 3
   (14 pools), `unfollow` of pools 0 to 9 leaves `follow: null`, so the node
   follows **nothing**, with no message.

3. A node's own export cannot be imported. `n3` follows pool 11 (part 2,
   release kept). `poon policy export` writes the release with the series and
   the follow rule; then

```
$ poon policy import -dir n7 pexp3.json      poon: follow in the file: nodepolicy: invalid: pool 11 out of range; nothing was changed
$ poon policy import -dir n3 pexp3.json      (the same, into the node that wrote it)
```

   The plan of an import that works also counts pools without the release.

## The same bug stays after the count is right

Even with the true count (12), `unfollow` of pools 0 to 6 gives `left = 5 <=
7`, and the whole follow is rewritten as a list of pools 7 to 11. A variable
series grows: a whole follow means "every pool, including the ones the next
release adds", and a list does not. The "shorter form" is only an equivalent
form for a series whose pool count never changes.

## What could happen

- An owner who unfollows a few pools of a variable series stops following
  every later part, or nothing at all, and is not told. A node that follows
  less misses the heavier ledger of those pools (information, not a rule),
  and the owner believes the opposite.
- A light wallet user cannot follow the pools of any mint past part 1.
- A policy file that works on the node that wrote it fails on any other.

## Suggested fix (Blue's call)

- One way to build the series from a `SeriesInfo`, applying `Release` (an
  invalid kept release is ignored there, `Accepted()` reports it), used by
  `Policy.series`, `ExportSeries`, the merge and the plan counts. Grep
  `series.New(si.Params)` in `pkg/nodepolicy` and `pkg/cli` for the rest.
- In `unfollow`, never write the "shorter form" for a variable series: keep
  `all` with `except`, or say so.
- Tests: `poon unfollow` of five pools on a node at release 2 keeps a whole
  follow with five exceptions; `follow -pool 10` and `-mint 42` work at
  release 2 and say "waiting for a release" at release 1; export then
  import of a policy that follows a pool of part 2; `wallet follow` with a
  watched mint of part 2.

## Not tested

The GUI forms for follow (they call the same RPC).

## Resolution

Fixed in BLUE-026 (D167).

- `SeriesInfo.Build` builds a listed series with its kept release. `Policy.series` (follow, unfollow, wallet follow), `ExportSeries`, the import merge, the plan's pool counts and the CLI's `policySeries` all use it, so they count the same pools as the running node.
- `follow -pool` or `-mint` on a part the node does not know released yet is still refused, and the message names the part and says it is waiting for a release.
- `unfollow` never writes the "shorter form" for a variable series: it stays whole with its exceptions, so later releases' pools stay followed. The form is kept for fixed series, whose pool count never changes.
- Tests: `TestFollowSeesReleasedParts` (pool 3 and mint 13 of part 3; the export imports into itself and into an empty node, with 4 pools counted), `TestUnfollowKeepsAVariableSeriesWhole`.
