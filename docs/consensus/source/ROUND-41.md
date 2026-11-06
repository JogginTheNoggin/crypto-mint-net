# Source Round 41: consensus review (Black, Part D)

## Summary

This round adds a GPU grinder (D208, jerry's request). An OpenCL kernel
searches layer nonces for the two hashes the shipped series use, SHA-256
and SHA-512/256. It is loaded at run time with no cgo. Entries are still
ground on the CPU.

Blue's D209 fixes RED-124 to RED-127:
- a GPU driver failure falls back to every CPU core and stops quickly;
- `POON_OPENCL_ALL` reads as written;
- a reload keeps the node id;
- on Windows, OpenCL is loaded only from System32.

D209 carries Green's GREEN-066.

**Result: 10 clear, 0 with conditions, 0 blocked.**

- **No consensus rule or depth limit is added.**
  - The layer, ledger, entry, tx, series and policy rules are untouched.
  - The `engine` and `layer` changes are on the grinding side only: a
    chunk size, entry workers, and a search that sees its context.
  - `TestByteFormV1Frozen` passes on `main` and on PR #376. Black's
    cross-version outputs are byte for byte the same as Round 40's
    baseline on both.
- **The GPU's results are identical to the CPU's.** Black installed PoCL,
  an OpenCL driver that runs on the CPU, and checked this in two ways:
  - **Black's own check.** It compared the kernel's answer with a brute
    force over the consensus path itself, `layer.Layer{Entry,
    Nonce}.Strength(f)` with the series' `hash.Func`. The check covered
    836,623 nonces for both hashes, including ranges across the 32-bit
    word, near 2^56 and up to the last nonce. Every answer matched: the
    strongest layer, and the lowest nonce among equals.
  - **Blue's and Grey's tests.** `TestOpenCLSearchMatchesCPU`,
    `TestOpenCLPlanGrinds`, `TestBlue042DriverFailureFallsBack` and
    `TestBlue042ParallelSearch` all pass on PoCL, on `main` and on #376.
- **The GPU is never trusted.** It only proposes a nonce. Every ground
  layer goes through `Node.HandleLayer`, then `Engine.HandleLayer`, which
  hashes it again with the series' own hash (`l.Hash(p.s.Hash())`)
  before `AddLayerHashed` checks the floor and counts its weight. Kept
  layers above `grind.layer_floor` take the same path. So a faulty
  driver or kernel can only cost work: a weaker layer, or none. It can
  never put a layer into a ledger that another node would refuse. The
  bounty path is the same, since the payer hashes a claimed layer
  itself.

| Field | Value |
| --- | --- |
| Round | Source Round 41 |
| Consensus | None (D208, D209) |
| Other consensus suggestions | None |
| Base reviewed | `main` @ `6cf8002` (PR #369: D208; GUI #371 on top, `56c5e0a`) |
| Previous base | `main` @ `b94d861` |
| Blue branches | PR #376 @ `f971408` (BLUE-042, D209: RED-124 to RED-127; carries GREEN-066 #372) |
| Red | PR #370 @ `37d81fa` (RED-124 to RED-127) |
| Proposals | GREEN-066 (PR #372 @ `79191a3`, carried in #376); ORANGE-051 (PR #373 @ `34c2f87`, measurements; proposal 2 optional, for Grey) |
| Result | 10 clear, 0 with conditions, 0 blocked |

## The GPU grinder and the ledger (D208, D209)

### Same results as the CPU

The kernel answers one question: in this range of nonces, which nonce
gives the strongest layer, taking the lowest nonce among equals? That is
the CPU search's answer, and it decides only which layer this node
offers.

- **Black's check on PoCL.** For SHA-256 and SHA-512/256, Black ran the
  kernel on 44 ranges per hash, with random entries:
  - from nonce 0;
  - across `2^32`;
  - near `2^56`;
  - ending at `2^64 - 1`;
  - 40 random ranges of up to 20,000 nonces.

  For each range, Black hashed every nonce through `layer.Layer.Strength`
  with `hash.Lookup(id)`, which is the function the ledger uses. The
  kernel's nonce and strength equalled the brute force's every time, and
  the strength it reported equalled the consensus strength of that
  nonce.
- **The driver-failure path (RED-124).** After a driver error,
  `parallelSearch` searches the range on every core with the CPU's fast
  search, and gives the same answer (`TestBlue042ParallelSearch`,
  `TestBlue042DriverFailureFallsBack`).
- **Other hashes.** A series with any other hash keeps the CPU grinder
  (`grind.ErrNoGrinder` names why).

### Nothing found by the GPU is taken on trust

- `Engine.GrindLayerWith` returns the layer to `Node.WorkWith`, and that
  hands it to `HandleLayer(k, l, engine.Local)`. Each kept layer goes the
  same way.
- `Engine.HandleLayer` computes `lh := l.Hash(p.s.Hash())` itself, and
  `ledger.Pool.AddLayerHashed` checks the floor against `lh`. No caller
  passes in a hash or strength from the grinder. The only caller of
  `AddLayerHashed` is that line.
- A layer the GPU got wrong would fail `ErrWeakLayer` here, as a peer's
  would. So a kernel bug can only make the node slower. It never changes
  what any node accepts, and it can't produce a ledger that some nodes
  take and others refuse.

### The engine and layer changes

- `engine.Grind` gains `Chunk`, `EntryWorkers` and `SearchContext`, and
  `layer` gains `GrindSearchChunk` and `GrindSearchContext`. These choose
  how many nonces a search takes at a time, and how many goroutines build
  an entry. They are inputs to the search. The rules that check its
  output don't read them.
- `entry.BuildWith` is unchanged. An entry built with `EntryWorkers` is
  checked by `AddEntry` like any other.

### The rest

- **A reload keeps the node id (RED-126).** `p2p.Config.ID` is the hello's
  random id, drawn once per process. wire.md now says it lasts for the
  process. Nothing in a ledger, the peer book or the tracker treats it as
  an identity (RED-114), and a restart still draws a new one.
- **`POON_OPENCL`, `POON_OPENCL_ALL` and the Windows DLL path (RED-125,
  RED-127, GREEN-066).** These are local switches and loading.
- **The `purego` dependency.** A run-time loader for the OpenCL library.
  It is node code outside consensus. It doesn't touch hashing, encoding
  or checking.

## Clear

| Item | Why it is clear |
| --- | --- |
| D208: OpenCL GPU grinder for SHA-256 and SHA-512/256 layers | Node work outside consensus. Results equal the consensus hash's (Black's brute force on PoCL). Every layer is hashed again by the series' hash before it counts |
| D208: `engine.Grind.Chunk`, `EntryWorkers`; `layer.GrindSearchChunk` | Search inputs only. No rule reads them |
| D208: device matching, `POON_OPENCL`, doctor and bench output | Local setup and display |
| D209 (RED-124): driver failure falls back to every CPU core, same result; `SearchContext` | Same answer (tested). Output still checked by the ledger |
| D209 (RED-125): `POON_OPENCL_ALL` values | Local switch |
| D209 (RED-126): a reload keeps the node id | The hello's random id, not an identity. wire.md updated |
| D209 (RED-127): `OpenCL.dll` from System32 only | Loading safety |
| GREEN-066 (carried in #376): `OpenCLStatus`, `opencl` in `grind devices` and `doctor` | Display |
| Red (PR #370): RED-124 to RED-127 | Findings, fixed in #376. None touches a rule |
| ORANGE-051 (PR #373) | Measurements. The chunk gets 99.5% of the GPU's rate. Proposal 2 changes entry workers, not verdicts |

## Items for the owner

- **Nothing needs your approval this round.**
- **The GPU grinder can't fork anything.** It only searches for good
  nonces. Every layer it finds is checked again with the normal hash
  before it counts, exactly like a layer from a peer. A buggy driver can
  only slow a node down.
- **GPU and CPU agree.** Black ran the GPU kernel on an OpenCL driver that
  runs on the CPU, and compared it with the ledger's own hash on more than
  800,000 nonces for both hashes. Every result matched.

How this was checked:
- Read D208 and D209, `pkg/grind/opencl.go` and `opencl_kernel.cl` (the
  search's contract), `grind.go` (`parallelSearch`, fallback),
  `layer/layer.go` (`grindChunk`), `engine.go` (`GrindLayerWith`,
  `HandleLayer`), `ledger/pool.go` (`AddLayerHashed`), `node.go`
  (`WorkWith`), `p2p/host.go` (`Config.ID`), and the diffs of PRs #369,
  #370, #372, #373 and #376.
- Installed PoCL (`pocl-opencl-icd`) and ran Black's brute-force
  comparison and the OpenCL tests with `POON_OPENCL_ALL=1`.
- The Round 24 and Round 25 cross-version tests and `TestByteFormV1Frozen`:
  identical to Round 40's baseline on `main` and on `f971408`.
- `go test ./...` passes on #370, #372, #373 and #376. On `56c5e0a`
  (`main`), it passed except for one timing failure:
  `TestWorkWithFloorIsCapped` kept 99 layers against a limit of 98, while
  other packages loaded the machine. It passed 5 of 5 times when run
  again alone. This test measures node policy's kept-layer rate, not a
  rule, and it passes on #376.
- The grind, layer, engine, daemon, p2p and cli packages pass under
  `-race` on `f971408`. The OpenCL tests pass on PoCL on both.
