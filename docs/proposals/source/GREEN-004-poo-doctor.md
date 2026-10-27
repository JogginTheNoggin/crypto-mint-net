# GREEN-004: `poo doctor`, one command to say what is wrong

| Field | Value |
| --- | --- |
| Status | adopted |
| Based on | `claude/m10-watch` @ `4692b4e` |
| Area | `pkg/cli` (new command), reads `pkg/nodepolicy` and `pkg/rpc` |
| Effort | medium |
| Decided by | Grey: merged as prototyped in 70718f2 |

## Summary

A read-only command that checks a node's setup and prints a pass, warn or
fail line for each check with the fix: the policy parses and validates, the
data directory and `rpc.token` have safe permissions, `rpc.addr` is not
stale, the node answers, the p2p port is reachable on loopback, and the
followed series are known. No prototype yet.

## Problem

When something is off, the errors arrive one at a time from different
commands. For example, `poo status` with no node running says
`rpc: is the node running? open .../rpc.addr: no such file or directory`.
A new operator, or a GUI first-run screen, needs one place that answers
"is my node set up right, and if not, what do I do?".

## Idea

`poo doctor [-dir|-root|-policy] [-json]` runs checks in order and stops a
branch of checks when an earlier one fails:

1. A current node policy exists, parses, and passes the same validation
   `poo node` runs.
2. The data directory and `rpc.token` are not readable by others.
3. `rpc.addr` exists only while a node is running; a leftover file from a
   crash is reported with how to clear it.
4. `GET /v1/status` answers with the token.
5. When p2p is on, the listen address accepts a TCP connection on
   loopback, and the node has at least one peer or a seed or registry to
   find one.
6. Each followed series is in the policy's series list.

`-json` returns the same checks for the GUI to render, so both use the
same code.

## Cost and risk

About 250 lines and a test per check against a temporary directory. It is
read-only and contacts only the local node. It never dials anyone else's
machine.

## Prototype

On `claude/source-green`, based on `claude/m10-watch` @ `454cd3d`:
`pkg/cli/doctor.go`, `pkg/cli/doctor_test.go`, the `doctor` case and usage line
in `pkg/cli/main.go`, and a row in `docs/modules/poo.md`.

Checks, in order: `policy` (loads and validates), `files` (data directory
and `rpc.token` not readable by others; skipped on Windows), `series`
(every listed series accepted), `rpc` (the node answers with the token; a
missing `rpc.addr` is a warning, a leftover one that nothing answers is a
failure), and `p2p` (the reported listen address accepts on loopback, and
the node has peers or a way to find them). A check is skipped when one it
depends on did not pass. `-json` prints a `DoctorReport` for the GUI. The
command exits 1 only when a check fails, so a stopped node is a warning.

It never writes, never takes the data directory's lock (so it cannot stop
a node from starting), and dials only the node's own addresses.
