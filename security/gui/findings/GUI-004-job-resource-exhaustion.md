# GUI-004 — No bound on running jobs or event streams (local DoS)

- **ID:** GUI-004
- **Severity:** Low (requires the secret; local only)
- **Status:** open
- **Fix branch:** _(pending — GUI/Blue)_
- **Component:** `gui/jobs/jobs.go` (`Start`, `trim`), `gui/server/server.go` (`events`)

## What

`Runner.trim` caps *kept* jobs at `DefaultMaxJobs = 64` but explicitly never
drops a **running** job ("Running jobs are never dropped"). There is no cap on
how many jobs may be running at once and no cap on concurrent SSE event
streams. A caller holding the secret can therefore:

- `POST /api/jobs` repeatedly with a long-running command (e.g. a node that
  stays up) → unbounded concurrent `cli.Main` goroutines, each with its input
  goroutine and buffered output (up to `DefaultMaxOutput = 4 MiB` each), plus
  whatever FDs/stores each poo command opens;
- open many `GET /api/jobs/{id}/events` streams → one goroutine + flusher each,
  held open with a 15s heartbeat.

Memory is 4 MiB × (running + recently-finished-but-kept) jobs for output
buffers alone, and goroutine/FD growth is bounded only by what the OS allows
before the node process is killed.

## Why it matters

Low severity because it needs the launch secret, so the attacker is already a
local principal who (per the threat model) can run poo directly. The value is
mostly robustness: a buggy page, a runaway script driving the API, or a
curiosity-driven loop can wedge the node host. It is also a multiplier for
GUI-001 — if the secret leaks, a remote-ish (same-host) attacker can cheaply
knock the node over rather than merely read it.

## Reproduce

- Loop `POST /api/jobs {"args":["node", ...]}` (a command that blocks) a few
  hundred times; watch goroutine/FD/memory of the poo-gui process climb with
  no ceiling.

## Suggested fix (for GUI/Blue — do not implement here)

- Add a configurable cap on **concurrently running** jobs; reject `Start` with
  503 once reached (the API already surfaces `ServiceUnavailable` from
  `Start`).
- Cap concurrent event streams per process (and optionally per job), closing
  the oldest or refusing new ones past the limit.
- Consider a global output-memory budget across jobs, not just the per-job
  4 MiB cap.

## Notes / what is already right

Per-job output is already bounded (`DefaultMaxOutput`, oldest chunks dropped),
request bodies are capped (`MaxBytesReader`, `MaxBody = 1 MiB`), and
`ReadHeaderTimeout`/`IdleTimeout` are set. Keep these.
