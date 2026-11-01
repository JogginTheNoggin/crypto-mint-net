# GUI-018: `poo wallet follow` changes a running node in two steps, so a failure in between drops the series' follow rule and says "nothing was changed"

| Field | Value |
| --- | --- |
| Severity | low |
| Status | open |
| Found on | claude/gui-4-watch @ c9c9e1f (Source bc2e4bc, RED-030 / D136) |
| Component | pkg/cli/wallet.go (`walletFollow`), pkg/rpc/view.go (`POST /v1/follow`) |
| Surface | node (node.json's follow list). Reachable from the GUI's terminal, which runs the same `cli.Main`. |
| Fixed by | _(pending — Source, via GUI/Grey; the GUI needs no change)_ |

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
