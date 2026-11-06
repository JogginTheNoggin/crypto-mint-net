# GREEN-011: A reload onto a broken policy keeps the node running

| Field | Value |
| --- | --- |
| Status | adopted |
| Based on | `claude/m10-watch` @ `7e68fe0` |
| Area | `pkg/daemon`, `pkg/rpc` (`POST /v1/reload`) |
| Effort | small |
| Decided by | Grey, Source Round 4: as prototyped (D106); the check and the reopen are not atomic, a known gap |

## Summary

Today a mistake in `node.json` followed by `poon reload` (or SIGHUP) stops
the node for good. `poon reload` even prints "reloaded". With this change,
both reload paths check the file first. On a mistake, the node logs an
error and keeps running on the policy it has, and `poon reload` fails with
the reason. Prototyped, with a test.

## Problem

Reproduced at `7e68fe0`:
1. Start a node.
2. Set `"log": {"level": "loud"}` in its `node.json`.
3. Run `poon reload`.

`poon reload` exits 0 and prints `reloaded`. The node logs `reloading its
node policy`, fails to reopen with the validation error, and exits. Every
later command says "is the node running?". The same happens with SIGHUP.
Hand-editing `node.json` is the documented way to change most settings
(`docs/modules/nodepolicy.md`), so one typo takes the node offline until
someone notices, and nothing told the person who made the typo.

`poon follow` and `unfollow` are not affected, because `nodepolicy.Save`
validates before writing.

## Idea

- **`POST /v1/reload`.** Run `nodepolicy.Load` (parse and `Validate`)
  before saying yes. On failure, answer 422 `invalid` with "not
  reloading, the node keeps its current policy: <reason>". A missing file
  still gets `not_found`, as today.
- **`daemon.Run`.** Do the same check when the `Reload` channel fires
  (SIGHUP). On failure, log an error, "reload refused; still running on
  the policy it started with", with the reason, and keep waiting. A
  corrected file then reloads normally.
- **Not covered.** The check and the reopen are not atomic. A file that
  breaks between them still stops the node, as it does today. Closing
  that gap would mean handing the checked policy to `node.Open`, which is
  a larger change. The window is milliseconds and needs a second edit
  during the reload.

## Cost and risk

About 25 lines. A valid reload behaves exactly as before, and an invalid
one used to stop the node, so nothing that worked changes. This is a
local robustness gap, not a security one: only someone who can write the
data directory can cause it.

## Prototype

On `claude/source-green`:
- the reload goroutine in `pkg/daemon/daemon.go`
- `reloadNow` in `pkg/rpc/view.go`
- `pkg/daemon/reload_test.go`: a spoiled policy is refused by both
  paths, the node keeps serving, and the corrected file reloads
- notes in `docs/modules/daemon.md` and `rpc.md`

I also checked it by hand with `poon reload` and `kill -HUP`.
