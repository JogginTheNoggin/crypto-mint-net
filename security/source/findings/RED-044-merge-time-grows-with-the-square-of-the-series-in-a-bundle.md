# RED-044: importing a bundle takes time that grows with the square of its series, so a few MiB of series makes even a dry run take minutes while the node holds `policyMu`

| Field | Value |
| --- | --- |
| Severity | low |
| Status | fixed |
| Found on | `main` @ b17e892 (Source Round 17 Part B) |
| Component | pkg/nodepolicy/bundle.go (`mergeInto`), pkg/nodepolicy/follow.go (`AddFollow`, `checkFollow`, `series`) |
| Surface | a bundle file from someone else; `poo policy import` (a dry run is the default), `POST /v1/policy/import` |
| Fixed by | `claude/project-thread-ggo8dt` @ cfd1cbd (BLUE-018) |

## Summary

`mergeInto` calls `res.AddFollow(f)` for every follow entry of the file.
`AddFollow` calls `checkFollow`, which calls `Policy.series(id)`, which builds
every listed series with `series.New` until it finds the one it wants. A
bundle of n series, each followed, therefore costs n x n `series.New` calls.
Reading the bundle is cheap (26 ms for 4000 series); the merge is not.

## Reproduction

`TestRedBundleManySeriesIsQuadratic` (scratchpad
`red_r17b_nodepolicy_test.go.txt`, goes in `pkg/nodepolicy`): bundles of n
series that are each followed, `ReadBundle` then `Merge` into a default
policy:

```
n=500   (278 KiB):  read 4ms,  merge 177ms
n=1000  (556 KiB):  read 8ms,  merge 727ms
n=2000  (1.1 MiB):  read 14ms, merge 2.97s
n=4000  (2.2 MiB):  read 26ms, merge 10.4s
```

Four times the series, 16 times the time. The profile is `AddFollow` ->
`checkFollow` -> `series` -> `series.New`. By that growth the RPC body cap
(8 MiB, about 15,000 series) is about 2.5 minutes and the 16 MiB bundle cap
about 9 minutes. I did not run the larger sizes.

## Impact

CPU only, but it needs nothing from the operator beyond looking at the file:
`poo policy import hostile.json` is a dry run, and it hangs. Through the
node's endpoint the merge runs under `policyMu`, so follow changes and
schedule edits wait behind it for the whole time, and the full token holder's
own import is what starts it.

## Recommendation

In `mergeInto`, build the id-to-series map once (it already has `have`) and
add the follow entries with a lookup that does not rebuild the series
(`checkFollow` could take the map, or `AddFollow` could cache the built
series in the `Policy` for the call). That makes it linear. A cap on series
per bundle would also work but it is only needed if the linear version is
still slow; it is import-time only and touches no ledger rule.

## Resolution

Fixed (D150). `mergeInto` keeps the series it builds by id, and adds the
file's follow entries with `addFollows`, which checks each against that
map and indexes the follow list once. It follows exactly what `AddFollow`
one by one does; an entry it changes keeps its place. 4000 followed series
now merge in about 0.4 s (1000 in 0.1 s), against 10.4 s before. No cap
was added. Tests: `TestManySeriesMergeIsLinear` (4000 against 1000 must
stay under 9x), `TestAddFollowsMatchesAddFollow`.
