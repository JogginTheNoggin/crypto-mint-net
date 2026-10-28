# GREEN-016: The grinder's commands (for M11)

| Field | Value |
| --- | --- |
| Status | proposed |
| Based on | `claude/m10-watch` @ `0ac8e4e` |
| Area | M11: a new `pkg/grind`, `poo grind`, `/v1/grind/*`, the node policy, `watch`, `metrics`, `doctor` |
| Effort | medium (the commands; the grinders themselves are Grey's M11) |
| Decided by | |

## Summary

This proposes the user-facing side of M11, so that Grey can build the
grinders behind a command surface that is settled ahead of time. It covers:

- **`poo grind devices`**: what hardware there is, and which device
  "auto" picks.
- **`poo grind bench`**: how fast each device grinds this series' hash.
- **Manual work**: `build` and `work` gain `-device`, `-threads` and
  `-for DURATION`.
- **Long jobs in the node**: `poo grind start`, `jobs` and `stop`.
- **Automatic work**: a `grind` section in the node policy.
- **Visibility**: events, metrics and a doctor check.

The user never names a hash: every command takes a target (`-series`,
`-pool`, `-serial`, `-entry`), as M11 asks. This is a proposal only.
Grey owns M11 in Round 5.

## Problem

The M11 roadmap lists what the grinder must do: hardware detection,
manual, scheduled and automatic work, targets, and a registry of hashes.
It does not say how a person drives it. Without a plan, each piece tends
to get its own flags and output, and the GUI then has to wrap several
different shapes. Today the only knob is `-tries` (nonces), which people
can't reason about. They think in time ("grind for 30 seconds") and in
results ("until my payment is safe").

## Idea

Each command below is in `pkg/cli`, calls `pkg/grind` (or the node's RPC
when the work runs inside the node), and takes `-o table`, like every
other read command.

**1. `poo grind devices`.** It lists every device: `id` (`cpu`,
`gpu:0`, ...), `kind`, `name`, `threads`, `features` (for example
`avx2` or `sha-ni`, through `klauspost/cpuid`, which is already a
dependency), and `hashes` (the hash ids it has a fast grinder for, with
`generic` marking the fallback). The device that `auto` would choose is
marked. With a running node it asks the node (`GET /v1/grind/devices`),
because the node is what grinds. Without one, it detects locally and says
so.

**2. `poo grind bench [-series ID | -pool K | -serial N] [-device D]
[-seconds 5]`.** It reports hashes per second per device for the
target's hash. This is also what `auto` uses to choose a device, so the
choice can be explained. Orange can use the same numbers.

**3. Manual work.** `poo build` and `poo work` gain three flags:
- `-device D` (default `auto`)
- `-threads N` (default: all but one core, so the node stays
  responsive)
- `-for 30s`, as an alternative to `-tries`

Their reply gains `device`, `fallback` (true when the device had no fast
grinder for this hash, with the reason), `hashes`, `seconds` and
`rate`.

**4. Jobs inside the node.** These run until a goal is met, stopped, or
the node stops:
- `poo grind start TARGET [-device D] [-threads N] [-for D |
  -until-from W]` returns a job id. `-until-from W` stops once the
  target's `from(e)` reaches W, which is the "until my payment is safe"
  case and uses the same threshold the wallet uses.
- `poo grind jobs` shows each job's target, device, rate, run time,
  layers added and current `from`.
- `poo grind stop ID|-all`.

A job is `POST /v1/grind`, `GET /v1/grind` and `DELETE /v1/grind/ID`.
`start` and `stop` are writes, so view-only mode (D101) and the view
token refuse them. `devices` and `jobs` are reads. `bench` grinds, so
it is local only.

**5. Automatic and scheduled work.** An optional `grind` section in
`node.json`, applied on reload like everything else:

```json
"grind": {
  "device": "auto", "threads": 0,
  "jobs": [
    {"series": "…", "pools": [1], "target": "tip", "hours": "22-06"},
    {"issuer": "…", "target": "pending"}
  ]
}
```

`target` is `tip` (keep adding layers on the selected tip), `pending`
(build entries from pending transactions as they arrive), or an entry
hash. `hours` is local time on the node's clock. That is fine outside
consensus, which reads no clock. `Validate` checks the section, so GREEN-011
refuses a reload onto a bad one.

**6. Visibility.**
- A `grind` event kind for `poo watch` (job started, stopped, layer
  found).
- `poo_grind_hashes_total{device,hash}` and
  `poo_grind_jobs{state}` in `/v1/metrics`.
- An info log line for each job start and stop.
- A `grind` check in `poo doctor`: devices found, the policy's device
  exists, and a fallback is reported as a warning with the reason.

**7. Modularity, as M11 asks.** `pkg/grind` keeps a registry keyed by
`hash.ID` with a `generic` CPU entry. The commands above only ever see
the `Grinder` interface and the device list, so a new hash or device
changes no command, flag or reply field.

## Cost and risk

The commands are thin over `pkg/grind`. The real work is the grinders,
which M11 does anyway. Risks:
- **Starving the node.** Jobs that use every core could starve the node,
  hence the default of all cores but one and the per-job `-threads`.
- **Abuse through the RPC.** A job started over the RPC is local
  authority (the full token). That is the same trust as `build` and
  `work` today, but long-running, so Red should look at limits: the
  number of jobs and the total threads.

## Prototype

None. This shapes Grey's M11 work instead of racing it. In Round 5 Part B,
Green can build any piece Grey hands back, such as `grind devices` and
`bench`, or the `-for` flag.
