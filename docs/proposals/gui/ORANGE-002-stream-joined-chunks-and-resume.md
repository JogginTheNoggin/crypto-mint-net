# ORANGE-002: One event stream message per batch of writes, and resume without a replay

| Field | Value |
| --- | --- |
| Status | adopted; chunk events also carry next, for ?from= resumes (GUI-008) |
| Based on | claude/gui-4-watch @ 9b4b19c |
| Area | gui/server (GET /api/jobs/{id}/events) |
| Effort | small |
| Decided by | |

## Summary

The event stream sent one server-sent event for every write a command
made. `poo watch` makes one write per event, so the browser parsed and
dispatched one message per watch event. The server now joins adjacent
writes of the same stream into one `chunk` event, up to 64 KiB. Each
chunk event also carries an SSE `id`, which is the seq to read next.
When a browser loses the stream, it reconnects on its own with
`Last-Event-ID`, and the server continues from there. Before, a
reconnect replayed the job's whole output from `from=0`, so every line
showed twice. The text is byte for byte the same. Only the framing
changes.

## Problem

- **Per-write framing.** A 120-byte watch line cost 215 bytes on the
  wire (JSON in JSON, plus the event header). It also cost one
  `EventSource` message, two `JSON.parse` calls and one handler call in
  the page.
- **Replay on reconnect.** `EventSource` reconnects by itself after a
  dropped connection. The server sent no `id`, so the browser asked for
  the same URL (`from=0`) and got the job's kept output again (up to
  4 MiB). Watch, the terminal and every running panel then showed
  everything twice. This is a correctness bug as well as a cost.

## Idea

- In `events`, `coalesce(st.Chunks)` groups adjacent chunks of one
  stream, up to `MaxEventData` (64 KiB). Each group is sent as one
  `chunk` event whose `seq` is the first chunk's. A group is only made
  of chunks that were already waiting when the server read the job.
  Output that arrives slowly still goes out at once, one event per write
  as before.
- Each chunk event is preceded by `id: <last seq + 1>`. A request that
  carries `Last-Event-ID` starts there when it is later than `from`.
- No client change is needed. `api.follow` uses `c.data` and
  `c.stream`, never `seq`, and `EventSource` handles the id itself.

## Measurements

`go test -bench Stream ./server`: a job printing 10,000 watch-sized
lines as fast as it can, read through the real handler over loopback.

| | before | after |
| --- | --- | --- |
| SSE events per line | 1.000 | **0.0025** (about 400 lines an event) |
| bytes on the wire per line | 214.9 | **157.2** |
| server time per line | 2998–3529 ns | **1437–1539 ns** |

In the browser at full rate (ORANGE-001's table, after both changes),
Watch took 20,000 events in 1.0 s instead of 3.8 s. At a steady 200
events a second there is about one write between reads, so the framing
barely changes. That case is fixed by ORANGE-001.

The job runner itself is not a bottleneck. `go test -bench . ./jobs`
gives 370–410 ns a write with no follower and 575–655 ns with 4
followers. Starting a job while 64 finished jobs are kept takes 11–13 µs.

## Cost and risk

- The SSE contract changes slightly: "a chunk is one write" becomes "a
  chunk is one or more writes of one stream". The doc comment on
  `events` says so. Nothing in the page depends on the old meaning.
- `TestStreamJoinsAndResumes` covers the joining, the ids and a
  reconnect that sends `Last-Event-ID`. `TestJobStreamAndInput` passes
  unchanged.

## Prototype

Branch `claude/project-thread-f0df7u`: `gui/server/server.go`, the new test in
`server_test.go`, and the benchmarks in `gui/server/bench_test.go` and
`gui/jobs/bench_test.go`.
