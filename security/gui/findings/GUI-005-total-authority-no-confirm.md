# GUI-005 — A leaked secret is total node control; no confirmation on destructive commands

- **ID:** GUI-005
- **Severity:** Info (design / defense-in-depth)
- **Status:** open
- **Fix branch:** _(pending — GUI/Blue)_
- **Component:** architecture — `gui/server/server.go` (`/api/jobs`, `/api/run`), `gui/jobs/jobs.go`

## What

The GUI exposes the **entire** `poo` command surface through two unqualified
endpoints (`POST /api/jobs`, `POST /api/run`) that pass arbitrary argument
vectors to `cli.Main`. There is no notion of a safe vs. destructive command,
no allow-list, no per-command authorization, and no confirmation step. The
single launch secret is therefore an all-or-nothing credential: holding it is
equivalent to a root shell over the node — reading and rewriting the wallet,
changing the passphrase (`poo wallet passwd`), mutating the ledger, managing
peers, anything the headless tool can do.

This is a deliberate and reasonable design for a local single-user admin tool,
and it is **not** a bug on its own. It is recorded here so the team treats it
as the explicit threat model rather than an accident, because it sets the blast
radius for every other finding:

- it is what makes GUI-001 (secret leak) a *total* compromise rather than a
  partial one;
- it is what makes GUI-004 (resource exhaustion) reach the whole node.

## Why it matters

When the authority is total and binary, the only thing standing between "local
convenience tool" and "someone owns the node" is the secret. That concentrates
all risk onto GUI-001's leak channels and onto the loopback boundary. A small
amount of defense-in-depth would blunt the worst outcomes of a leaked secret
or a tricked click.

## Suggested direction (for GUI/Blue / GUI/Grey — not a code change to make here)

- **Document** this authority model explicitly in `docs/gui.md` and
  `SECURITY.md`: "the poo-gui secret is equivalent to full control of the node;
  treat it like the wallet passphrase."
- Consider a **confirmation gate** for a named set of destructive/irreversible
  commands (passphrase change, key deletion, anything that rewrites persistent
  state), surfaced in the UI and enforceable server-side via a one-time
  confirm token — so a single cross-origin trick or a stray click cannot run
  them silently. (Keep it server-enforced, not just a UI dialog, or it adds no
  real boundary.)
- Longer term, consider whether a read-only mode (views/telemetry only, no
  state-changing commands) is worth offering for the common "just watch the
  node" use.

## Notes

Mitigation of this item is a policy/UX decision for jerry and GUI/Grey as much
as a code change; it is filed as Info so it is tracked, not because an
immediate fix is required. The concrete, actionable items are GUI-001 through
GUI-004.
