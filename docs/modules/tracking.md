# pkg/tracking

## Summary

The tracker records what a full node's peers do, so the node can rank the
useful ones and act against the ones that misbehave. It follows the
design's "Peer Tracking and Telemetry". It keeps two scopes. **Flags** are
node-wide, per address: each offence adds a flag with a severity and an
expiry, and the flag total decides whether an address may be a priority
peer, is timed out, or is blocked. **Measures** are per mint policy: pings
answered, round trips, first deliveries, requests answered, chase outcomes
and more, which add up to a peer's **points**. A **template** (basic,
standard, detailed or a custom one) decides what is recorded for each
peer. Nothing the tracker records is ever sent to other nodes, and every
value here is node policy. The tracker reads no clock: each call passes
the time, so tests and the simulator drive it. It is saved as
`tracking.json` in the data directory.

## Keys

Records are kept by **key**, from `Tracker.Key(addr)`: the IP address of a
remote node (so one machine is one record whatever port it uses), the
lowercased host name for a name, and the full `host:port` for loopback, so
a local test network of many nodes on one machine keeps them apart.
`LoopbackAsOne` treats every loopback port as one address (decision D50).

## Flags, timeouts and blocks

| Offence | Severity | Expires | Raised by `p2p` when |
| --- | --- | --- | --- |
| `invalid-item` | 5 | 24 h | a pushed entry or layer breaks the rules |
| `invalid-message` | 5 | 24 h | a message cannot be decoded (also closes the connection) |
| `over-budget` | 1 | 1 h | a peer sends more of a kind than its budget per window |
| `repeat-flood` | 2 | 1 h | it repeats more held items or request ids than the repeat limit |
| `false-branch` | 5 | 24 h | a chase shows its branch is provably false |
| `contradiction` | 3 | 24 h | it contradicts itself |

Each offence has a `Rule{Severity, Expiry, Action}`; an action of
`timeout` or `block` applies at once whatever the total. Then the limits
(`Limits`) apply to the live flag total:

- at **5** an address may not be a priority peer;
- at **10** it is **timed out** for 10 minutes: disconnected and refused,
  its categories held for its return;
- at **30** it is **blocked** for 24 hours; so is an address due a fourth
  timeout after **3** within the flag history.

A block can also cover a whole network range (`BlockRange`). `Status`
reports all of it; `Allowed` is false while timed out or blocked. The
owner can time out, block and lift by hand (`TimeOutFor`, `BlockFor`,
`Lift`), and clear an address's flags, measures or whole record (`Clear`).

When a timed-out peer returns, `Held` gives back the categories it had,
and by default counts the pings it missed while away, one per ping period
(turn this off with `NoTimeoutMisses`).

## Budgets and repeats

`Budget(addr, kind, now)` counts items of one kind per window (one minute
by default): transactions 1200, entries 1200, layers 6000, requests 600,
discovery messages 120. `Spend(addr, kind, n, now)` counts a message
that costs n units: the `revert` budget (200,000 a minute) counts what a
pushed or requested entry makes the node revert and apply to check it,
one unit per entry and one per transaction (RED-011, RED-012, D111,
D114). Over budget, the message is dropped and the
address flagged once per window. `Repeat` counts held items and repeated
request ids against `RepeatLimit` (600 a minute). `StartChase` counts new
stated weights above this node's own against `ChaseBudget` (30 per ten
minutes).

## Templates

| Template | Records |
| --- | --- |
| `basic` | offences, budgets, repeats, chases; last heard, pings, round trips, first deliveries, requests, chase outcomes |
| `standard` | basic plus contradictions, connections, request outcomes, items, reply timing, stated weights, denials, cost |
| `detailed` | standard plus a log of events and traffic per message type |

The node-wide settings are offences, budgets, repeats, chases,
contradictions, connections, log and traffic; the rest are per mint
policy. A custom template is `Custom{From, On, Off}`: a base template and
settings added and removed. Templates may build on each other, but not in
a cycle, and the supplied three cannot be redefined.

The template for an address is, in order: its own (`SetTemplate`, from
the owner), the one the node policy names for it (`Peers`), everything
its groups' templates record together (groups: priority, backup,
candidate, unlisted, inbound, outbound; set by `p2p` through
`SetGroups`), or the default (basic). An
address whose flag total reaches `EscalateAt` (5) is recorded with at
least the standard template, so a suspect is watched more closely.

## Points

`Points(addr, section, only, now)` adds, for what the template records,
within the activity window (one hour):

- pings: 2 × answered / sent;
- round trip: 1 / (1 + average seconds);
- first deliveries: 1 × log2(1 + count);
- requests: 2 × answered / (answered + timed out);
- reply timing: 1 / (1 + average seconds);
- chases: 2 × (confirmed − nowhere) / chases;
- denials: −2 per priority-peer denial later shown wrong;
- cost: −0.1 per MiB served beyond what it sent, −0.5 per large request.

`only` restricts the sum to some measures: the ranking compares a backup
with a priority peer only on pings, round trips and first deliveries,
which both have (the design's fair comparison). A new peer starts at
`StartingPoints`.

## Retention

Expired flags go after the flag history (7 days); the detailed log keeps
an hour, at most 256 events; at most 8192 addresses are kept, the least
recently seen dropped first. A node policy sets each one
(`tracking.flag_history_seconds`, `tracking.log_seconds`,
`tracking.log_size`, `tracking.max_addresses`, D159).

## API

`New(cfg, path)`, `Save`, `Key`, `Status`, `Offend`, `TimeOutFor`,
`BlockFor`, `BlockRange`, `Lift`, `Hold`, `Held`, `Clear`, `Forget`,
`SetGroups`, `SetTemplate`, `Settings`, `Budget`, `Spend`, `Repeat`, `StartChase`,
`Connection`, `Message`, `Heard`, `Ping`, `FirstDelivery`, `Item`,
`Request`, `Stated`, `ChaseOutcome`, `Denial`, `Denials`, `Cost`, `Points`,
`LastHeard`, `Report`, `Keys`, `Ranges`. `Counter` is a windowed counter
of eight buckets, `Average` an exponentially weighted average.

## Tests

`tracking_test.go`: windowed counters and averages, templates (custom,
cycles, redefinitions), keys, flags leading to loss of priority, timeouts
and blocks, repeated timeouts, immediate actions and range blocks, budgets
and repeats, points and fair points, the standard measures, the choice of
template by peer, group and escalation, the detailed log, held categories
and missed pings, clearing and forgetting, a save and load, and trimming.

## Extending

A new offence is a new `Offence` constant and a default `Rule`; a new
measure is a new `Setting` bit (choose its scope), a field in `Measures`,
a recording call, and, if it counts, a term in `points` with a weight in
`Weights`.
