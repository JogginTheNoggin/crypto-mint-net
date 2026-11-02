# GUI-008: Background Watch holds its event stream and node connection until Stop

| Field | Value |
| --- | --- |
| Severity | info |
| Status | fixed (verified by GUI/Red R4B @ 50a0edd; resume uses the chunk event's next with ORANGE-002) |
| Found on | claude/gui-4-watch @ 9b4b19c |
| Component | gui/web/static/watch.js (`watchView.mount`, the singleton `watchPanel`) |
| Surface | local (browser tab + one node RPC connection) |
| Fixed by | `claude/gui-blue` |

## Summary

Round 3B made the Watch panel a module-level singleton that is built once and
kept, so a running `poo watch` and its table survive switching views
(GREEN-015). Leaving the view only detaches the panel from the DOM
(`stop: () => watchPanel.remove()`); it does **not** call `unfollow` or cancel
the job. That is the intended behaviour ("The watch keeps running when you switch
views; Stop ends it"), and the Watch tab shows a busy dot while it runs.

The observation for the record: a watch the user forgets to Stop keeps, for the
rest of the session, one `EventSource` against `/api/jobs/{id}/events` (one of the
`MaxStreams = 32` slots), one running job (one of the `MaxRunning = 16` slots), and
the node RPC connection that `poo watch` holds — with events still being parsed and
appended to a detached, invisible table in the background. Memory is bounded (the
GUI-007 caps: 1000 rows, 64 KiB errors, 1 MiB line), so this is not a leak that
grows without limit; it is a liveness/resource-lifetime point, not a crash.

## Reproduction

1. Open Watch, Start watching a busy node, then switch to another view without
   pressing Stop.
2. The job and its event stream keep running (visible as the dot on the Watch tab
   and as a running `poo watch` in the Jobs view); events are still parsed into the
   detached panel.
3. It is released only by returning to Watch and pressing Stop, or by ending the
   session.

## Impact

Negligible and local. Because the panel is a singleton there is only ever one
background watch, so it cannot accumulate toward the `MaxStreams`/`MaxRunning`
caps the way repeated starts could — it holds exactly one stream slot, one job
slot and one node connection until Stop. No remote or cross-origin vector (the
stream is the user's own node over the loopback API; cross-origin pages cannot
load the authenticated page under `SameSite=Strict`). Filed **info** so the
trade-off is recorded, not because a fix is required.

## Recommendation

Optional, a design call for GUI/Grey:

- Keep the current behaviour but make the cost visible beyond the tab dot (e.g. a
  line on other views, or reuse the no-node/`node-banner` slot to say "a watch is
  running in the background").
- Or, while the panel is detached, close the `EventSource` and remember the last
  `seq`, then re-attach with `?from=<seq>` when the view is reopened — the job
  keeps running server-side (so no events are missed, within the server's retained
  output) but the browser stream slot is freed while the view is away.
- Or an idle timeout that Stops a background watch after a long detached period.

## Notes — Round 3B verification (no new vulnerability)

This round re-attacked the Round 2 fixes and reviewed the new surface; all held:

- **GUI-006 (fixed, verified).** Terminal history now drops any line that starts
  with whitespace or that `api.secretLine` flags as carrying a secret flag
  (`restore`, `seed`), old saved lines are migrated out on load, and a "Forget
  history" button clears storage. The Jobs list hides secret values server-side via
  `shell.Redact` and suppresses "Run again in terminal" for a redacted line. Evasions
  of the client-side `secretLine` heuristic that I could construct (e.g.
  `-restore"SEED"`) are parsed by poo's own flag package as an unknown flag, so they
  do not deliver a seed; the residual mismatch (client flags `sim -seed` while
  `Redact` exempts `sim`) is harmless over-caution. No way found to persist a usable
  seed to storage.
- **GUI-007 (fixed, verified).** `held` (paused), the error box and the unfinished
  line are all bounded (`MAX_EVENTS`, `MAX_ERR`, `MAX_LINE`), with drop notices and
  an e2e (`gui/e2e/bounds.mjs`).
- **Explore links (new) — safe.** `render.lookupLink` only builds a link when the
  value is `isHex` (`[0-9a-f]+`, ≥24), so the `#/explore?tid=/address=<v>` href cannot
  carry a scheme or extra query parameters. The consumed param flows only into a
  positional argument of a read command (`tx`/`address`), never a shell. Cross-origin
  pages cannot reach it (the port is loopback and ephemeral, and `SameSite=Strict`
  withholds the session cookie on cross-site navigation, so the page returns 401).
- **Pools on `tx`/`unspent`, the no-node banner — safe.** All command output is
  rendered through `textContent`/`render`, so node- and peer-controlled fields cannot
  inject HTML; the banner is a display-only collapse keyed on an exact stderr phrase
  with empty stdout.

## Fix (GUI/Blue, branch `claude/gui-blue`)

Took the second option. Leaving the Watch view now calls the panel's `detach`, which closes the `EventSource`. The `poo watch` job keeps running on the server, and the page remembers `next`, the job's next output sequence number (the server's chunk `seq`, not the event's own `seq`). Coming back calls `attach`, which follows again with `?from=next`, so nothing is shown twice and nothing is missed. The one exception is output the server dropped past its 4 MiB per-job buffer: the existing `dropped` event resets the partial line and says so in the status line and the counts. While away, the browser holds no stream slot and parses nothing. The job slot and the node connection are still held until Stop, as designed, and the tab dot still shows. If the job ends while away, the exit arrives on return.

Resume uses only the server's existing `from` parameter and the `chunk.seq` and `dropped.next` it already sends, so the scheme stays compatible with any stream-resume id work (GUI/Orange) that keeps that contract.

Regression test, in gui/e2e/watch.mjs: against a real node, it watches, switches to Jobs, and asserts the watch job's `EventSource` is closed while the tab dot stays. It then claims a mint while away, returns, and asserts the new transaction row appears exactly once over a single new stream with `from > 0`. The test times out against the unfixed watch.js.
