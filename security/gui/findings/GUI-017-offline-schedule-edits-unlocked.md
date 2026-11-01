# GUI-017: Schedule changes made without a node still lose each other, and can be written behind a running node

| Field | Value |
| --- | --- |
| Severity | low |
| Status | fixed by Source (D134: offline edits take the node.json lock), merged Round 12; pending Red's verification |
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
