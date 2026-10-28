# GUI security findings (Red team)

This directory holds the GUI (poo-gui) Red team's findings. Red attacks the
newest GUI/Grey branch (what counts as "main" until jerry merges) and writes
reports with suggested fixes. Red never implements fixes — GUI/Blue does, on
its own branch, and notifies GUI/Grey to review and merge.

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
| [GUI-009](GUI-009-secret-output-retained.md) | Low | Output of seed-printing commands (`wallet seed`, `keygen`) is kept and replayable from Jobs and the event stream | open |
