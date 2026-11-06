# GREEN-066: grind devices, doctor and bench with and without a GPU driver

| Field | Value |
| --- | --- |
| Status | proposed (prototyped) |
| Based on | `main` @ `6cf8002` |
| Area | `pkg/grind` (`OpenCLStatus`, `openCLOff`), `pkg/rpc` (`opencl` in `/v1/grind/devices`), `pkg/cli` (`opencl` in `poon grind devices`, doctor's grind line), docs (running-a-node, modules/grind) |
| Effort | small |
| Consensus | none: a status line and one environment value. Grinding, plans and results are unchanged |
| Decided by | Grey; Black has nothing to check; White may note the reply field below |

## Summary

BASE.md asked Green to walk `poon grind devices`, `doctor` and `grind
bench` as a new user, with and without a GPU driver. GPUs are tested
only on jerry's computer, so this walk ran in the container, which has
no GPU. Green ran it three ways:

- **With no driver.** The container started with the ICD loader and no
  vendor installed.
- **With a driver.** PoCL installed, plus `POON_OPENCL_ALL=1`, which
  lists PoCL as `gpu:0`.
- **With the loader pointed at nothing.** `OCL_ICD_VENDORS=/nonexistent`.

Each run went through `grind devices` (JSON and `-o table`), `doctor`
with and without a node, `grind bench`, `grind bench -device gpu:0`, and
`POON_OPENCL` set to `off`, `false` and a bad value.

With a driver, devices, doctor and bench agree: `gpu:0` grinds sha256
and sha512-256, `auto` picks it for those, doctor says "grinds on: cpu,
gpu:0", and the bench measures it. Two things got in the way:

1. **With no GPU listed, nothing said why.** On a machine with no
   driver, or whose GPU the system does not list (a VM, a container,
   some laptops), `poon grind devices` showed only `cpu`, and doctor
   said "grinds on: cpu". The reason (no OpenCL library, no platform,
   turned off) is only ever attached as a note to a GPU the system
   lists. With no GPU listed, the person learns nothing, and someone
   who expected a GPU has no next step.
2. **`POON_OPENCL=false` (or `no`) left the GPU grinder on, silently.**
   Only `off` and `0` turned it off.

## The change

- `grind.OpenCLStatus()` gives one line from the OpenCL probe:
  - "OpenCL: N devices", or
  - "no GPU grinder: " followed by the reason, such as "turned off by
    POON_OPENCL", or "no OpenCL platform (error -1001): install the GPU
    vendor's OpenCL driver".
  - A build without the GPU grinder (FreeBSD, or another architecture)
    says so.
- `poon grind devices` prints it as `opencl`. When the node answers, the
  node's status is used (`/v1/grind/devices` gains `opencl`).
- Doctor's grind line adds it when no GPU grinds, for example:
  "grinds on: cpu; jobs in the policy: 0; no GPU grinder: no OpenCL
  platform (error -1001): install the GPU vendor's OpenCL driver".
- `POON_OPENCL` turns the grinder off for `off`, `0`, `false` or `no`, in
  any case.
- Docs: running-a-node.md and modules/grind.md.

## How it was tested

- `TestGrindDevicesSayWhyNoGPU` (pkg/cli) fails on main ("grind devices
  opencl = """). It checks that `opencl` and doctor both say "turned off
  by POON_OPENCL", which the CLI tests set.
- `TestOpenCLOffAndStatus` (pkg/grind) covers the values and the line.
- By hand, the walk above in each of the three setups. The status reads:
  - "no OpenCL platform (error -1001) ..." with the loader pointed at
    nothing;
  - "the OpenCL driver lists no GPU" with PoCL but no
    `POON_OPENCL_ALL`;
  - "OpenCL: 1 device" with `POON_OPENCL_ALL=1`;
  - "turned off by POON_OPENCL" with `POON_OPENCL=false`.
- gofmt, go vet, staticcheck, `go test -race ./...`, and a FreeBSD
  build.

## For White

`/v1/grind/devices` and `poon grind devices` gain an optional `opencl`
string. No file or wire format changes.

## Not done (ideas for later, for jerry's GPU run)

- **`auto` picks the GPU even when it is slower.** With PoCL as `gpu:0`,
  the bench measured sha256 at 20 M/s on `gpu:0` and 30 M/s on the CPU,
  yet `auto_for` gave sha256 to `gpu:0`. On a real discrete GPU this
  will not happen, but an integrated GPU might be slower than a strong
  CPU. Doctor could compare a short bench, or `auto` could prefer the
  faster device measured once. That is Grey's call, and it should be
  checked on jerry's machine.
- **Bench reasons say "asked for:" when nobody asked.** `poon grind
  bench` with no `-device` runs every device, and each plan's reason
  reads "asked for: gpu:0 has a gpu-opencl grinder for sha256". It
  could read "bench:", and use the CPU plan's wording, "the gpu-opencl
  grinder for sha256".
