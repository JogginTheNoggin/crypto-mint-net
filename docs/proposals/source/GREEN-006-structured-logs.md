# GREEN-006: Structured, levelled node logs with `log/slog`

| Field | Value |
| --- | --- |
| Status | deferred |
| Based on | `claude/m10-watch` @ `c71f774` (first proposed on `4692b4e`) |
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

On `claude/source-green`, based on `claude/m10-watch` @ `c71f774`, built
in Source Round 2 Part B:

- **`pkg/logging`** (new, standard library only). `New(w, level, format)`
  takes one of three formats. `plain` is the default: no time,
  `component: ` as a prefix, then the message and `key=value` fields.
  `text` and `json` are slog's own handlers. A value with a space, quote
  or newline is quoted, so one event is always one line.
- **`pkg/nodepolicy`.** An optional `log` section, `{level, format}`. It is
  a pointer, so saved policies without it do not change. `Validate`
  checks it.
- **`pkg/node`.** `Options.LogLevel`, `LogFormat` and `Logger`. The node
  builds the logger from its policy when it opens, so a reload picks up a
  new log section. `Node.Logger()` hands it on. The five node messages
  become warn or info events with fields.
- **`pkg/p2p`.** `Config.Logger`. The 11 `logf` calls become events with
  fields and levels: offences, invalid items, archived peers and registry
  failures are warn, and saving the peer book is error. One new debug
  event logs each received message, with its type and size only. It is
  skipped without formatting anything unless debug is on.
- **`pkg/daemon`.** Its own lines (started, listening, reloading, stopped)
  go through the node's logger, and `Options.LogLevel` and `LogFormat`
  pass the overrides on.
- **`poo node -log-level L -log-format F`**, checked before the node
  starts.
- **Docs.** `docs/modules/logging.md` (formats, the events at each level,
  and the rule "a peer's data only as a size or type"), the `log` section
  in `nodepolicy.md`, and the `poo node` row in `poo.md`.
- **Tests.** `pkg/logging` (the exact plain lines, levels, json and text,
  bad settings), `pkg/daemon/log_test.go` (plain by default, json from the
  policy with node, p2p and rpc components, the run's level overriding the
  policy's), and `nodepolicy.TestLogSection`.

What changes for a reader: default lines now read `p2p: connected
peer=… addr=… outbound=true` rather than `p2p: connected to … at …
(outbound true)`. Nothing in the repository or in the GUI branches parses
these lines. The GUI shows them as they are.

For Red: the debug level is the new surface. It logs only `type` and
`bytes` for each received message, so it costs one line per message, and
the bytes are capped by `wire.MaxFrame`. A node at debug under a message
flood writes a lot of log. That is why debug is opt-in and never the
default.
