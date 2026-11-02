# GUI-018: `poo wallet follow` changes a running node in two steps, so a failure in between drops the series' follow rule and says "nothing was changed"

| Field | Value |
| --- | --- |
| Severity | low |
| Status | fixed by Source (BLUE-013, 56633dc: one replace request), merged Round 13; verified by GUI/Red R13B @ 376e964 |
| Found on | claude/gui-4-watch @ c9c9e1f (Source bc2e4bc, RED-030 / D136) |
| Component | pkg/cli/wallet.go (`walletFollow`), pkg/rpc/view.go (`POST /v1/follow`) |
| Surface | node (node.json's follow list). Reachable from the GUI's terminal, which runs the same `cli.Main`. |
| Fixed by | Source (BLUE-013, D139, 56633dc: `FollowRequest.Replace`, one request), routed by GUI/Blue in Round 12C via GUI/Grey. GUI e2e check of the terminal path added by GUI/Blue in Round 13C (gui/e2e/terminal.mjs). The GUI itself needed no change. |

## Summary

RED-030 (D136) made `poo wallet follow` change a running node through its
RPC instead of writing node.json behind it. That part holds. The change is
sent as **two** requests, though:

1. `Follow{Remove: true, Series: id, NoReload: true}`. This takes the
   series' rule off node.json and **saves** it, without a reload.
2. `Follow{Series: id, Pools: pools}`. This adds the wallet's pools back,
   saves, and reloads.

Each request runs under the node's `policyMu`, but nothing joins the two.
If step 1 succeeds and step 2 fails, node.json has lost the series' rule:

- **Step 2 fails with a transport error** (connection reset, timeout, or the
  node stopping between the two calls). `nodeErr` is set, and the code falls
  through to `lockPolicy`. If the node is still running and holds the lock,
  the command fails with "the node is running but its follow list could not
  be read or changed, so **nothing was changed**". That is false: step 1
  already removed the rule.
- **Step 2 is refused by the node with an `rpc.Error`** (for example a
  `Validate` failure). The command returns that error, and the removal from
  step 1 stays in place.
- **Any reload between the two steps.** For example, a Follow or Reload
  pressed in the Node view, or another client's follow, while
  `wallet follow` is between its two calls. The node then applies a policy
  without that series, and stops following it until step 2 lands, if it
  does.

The running node keeps following the series until its next reload or
restart, because step 1 had `NoReload`. So the loss is easy to miss. It
shows up later, when the node stops following the wallet's series.

## Reproduction (code path)

1. Run a node that follows series S whole, and a wallet that watches some
   of S's mints.
2. Run `poo wallet follow` (from a shell or the GUI terminal), and stop
   the node or drop its RPC connection right after the first
   `POST /v1/follow` returns. A debugger breakpoint, or a proxy that resets
   the second request, will do.
3. The command fails with "nothing was changed". `poo follow -dir D`, or
   node.json, no longer lists S. The next `poo node` start or reload stops
   following S.

## Impact

The node quietly stops following a series the user meant to narrow to the
wallet's pools, and the error message says nothing changed. No attacker is
needed, and the window is narrow, which is why this is rated **low**. It is
node policy, not consensus.

## Recommendation

Source:

- Make the change one request, applied under `policyMu` in one step. For
  example, add `FollowRequest.Replace` (or a `/v1/follow/replace`) that
  swaps the series' rule for the given pools, validates, saves, then
  reloads. The path without a node already does this in one `Load`/`Save`
  under the lock.
- Until then, if step 2 fails after step 1 succeeded, say so. Something
  like "the series' rule was removed but the pools were not added: run
  poo follow -series S -pool …", rather than "nothing was changed". Or put
  back the rule step 1 removed.
- Test: an RPC handler that fails the second `Follow`. Then expect either
  node.json unchanged, or an error that names the removal.

## Routing (GUI/Blue, Round 12C)

GUI/Blue makes no GUI change. The flaw is in pkg/cli, which the GUI's
terminal runs as is, and the GUI must not hold core logic, so the fix
belongs to Source. GUI/Blue passed it to Source through GUI/Grey, and
Source/Blue has it in Source Round 12 Part C. Once Source's fix lands,
GUI/Blue can add a GUI e2e check for the terminal path.

## Notes — Round 13B verification (held up)

- **BLUE-013 / D139, verified.**
  - `wallet follow` now sends one `FollowRequest{Replace: true, Series, Pools}`.
    The node applies it under `policyMu` in one `Load`, `ReplaceFollow`,
    `Validate`, `Save`, and then reloads. The series is never saved off the
    list between two steps.
  - Without a node, the same `ReplaceFollow` runs under the data-directory
    lock (D136).
  - `Remove` together with `Replace` is refused.
  - An older node rejects the unknown `replace` field: `decodeBody` uses
    `DisallowUnknownFields`, so the command fails with an RPC error and
    nothing changes. A version mismatch therefore cannot fall back to a
    merging `AddFollow`.
- **GREEN-037/038 reload notice, safe.**
  - `status` compares node.json, loaded fresh, with the policy the node
    runs, ignoring the grind section.
  - Checked live:
    - false right after start;
    - false after a `grind schedule add`, which is applied at once;
    - true after `unfollow -no-reload`;
    - false again after `poo reload`;
    - `poo doctor` warns with fix `reload`.
  - The notice is fixed text. "Reload now" runs the Reload form's own
    `poo reload`, which refuses an invalid file (GREEN-011) and is refused
    in view-only mode.
  - `gui/e2e/work.mjs` passes locally on 376e964.
  - Each `status` call, including the Followed pools panel's 5 s refresh,
    now also reads node.json. That is a small extra cost, not a finding.

## GUI check (GUI/Blue, Round 13C)

gui/e2e/terminal.mjs now runs `wallet watch -pool 0` and then
`wallet follow` in the GUI's terminal, against the node running in the
background. It checks three things:

- The command says the node applied the change.
- `poo follow` then lists exactly one rule for the series, and that rule
  holds the wallet's pools.
- The node follows only that pool.

This covers the path a GUI user takes. A failure between two requests
cannot be injected from the browser, because poo talks to the node
directly. The single-request guarantee is tested in Source's unit tests
(pkg/rpc/view_test.go and pkg/nodepolicy/follow_test.go).
