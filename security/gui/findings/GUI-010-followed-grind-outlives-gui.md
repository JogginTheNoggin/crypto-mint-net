# GUI-010: A followed grind keeps running in the node after poo-gui dies

| Field | Value |
| --- | --- |
| Severity | low |
| Status | open |
| Found on | claude/gui-4-watch @ 8b044ac |
| Component | gui/web/static/work.js ("Run a job"), pkg/cli/grind.go (`grindStart`, follow mode), gui/cmd/poo-gui/main.go and cmd/poo/main.go (signals), pkg/grind/jobs.go (`Manager.Start`) |
| Surface | local (host CPU and power), node |
| Fixed by | _(pending — GUI/Blue, with a core part for Source)_ |

## Summary

The Work view's **Run a job** uses `poo grind run`. That command starts a job in
the node (`GrindStart`) and follows its events. When the GUI job is cancelled,
for example with Stop or Ctrl-C, `grindStart` sees `ctx.Err()` and calls
`GrindStop(id)`, so the node's job ends too. The view promises this: "It keeps
running when you switch views; Stop ends it."

That clean-up happens only when the client process is still alive to send
`GrindStop`. The node job has no tie to the client that started it.
`grind.Manager.Start` takes a `Spec` only. Closing the watch stream that the
follower holds does not stop the job, and a job started without `-for` runs
until someone stops it.

So the job is left running, using every core but one by default, whenever
poo-gui (or a headless `poo grind run`) ends without running its cancel path:

- **The terminal is closed (SIGHUP).** poo-gui and poo only use
  `signal.NotifyContext(..., os.Interrupt, syscall.SIGTERM)`. Go's default
  action for SIGHUP is to exit at once, so `Runner.Close` and the `GrindStop`
  call never run. Starting poo-gui from a terminal and later closing that
  window is the common case.
- **SIGKILL, a crash, an OOM kill, or a laptop that sleeps and loses the
  process.**
- **`Runner.Close` times out** before the job's `GrindStop` round trip
  finishes. It returns after its timeout and `main` calls `os.Exit`.

## Reproduction

1. Start a node, then start poo-gui from a terminal.
2. In Work → Run a job, choose target `tip` and leave **For** empty. Press Run
   and follow. The rate chart starts.
3. Close the terminal window. poo-gui exits on SIGHUP.
4. Run `poo grind jobs`, or check `top`. The job is still `running` and the
   node is still grinding on all cores but one. It only stops after
   `poo grind stop ID`, a node restart, or a reload.

The same happens with `poo grind run tip` in a shell whose window is closed.

## Impact

Unattended full-CPU use, along with the heat, battery and power cost, that the
user believes they stopped or that they never knew they left running. Nothing
leaks, and it is the user's own node. The job is visible in `poo grind jobs`
and in the Work view's Jobs panel. Rated **low**, a resource and lifetime
issue: it breaks the "Stop ends it" contract in the cases a user is most likely
to hit, and nothing ends it by itself.

There is a small browser-side variant. A reload of the page drops the live
panel's `job` handle while the poo-gui job keeps running. The panel then shows
"Not running." with Run enabled. Pressing Run again starts a second grind next
to the first. The first can be stopped only from Jobs.

## Recommendation

Core (Source), so headless and GUI behave the same:

- Give a followed job a **lease**. `poo grind run` asks for a job that stops
  when its follower is gone. Either the node ties the job to the event stream
  that started it, or the client renews the job (for example each progress
  tick) and the node stops it after a few missed renewals. `poo grind start`,
  the background kind, keeps today's behaviour.
- Also handle **SIGHUP** like SIGTERM in `cmd/poo` and `gui/cmd/poo-gui`, so
  closing the terminal runs the normal cancel path.

GUI:

- On mount, the live panel should re-adopt a running `poo grind run` job it
  finds in the job list, the same way `nodeJob()` finds `poo node`, so a reload
  shows it and its Stop button again.

Test: start `grind run` through the API, close the follower's connection
without cancelling (or kill the client), then assert that the node's job ends
within the lease.

## Notes — Round 5B verification (held up)

- **GUI-009 fix (`cli.SecretOutput`, D110), verified.** The core list
  (`keygen`, `wallet seed`) matches the only two commands that print a seed. A
  grep of `pkg/cli` finds no other seed or private-key output. `SecretOutput`
  ignores dash words, so `-view-only keygen` and flags in any order are still
  caught. A subcommand is always `args[0]` after the command, so no flag value
  can push `seed` out of place.
- In the runner, the output is given once to the first reader that reads to
  the end: an `events` read with `from <= next` once the job is done, or
  `Outputs()` for `POST /api/run`. After that it is dropped (`withhold`), and
  is dropped after `SecretKeep` (1 minute) if no one reads it. `info()` reads
  from `1<<62`, so listing jobs never uses up or reveals it. A later `?from=0`
  or `Last-Event-ID` gets `dropped {withheld: true}` and no chunk. The old
  `st.Chunks[0]` index on `dropped` (which would panic on an empty slice) is
  now guarded. No route other than `events` and `run` returns output.
- **GUI-009 UI, verified.** Jobs shows "output not kept" in place of Output.
  The result box hides Copy output when the server marks `secretOutput`.
  `render` shows any `seed` field masked behind Show, with no click-to-copy.
  `wallet seed`'s plain hex goes through `secretValue`.
- **Work view, safe.** Every field is passed as its own argv word through
  `flag()`. Plans and found rows are built with `el`/`render` and
  `textContent`. The rate chart's SVG uses only `setAttribute` with numbers.
  The event list and line buffer are bounded (500 rows, 120 points, 1 MiB
  line, 64 KiB errors).
- **Stop -all.** It needs no confirmation beyond the form's danger style. It
  maps to `StopAll("")`, which also stops node.json's scheduled jobs until the
  next reload or restart. The form's "Every job" does not say so, which is a
  Green-type wording point rather than a vulnerability. View-only refuses
  `grind start`, `run` and `stop`. It allows `devices`, `jobs` and `bench`.
  `bench` grinds locally for up to 60 s per hash and device, with threads
  capped at `MaxThreads`. That is CPU use by a view-only user, but no more
  than `sim` already allows.
- **Health check (`poo doctor`), safe.** Every check is drawn with
  `textContent`. "Run in terminal" appears only for `fix_args` or a fix that
  matches a strict `poo word…` pattern. The button shows the exact line before
  it runs it, and the core writes these fixes (`init`, `node`), not peers.
- **ORANGE-006/007/008, safe.** Watch draw-rate limiting, the batched
  scrollback queue (bounded at `MAX_CHARS` even when the tab's timers are slow)
  and the last-1 MiB long-text result are all display-only, and still use
  `textContent` or `appendData`.
