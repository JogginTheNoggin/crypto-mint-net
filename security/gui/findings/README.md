# GUI security findings (Red team)

This directory holds the GUI (poo-gui) Red team's findings. Red attacks the
newest GUI/Grey branch (what counts as "main" until jerry merges) and writes
reports with suggested fixes. Red never implements fixes — GUI/Blue does, on
its own branch, and notifies GUI/Grey to review and merge.

Round 21 Part B reviewed main @ 4dc7d3d (PR #118: the settings export
and import forms, `-map-path`, the plan's follows, fit and now_used lines,
GUI-GREEN-046's Copy, ORANGE-015's long plans, and the R20B hardening).

New:
- GUI-024 (Medium): on Windows, the new **Copy** quotes a line for
  `CommandLineToArgvW`, not for a shell. A field holding `x$(calc)`,
  `a;calc` or `x"&calc&"y` is copied so that PowerShell or cmd.exe runs
  `calc` too. A link such as `#/explore?address=…` can fill the field.
  Shown with Playwright and `/api/info` answering `grammar: "windows"`.
  POSIX is safe.

Held up:
- **The settings import is pinned.** I previewed a settings file, then
  changed a peer in it on disk. `poo settings import FILE -apply
  -if-digest D` then exits 1 with "this is not the one previewed", and
  node.json is unchanged. The GUI's settings form is the same
  `importForm`, so a form change still removes Apply.
- **The R20B note is done.** A preview with no digest now gets no Apply,
  and says so.
- **The new plan lines are text.** `follows`, `fit` and `now_used` are
  drawn with `el` (text nodes), and the page has no `innerHTML` but the
  terminal's fixed frame.
- `settings` is a write command, so the view token is refused it.

Round 20 Part B reviewed main @ 9628426 (PR #107: Red #99,
GUI-GREEN-045's subcommand parity check, Orange #101, and the Source
changes merged with them: D152 `-if-digest` checks, BLUE-019 followed
series and pools, and the plan's follow lines).

There are no new findings. No file under gui/web changed this round. The
GUI's surface moved only through poo.

Held up:
- **Import is still pinned.** I re-ran the R19B Playwright check on
  9628426:
  - picking another file after Preview removes Apply;
  - Apply's dialog names A.json and carries `-if-digest <64 hex>`;
  - nothing from B reaches node.json.
- **An empty digest is no longer a missing one (D152).**
  `-apply -if-digest ""` exits 1 with "nothing was changed".
  `-if-digest` without `-apply` is refused.
- The `views`, `viewonly` and `parity` e2e tests pass locally. The new
  subcommand parity check lists every subcommand `poo help` shows, and
  the only ones left out are `completion bash|zsh|fish`.

Hardening note for GUI/Blue (optional, not a finding): the GUI fails
open on the digest. `importForm` adds `-if-digest` only when it could
read a digest from the preview's JSON. If it ever read none, Apply would
run unpinned, without saying so. poo always prints one with `-json`
today, so this can't happen now. Showing no Apply when the digest is
missing would keep it that way.

Round 19 Part B reviewed main @ e71e317 (PR #97):
- the import form's `-settings` option;
- Apply pinned by `-if-digest` (D151), with the picked file streamed to
  poo (ORANGE-014);
- views split into one file each;
- Source's D151 `Unlock` seal check.

There are no new findings.

Verified:
- GUI-022 is fixed. A stale `wallet seed` after a rotation is now
  refused, and `cosign` uses the same `Unlock`.
- GUI-023 is fixed:
  - In the GUI (Playwright), picking another file removes Apply, and
    Apply names the previewed file and carries its digest.
  - In the CLI, a path changed on disk after its preview is refused by
    `-if-digest`.

Held up:
- The plan, including the new "read first" defence line, is drawn as
  text.
- `policy import` is still refused to the view token.
- The view split changed no argv building that Red checked before; the
  parity e2e test covers it.

Round 18 Part B reviewed main @ 3804c55:
- the new Export and Import node policies forms (PR #81, D149): a browser
  file picker sent as `poo policy import -`, the plan preview, and Apply;
- Source's GUI-022 fix (D149, `sameSeal` in `Wallet.Hold`);
- D148 (wallet writes are synced, and a waiting command says so).

New, for GUI/Blue:
- GUI-023 (Low): picking another file after Preview leaves the Apply
  button, and Apply imports the newly picked file, which was never
  previewed. Reproduced with Playwright: A was previewed, B was applied,
  and B's peer landed in node.json.

GUI-022 is partly verified. Every command that `Hold`s now refuses a
stale seal (the stale `passwd` and stale `address` races are refused).
`wallet seed` and `wallet cosign` never `Hold`. A stale "Show seed" job
answered with the old passphrase after a rotation still printed the seed.
That part stays open with Source.

Held up:
- The plan is drawn as text only.
- `policy export` and `policy import` are refused to the view token (only
  `policy list` is allowed).
- Names in an import file go through `checkName`.
- A file can make a policy current only when none is.

Round 17 Part B reviewed main @ 052d032:
- GUI-GREEN-042 (the parity check; Explore's address `-below`/`-tid`,
  and Pools with every filter);
- GUI ORANGE-013 (a long input line sent in parts, `more: true`);
- the Source changes merged with them: BLUE-016 / D146 (`Wallet.Hold`),
  D147 (Windows data-directory lock), `submit` reading build and work
  replies, and the size-hinted entry reads (ORANGE-025).

There is one new finding, for Source:
- GUI-022 (Medium): `Hold` merges a file saved meanwhile, seal included.
  - A command opened before a passphrase change now takes the old
    passphrase and acts.
  - A stale `passwd` overrides the owner's rotation. Reproduced: after
    `old` to `rotated`, a stale `passwd` set `mine`. `rotated` then
    failed and `mine` worked.
  - Under D145 both were refused.

Verified:
- GUI-017's Windows residual is closed by D147 (code review and a
  `GOOS=windows` build).
- Input sent in parts holds:
  - Stop between parts fails the read.
  - End input waits until every part has arrived.
  - A secret prompt reads the whole line.
  - Every part goes through the same POST checks.
- GUI-GREEN-042's new fields are single argv elements.

Round 16 Part B reviewed main @ 6a1cdce, the first base after the stack
was merged:
- GUI/Blue's GUI-021 fix (#68): Stop fails the read, and an explicit end
  of input is added (`/eof`, Ctrl-D, End input, "Send and end input").
- GUI-GREEN-041:
  - the build and work grind fields (`-device`, `-threads`, `-for`,
    `-max-tries`, `-start`, `-serial`);
  - the Wallet view's shared `-series` picker;
  - `receive -serial/-tid` pairs.

GUI-021 is verified live:
- Stop exits 1 and the node does not get the entry.
- End of input submits it.
- End of input at a passphrase prompt fails with "nothing was entered".

GUI-GREEN-041 is safe:
- Each new field is one argv element.
- build and work stay write commands, refused in view-only mode.
- The series picker keeps only a series id in browser storage, never a
  secret.

There are no new findings. GUI-017's Windows residual is unchanged (Source).

Round 15 Part B reviewed `claude/gui-4-watch @ 55cf745`:
- Source 46bcb9e: the wallet's compare-and-write lock and the new
  passphrase confirm (D145).
- The `-bytes` checkboxes on Build and Add a layer.
- Submit with `-f FILE` or `-`.
- The GUI's count of secret asks (`State.Asks`).

GUI-019 and GUI-020 are verified fixed:
- Both R14B races are now refused rather than lost.
- A typed new passphrase is asked twice, and the second masked field
  shows.
- The masking order holds.

`-bytes` prints only public entry and layer bytes. `-f FILE` reads a path
the full-token user names, and an error shows at most one byte of a
non-hex file. View-only mode refuses `submit`.

There is one new finding, for GUI/Blue:
- GUI-021 (Low): Stop on a `submit -f -` job sends the pasted entry. Stop
  is the GUI's only end of input, and `io.ReadAll` treats it as one. In
  the GUI terminal, Ctrl-C therefore does what Ctrl-D does in a shell.

Also noted:
- The refused-save-after-payment case is Source's RED-039, already fixed
  in the Source Round 16 queue.
- `wallet.Save` still doesn't fsync.
- GUI-017's Windows residual could reuse the wallet's new `LockFileEx`.

Round 14 Part B reviewed `claude/gui-4-watch @ 3a7a39c`:
- the new Wallet forms (GREEN-039);
- `follow -replace`;
- `poo submit entry|layer` (D142).

Secret handling held:
- `wallet seed` is withheld by the server (`cli.SecretOutput`) and has no
  Copy.
- `passwd` and the shared-policy `self` key ask through masked prompts.
- No new command prints a secret.
- `submit` is a write command, refused when view-only.

There are two new findings, both for Source:
- GUI-019 (Medium): wallet commands overwrite each other's saves. A form
  waiting at its passphrase prompt reverted a passphrase change, so the old
  passphrase worked again, and it dropped an address given out meanwhile.
- GUI-020 (Low): a new passphrase is never asked for twice.

Round 13 Part B reviewed `claude/gui-4-watch @ 376e964`:
- Source's GUI-018 fix (BLUE-013 / D139: `wallet follow` is one replace
  request).
- GREEN-037/038 (`status` and `doctor` report a node.json change waiting
  for a reload; the Node view shows a notice with "Reload now").

Both held, and there are no new findings. GUI-017's Windows residual is
unchanged.

Round 12 Part B reviewed `claude/gui-4-watch @ c9c9e1f`:
- Source's GUI-017 fix (D134: offline edits take the data directory's lock;
  D136 does the same for follow, `wallet follow` and `init`).
- GREEN-036 (doctor `fix_args` carry `-dir`, and the Health check runs them
  as printed; new Getting started steps).
- GUI/Blue's GUI-016 wording (#51).

All held on unix. The R10B race lost nothing in 20 runs. Windows has no lock,
so GUI-017 remains there. There is one new Low for Source: GUI-018.
`wallet follow` changes a running node in two requests, and a failure
between them drops the series' rule while saying nothing was changed.

Round 11 Part B reviewed `claude/gui-4-watch @ 4be2230`: GUI/Blue's GUI-016
fix (a refused Remove shows poo's reason) and GREEN-035 (the summary shows a
layer rate kept with no floor). Both held, and `e2e/work.mjs` passes locally.
There are no new findings. Two optional wording nits for Blue are in GUI-016's
notes. GUI-017 is still open with Source: Source's base has not changed since
72d6f25.

Round 10 Part B reviewed `claude/gui-4-watch @ 4108866`:
- Source's GUI-014 core (D125: edits applied on the node under its lock,
  `If-Match`).
- GREEN-034's `-if-version` on the GUI's Remove.
- GUI-015's fix (D127: `MaxLayerRate` 50, clamped on load per D128; the
  form shows poo's limits).
- GREEN-033's "now …" hints.

All of these held when a node is running. There are two new Low gaps:
- GUI-016: a Remove that poo refuses shows nothing.
- GUI-017: changes made without a node still read and write node.json whole
  with no lock. Eight concurrent `add`s kept 3 to 6 of 9 jobs in all 20 runs.
  Any error from the node's read also makes poo write the file behind a
  running node. GUI-017 is for Source.

Round 9 Part B reviewed `claude/gui-4-watch @ 6fae281`: GUI/Blue's GUI-014
GUI part (one schedule change at a time), GREEN-032, the new layer floor and
rate in Schedule defaults (D122), and Source's lease-renew retry (RED-019,
D120). All of these held. One new Low gap: GUI-015. The form accepts a layer
rate up to 1000 a second, but default peers take about 100 a second and then
time out and block the sender, and nothing warns about it.

Round 8 Part B reviewed `claude/gui-4-watch @ a93ccb6`: GUI/Blue's GUI-012 and
GUI-013 fixes, GREEN-030 (jump list), GREEN-031 (Schedule Remove with
`-target`), ORANGE-011, and Source m10-watch @ 0fd59fa (the in-node lock on
node.json edits, RED-013). GUI-012 is verified, and GUI-013 is verified for the
case it describes. One new Low gap: GUI-014. `poo grind schedule` changes are
made by the client as a GET followed by a PUT of the whole section, so two
overlapping changes lose one (for example a Remove and an Add in the GUI, or the
GUI and a shell). `-target` also cannot tell jobs that share a target apart.

Round 7 Part B reviewed `claude/gui-4-watch @ b20bf8f`: the GUI-011 fix, Source's
schedule commands and leases (9972137), the new Schedule panel, GREEN-028/029, and
ORANGE-010's event-stream leak fix. The leak fix holds when you switch views, and
GUI-010 (core lease) and GUI-011 are now verified. Two new Low gaps:
- GUI-012: running a form again in the same area still leaks the earlier job's
  stream, and orphans that job if it was waiting for input.
- GUI-013: the Schedule panel removes jobs by their position when the table was
  drawn, so a stale table can remove the wrong job.

Details and notes are in GUI-013.

Round 6 Part B reviewed `claude/gui-4-watch @ e7df74b`: GUI/Blue's GUI-010 fix
(SIGHUP in poo-gui, re-adopting a running grind on reload), GUI/Green's jobs table
with a Stop for each job, device suggestions and H/s rates, and GUI/Orange's paced
Work drawing and bounded error box. All of these held. One new Low gap: GUI-011.
Catching SIGHUP overrides `nohup`, so a detached poo-gui and the node it hosts now
stop when the session ends, and the node's documented SIGHUP reload now stops the
GUI as well.

Round 5 Part B reviewed `claude/gui-4-watch @ 8b044ac`: the new Work view on
`poo grind` (devices, jobs, live run, background jobs, stop -all, bench), the
GUI-009 fix through the core's `cli.SecretOutput`, the Health check panel, and
GUI/Orange's ORANGE-006..008. GUI-009's fix held and is now marked fixed. One
new Low gap: GUI-010, where a followed grind outlives poo-gui if the GUI dies
without stopping it (details and the other verifications are in GUI-010's
notes).

Round 4 Part B reviewed `claude/gui-4-watch @ 50a0edd`: GUI/Blue's GUI-008 fix
together with GUI/Orange's stream resume ids (`Last-Event-ID` and the chunk's
`next`), Watch frame batching and capped scrollback, the hidden seed on wallet
restore (`-restore -`), the single wallet-file field, Settings in Getting started,
and the ledger picture in Explore. All of these held (details in GUI-009's notes).
One new Low gap: GUI-009, where a seed printed by `wallet seed` or `keygen` stays
replayable from Jobs and the event stream.

Round 3 Part B reviewed `claude/gui-4-watch @ 9b4b19c`: GUI/Blue's GUI-006 and
GUI-007 fixes (re-attacked — both hold; see GUI-008's notes for details), and the
new surface (background Watch across views, the Explore "look up" links, Pools
lookups moved onto `tx`/`unspent`, and the single no-node banner). No new
vulnerability; one Info-level trade-off recorded as GUI-008.

Round 2 Part B reviewed `claude/gui-4-watch @ 93ed36a` (PR #18): the new
`-view-only` mode, the D101/D89 secret masking (a re-check of GUI-003), and the
Watch/Explore views. Verified as solid: `-view-only` is a process-wide flag the
core enforces with a default-deny command table plus a read-only view token, with
no client bypass; the new masking is driven by the core's `cli.Stdio.Secret`
signal (all interactive passphrase reads go through it) and no longer sniffs
output; the GUI-001 sign-in fix (hashed, single-use, TTL tokens; an independent
session cookie) holds; and Watch/Explore render every field via `textContent`, so
node- or peer-controlled event data cannot inject HTML. New gaps: GUI-006 (secrets
passed as arguments, not the interactive path, still persist) and GUI-007 (Watch's
unbounded client buffers). Both low.

First round reviewed against `claude/gui-5-release` (the GUI/Grey stack
#10←#11←#12←#14, 2026-10-27). The `gui/server`, `gui/jobs` and `gui/shell`
packages are unchanged across that stack, so every finding below applies to
the whole stack; the later `gui/web/static` view/render code was checked for
XSS and is clean (all output rendered via `textContent`, no HTML sinks).

One file per finding, named `GUI-NNN-short-slug.md`. Each finding carries:

- **ID** — stable `GUI-NNN`.
- **Severity** — Critical / High / Medium / Low / Info, with the threat model
  it assumes (most of the GUI's exposure depends on who shares the host).
- **Status** — `open` (needs a fix), `fix-proposed` (Blue has a branch),
  `fixed` (Grey merged and verified), or `wontfix` (with a reason).
- **Fix branch** — the GUI/Blue branch that addresses it, once one exists.

## Threat model (shared by all findings)

poo-gui is a local admin tool: it binds a loopback port and runs the full
`poo` command surface through `cli.Main`. Anyone who holds the launch secret
(or the session cookie derived from it) can run **any** poo command with the
node's privileges — read and write the node's files, keys, wallet and ledger.
So the whole security of the GUI rests on two things: the secret staying
secret, and the loopback boundary not being crossed by another origin on the
same machine. The findings below are graded against that.

Attackers considered: (a) a co-located OS user on a shared/multi-user host;
(b) a malicious or compromised web origin the victim's browser visits; (c) a
second service listening on another loopback port (same site). The remote
Internet is **not** in scope — the port is loopback-only — except through (b).

## Index

| ID | Severity | Title | Status |
|----|----------|-------|--------|
| [GUI-001](GUI-001-launch-secret-leak.md) | Medium | Launch secret leaks via process args and browser history; never expires | fixed (fca4369) |
| [GUI-002](GUI-002-shell-split-divergence.md) | Medium | Terminal line-splitting diverges from a real shell (violates "identical to headless") | fixed (fca4369) |
| [GUI-003](GUI-003-passphrase-mask-heuristic.md) | Low | Passphrase masking is an output-regex guess and can fail open | fixed (fca4369) |
| [GUI-004](GUI-004-job-resource-exhaustion.md) | Low | No bound on running jobs or event streams (local DoS) | fixed (fca4369) |
| [GUI-005](GUI-005-total-authority-no-confirm.md) | Info | A leaked secret is total node control; no confirmation on destructive commands | fixed (documented, fca4369) |
| [GUI-006](GUI-006-secret-args-in-history.md) | Low | Secret-bearing command arguments (wallet seed) persist in terminal localStorage history and the Jobs list | fixed (verified R3B @ 9b4b19c) |
| [GUI-007](GUI-007-watch-unbounded-buffers.md) | Low | Watch view's paused-event and error buffers grow without bound (local tab DoS) | fixed (verified R3B @ 9b4b19c) |
| [GUI-008](GUI-008-background-watch-lifetime.md) | Info | Background Watch holds its event stream and node connection until Stop (by design; trade-off recorded) | fixed (verified R4B @ 50a0edd) |
| [GUI-009](GUI-009-secret-output-retained.md) | Low | Output of seed-printing commands (`wallet seed`, `keygen`) is kept and replayable from Jobs and the event stream | fixed (verified R5B @ 8b044ac, core `cli.SecretOutput`) |
| [GUI-010](GUI-010-followed-grind-outlives-gui.md) | Low | A grind followed from Work keeps running in the node when poo-gui dies without its cancel path (SIGHUP, kill) | fixed (GUI part verified R6B; core lease verified R7B @ b20bf8f) |
| [GUI-011](GUI-011-sighup-stops-nohup-gui.md) | Low | poo-gui now stops on SIGHUP even under nohup, and SIGHUP no longer only reloads a node it hosts | fixed (verified R7B @ b20bf8f) |
| [GUI-012](GUI-012-form-resubmit-leaks-stream.md) | Low | Pressing a form's button again leaks the previous job's event stream and orphans a job waiting for input | fixed (verified R8B @ a93ccb6) |
| [GUI-013](GUI-013-schedule-remove-by-stale-index.md) | Low | The Schedule panel's Remove goes by position, so a stale table can remove a different scheduled job | fixed for its own case (verified R8B @ a93ccb6); duplicate targets and concurrent edits in GUI-014 |
| [GUI-014](GUI-014-schedule-edits-lost-update.md) | Low | Schedule changes are read, changed and written back by the client, so two at once lose one; `-target` cannot tell jobs with the same target apart | fixed: GUI part (PR #41) and core edits on the node (D125, merged Round 10); the GUI's Remove also sends `-if-version` (GREEN-034, Round 10); verified R10B @ 4108866 with a node; without one, see GUI-017 |
| [GUI-015](GUI-015-layer-rate-above-peer-budget.md) | Low | Schedule defaults allow a layer rate (up to 1000/s) above what default peers take (~100/s), so a steady grinder is timed out and then blocked, with no warning | fixed (core D127 cap 50; GUI form PR #44, merged Round 10; verified R10B @ 4108866) |
| [GUI-016](GUI-016-refused-remove-is-silent.md) | Low | A Schedule Remove that poo refuses (`-if-version`, `-target`) shows no message; the job just stays listed | fixed (PR #49, merged Round 11; verified R11B @ 4be2230) |
| [GUI-017](GUI-017-offline-schedule-edits-unlocked.md) | Low | Schedule changes made without a node still read and write node.json whole with no lock, so they lose each other; any read error also makes poo write behind a running node | fixed by Source: D134 on unix (verified R12B), D147 on Windows (verified R17B by code review and build); closed |
| [GUI-018](GUI-018-wallet-follow-two-step-loses-rule.md) | Low | `poo wallet follow` changes a running node in two requests (remove, then add), so a failure in between drops the series' follow rule while saying "nothing was changed" | fixed by Source (BLUE-013), merged Round 13; verified R13B @ 376e964; GUI terminal e2e check added R13C |
| [GUI-019](GUI-019-wallet-commands-overwrite-each-other.md) | Medium | Wallet commands read the whole wallet.json and write it back; one waiting at a passphrase prompt in another form undoes the rest, even a passphrase change (the old passphrase works again) | fixed by Source (wallet.Save compare-and-write, D145), merged Round 15; verified R15B @ 55cf745 |
| [GUI-020](GUI-020-new-passphrase-not-confirmed.md) | Low | A new wallet passphrase is typed once into a masked field and never confirmed, so a typo locks the wallet | fixed by Source (asks twice, D145), merged Round 15; the GUI shows the second ask (State.Asks); verified R15B @ 55cf745 |
| [GUI-021](GUI-021-stop-submits-pasted-entry.md) | Low | Stop (or Ctrl-C in the terminal) on `poo submit -f -` hands the pasted entry or layer to the node: a cancelled job's input reads as end of input, and the GUI has no other end of input | fixed by GUI/Blue (PR #68), merged Round 15; verified R16B @ 6a1cdce |
| [GUI-022](GUI-022-hold-adopts-new-seal.md) | Medium | `Wallet.Hold` (D146) adopts a file saved meanwhile, seal included, so a command opened before a passphrase change still works with the old passphrase, and a stale `passwd` re-seals over the owner's rotation | fixed by Source (D149 + D151), merged Round 19; verified R19B @ e71e317 |
| [GUI-023](GUI-023-import-apply-other-file.md) | Low | Import's Apply sends whatever file is picked now, not the one previewed, so a file never reviewed is applied; the confirm shows only `-apply -` | fixed by GUI/Blue (Round 18C) and Source `-if-digest` (D151), merged Round 19; verified R19B @ e71e317 |
| [GUI-024](GUI-024-copied-line-runs-in-windows-shell.md) | Medium | On Windows, Copy quotes the command line for `CommandLineToArgvW`, not a shell, so a field value (one a link can fill) runs other commands when the line is pasted into PowerShell or cmd.exe | fixed by GUI/Blue in Round 21C (Copy copies only words both shells take literally, else it is off); awaiting verification |
