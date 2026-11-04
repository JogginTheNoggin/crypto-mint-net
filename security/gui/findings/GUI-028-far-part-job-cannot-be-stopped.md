# GUI-028: A far part number starts a job that Stop can't end, and 16 of them leave the GUI unable to run anything

| Field | Value |
| --- | --- |
| Severity | low |
| Status | open |
| Found on | main @ b9fed52 (PR #182, Series and keys → "Release a part", Show parts, Check a release; D165, D168) |
| Component | gui/web/static/views/crypto.js (`releaseForm`: Preview runs `poo series parts FILE -from K -to K` for any K); gui/jobs/jobs.go (Stop is `Job.Cancel`, a context cancel; a cancelled job still counts against `DefaultMaxRunning` = 16); pkg/series/parts.go (`PartID` hashes up to K part ids and never looks at a context); pkg/cli/seriesparts.go (`series parts`, `series release -part`, `-check` call `PartID`) |
| Surface | local (the GUI process: its CPU and its job slots) |

## Summary

On a variable series with no `max_parts`, any part K up to about 2^62 is
valid (D165 refuses a cap, rightly). Working out part K's id hashes K
part ids the first time a process needs it. That runs at about 3.7
million parts a second here: part 10^7 took 2.7 s, so part 5×10^10 takes
close to 4 hours, and part 10^12 about 3 days.

In a shell, Ctrl-C ends `poo`, since it is its own process. The GUI runs
`poo` in its own process (cli.Main), and Stop only cancels the job's
context. `PartID` never checks that context. So:

1. In Release a part, a part typed with a few zeros too many (50000000000
   for 50) and Preview starts `poo series parts FILE -from 50000000000
   -to 50000000000`. The status reads "running…" and a CPU core is busy.
2. **Stop does nothing.** The button goes, the status still says
   "running…", and the job keeps hashing. `/api/jobs` lists it as not
   done. Closing poo-gui waits 15 s and gives up ("jobs: still running
   after 15s").
3. **Each such job keeps one of the 16 job slots.** After 16 of them, a
   Preview typed again for instance, every new command is refused, the
   views' own refreshes included: "jobs: too many commands running; stop
   one first". (Stop node still works: it cancels the node's job, and
   the node does look at its context.) Stop can't free a slot, so only a restart
   of poo-gui does.

The same holds for Show parts with a far From or To, Sign (`series
release -part K` works out part K−1), and Check a release or Add a
release on a release of a far part. Those last two need a release the
issuer really signed: `CheckRelease` checks the witness before it hashes
the chain, so a forged far release is refused at once (checked: a
release edited to part 10^11 is refused in under a second).

## Reproduction

poo-gui at b9fed52, `r25/series.json` (variable, 8 mints, part size 4,
no max parts). Playwright, with the script kept outside the repository.

1. Series and keys → Release a part. Series file r25/series.json, Part
   `50000000000`, Preview.
   - CPU of poo-gui: 0.00 s/s idle, 1.40 s/s while previewing.
   - Press Stop. After 1.5 s the status still reads "running…", and the
     CPU stays at 1.40 s/s for the next 4 s.
2. Through the page's own `api.js`, start 16 previews of far parts and
   cancel each at once.
   - `/api/jobs`: 16 not done, 2 s after the cancels.
   - `api.start(['version'])`: "jobs: too many commands running; stop
     one first". Opening Series and keys shows the same error.
3. Closing poo-gui: "poo-gui: jobs: still running after 15s".

## Impact

Nothing is signed or sent, and no one else can start it: the part is
typed by the user, or it comes from a release the issuer signed. But a
slip of the keyboard leaves a job that looks stoppable and isn't. It
holds a core for hours. Once 16 are stuck, the GUI can't run any new
command, not even a refresh of the node's status, and the only way out
is to restart poo-gui. Rated **low**.

## Recommendation

- **Source (pkg/series, pkg/cli), the real fix:** let a long part id
  chain stop when its command is cancelled. `extendLocked` already works
  in chunks of `partChunk` (16,384 ids). A `PartIDContext(ctx, k)`, or a
  check of `ctx.Err()` between chunks, used by `series parts`, `series
  release -part` and `-check`, and `series add-release` when it checks
  offline, would end the job within a few milliseconds of Stop. The
  chain keeps what it worked out, so nothing is lost. No consensus rule
  changes, and part K stays valid.
- **GUI/Blue (crypto.js), meanwhile:**
  - Before Preview, Sign or Show parts runs on a part above about 10^8
    (some 30 s here), confirm it with the cost: "Part 50000000000 takes
    poo about 50000000000 hashes to work out (hours), and it can't be
    stopped once started. Run it?" The GUI knows K, so this needs no
    poo call.
  - In `runJob`, if a job hasn't ended a few seconds after Stop, say so
    instead of "running…": "Stop asked; poo is still working and will
    end on its own. It holds one of 16 job slots until then." Then the
    user knows why later commands are refused.
- e2e (views.mjs): Release a part with Part 50000000000. Expect the
  confirm, or, after the Source fix, Stop to end the job (`/api/jobs`
  done) within 2 s.
