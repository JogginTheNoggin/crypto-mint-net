# pkg/logging

## Summary

Makes the node's logger: the standard library's `log/slog` with a level
and a format from the node policy's `log` section, or from
`poo node -log-level` and `-log-format` for one run (GREEN-006). `pkg/node`
builds the logger when it opens, `pkg/p2p`, `pkg/rpc` (through the daemon)
and `pkg/daemon` log through `node.Logger()`, each under its own
`component`. Consensus packages never log.

## Formats

| Format | Looks like | For |
| --- | --- | --- |
| `plain` (default) | `p2p: connected peer=4f1c… addr=203.0.113.5:8646 outbound=true` | a terminal, the GUI's node output |
| `text` | `time=… level=INFO msg=connected component=p2p peer=…` | log files |
| `json` | `{"time":…,"level":"INFO","msg":"connected","component":"p2p",…}` | log collectors |

`plain` has no time and no level for info lines, and it prefixes
`warning: ` or `error: ` for the others. Values with spaces, quotes or
newlines are quoted, so one event is always one line, even when a value
came from a peer.

## Levels and events

| Level | Events |
| --- | --- |
| `debug` | every message received from a peer: its type and size only, never its payload |
| `info` | start and stop, listening addresses, connections and disconnections, dial failures, chases, category changes, store compaction |
| `warn` | peer offences and invalid items, archived peers, failing registries, series or follow rules not accepted, an unfinished write dropped |
| `error` | the peer book could not be saved |

## API

- `New(w, level, format) (*slog.Logger, error)`: an empty level or format
  means the default; a nil `w` discards.
- `Check(level, format) error` and `ParseLevel(s)`: used by
  `nodepolicy.Validate` and `poo node`.
- `Component` is the attribute key that `plain` prints as a prefix.
- `node.Options{Log, LogLevel, LogFormat, Logger}` and
  `p2p.Config{Log, Logger}`: the older `Log io.Writer` alone still works
  and gives the `plain` format at `info`.

## Extending

Log with fields, not with formatted strings: `h.log.Info("chase",
"pool", k.String(), "peer", p.short(), "result", out)`. Keep messages short
and stable, since people filter on them. Anything a peer sends goes in at
`debug` level as a size or a type, never as a payload, so a peer cannot
fill the disk or forge lines.
