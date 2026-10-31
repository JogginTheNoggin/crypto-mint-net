# GUI-011: poo-gui now stops on SIGHUP even under nohup, and SIGHUP no longer reloads a node it hosts

| Field | Value |
| --- | --- |
| Severity | low |
| Status | fixed (PR #33, merged Round 7; pending Red's verification) |
| Found on | claude/gui-4-watch @ e7df74b |
| Component | gui/cmd/poo-gui/main.go (`stopSignals`), pkg/cli/node.go (`runNode`'s SIGHUP reload) |
| Surface | local (availability of a node hosted in poo-gui) |
| Fixed by | GUI/Blue on claude/gui-blue (Round 6C) |

## Summary

The GUI-010 fix added `syscall.SIGHUP` to poo-gui's `signal.NotifyContext`, so
closing the terminal runs the cancel path. Two side effects come from how Go
handles signals.

1. **nohup and other ignored SIGHUP.** The `os/signal` docs say that if a
   program starts with SIGHUP ignored and then calls `Notify` for it, Go
   installs a handler and the signal "will no longer be ignored". So
   `nohup poo-gui -no-open &`, or a service manager that ignores SIGHUP, used
   to survive the session ending. Now poo-gui stops when the session ends.
   That includes a `poo node` it is hosting from the Node view, since that node
   runs in the GUI process as a job. I checked this with a small program run
   under `nohup`: `signal.Ignored(SIGHUP)` was `true`, and after
   `NotifyContext(..., SIGHUP)` a SIGHUP cancelled the context.
2. **SIGHUP means reload for `poo node`.** `runNode` (pkg/cli/node.go)
   registers its own `signal.Notify(hup, SIGHUP)` and reloads the node policy
   on it. docs/modules/poo.md, running-a-node.md and D-notes describe
   "SIGHUP to `poo node`" as the way to reload after a hand edit. Before this
   change, a GUI-hosted node got that same reload, and poo-gui kept running
   because the node's handler had replaced Go's default exit. Now both
   handlers fire: the node starts a reload, and poo-gui cancels every job and
   exits. An operator who sends the documented reload signal stops the node
   and the GUI.

## Reproduction

1. `nohup poo-gui -no-open > gui.log 2>&1 &`, sign in, and start a node from
   the Node view.
2. Log out or close the SSH session. Before e7df74b the process stays up.
   Now it exits, and the node with it. The same happens with
   `kill -HUP <pid>` after a manual edit of node.json.

## Impact

This is an availability regression. It needs no attacker, but it stops a node
that the user deliberately detached or asked only to reload. Nothing leaks.
Rated **low**.

It also matters for the core item that GUI-010 left to Source. If SIGHUP is
added to `cmd/poo`'s stop signals in the same way, `poo node`'s documented
SIGHUP reload becomes a shutdown in the headless node too.

## Recommendation

- In poo-gui, add SIGHUP to the stop signals only when it was **not** ignored
  at start: `if !signal.Ignored(syscall.SIGHUP) { … }`. nohup and service
  setups then keep their meaning.
- Decide what SIGHUP means for a poo-gui that hosts a node, and write it in
  docs/gui.md next to the existing sentence. The narrowest option: SIGHUP
  stops poo-gui only when it has a controlling terminal, for example when
  stdin is a terminal, which is the case the fix was for. Otherwise SIGHUP is
  left to the hosted node's reload, as before. A GUI-hosted node's reload
  stays available through `poo reload` and the Node view's Reload either way.
- For Source (via GUI/Grey): do not put SIGHUP in `cmd/poo`'s global stop
  list. `poo node` uses it to reload. If the headless `poo grind run` needs it,
  add it in that command only, or rely on the lease from GUI-010.
- Test: start the test binary with SIGHUP ignored (`exec.Cmd` with
  `SysProcAttr`, or run `signal.Ignore` first in the child) and assert that
  SIGHUP leaves it running. With a node job running, assert that SIGHUP
  reloads it rather than stopping it, if that is the behaviour chosen.

## Notes — Round 6B verification (held up)

- **GUI-010 re-adopt on reload, verified.** `liveJob()` finds an unfinished
  `grind run` in the GUI's own job list and follows it from 0, with Stop on and
  Run off. There is one cosmetic race. If Run is pressed during the moment
  between reload and `liveJob` resolving, the old job can be adopted while
  `api.start` is still pending. The panel then follows two streams, because
  `follow()` overwrites `unfollow`. This needs a click inside a sub-second
  window and shows only mixed rows. It is not a security issue, so it is
  recorded here only.
- **GUI-010 SIGHUP, verified.** It does what the fix says for an
  interactive terminal (`TestHangupStopsCleanly`). The side effects are above.
- **Green's jobs table, safe.** Every node field goes through `el`
  (`textContent`). The `pill-*` class is chosen from fixed strings. Each Stop
  runs `grind stop ID` as a single argv word through `api.run`. View-only
  refuses it in the core.
- **Device suggestions, safe.** The `<datalist>` options are built with `el`
  from `poo grind devices` output, and the input still takes any text, which
  poo validates.
- **H/s rates, safe.** `render`'s `rate` key formats numbers only.
- **Orange's paced Work drawing (ORANGE-009) and error box, safe and
  bounded.** `paced()` draws at most every 100 ms. `newFound` is capped at
  2×`MAX_EVENTS`, the rate line keeps 120 points, and the error box keeps the
  last 64 KiB for both stderr and non-JSON stdout lines (this was unbounded
  for stdout before). `draw.now()` on exit flushes what is waiting.

## Fix (GUI/Blue, Round 6C)

- **What SIGHUP means is now decided by `hangup(ignored, tty)`** in
  `gui/cmd/poo-gui/main.go`. It replaces the fixed `stopSignals` list:
  - Interrupt and SIGTERM always stop poo-gui through its cancel path.
  - **Started from a terminal** (stdin is a terminal) and SIGHUP not ignored:
    SIGHUP stops poo-gui, which is the GUI-010 case.
  - **SIGHUP ignored at start** (`signal.Ignored`, which is what nohup does):
    poo-gui never calls `Notify` for SIGHUP, so the signal stays ignored and
    poo-gui outlives the session. A node it hosts still calls `Notify` for
    its reload, as it did before GUI-010.
  - **No terminal and SIGHUP not ignored** (a service manager, for example):
    poo-gui catches SIGHUP into a channel no one reads, so neither its cancel
    path nor Go's default exit runs. A hosted node gets the same signal on
    its own channel and reloads its node policy.
- **Terminal check.** `isTerminal` uses the standard library only, since the
  GUI takes no outside dependencies (imports_test.go). It runs the `TCGETS`
  ioctl on Linux (tty_linux.go) and `TIOCGETA` on darwin and the BSDs
  (tty_bsd.go). Elsewhere it checks for a character device (tty_other.go),
  which only matters where SIGHUP is never sent, such as Windows. `go vet`
  passes for linux, darwin, freebsd, openbsd and windows.
- **Double-follow race (from this finding's notes).** The Run a job panel
  keeps Run off until the job list has answered. A Run pressed in that
  moment can no longer start a job while the old one is being taken up.
- **Docs.** docs/gui.md has a new Signals section with the table above.
- **Tests.**
  - `signal_test.go` pins the four cases in `TestHangupTable`, and checks
    `isTerminal` on a pipe and on a pty.
  - It runs poo-gui's `main` in a child process: on a pty, SIGHUP gives exit
    0 (`TestHangupStopsCleanly`). With SIGHUP ignored through `sh -c 'trap ""
    HUP; exec …'`, on a pty and on a pipe, SIGHUP leaves it running and
    SIGTERM then gives exit 0 (`TestHangupNohup`). On a pipe, SIGHUP leaves
    it running (`TestHangupNoTerminal`).
  - With the GUI-010 list put back, the table, nohup and no-terminal tests
    all fail.
  - `e2e/work.mjs` sends SIGHUP to a poo-gui hosting a node (stdin is not a
    terminal). It checks that the node logs "reloading its node policy",
    still shows as running, and that the GUI keeps answering.
- **For Source.** Do not put SIGHUP in `cmd/poo`'s global stop signals. The
  coordinator reports that Source already handles it this way
  (ReloadsOnHangup, with nohup respected).
