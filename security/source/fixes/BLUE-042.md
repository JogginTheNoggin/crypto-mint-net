# BLUE-042: The GPU grinder's driver failure moves to every CPU core and stops quickly; OpenCL loaded safely on Windows; a reload keeps the node id

| Field | Value |
| --- | --- |
| Fixes | RED-124 (low), RED-125 (low, in part), RED-126 (low), RED-127 (low) |
| Branch | `claude/project-thread-ggo8dt` (PR against `main`) |
| Base | `main` @ `56c5e0a` (Source Round 41 Part C), with GREEN-066 (#372) merged in |
| Code | `4a30ad4` |
| Decisions | D209 |
| With | GREEN-066 (#372), carried: it changes the same `grind.go` and `opencl.go`. ORANGE-051 (#373) is docs only. |
| Review | No consensus change. For White: `POON_OPENCL_ALL` takes more on values; the node id now lasts the process (wire.md says so). No GPU was used; the OpenCL path was checked on PoCL. |

## Summary

- **RED-124.** `layer.SearchContext`; the failed GPU's ranges searched on
  every core with context checks; retried after a minute; logged and shown.
- **RED-125.** `POON_OPENCL_ALL` on values; driver crashes and hangs left
  for Grey.
- **RED-126.** `p2p.Config.ID`, one id per process across reloads.
- **RED-127.** `LoadLibraryEx` with System32 only.

## Consensus

None.

## Tests

- `pkg/grind/blue042_test.go`: `TestBlue042DriverFailureFallsBack`, `TestBlue042ParallelSearch`, `TestBlue042OpenCLAllValues`
- `pkg/daemon/blue042_test.go`: `TestBlue042ReloadKeepsNodeID` (fails on the code before the fix; the grind tests do not build against it)
- `POON_OPENCL_ALL=1 go test ./pkg/grind` on PoCL: every OpenCL test passes.
