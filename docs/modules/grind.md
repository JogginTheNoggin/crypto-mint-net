# pkg/grind

## Summary

Does a node's hash work: finding the nonces of entries and layers, as fast
as the machine allows (milestone M11, decision D109). It has three parts:

- **Devices.** `Detect` lists the CPU, with its hashing features, and any
  GPUs the system lists.
- **Grinders.** A `Registry` holds one grinder per hash and kind of
  device, plus a generic one that grinds any hash more slowly.
- **Plans.** `Choose` picks a device and grinder for a hash, by name or
  automatically, and says why.

A `Manager` runs jobs inside a node against targets, so the user names
what to work on and never a hash. A job can be started by hand, run on a
schedule, or come from the node policy:

- `tip` adds layers to the selected tip.
- `pending` builds entries from pending transactions.
- An entry hash adds layers to that one entry.

The same code serves `poo build`, `poo work` and `poo grind`, the RPC and
the GUI. Grinding is outside consensus: every search finds what the
plain one-goroutine search finds for the same nonces.

## Devices

| Field | Meaning |
| --- | --- |
| `id` | What commands take: `cpu`, `gpu:0`, ... |
| `kind`, `name`, `threads` | The kind (`cpu`, `gpu`), the model or vendor and driver, and the logical cores |
| `features` | For the CPU: `sha` (x86 SHA-NI), `avx2`, `avx512f`, `asimd`, `sha2`, `sha3`, `sha512` (Arm) |
| `hashes`, `generic` | The hashes it has a fast grinder for, and whether it can grind any other |
| `auto_for` | The hashes for which `auto` picks it |
| `note` | For example, a GPU found with no grinder for it yet |

The CPU probe uses `klauspost/cpuid`. The GPU probe reads
`/sys/class/drm/card*/device` on Linux (vendor and driver). There is no
GPU grinder yet, so GPUs are listed with a note and `auto` passes them
over. Registering a grinder for kind `gpu` makes them used, with no other
change. When two probes find the same id (the system's list and a grinder's
own probe), the first one is kept.

## Grinders and plans

- The CPU has a `cpu-midstate` grinder for every hash `pkg/hash` knows. It
  is a `hash.Grinder`, which keeps the digest's state after the fixed
  prefix (ORANGE-006).
- `cpu-blake3-cv` grinds BLAKE3 keeping BLAKE3's chaining values after the
  prefix, which `hash.Grinder` cannot (ORANGE-009): an entry try of a
  256-transaction entry goes from about 811 ns to 152 ns. It is
  registered ahead of the midstate one, so it is BLAKE3's CPU grinder.
- `blake3LayerLanes` tries sixteen BLAKE3 layer nonces at once on
  AVX-512, about 5 times faster (ORANGE-011). It builds only with
  `GOEXPERIMENT=simd`, which is experimental in Go 1.27; the opt-in
  build wires it as `cpu-blake3-avx512` (below).
- With `GOEXPERIMENT=simd` on an AVX-512 CPU, `cpu-sha256-avx512` and
  `cpu-blake3-avx512` search sixteen layer nonces at once through
  `Grinder.Search`, which gives a `layer.Search` (ORANGE-012): about 3.6
  times the SHA-256 layer rate. A default build has neither.
- `cpu-generic` grinds any hash through `hash.Lookup`, so a hash added to
  `pkg/hash` can be ground at once. It gets a fast grinder by
  registering one.
- `Choose(devs, hashID, device, threads)` returns a `Plan{device, kind,
  grinder, hash, threads, fallback, reason}`.
  - `device` `""` or `auto` picks, in order of preference:
    1. A device other than the CPU with a fast grinder for the hash.
    2. The CPU's fast grinder.
    3. A generic grinder, which sets `fallback`.
  - `threads` 0 means every logical core but one, so the node stays
    responsive.
  - An unknown device, or one with no grinder for the hash, is an error
    that lists the devices there are.
- `Plan.Engine(tried)` gives the `engine.Grind` that `Node.BuildWith`
  and `WorkWith` take. It searches on `threads` goroutines and counts
  tries for a rate (ORANGE-007: the same result whatever the number of
  workers).
- With the node policy's `grind.layer_floor` above 0, each search also
  hands the node the strongest layer of every 4,096 nonces at or above the
  floor (`engine.Grind.Floor`, `layer.Keep`), and `Job.Layers` counts them
  (ORANGE-010, D118). The strongest of the search still goes in first.
- `RunBench(ctx, plan, d)` grinds layers on a random entry for `d` and
  reports tries a second. With the prefix saved, an entry try costs the
  same as a layer try.

## Jobs

A `Spec`:

| Field | Meaning |
| --- | --- |
| `target` | `tip` (the selected tip, or the best branch while none is selected, such as an entry just built), `pending`, `tx:TID` (one payment, below), or an entry hash |
| `series`, `pools` | Narrow the pools; empty: every followed pool (for an entry, the pool that holds it) |
| `device`, `threads` | As for `Choose`; empty: the node policy's `grind` section, then auto |
| `for` | Stop after this long (`30s`, `2h`) |
| `until_from` | With an entry or `tx:`: stop once from(e) reaches this weight, "until my payment is safe"; a `tx:` target needs it |
| `hours` | Work only within these local hours, `22-06` (across midnight is fine) |
| `lease` | Stop unless renewed (`Renew`, `POST /v1/grind/{id}/renew`) within this long, 1s to 10m: a job its follower started ends when the follower is gone |

A **`tx:TID` job** (GREEN-019) follows one payment with the node's own
`Engine.Payment`, the call `poo tx` reads, so the two always agree. While
the payment is pending the job builds an entry for it, then adds layers
to that entry (or to the best branch, if the entry was built before).
Once it is selected the job adds layers to its entry until from(e)
reaches `until_from` and ends `done`. It ends `failed` if the payment
was held and then is neither pending nor in an entry (dropped, or spent
twice), and waits while the node has not seen it.

A job that would work now is refused at `Start` when no device can grind
one of its hashes (`ErrNoDevice`), as build and work are. A job with
`hours` is accepted, since its device may be there by then; it fails
then if not (GREEN-017).

Each job works in slices of 2 seconds (`Slice`). After each slice it looks
again at its target, its hours and its goal. A layer search starts at a
random nonce, so slices and jobs never repeat each other.

- **States:** `running`, `waiting` (outside its hours, or nothing to work
  on, with the reason and a `hint` of what would help, such as building
  an entry first for a tip job), `done`, `stopped` and `failed`. A
  stopped job's reason says why: `stopped`, its lease ran out, or the
  node stopped.
- **Events:** `start` (once, with the spec), `plan` (the device and
  grinder for a pool's hash), `progress` (tries, and the rate since the
  last one, at most once a second: `ProgressEvery`), `found` (the layer or
  entry), `waiting` (with the hint) and `end`. `start`, `waiting` and
  `end` are job-wide: they have no series or pool, and a watch filter on
  series or pools passes them when the job's spec can work on one of
  them. They go to `OnEvent` listeners, which the daemon sends to the
  watch stream as kind `grind`.
- **Limits:**
  - At most 16 jobs at once (`MaxJobs`). A start holds its slot from the
    count to the insert, so concurrent starts cannot pass it (RED-014).
  - The last 32 ended jobs are kept (`MaxDone`).
  - Threads are capped at the device's.
  - Stopping a job keeps what it found.
- **Schedule:** `Schedule(section)` applies a node policy's `grind`
  section while the node runs: it stops the jobs the policy started, takes
  the new defaults and starts the new jobs, so `PUT /v1/grind/schedule`
  and `poo grind schedule` need no reload and leave user jobs running.
  Calls run one at a time, so two never leave both policies' jobs
  running (RED-013).
- **Totals:** these feed the `poo_grind_tries_total{device,hash}` and
  `poo_grind_jobs{state}` metrics.

## Extending

- **A new kind of device:** call `Default.RegisterProbe(func() []Device)`
  and `Default.Register(Grinder{Kind: ..., Hash: id, New: ...})`. `New`
  returns a `hash.NewHasher`, a maker of `hash.Hasher` (`Hash(suffix)`
  after a fixed prefix). Each search worker makes its own. The search
  loops in `pkg/entry` (`BuildWith`) and `pkg/layer` (`GrindWith`) take
  any hasher, so a device needs no change elsewhere.
- **A new hash:** `cpu-generic` grinds it as soon as `pkg/hash` knows it.
  For speed, register a grinder for it.
- **A device that searches whole ranges itself,** as a GPU kernel would:
  it can still sit behind `Hasher`, one try at a time. A range interface
  can come with the first such grinder.

## Tests

- `grind_test.go`:
  - The default plans for every hash.
  - The fast and generic grinders agree with `layer.Grind` on one thread
    and several.
  - A registered test device is picked by auto for its hash only.
  - GPUs from a fake `/sys/class/drm`.
  - Hours windows and spec checks.
  - Every kind of job on a real node: pending builds an entry, tip selects
    it, an entry job stops at its from(e), waiting, stop and `for`, along
    with the events, totals, limits and bench.
  - `TestJobEvents`: one start per job, progress at most once a second,
    hints, and a build's try counted. `TestLease`: a lapsed lease stops
    a job, a renewed one runs on. `TestTxTarget`: a payment from pending
    to safe.
- `blake3_test.go`, `blake3_lanes_test.go` (with `GOEXPERIMENT=simd`):
  both BLAKE3 grinders agree with the plain search.
- `pkg/rpc/grind_test.go`: the endpoints, the plan and rate in build and
  work replies, bad options, the view token, and the metrics.
- `pkg/daemon/grind_test.go`: policy jobs run by themselves, put events on
  the watch stream, and wait outside their hours.
- `pkg/cli/grind_test.go`: every `poo grind` command, `poo work -for`
  with a device, an interrupted `run` that stops its job and exits 0, a
  lapsed lease, and the schedule with and without a running node.
- `pkg/watch`'s `TestGrindJobEvents`: job-wide events reach a watcher of
  the job's series and pools.
