# GUI-015: Schedule defaults let a layer rate above what default peers take, so a steady grinder gets itself blocked

| Field | Value |
| --- | --- |
| Severity | low |
| Status | fixed (core cap D127 on claude/m10-watch; GUI form on claude/gui-blue, Round 9C), pending Red's verification |
| Found on | claude/gui-4-watch @ 6fae281 |
| Component | gui/web/static/work.js (Schedule defaults: Layer floor, Layer rate), pkg/nodepolicy/nodepolicy.go (`MaxLayerRate`), pkg/tracking/tracking.go (layer budget, over-budget flags) |
| Surface | p2p (this node's standing with its peers) |
| Fixed by | Source Grey (D127: `MaxLayerRate` 50, claude/m10-watch @ 1ae4d46); GUI/Blue on claude/gui-blue (the form) |

## Summary

The new **Schedule defaults** form (D122) sets `layer_floor` and `layer_rate`
with `poo grind schedule set -layer-floor N -layer-rate R`. `layer_rate` is the
largest number of extra layers a second that `layer_floor` makes the node send.
The core accepts 0 to `MaxLayerRate` = **1000**. The form's only guidance is the
placeholder "most layers a second the floor adds (0: default)".

A default peer (`tracking.DefaultConfig`) takes **6,000 layers a minute**, about
100 a second, from one address. Past that it drops the layers and, once in each
one-minute budget window, adds an **over-budget** flag: severity 1, lasting an
hour. With the default limits (TimeOut at 10, Block at 30, a 24 h block after 3
time-outs), a node that stays over budget minute after minute gets:

- about 10 flags, and so a **time-out**, after roughly ten minutes;
- about 30 flags, and so a **block**, after roughly half an hour, from every
  default peer it floods.

A user who wants all their strong layers sent sets a high rate (anything over
about 100, up to the allowed 1000) together with a low floor. Their node then
sends more layers than its peers take for as long as grinding finds them.
Peers drop the extra (strongest first is not guaranteed past the budget) and
then time out and block the node. Its transactions, entries and layers stop
reaching those peers too, not only the extra layers. Nothing in the GUI or
`poo grind schedule set` warns about this.

## Reproduction

1. Run a node with the default tracking settings on its peers, and a series
   that grinding finds layers on quickly (for example a low-difficulty test
   series).
2. Work → Schedule defaults: Layer floor `1`, Layer rate `1000`, then Set.
   Start a grind job on `tip`.
3. On a peer, `poo tracking` / `poo peer` shows over-budget flags for this
   node's address building up each minute, then a time-out, then a block.

## Impact

The user's own node loses its peers for up to 24 hours because of a setting
the GUI offers with no warning. That is a self-inflicted availability loss on
the p2p side, and it needs no attacker. It does not affect consensus: the
floor and rate change only what this node sends, never what any node accepts.
Rated **low**.

## Recommendation

- **GUI.** In the Layer rate help, say that default peers take about 100
  layers a second from one node, and that a node sending more than that for
  long is timed out and then blocked. When the value is over 100, show a
  warning next to the field or a confirm before Set. The form should not
  change the limit itself; it should only make it visible.
- **Core (Source, via GUI/Grey), its call.** Either keep `MaxLayerRate` at
  1000 for private networks whose peers raise their budget, and have
  `schedule set` print a warning when the rate is above the default peer
  budget, or cap the default at the peer budget and require an explicit flag
  to go past it. This is p2p relay policy, not consensus, so it does not need
  a Black review. If Source does change the peer budgets, the cross-check
  belongs there.
- **Test.** A GUI e2e check that Layer rate 1000 shows the warning. A core
  test that `schedule set -layer-rate 1000` prints the over-budget warning.

## Notes — Round 9B verification (held up)

- **GUI-014 GUI part, verified.** One `gate()` covers Remove, Add and Set, and
  each holds it until `panel.reload()` has drawn the table again. Buttons
  drawn during that reload join the gate disabled. A submit made while the
  gate is held is dropped, not queued. `reload()` waits for a run in progress
  and then starts a new one, so the redraw always reads after the change.
  Leaving the view during a change leaves that gate held, but the gate belongs
  to that mount and is rebuilt on the next one. As the fix notes, changes from
  another tab, the terminal or a script can still race in the core. That
  remains the open core part of GUI-014.
- **RED-019 / D120 lease renew, verified.** It retries a failed renew on each
  tick until the lease would have run out, and stops on not_found or a done
  job. One timeout no longer kills a followed grind (the note from R7B).
- **GREEN-032, safe.** A field hidden by `only` is not sent, including in
  the live Run panel. `-h` now returns success. `run()` re-applies `showOnly`
  before it submits.
- **`IsZero`, safe.** A grind section with only `layer_floor` or `layer_rate`
  set is no longer dropped when it is saved.

## Fix (GUI/Blue, Round 9C)

- **The limit is the core's.** Source Grey took this finding in its Round 9
  Part A. D127 lowers `nodepolicy.MaxLayerRate` from 1000 to 50, half of the
  100 a second that default peers take from one node. A section asking for
  more doesn't validate, so `poo grind schedule set -layer-rate`, the RPC
  and the GUI form all refuse it. Since the most poo allows is now under
  the peer budget, the warning or confirm above 100 that this finding
  recommended can no longer trigger, so the GUI doesn't add one.
- **The form says why there is a limit and leaves the number to poo**
  (`gui/web/static/work.js`). The Layer rate field's help now explains that
  peers take only so many layers a second from one node, time out and then
  block a node that keeps sending more, and that poo keeps the rate well
  inside that and refuses a higher one, naming the range. The GUI may import
  only `pkg/cli`, and `pkg/cli` doesn't export `MaxLayerRate`, so the GUI
  doesn't copy the number. That way it can't drift from the core.
- **Test.** `e2e/work.mjs` checks the help text. It submits a Layer rate of
  100000, then checks that the form shows poo's refusal ("layer_rate must be
  0 (the default, N) to M") and that the schedule keeps the rate it had. It
  passes on this branch (core cap 1000), and also on this branch with
  claude/m10-watch @ 1ae4d46 merged in (core cap 50).
- **Asked of Source (via GUI/Grey).** Make the limit visible through poo, so
  the GUI can show the exact range up front and so can a script. For
  example, put `layer_rate_max` in `poo grind schedule`'s reply, or export
  it from `pkg/cli` the way `cli.SecretFlags` is. The GUI would then set
  the field's range from it.
