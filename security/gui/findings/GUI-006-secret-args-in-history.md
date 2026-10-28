# GUI-006: Secret-bearing command arguments persist in terminal history and the Jobs list

| Field | Value |
| --- | --- |
| Severity | low |
| Status | fixed (verified by GUI/Red R3B @ 9b4b19c); seed entry now via `-restore -` (D106) in Round 4 |
| Found on | claude/gui-4-watch @ 93ed36a |
| Component | gui/web/static/terminal.js (history), gui/server/server.go (`info`/`/api/jobs`), gui/jobs/jobs.go (`Args` retention) |
| Surface | wallet / local |
| Fixed by | `claude/gui-blue` |

## Summary

The D101/D89 masking (re-checked this round — see GUI-003) is solid for secrets
poo reads **interactively**: the core signals `cli.Stdio.Secret` and the
terminal/forms mask the field and don't echo it. But it does nothing for secrets
passed as **command-line arguments**, and poo has such arguments — most importantly
the wallet seed:

- `poo wallet create -restore <64-hex-seed>` (restore a wallet from its seed)
- `poo keygen -seed <hex>` (make a key from a given seed)

The wallet seed restores every key in the wallet, so it is as sensitive as the
passphrase. When one of these runs through the GUI **terminal**, the whole line —
seed included — is:

1. **echoed** to the scrollback (`write(text + '\n', 'echo')`, terminal.js), and
2. **persisted to `localStorage`** under `poo-gui-history` (terminal.js
   `saveHistory`, last 200 lines), surviving across sessions and reboots with no
   expiry and no way for the user to exclude a line, and
3. kept in the server's job list as `Args`/`Command` and shown in the **Jobs**
   view and `GET /api/jobs` (`info()` → `shell.Quote(j.Args)`) for the life of the
   process.

The project is partly aware of this: the Wallet view's "Create or restore" form
labels the restore field "Part of the command line, as in a shell: it shows in the
preview and in Jobs," and keygen's seed field says the same. That disclosure covers
the **form** path and the in-memory preview/Jobs display. It does **not** mention
the **terminal** path's `localStorage` persistence, which is the part that matters
most: a secret written to disk, unencrypted, in the browser profile, outliving the
session.

## Reproduction

1. Start `poo-gui` (no `-view-only`), open the terminal.
2. Type `wallet create -restore 00112233...<64 hex> -wallet /tmp/w.json`.
3. Observe the seed echoed in the scrollback and the Jobs view command line.
4. In devtools: `localStorage.getItem('poo-gui-history')` → the full line with the
   seed is stored, and is still there after closing and reopening the browser.
5. Any local process or user with read access to the browser profile can recover
   the seed from `localStorage` long after the command ran.

## Impact

Full wallet compromise (the seed restores every key) from an at-rest, unencrypted
copy in `localStorage`, with no attacker position needed beyond local filesystem
access to the browser profile (or a glance at the screen / a screen share). It
needs the user to have actually passed a seed on the command line, and a shell user
running the same `poo` command would also leak it to shell history — but the GUI
adds a persistence channel the user is neither warned about (in the terminal path)
nor able to clear selectively, and that a shell user can avoid (`HISTCONTROL`, a
leading space). Rated **low** because it is local and user-initiated; the at-rest
seed is what keeps it from being merely informational.

## Recommendation

- Treat a small set of known secret-bearing flags (`-restore` for `wallet create`,
  `-seed` for `keygen`) specially in the terminal: do **not** write a line
  containing them to `localStorage` history (the in-session up-arrow buffer is
  fine), mirroring a shell's `HISTCONTROL=ignorespace`. Optionally redact the value
  (`-restore <hidden>`) in the echo and in the Jobs command display.
- Better: offer an interactive, masked way to supply a seed (read it through the
  same `cli.Stdio.Secret` path the passphrase uses) so restore doesn't need the
  seed on the argv at all; steer the Create/restore form to that path.
- Extend the existing UI disclosure to name the `localStorage` persistence, or add
  a "Clear terminal history" control, so the user can reason about and remove it.
- Test: an e2e asserting that after `wallet create -restore <seed>` in the
  terminal, `localStorage['poo-gui-history']` contains no line with the seed.

## Notes / what is already right

Interactive passphrase reads are correctly masked and never stored (GUI-003 fix,
verified this round), and input sent to a running job is kept out of history. This
finding is only about secrets that arrive as arguments rather than through the
secret-read path.

## Fix (GUI/Blue, branch `claude/gui-blue`)

- `gui/shell` declares `SecretFlags` (`restore`, `seed`) and `Redact(args)`, which replaces a secret flag's value with `<hidden>`. `sim` is exempt, because there `-seed` is a random-number seed. `TestSecretFlagsExist` fails if poo renames either flag.
- **History.** The terminal never writes a line to `localStorage` if it holds a secret flag (`api.secretLine`, which uses the server's list from `/api/info`), or if it starts with a space (`HISTCONTROL=ignorespace`). Up-arrow still recalls such a line for the session. Lines saved by older versions are filtered out on load, which purges seeds already stored. A **Forget history** button clears the saved history.
- **Jobs.** `info()` redacts `Args` and `Command` in `POST /api/jobs` and `GET /api/jobs` and sets `redacted`. The Jobs view hides "Run again in terminal" for such rows. The command itself still gets the real argv.
- **Forms.** The restore and keygen-seed help now says the value shows in the preview, is hidden in Jobs, and is never saved.
- Echo is left as typed, as a shell does. The scrollback is not persisted.
- Not done, belongs to Source: an interactive masked seed entry (for example `wallet create -restore -` reading through `cli.Stdio.Secret`) would keep the seed off the argv entirely. Passed to GUI/Grey for the Source teams. Also worth considering in the core: let poo declare its secret flags, so the GUI list cannot drift.
- Regression tests: `TestRedact`, `TestSecretFlagsExist` (gui/shell); `TestJobsHideSecretFlags` (gui/server); in gui/e2e/terminal.mjs, checks that the `keygen -seed` line and a space-prefixed line are not in `localStorage`, that up-arrow recalls the seed line, and that Jobs shows `<hidden>` with no Run again button.
