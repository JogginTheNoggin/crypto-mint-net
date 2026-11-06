# RED-125: `POON_OPENCL` is off only for the exact words `off` and `0`, and a driver that crashes ends the node, with no fallback

| Field | Value |
| --- | --- |
| Severity | Low (a switch that fails open in a way the person cannot see, and a crash path that has only that switch to stop it) |
| Status | open |
| Found | Source Round 41 Part B, on `main` @ `6cf8002` (D208) |
| Area | `pkg/grind/opencl.go` (`loadOpenCL`, `openCLDevices`), `pkg/grind/opencl_unix.go` |
| Fixed by | |
| For Black | Not a rule. |

## What is wrong

**The switch.** `loadOpenCL` turns the GPU off only when `POON_OPENCL` is exactly `off` or `0`. `OFF`, `Off`, `false`, `no`, ` off` and the empty string leave it on, with no note. `POON_OPENCL_ALL` is on only for exactly `1`. A person who wrote `POON_OPENCL=false` to keep a driver away gets the driver, and the output does not say so.

**A crash is not an error.** The fallback of D208 covers error codes the driver returns. A driver that faults inside a C call (a segfault in the ICD loader, a vendor library, a GPU kernel compiler) is a signal in the Go process, and Go does not recover it: the node exits. The library is opened and the platforms listed the first time any command lists devices (`poon doctor`, `poon grind devices`, a node's first grind plan), so a driver that crashes at `clGetPlatformIDs` crashes every one of those, and the only way out is the switch above, set exactly. A hung call has the same shape: nothing cancels a C call, and `calibrate` runs inside `openCLKernel` with `clKernelsMu` held, so one hung kernel build blocks every later plan.

## How I checked it

`go build ./cmd/poon`, PoCL installed, `POON_OPENCL_ALL=1 poon grind devices` with each value (the devices listed):

```
off, 0:                         cpu
OFF, Off, false, no, " off", "": cpu, gpu:0
```

The crash and the hung call are from the code and from how Go treats signals in cgo-free C calls; I did not make a driver crash or hang.

## Suggested fix (Blue's call)

- Read the switch as a person writes it: trim and lower-case, and take `off`, `0`, `false`, `no` as off and `1`, `true`, `yes`, `on` as on; an unknown value is a warning on standard error ("POON_OPENCL=maybe is not off or on; taking it as on").
- Say in `poon grind devices` and `doctor` when the switch is set, either way.
- Probe the driver in a child process once (`poon grind probe`, a few seconds' limit), cache what it says, and only then load the library in the node. A crash or a hang then costs the probe, not the node.
- Do the calibration outside `clKernelsMu`, with a time limit.

## Not tested

A crashing or hung driver; macOS and Windows.
