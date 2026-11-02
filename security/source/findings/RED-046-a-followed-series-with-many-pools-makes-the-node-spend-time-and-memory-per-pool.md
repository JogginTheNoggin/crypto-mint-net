# RED-046: a series with very many pools, followed whole, makes the node spend time and memory for every pool at load, and a default import now carries exactly that

| Field | Value |
| --- | --- |
| Severity | medium |
| Status | open |
| Found on | `main` @ 533dbcd (Source Round 18 Part B) |
| Component | pkg/node/node.go (`load`, `logPath`), pkg/engine/engine.go (`FollowedPools`); pkg/nodepolicy/bundle.go (`Merge`, `Followed`) |
| Surface | a bundle file from someone else; `poo policy import -apply` (the default import), `POST /v1/policy/import` |
| Fixed by | - |

## Summary

D150 limits a default import to what to follow: `series`, `follow`,
`issuers.accept`. That closes RED-043. But a series bundle carries the
series' own parameters, and `series.Params.validate` bounds none of them:
`Mints` is a `uint64`, `PoolSize` may be 1, so a series may have up to 2^64
pools. `Merge` never looks at the pool count. A `follow` entry with no pools
follows the whole series, and `node.load` then walks every pool: it
builds the list from `engine.FollowedPools()` (one key per pool) and for each
pool calls `logPath` and `os.Stat`. Time and memory therefore grow with the
number of pools, and the default import is exactly the thing that brings in
such a series and its whole-series follow. After the import is applied the
node does not start in reasonable time or memory, and every later start does
the same until node.json is edited by hand.

## Reproduction

`TestRedHugeSeriesFollowedWhole` (scratchpad `red_r18b_rpc_test.go.txt`,
goes in `pkg/rpc`): a node.json with one series of `n` pools (PoolSize 1)
followed whole, then `node.Open`:

```
65536 pools:   node.Open 109ms,   allocated 64 MiB,   heap now 16 MiB
262144 pools:  node.Open 484ms,   allocated 233 MiB,  heap now 29 MiB
1048576 pools: node.Open 2.46s,   allocated 901 MiB,  heap now 54 MiB
4194304 pools: node.Open 10.2s,   allocated 3769 MiB, heap now 260 MiB
```

Linear in the pools: about 2.4 microseconds, 0.9 KiB allocated and 64 bytes
kept for each. 2^32 pools is about three hours and 275 GiB kept; 2^40 is out
of reach. A CPU profile at 1M pools puts 55% in `os.Stat` through `logPath`
and the rest in `FollowedPools` and hex encoding. I did not reload a live
node with it; the start path is the measured one, and a reload runs `load`.

## Impact

Availability of a node, from a file that looks harmless: the plan of a
default import shows `+ series: {...}` and `+ follow: ...`, the same lines as
for a normal series (the series line is cut at 60 characters, so `mints`
may be the only sign, and nothing says how many pools follow from it). The
operator has to know that `-series` with `-pool` is how a big series is
followed, and that a whole-series follow of a huge one is the trap. The same
holds without a bundle (`poo follow` of a series listed with huge params),
but a bundle is how a stranger's series reaches node.json.

## Recommendation

Do not make the node pay per pool for pools it has not seen:

1. Create per-pool state, files and log paths when a pool is first used
   (an entry, a request, a follow check), and keep the whole-series follow
   as the `all` flag the engine already has (`f.all`), not as a list of pool
   keys. `FollowedPools` becomes an iterator over pools that exist, or is
   bounded by pools with data.
2. In the plan, state the pool count of every series it adds and every
   follow it makes ("follows 4,194,304 pools whole"), and put a whole-series
   follow of more than some number of pools under `Plan.Defences`.
3. Do not cap the series: that would be a consensus rule, and the design
   sets none. A node policy that refuses to follow a series whole above N
   pools would be node policy, but (1) is better because it asks nothing of
   anyone.

For Black: no change to what is valid; the series parameters are untouched.
Please confirm that (1) cannot change which entries or ledgers a node accepts.

## Resolution

Open.
