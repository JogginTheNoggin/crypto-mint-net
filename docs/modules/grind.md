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
change.

## Grinders and plans

- The CPU has a `cpu-midstate` grinder for every hash `pkg/hash` knows. It
  is a `hash.Grinder`, which keeps the digest's state after the fixed
  prefix (ORANGE-006).
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
- `RunBench(ctx, plan, d)` grinds layers on a random entry for `d` and
  reports tries a second. With the prefix saved, an entry try costs the
  same as a layer try.

## Jobs

A `Spec`:

| Field | Meaning |
| --- | --- |
| `target` | `tip` (the selected tip, or the best branch while none is selected, such as an entry just built), `pending`, or an entry hash |
| `series`, `pools` | Narrow the pools; empty: every followed pool (for an entry, the pool that holds it) |
| `device`, `threads` | As for `Choose`; empty: the node policy's `grind` section, then auto |
| `for` | Stop after this long (`30s`, `2h`) |
| `until_from` | With an entry: stop once its from(e) reaches this weight, "until my payment is safe" |
| `hours` | Work only within these local hours, `22-06` (across midnight is fine) |

Each job works in slices of 2 seconds (`Slice`). After each slice it looks
again at its target, its hours and its goal. A layer search starts at a
random nonce, so slices and jobs never repeat each other.

- **States:** `running`, `waiting` (outside its hours, or nothing to work
  on, with the reason), `done`, `stopped` and `failed`.
- **Events:** `start` (with the plan per hash), `progress` (tries and
  rate), `found` (the layer or entry), `waiting` and `end`. They go to
  `OnEvent` listeners, which the daemon sends to the watch stream as kind
  `grind`.
- **Limits:**
  - At most 16 jobs at once (`MaxJobs`).
  - The last 32 ended jobs are kept (`MaxDone`).
  - Threads are capped at the device's.
  - Stopping a job keeps what it found.
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
- `pkg/rpc/grind_test.go`: the endpoints, the plan and rate in build and
  work replies, bad options, the view token, and the metrics.
- `pkg/daemon/grind_test.go`: policy jobs run by themselves, put events on
  the watch stream, and wait outside their hours.
- `pkg/cli/grind_test.go`: every `poo grind` command, `poo work -for`
  with a device, and an interrupted `run` that stops its job and exits 0.
