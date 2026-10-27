# GREEN-006: Structured, levelled node logs with `log/slog`

| Field | Value |
| --- | --- |
| Status | deferred |
| Based on | `claude/m10-watch` @ `4692b4e` |
| Area | `pkg/daemon`, `pkg/p2p`, `pkg/node`, node policy `log` section |
| Effort | medium |
| Decided by | Grey: wanted, but it touches every log line; build after M10 merges, with Red reviewing the debug level |

## Summary

Replace the daemon's `fmt.Fprintf` log lines with the standard library's
`log/slog`, with a level and a text or JSON format chosen in the node
policy (`log.level`, `log.format`). Events would carry fields such as
pool, peer and reason, which log tools can filter. No prototype yet.

## Problem

The node writes free-form lines (`p2p: listening on ...`). There is no
way to turn detail up while debugging a peer or down on a quiet server,
and nothing machine-readable for log shipping. Peer offences and rejected
items are the events an operator most wants to see, and they are not
logged in a filterable way.

## Idea

- One `*slog.Logger` made in `pkg/daemon` from the node policy and passed
  to `node`, `p2p` and `rpc` the way the log writer is now.
- Default `info` level and text format, which read the same as today.
- `debug` adds per-message peer traffic, `warn` keeps offences, blocks
  and store repairs.
- `poo node -log-level debug` overrides the policy for one run.
- Consensus packages stay untouched: they return errors, and the callers
  log them.

## Cost and risk

Medium: many call sites, but mechanical. Logging a peer's message at debug
level must stay bounded (sizes and types, not payloads), or a peer could
fill the disk. Red should review the debug level once it is built. It adds
no dependency (`log/slog` is in the standard library).

## Prototype

None yet.
