# ORANGE-051: Round 41 measured; the GPU grinder's chunk, its stop, and entries on every core but one

| Field | Value |
| --- | --- |
| Status | proposed |
| Based on | `main` @ `6cf8002` |
| Area | `pkg/grind` (`gpu-opencl`, `calibrate`, D208), `pkg/layer` (`GrindSearchChunk`), `pkg/entry` (`BuildWith`), `pkg/engine` (`EntryWorkers`) |
| Effort | measurements only; two proposals |
| Decided by | Grey |

## Summary

D208 adds an OpenCL grinder. It searches layers on the GPU in chunks
sized to take about 50 ms, and it grinds entries on the CPU with every
logical core but one. BASE.md asked for the chunk size and the stop
latency with PoCL, and for the cost of entries ground on every core but
one.

Following jerry's rule, the GPU itself is measured only on jerry's
computer. These figures are from PoCL, the CPU OpenCL driver, in the
container. I have asked the node thread for the same runs on the RTX
2070 SUPER, and I will add them here when they come back.

**1. With PoCL, the calibrated chunk loses about 5% of the throughput.**

`calibrate` times one search of 2^20 nonces and picks the largest power
of two that should take at most 50 ms. With PoCL on the 4 cores, it
picked 2^19 for both hashes.

| chunk | SHA-256 | per call | SHA-512/256 | per call |
| --- | --- | --- | --- | --- |
| 2^14 | 8.7 M/s | 1.9 ms | 7.6 M/s | 2.2 ms |
| 2^16 | 10.8 M/s | 6.1 ms | 10.0 M/s | 6.6 ms |
| 2^18 | 17.6 M/s | 15 ms | 11.2 M/s | 23 ms |
| 2^19 (calibrated) | about 17.7 M/s | 30 ms | about 12 M/s | 45 ms |
| 2^20 | 17.7 M/s | 59 ms | 12.4 M/s | 84 ms |
| 2^24 | 18.0 M/s | 0.93 s | 14.2 M/s | 1.18 s |
| 2^26 | 18.9 M/s | 3.6 s | 14.6 M/s | 4.6 s |

- Below 2^18, each call's fixed cost shows: 2^16 loses about 40%.
- From the calibrated chunk to 2^26, the gain is 5 to 15%, but each
  call then takes seconds, and a stop waits that long.
- A run of 2^22 took 1.6 s once, about four times its rate. Running it
  again three times gave the normal 0.26 to 0.33 s, so it was the
  shared machine, not the grinder.
- Building and calibrating a kernel takes 2.2 to 2.8 s with PoCL, once
  per hash per device.
- For comparison, the CPU grinder on these 4 cores does 32.5 M/s for
  SHA-256. So PoCL is no faster than the CPU here, as expected; it only
  exercises the code.

**2. A stop takes one chunk at most: 19 ms typically, under 50 ms.**

`GrindSearchChunk` looks at its context between chunks. Each stop was
asked for 100 to 400 ms into an endless search, 15 times for each chunk
size:

| chunk | SHA-256, median / worst | SHA-512/256, median / worst |
| --- | --- | --- |
| 2^17 (a quarter) | 3.3 ms / 13 ms | 6.1 ms / 12 ms |
| 2^19 (calibrated) | 19 ms / 43 ms | 18 ms / 49 ms |
| 2^21 (four times) | 61 ms / 159 ms | 128 ms / 218 ms |

The calibrated chunk holds a stop under the 50 ms target. The plan runs
one search worker on the GPU (`Threads` 1), and the kernel's lock lets
only one call run at a time anyway.

**3. Entries on every core but one: the node keeps working, but takes
29 to 35% fewer transactions while an entry is ground.**

An entry search with SHA-512/256 and a minimum strength out of reach ran
beside a node taking distinct claims, on the 4 cores:

| entry workers | claims, no sync | claims, synced | entry tries |
| --- | --- | --- | --- |
| none | 31,200/s | 19,800/s | |
| 1 | 32,100/s | 16,800/s | 3.0–3.3 M/s |
| 3 (every core but one, D208) | 22,200/s | 12,800/s | 4.2–4.6 M/s |
| 4 (every core) | 20,400/s | 12,000/s | 4.6–5.1 M/s |

- With 3 entry workers, the node takes 29% fewer claims without sync,
  and 35% fewer synced. The worst single claim was 24 to 34 ms against
  16 to 17 ms with no entry search, so the node stays responsive.
- Leaving one core free is worth only 5 to 10% over taking them all.
  Most of the slowdown comes from the entry search and the claims'
  signature checks sharing cores.
- With the reserve series' minimum entry strength of 20 bits, an entry
  takes about 2^20 tries: about 0.2 s on 3 cores with SHA-512/256 here.
  So the cost lasts a fraction of a second per entry, then the GPU
  grinds layers while the CPU is free.

**4. A lead: the entry search gains only 1.75 times from 4 cores.**

| workers | entry, SHA-256 | layer (CPU) | entry, SHA-512/256 | layer (CPU) |
| --- | --- | --- | --- | --- |
| 1 | 9.4 M/s | 8.8 M/s | 2.6 M/s | 2.8 M/s |
| 2 | 10.6 M/s | 17.1 M/s | 3.9 M/s | 5.7 M/s |
| 3 | 14.2 M/s | 24.4 M/s | 5.7 M/s | 8.2 M/s |
| 4 | 16.4 M/s | 32.5 M/s | 6.6 M/s | 10.6 M/s |

- The layer search uses the lane grinders and gains 3.7 times from 4
  cores. The entry search calls `hash.Grinder.Hash` once per nonce and
  gains 1.75 times.
- It is not the rounds of 4,096 nonces: rounds of 2^14 or 2^16 changed
  little (17.9 and 20.4 M/s at 4 workers).
- It is not allocation: `Grinder.Hash` makes none.
- It is not false sharing between the workers' `Grinder`s: padding
  between them changed little.
- Plain `sha256.Sum256` on 4 goroutines does gain 4 times (9.6 to
  38.5 M/s). The profile shows the time in `blockSHANI` and
  `Digest.Sum`.
- I parked the root cause. It matters more now: on jerry's 12 threads,
  D208 gives entries 11 workers. The dev1 run includes entry scaling to
  12 threads.

## Proposals

**Proposal 1: keep the 50 ms chunk.** With PoCL it costs about 5% of the
throughput and holds a stop under 50 ms. The dev1 figures will show
whether a real GPU's per-call cost makes 2^20 nonces too short to time:
at about 1 G/s, 2^20 nonces is 1 ms, close to the launch cost. If it is
too short, `calibrate` could time a larger first search, for example
until 10 ms have passed.

**Proposal 2: find why the entry search does not scale.** Profile
`entry.BuildWith` with several workers. If `Grinder.Hash` is the cause,
the entry search could use the lane grinders, as the layer search does.
The entry prefix differs from a layer's (the tag, the parent and the
tids, then the nonce), but its midstate is fixed per entry. This changes
only how fast a node builds an entry; the entry and its hash are the
same.

## Measurements

The machine has 4 vCPUs (Xeon with SHA-NI, AVX2 and AVX-512), has 15 GiB
of memory and runs Go 1.27.3. PoCL 5.0 (`pocl-opencl-icd`) was
installed in the container; `clinfo` lists one device,
`cpu-skylake-avx512`. The harnesses were internal test files outside the
commit, run with `POON_OPENCL_ALL=1`.

- **Chunk:**
  - `openCLKernel` for each hash, timed from build to the end of
    calibration.
  - Then `k.search` called back to back for 0.8 s at each chunk size.
  - The CPU figure is `layer.GrindHash` on 1 to 4 workers.
- **Stop:**
  - `Default.Choose` on the PoCL device, then `GrindSearchChunk` with no
    end.
  - Cancelled after a random 100 to 400 ms; the time from `cancel` to
    the return.
- **Entries beside the node:**
  - A node from the `pkg/node` benchmark, taking 40,000 distinct claims
    for 3 s, from 4 goroutines without sync or 64 synced.
  - `entry.BuildWith` of 256 transactions with the given workers in the
    background, its tries counted.
- **Entry scaling:** `BuildWith` for 2 s with 1 to 4 workers. Then the
  round size changed by hand, padding between the `Grinder`s, and
  `sha256.Sum256` on its own.
- **dev1:** the same harnesses are in `/mnt/project-files/node-testing/orange-r41/`
  for the node thread to run.

## What changed

Only this document and its row in the proposals README.

## Consensus impact

None. This PR changes no code.
- Proposal 1 changes how long a GPU call takes.
- Proposal 2 changes how fast an entry is searched, not which nonce
  is found or what any entry or hash is.
- No verdict changes and no depth limit is added.
