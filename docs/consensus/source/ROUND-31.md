# Source Round 31: consensus review (Black, Part D)

## Summary

This round does two things:
- It makes taking a snapshot cheaper and less frequent (D182, from
  ORANGE-040).
- It adds opt-in router port mapping, so a node behind a home router can
  take inbound connections (D182). Blue's fixes for Red's three findings
  (D183, RED-090 to RED-092) and Green's status work (GREEN-056) are also
  part of it.

**Result: 10 clear, 0 with conditions, 0 blocked.**

- **No consensus rule or depth limit is added.**
  - A snapshot is still the node's own cache of its log. It is never sent
    to a peer or taken from one.
  - Port mapping changes only whether other nodes can dial this one. That
    is information reaching a node, never which ledger is valid.
  - No rule, byte form or wire message changes. The self-check token
    rides in the hello's existing `Agent` text.
- **The one-pass gather writes the same snapshot with the same
  fingerprint.** Black restored the random-history twin after every step,
  using `SnapshotPrint`, and checked three things each time:
  - its fingerprint equals `Fingerprint`;
  - its records match `Snapshot`'s;
  - the restored pool matches the in-memory pool.

  That was 150 seeds × 300 steps, with and without pruning, at windows of
  1 and 2: 60,231 gathers compared at each window, with 691
  reorganisations. Everything was identical.
- **Snapshot cadence decides only when the node writes its own file.** A
  start after a crash replays more records, through the normal checks.
- **Port mapping can't change what a node accepts.** It touches what the
  node advertises about itself and how it cleans up at the router. The
  only handshake change is in the self-connection branch, which still
  refuses the connection.
- **Fixed series, transactions and entries are unchanged.** The Round 24
  and Round 25 cross-version tests are byte for byte the same on `main`
  and on PR #238.

| Field | Value |
| --- | --- |
| Round | Source Round 31 |
| Consensus | None (D182, D183) |
| Other consensus suggestions | None |
| Base reviewed | `main` @ `4c0f708` (PR #231: D182, with Round 30's PRs merged; GUI #232 on top) |
| Previous base | `main` @ `cf8c859` |
| Blue branches | PR #238 @ `e4e9751` (BLUE-032, D183: RED-090 to RED-092; carries GREEN-056) |
| Red | PR #233 @ `f11adb5` (RED-090 to RED-092) |
| Proposals | ORANGE-041 (PR #235 @ `0b9fe67`); GREEN-056 (PR #234 @ `03b8e9b`, carried in #238); GUI ORANGE-023 (GUI PR #236, a Source ask) |
| Result | 10 clear, 0 with conditions, 0 blocked |

## The one-pass gather (D182, ORANGE-040 proposal 1 in part)

`Pool.SnapshotPrint` works out `tidsByNode` once. It hands the result to
both the record writer and the fingerprint, where before each made its
own pass.

- **The same function on both sides.** `Fingerprint()` is now
  `fingerprint(tidsByNode())`, and the gather calls `fingerprint(byNode)`
  with the map it just used. Replay, restore and `-reverify` all reach the
  same `fingerprint` body, so the header and the checks can't drift apart.
- **The writer only reads the map.** `snapshot` reads `byNode[n]` for each
  selected entry, and neither sorts nor appends to it. So the fingerprint
  sees exactly the tids the records hold.
- **Same moment, by construction.** Before, the header fingerprint came
  from a second `Engine.Fingerprint` call, outside the lock that took the
  records. Now both come from one pass under one lock.
- **The fingerprint stays in the header.** RED-088's per-start check still
  compares each restored pool with it, so a writer/restore mismatch still
  means a full replay. Round 30 cleared either design, and this is the one
  that keeps the check.

### Black's differential

Black changed the twin's restore to gather with `SnapshotPrint`, and
restored it from that snapshot after every step. Each time it checked
that:
- the print equals `Fingerprint()` on the twin and on the in-memory pool;
- every ledger and entry record equals what `Snapshot` writes. The
  unspent records hold the same portions, in the map order `state.ForEach`
  gives. That order was free before D182 as well, because `Restore` reads
  a set and `Fingerprint` sorts;
- the restored pool has that fingerprint, passes `CheckOff`, and agrees
  with the in-memory pool at every later step.

| Restores | Window | Seeds × steps | Gathers compared | Reorganisations | Result |
| --- | --- | --- | --- | --- | --- |
| every step | 1 | 150 × 300, plus pruned | 60,231 | 691 | identical |
| every step | 2 | 150 × 300, plus pruned | 60,231 | 691 | identical |
| every 3 steps | 2 | 150 × 300, plus pruned | 23,888 | 691 | identical |
| every 7 steps | 1,000 (nothing cold) | 150 × 300, plus pruned | 11,213 | | identical |

The run that keeps nothing cold trips two guards: every seed must go cold,
and there must be reorganisations to count. Those are the guards, not
differences. No comparison failed.

## Snapshot cadence (D182, ORANGE-040 proposal 2)

A pool's snapshot is due after `max(snapshot_every, records / 10)` new
records (`snapshotDue`).

- **It decides only when the node writes its own file.** Whether a
  snapshot is used is still decided by the D178 and D181 checks.
- **A late snapshot costs a replay.** A start after a crash replays at
  most a tenth of the log, through `AddEntry` and `AddLayer` with every
  check.
- **Status and doctor.** `snapshots.due` and doctor's "behind" are
  display only.

## Port mapping (D182, D183)

### It is node policy and decides nothing about ledgers

- **Off by default and local to the machine.** `p2p.port_map` and its
  gateway are in `MachineLocal`, so a shared policy file never turns it
  on. The lease is a bounded setting (120 to 86,400 seconds).
- **It only changes what the node advertises.** The router's external
  address reaches `advertised()`, which feeds the hello's contact,
  `Listed`, `isSelf` and the dial skip list. None of these read a ledger,
  an entry or a witness.
- **Inbound limits are unchanged.** `max_inbound`, the per-range book and
  verified limits, and the tracker gate inbound connections exactly as
  before. The accept and handshake paths are the same apart from the
  self branch below.
- **Each way it can fail means less information, not a different
  verdict.** These cases are:
  - an unconfirmed or refused mapping;
  - a router that lies;
  - a carrier-grade NAT address.

  In each, the node can be dialled only at an address set by hand, or
  makes outbound connections alone, as every node did before D182.

### The self-connection branch (RED-090, D183)

`handshake` now calls `sawSelfProbe(theirs.Agent)` on an inbound hello
that carries this node's own id, then returns `errSelf` as before.

- **The connection is still refused.** The return is unchanged, so no
  peer is created, nothing is read after the hello, and no entry or layer
  can arrive on that connection.
- **The token decides only whether an address is advertised.**
  `sawSelfProbe` closes a channel that `confirmSelf` is waiting on, and
  only for a token this node made and is still waiting for. An unknown
  token, or `Agent` text without the prefix, does nothing.
- **No wire change.** The token is hex text in `Agent`, which old nodes
  already treat as free text.

### Cleaning up at the router (RED-091, RED-092)

These change what the node asks its router and when:
- removing a mapping that failed part way;
- refusing lifetimes that are too short, or 0;
- capping the renewal by the lease;
- ending the dial-back with the run.

None of them touches a peer, a ledger or a verdict.

## ORANGE-041 proposals

**Proposal 1: hash the fingerprint in sections, in parallel.** Clear for
consensus. The fingerprint is local, and a mismatch costs one replay. For
the check to stay as sensitive as today:
- each section (the nodes, each range of chain tids, the unspent
  portions) is hashed with its kind and length, and the section hashes
  are joined in one fixed order;
- the ranges are cut at fixed boundaries that don't depend on the core
  count. Otherwise a restart with a different `GOMAXPROCS` would distrust
  a good snapshot and replay for nothing;
- `Fingerprint`, the gather and `-reverify` keep sharing one function;
- `snapshotSemantics` is bumped in the same commit, as its own comment
  asks for a change to what a snapshot holds;
- the twin's restore-and-compare stays, and a test changes one field in
  each section and expects a different print.

**Proposal 2: send PCP and NAT-PMP together under `auto`.** Clear. It
changes only how the node asks its router.

## GUI ORANGE-023: report unused snapshots as a count

GUI Orange asks Source to print `snapshots_unused` in `poo status` as a
count with a few reasons, and not one line per pool.

- **Clear.** It is output only. Which snapshots are used, and the replay
  that follows when one isn't, stay the same.
- **Keep the full list somewhere.** `poo doctor`, the start log or
  `-reverify` should still list each pool's reason. That way a damaged
  file, as opposed to an upgrade, stays visible (GREEN-055's point).

## Clear

| Item | Why it is clear |
| --- | --- |
| D182: `SnapshotPrint` (ORANGE-040 proposal 1 in part); the header fingerprint kept | Same records and the same fingerprint as before (Black's every-step restore runs). The RED-088 start check stays |
| D182: `snapshotDue`, `snapshots.due`, doctor's "behind" (ORANGE-040 proposal 2) | When the node writes its own file. A late snapshot costs a replay |
| D182: `p2p.port_map`, gateway, lease; machine-local; `poo init -port-map` | Node policy, off by default. Whether this node can be dialled is information |
| D182: advertised address from the router, confirmed by dial-back; inbound limits unchanged | What the node says about itself. No inbound cap or verdict changes |
| D183 (RED-090): self-check token in `Agent`; `sawSelfProbe` in the self branch | The self connection is still refused. The token decides only whether to advertise. No wire change |
| D183 (RED-091, RED-092): failed mappings removed, lifetimes checked, renewal capped, stop not delayed | What the node asks its router |
| GREEN-056 (carried in #238): port-map status says why, where and when | Status and doctor |
| ORANGE-041: measurements; proposals 1 and 2 | Measurements. Proposal 1 is clear on the terms above; proposal 2 is router traffic |
| GUI ORANGE-023: unused snapshots as a count | Output only. Keep the full reasons in doctor or the log |
| Red (PR #233): RED-090 to RED-092 | Findings, fixed in PR #238. None touches a rule |

## Items for the owner

- **Nothing needs your approval this round.**
- **Port mapping can't fork anything.** It only lets other nodes reach
  yours. It is off unless you turn it on, a shared settings file can't
  turn it on, and it changes nothing about which ledger a node picks.
- **Faster, rarer snapshots still restore the same ledger.** Black
  restored from the new one-pass snapshot after every step of random
  histories, 60,231 times per run, and it always matched.

How this was checked:
- Read D182 and D183, `ledger/snapshot.go` (`SnapshotPrint`, `snapshot`,
  `fingerprint`, `tidsByNode`), `engine/snapshot.go`, `node/snapshot.go`
  (`collect`, `snapshotDue`, `loadSnapshot`), `p2p/host.go`,
  `p2p/discovery.go`, `p2p/admin.go`, `p2p/peer.go` (`handshake`),
  `p2p/portmap.go` (`advertised`, `confirmSelf`, `sawSelfProbe`),
  `nodepolicy` (`MachineLocal`, `BoundPortMapSeconds`), and the diffs of
  PRs #233 to #236 and #238.
- Black's random-history runs on `4c0f708`, gathering with
  `SnapshotPrint` and restoring every 1, 3 or 7 steps, at windows of 1, 2
  and 1,000.
- The Round 24 and Round 25 cross-version tests: identical on `4c0f708`
  and `e4e9751`.
- `go test ./...` passes on `4c0f708`, #234, #235 and #238. The ledger,
  node, engine, p2p and portmap packages pass under `-race` on
  `e4e9751`.
