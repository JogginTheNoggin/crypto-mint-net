# RED-124: After a driver error the GPU grinder searches on one CPU goroutine in GPU-sized chunks, so a stop takes seconds, and nothing says why

| Field | Value |
| --- | --- |
| Severity | Low (the result is right; the cost is a job that does not stop for seconds, a slow search for the rest of the run, and no sign of either) |
| Status | fixed |
| Found | Source Round 41 Part B, on `main` @ `6cf8002` (D208, `pkg/grind/opencl.go`) |
| Area | `pkg/grind/opencl.go` (`layerSearch`, `fail`, `cpuSearch`, `calibrate`), `pkg/layer/layer.go` (`grindChunk`) |
| Fixed by | |
| Fixed by | `claude/project-thread-ggo8dt` @ HASH (BLUE-042) |
| For Black | Not a rule: the nonce and strength a failed search returns are the CPU's, which I checked equal to `layer.GrindHash`. |

## What is wrong

D208 says a driver failure moves the search to the CPU with the same result. The result is the same. The way it moves is not what the plan was made for:

- **One goroutine, scalar.** `layerSearch` calls `cpuSearch`, a loop on the calling goroutine with the plain hasher. A GPU plan has one worker (the GPU's), so the whole machine grinds on one core, no SIMD lanes, while the entry threads sit idle.
- **A GPU-sized chunk.** The plan's chunk came from `calibrate` for the GPU (about 50 ms of the card: 2^27 nonces at jerry's 4.5 G/s). `grindChunk` looks at the context only between chunks, and `cpuSearch` has no check of its own. A stop, a shutdown or a job cancel waits for a chunk of 2^27 nonces on one core.
- **For the rest of the process.** `k.failed` is never cleared, the kernel stays in the `clKernels` cache, and every later plan for that device gets the same failed kernel. The device is still listed as the GPU grinder.
- **Silent.** `fail` stores the error and nothing reads it. The comment on `layerSearch` says "Failed says why", but there is no `Failed` method, no log line, and `poon grind devices`, `doctor` and the job's status show nothing.

Driver errors are not rare on a desktop GPU: a suspend and resume, a display driver reset, an out-of-memory from another program.

## How I checked it

PoCL (the CPU OpenCL driver, `POON_OPENCL_ALL=1`) on a 4-core container; no GPU (jerry's rule). Throwaway test in `pkg/grind` (removed; copy kept with Red's notes) builds the kernel, calls `k.fail(err)` as a driver error would, then:

```
sha256:      failed kernel, 8,388,608 nonces on one goroutine: 10.0 M/s, same nonce and strength as layer.GrindHash: true
sha256:      layer.GrindHash with 3 workers: 30.8 M/s
sha256:      cancel after 100 ms, chunk 2^24: returned after 1.7 s
sha256:      cancel after 100 ms, chunk 2^27: returned after 13.4 s
sha512-256:  one goroutine 3.8 M/s; 3 workers 10.7 M/s
sha512-256:  cancel after 100 ms, chunk 2^24: returned after 4.6 s
sha512-256:  cancel after 100 ms, chunk 2^27: returned after 36.1 s
```

2^27 is what the card's rate gives (4.5 G/s x 50 ms). On a machine with SIMD lanes the one-goroutine gap to the CPU plan is wider than here. I did not make a real driver fail.

## Suggested fix (Blue's call)

- On the first error, make the plan the CPU's: the CPU grinder, the CPU chunk and every worker. For a search already running, `cpuSearch` should check the context every few hundred thousand nonces (or split its range across goroutines with the lanes grinder).
- Clear or re-probe `failed` at the next job (or after a minute), so a driver that came back is used again.
- Log the error once, and show it as the device's note in `poon grind devices` and `doctor` ("gpu:1: grinding on the CPU since: …"), and in the job.

## Not tested

A real driver failure; a hung driver that never returns (a C call cannot be cancelled, so nothing can stop it; see RED-125).

## Resolution

Fixed in BLUE-042 (D209). The GPU grinder's search now sees the grind's context (`layer.SearchContext`). After a driver error the range is searched on every core (`parallelSearch`, with the CPU's SIMD search where registered), each worker looking at the context every 2^16 nonces, with the same result. Red's 2^27 case stopped in well under a second in the test. Later ranges stay on the CPU for a minute, then the GPU is tried again. The error is logged once and `OpenCLStatus` (`poon grind devices`) names the device and error while it lasts. The plan's chunk stays the GPU's; with the context checked inside, that no longer delays a stop.

Tests: `TestBlue042DriverFailureFallsBack`, `TestBlue042ParallelSearch`; the OpenCL tests pass on PoCL through the new search.
