# RED-127: On Windows the OpenCL library is loaded by bare name, so a DLL next to the program can be found first

| Field | Value |
| --- | --- |
| Severity | Low (it needs a file written next to `poon.exe` or in the working directory; that person could replace `poon.exe` too, except where the program is in a shared or download folder) |
| Status | open |
| Found | Source Round 41 Part B, on `main` @ `6cf8002` (D208, `pkg/grind/opencl_windows.go`) |
| Area | `pkg/grind/opencl_windows.go` |
| Fixed by | |
| For Black | Not a rule. |

## What is wrong

`openCLLibrary` on Windows calls `syscall.LoadLibrary("OpenCL.dll")`. That is the plain `LoadLibrary`, with Windows' default search order: the program's own folder comes first, then the system folders. The ICD loader that GPU drivers install is in `System32`, so no search is needed, but the search is what runs. A file named `OpenCL.dll` beside `poon.exe` (a download folder where the zip was unpacked and a second download dropped) is loaded and run, in `poon doctor`, `poon grind devices` and a node's first grind plan. This is the usual DLL-planting shape; the `syscall` documentation for `LoadDLL` warns of the same search and points to `golang.org/x/sys/windows` for system DLLs (I did not check that `LoadLibrary` is documented the same way).

On Linux the loader is opened by name as well (`libOpenCL.so.1`) with `dlopen`'s default paths, which honour `LD_LIBRARY_PATH`; that one is the system's normal behaviour and the person's own environment.

## How I checked it

From the code only. I have no Windows machine here and did not run it.

## Suggested fix (Blue's call)

Load it from `System32` only: `LoadLibraryEx` with `LOAD_LIBRARY_SEARCH_SYSTEM32` (or `golang.org/x/sys/windows.LoadLibraryEx` / `NewLazySystemDLL("OpenCL.dll")`), and pass that handle to purego. If the DLL is not there, say so as now.

## Not tested

Windows.
