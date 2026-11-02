# GUI-017: Schedule changes made without a node still lose each other, and can be written behind a running node

| Field | Value |
| --- | --- |
| Severity | low |
| Status | fixed by Source (D134 on unix, merged Round 12; D147 `LockFileEx` on Windows, merged Round 17); verified R12B on unix, R17B on Windows by code review and `GOOS=windows` build |
| Found on | claude/gui-4-watch @ 4108866 (Source claude/m10-watch @ 72d6f25) |
| Component | pkg/cli/grind.go (`grindSchedule`: the path without a node), pkg/nodepolicy (`Load`, `Save`, `Lock`) |
| Surface | node (node.json's grind section) |
| Fixed by | _(pending — Source, via GUI/Grey; the GUI needs no change)_ |

## Summary

D125 closed GUI-014 for a running node. `poo grind schedule add|remove|set`
now sends a `GrindEdit` to `POST /v1/grind/schedule/edit`, and the node
applies it under `policyMu`. That holds. Eight `add`s started at once
against a running node kept all eight, in 5 runs out of 5.

The path without a node is unchanged. When there is no node, `grindSchedule`
does `nodepolicy.Load(path)`, then `edit.Apply(cur)`, then
`nodepolicy.Save(path, p)`. That reads the **whole** node.json, changes it
in the process and writes the whole file back, with no lock. GUI-014's
recommendation said this path "needs the same read, check and write under a
file lock". `Save`'s temporary file and rename stop torn files (D114), but
they don't stop lost updates.

Two consequences:

1. **Lost changes with no node running.** The GUI's Work view edits
   node.json directly when the node is stopped, as do the terminal and
   scripts. The GUI's gate only orders one page. Two changes at once from
   two tabs, a tab and a terminal, or a script lose all but one, and every
   command reports success.
2. **Writing behind a running node.** The CLI takes the path without a node
   whenever `cl.GrindSchedule()` returns **any** error, not only when there
   is no node. A node that is busy, still starting, or replying 5xx gets its
   node.json rewritten from outside `policyMu`:
   - that write can undo a change the node is saving at the same moment
     (a schedule edit, or a follow);
   - the running grind manager is not told, so the node keeps running the
     old schedule until its next change or restart;
   - the command still says it succeeded.

`edit.Version` (`-if-version`) is checked in the path without a node too.
But it is checked against the copy read a moment earlier, inside the same
unlocked window, so it does not close the race.

## Reproduction

No node running, using `poo init -example` for the data directory:

```sh
poo grind schedule add -dir D pending
for h in 1 2 3 4 5 6 7 8; do poo grind schedule add -dir D -hours $h-$((h+1)) tip & done; wait
poo grind schedule -dir D        # expect 9 jobs
```

Across 20 runs, the schedule kept **3 to 6** of the 9 jobs. Every run lost
some, and all `add`s exited 0. With `poo node -dir D` running, the same
script kept 9 in every run.

## Impact

Wrong grind schedule with no error, as in GUI-014, but only when changes
overlap without a node, or when the node fails to answer the CLI's read.
Case 2 also leaves the running node out of step with its own file. No
attacker is needed. Rated **low**. This is node policy, not consensus.

## Recommendation

Source, so the GUI and headless are fixed together:

- For the path without a node, hold a lock across `Load`, `Apply` and
  `Save`. The data directory's `LOCK`, which `poo node` already holds through
  `nodepolicy.Lock(dir)`, fits:
  - Take it with `LOCK_NB`. If it is free, there is no node, so edit the file
    under it. Concurrent offline edits then queue, or retry briefly, rather
    than racing.
  - If it is held, a node is running. Do not write the file. Report the
    node's error instead, for example "the node is running but did not
    answer: <err>".

  That closes both consequences above. If `LOCK` must stay the node's
  alone, a separate `node.json.lock` taken by every writer of the file
  works for case 1. Case 2 then needs the "node is running" check another
  way.
- Only fall back to the file when the dial shows no node: connection refused,
  or no RPC address file. Don't fall back on any error from
  `GrindSchedule()`.
- Other commands that write node.json without a node, such as `follow`, have
  the same pattern (noted in GUI-013's R7B notes). One helper,
  `nodepolicy.Edit(dir, func(*Policy) error)`, could cover them all.
- Test: in Go, with no node, run 8 `grindSchedule add` calls in goroutines,
  then expect 8 more jobs. With the data directory locked, expect the
  path without a node to refuse instead of writing.

## Notes — Round 10B verification (held up)

- **GUI-014 core (D125), verified for a running node.**
  - `add`, `remove` and `set` are one `GrindEdit` that the node applies under
    `policyMu`.
  - A numbered removal carries the whole job (`SameJob`, JSON of every
    field), so the window between the CLI's read and its POST cannot
    remove another job, even with the same target.
  - `PUT` with `If-Match` refuses a stale version with 409 / `CodeConflict`.
  - `Version` hashes the whole section, including the defaults, so any change
    moves it.
  - A newer node with job fields an older CLI drops fails `SameJob`, which is
    safe: the removal is refused.
- **`-if-version` (GREEN-034), verified.** After an Add from elsewhere,
  `remove -if-version <old>` exits 1 with "the grind schedule changed
  meanwhile: list it again". The GUI's Remove passes the version its table
  was read at. GUI-016 covers how the GUI reports the refusal.
- **For Source, help text.** `remove -target` now says "a removal names the
  whole job anyway, so a stale number is refused". That holds only for the
  CLI's own read-then-POST window. A number a script took from an
  **earlier** listing is resolved against the current schedule. Checked
  with jobs [tip 22-06, tip], `remove 1` elsewhere, then `remove -target tip 1`:
  it exited 0 and removed the other `tip` job. Only `-if-version` refuses
  that. The help should point scripts at `-if-version`.
- **GUI-015, verified.**
  - Core: `MaxLayerRate` is 50, half the default peer budget of about 100 a
    second. `set -layer-rate 1000` exits 1 with "layer_rate must be 0 (the
    default, 10) to 50".
  - Old files: a node.json with `layer_rate` 1000 still opens. `Load`
    clamps it to 50 (D128), and `poo doctor` warns "grind.layer_rate 1000
    is over the most, 50: 50 is used". The schedule then shows 50, the rate
    actually used.
  - GUI: the Layer rate field explains peer time-outs and blocks, and its
    placeholder shows the range from the reply's `limits`, so it cannot
    drift from the core. A refused value shows poo's error with the range,
    and the schedule keeps its rate.
- **GREEN-033 defaults as placeholders, safe.** The form sets only
  `placeholder` (`hint`), so an empty field still sends nothing. A stale
  "now …" hint can't write an old value over a newer one.
- **`limits` and `version` in the reply, safe.** They are drawn with
  `textContent` in placeholders and passed as one argv element
  (`flag('if-version', …)`), not through a shell line.
## Notes — Round 12B verification (held up on unix)

- **D134, verified.**
  - With no node, `grind schedule add|remove|set` takes the data
    directory's `LOCK` without waiting, before `Load`, and holds it through
    `Save`.
  - When the lock is held, it refuses with "a node or another command holds
    …, so nothing was changed". If the node answered with an error, it
    instead refuses with "the node is running but … could not be read".
  - The R10B race was re-run 20 times: no change was lost. Every add
    either landed or exited non-zero. Runs kept 3 to 5 of 9 jobs, each
    matching 1 plus the number of adds that succeeded.
  - `follow`, `wallet follow` and `init -dir` use the same lock (D136).
    `wallet follow` changes a running node in two steps, which is GUI-018.
  - A node hosted in poo-gui takes the lock on its own file descriptor, so
    the GUI's offline edits are refused while it runs, as they are for an
    outside node.
- **Residual, Windows.** `nodepolicy.Lock` is a no-op off unix
  (`lock_other.go`). There, offline edits still lose each other, and a
  change can still be written behind a running node, exactly as before.
  poo-gui supports Windows (its terminal has a Windows grammar). A
  `LockFileEx` lock on the same file would close it. This stays Low.
- **Wording, optional.** When another *command* holds the lock, the
  wrapped error still ends "data directory in use by another node".
- **Health check `fix_args` (GREEN-036), safe.**
  - The GUI no longer adds Settings flags. It runs `fix_args` as doctor
    printed it, and doctor appends `-dir <checked dir>` when `-root`,
    `-policy` or `-dir` chose the policy.
  - Every `fix_args` is built from constants (`init`, `node`,
    `grind schedule set -layer-rate 50`) plus the resolved directory. None
    of it comes from node.json values or peers.
  - The line goes through `api.quote`, then the terminal's own split, as
    each argument. A directory with spaces or quotes round-trips under the
    POSIX quoting (single quotes).
- **GREEN-036 Getting started links, safe.** The hrefs are fixed `#/node`,
  `#/watch`, `#/work` and `#/wallet`.
- **GUI-016 wording (Blue #51), safe.** A full stop is added, and "current"
  is claimed only when `reload()` returned exit 0.

## Notes — Round 15B

- The Windows residual is unchanged: `pkg/nodepolicy/lock_other.go` is
  still a no-op. The wallet now locks on Windows with `LockFileEx`
  (`pkg/wallet/lock_windows.go`, D145). The same code on the data
  directory's `LOCK` would close this residual.

## Notes — Round 17B

- **The Windows residual is closed (D147).** `pkg/nodepolicy/lock_windows.go`
  takes `LockFileEx(LOCKFILE_EXCLUSIVE_LOCK|LOCKFILE_FAIL_IMMEDIATELY)` on
  the data directory's `LOCK`. A lock that is already held maps to
  `ErrLocked`, so the refusal is immediate, as on unix.
  `lock_other.go` is now only for platforms that are neither unix nor
  Windows. Checked by reading the code and by a `GOOS=windows` build of
  `./pkg/...`. It has not been run on Windows.
