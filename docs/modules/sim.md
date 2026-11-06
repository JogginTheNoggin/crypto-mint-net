# pkg/sim

## Summary

A deterministic simulator of many full nodes. Every node is a real
`engine.Engine`, so every rule is the real one. Only the network is
simulated: links with latency, partitions and heals, flooding of what each
engine says to forward, and fetching what a node is missing. Time is
virtual, in milliseconds, and every random choice comes from one seed: the
topology, latencies, payments and the nonces builders try. A run therefore
repeats to the last hash, and a failure can be replayed by its seed. It
exists to run the design's "tests to run". It found a real bug in its
first week (a pending payment stuck after a partition, decision D72), and
another when run at larger sizes (a heavier ledger pruned while it was
being fetched, decision D79).
`poon sim` runs its scenarios from the command line, and
[../simulation.md](../simulation.md) records what they measured.

## The network

- `New(Config{Seed, Nodes, Degree, MinLatency, MaxLatency, Series, Engine})`
  makes the nodes, each following the whole series. The links form a ring
  plus `Degree` random links per node, each with a fixed latency drawn from
  the range.
- `At`, `After`, `Every`, `Run`, `RunUntil`, `Settle` drive the event queue.
  Events at the same time run in the order they were scheduled.
- `Partition(groups...)` cuts every link between groups. A message over a
  cut link is lost. `Heal()` restores the links, and the two ends of each
  one send each other every entry and layer they hold, as reconnecting
  nodes catch up.
- Messages: transactions, entries and layers, which are flooded when the
  engine says to forward; a request for a missing entry and its reply (the
  entry with its layers); a request for layers on a list of entries; and a
  stated weight.
- What a node does that the engine does not:
  - An entry whose parent it lacks, or a layer on an entry it lacks, waits.
    The node asks the sender for the missing entry. A lost request is
    asked again after 2 seconds.
  - After a switch made on requested data (which is never forwarded), the
    node asks the sender for the layers on its new selected ledger, and
    tells its neighbours its new tip (decision D36).
  - After requested work raises its weight without a switch, it states its
    weight to its neighbours. A lighter neighbour on the same tip asks for
    the layers, and one on another tip asks for the entry (decision D59).
- `Node.Private` holds back everything the node would send until
  `Release`: an attacker's secret rival.

This models what reaches each node and when. It does not model the byte
form, peer categories, ranking or chasing. Those are `pkg/p2p`'s, tested
over real connections (decision D71).

## Workloads

- `Wallets`: holders with keys from labels. `ClaimAll` claims every mint
  through random nodes. `Pay(node, serial)` spends one or two portions the
  node shows a holder owning, split or merged, and `RandomPayments` repeats
  that.
- `StartBuilder(node, pool, buildEvery, workEvery, tries)`: the node
  bundles its pending transactions every `buildEvery`. Every `workEvery`
  it tries `tries` nonces where an honest builder would: on the first
  entry that extends its tip and ties with it, else on the tip.
- `Node.BuildOn`, `Node.WorkOn`: entries and work anywhere, for attackers.

## Scenarios

| Scenario | Design test | What it reports |
| --- | --- | --- |
| `Load` | order and agreement, conservation, sibling entries under load | Agreement, conservation at every node, payments settled and their mean wait, the share of work on siblings |
| `Partition` | switch correctness, displaced transactions, agreement after drops | Whether the heavier side won and every node agrees. Of the losing side's payments: carried, lost, or contested (another payment spent the same portion) |
| `Attack` / `AttackSweep` | pre-grinding at checkout | Whether the receiver released at its threshold, and whether the attacker reversed the payment |
| `Sizing` | sizing sweep | Per pool size: settled payments, wait, sibling share, attack success |

In `Attack`, the attacker is a private node with `Share` of all the
network's work. It puts all of that on the attacked pool, while honest
builders spread theirs over every pool. It claims a mint, and once the
claim settles it builds a rival entry that pays itself, on the same parent
the honest payment will extend. It grinds on that entry for `HeadStart`,
then pays the receiver in public. When the receiver's node shows from(e)
at the threshold, the goods are out. When the rival is heavier at the
attacker's own node, it is published, and the attacker keeps grinding in
public until the receiver's node drops the payment or `GiveUp` passes.

## Measures

- `Agree(pool)`: every node has the same tip and the same weight.
- `CheckConservation(node)`: every claimed mint's unspent portions add up
  to the supply.
- `Work(node, pool)`: layer credit on the node's selected ledger, and
  elsewhere.
- `Stats()`: messages sent, delivered, lost to cuts, and fetches.

## Tests

`sim_test.go` checks:

- agreement and repeatability under load;
- load, partition (3 seeds, no uncontested payment lost) and attack
  scenarios;
- that a higher threshold leaves a weaker attacker fewer wins (skipped
  with `-short`);
- the sizing sweep, config errors, and a private node's release.

The scenarios run in seconds. `pkg/cli/sim_test.go` drives `poon sim`.

## Extending

- **A new scenario** is a function building a `Sim`, a workload and its
  measures, returning a JSON-friendly report. Add it to `poon sim` and to
  [../simulation.md](../simulation.md) with its numbers.
- **Another network behaviour** (say, a node that drops entries, or a
  different catch-up) belongs in `node.go`. Keep each message's handling in
  `receive`.
- **A different series**: every scenario takes a `*series.Series`. `poon
  sim` uses the worked example's.
