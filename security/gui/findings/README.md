# GUI security findings (Red team)

This directory holds the GUI (poo-gui) Red team's findings. Red attacks the
newest GUI/Grey branch (what counts as "main" until jerry merges) and writes
reports with suggested fixes. Red never implements fixes — GUI/Blue does, on
its own branch, and notifies GUI/Grey to review and merge.

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
| [GUI-001](GUI-001-launch-secret-leak.md) | Medium | Launch secret leaks via process args and browser history; never expires | open |
| [GUI-002](GUI-002-shell-split-divergence.md) | Medium | Terminal line-splitting diverges from a real shell (violates "identical to headless") | open |
| [GUI-003](GUI-003-passphrase-mask-heuristic.md) | Low | Passphrase masking is an output-regex guess and can fail open | open |
| [GUI-004](GUI-004-job-resource-exhaustion.md) | Low | No bound on running jobs or event streams (local DoS) | open |
| [GUI-005](GUI-005-total-authority-no-confirm.md) | Info | A leaked secret is total node control; no confirmation on destructive commands | open |
