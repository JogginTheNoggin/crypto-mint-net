# Simulation results

## Summary

What the simulator ([modules/sim.md](modules/sim.md)) measured at
milestone 7, re-run in full at milestone 8 on networks of up to 128 nodes. All runs use the worked example's series: 8 mints of 100
units, pool size 4 unless swept, minimum layer strength 0. The network
has 16 nodes, 4 of them builders, each trying 64 nonces per pool every
500 ms, with link delays of 20 to 200 ms. Every number can be reproduced
with the `poon sim` command beside it. The findings:

- Agreement and conservation held in every run (30 seeds each of load and
  partition).
- After a partition the heavier side always won, and every losing-side
  payment that conflicted with nothing came back, but only once a bug was
  fixed (D72).
- A receiver's threshold works. Raising it from 1,000 to 1,000,000 cuts a
  10% attacker's wins from 57% to 3%. An attacker with half the work still
  wins most races with a 2-second head start and a minute to try.
- Larger pools defend better against an attacker who concentrates on one
  pool.
- Latency is what makes sibling work: about 0% of work is lost at 50 ms,
  9% at up to 1 s, and 35% at up to 3 s.
- Networks of 32 and 64 nodes split for up to 2 minutes all came back
  together on the heavier side, but only once a pruning bug was fixed
  (D79). Before the fix, a node catching up could drop each entry of the
  heavier ledger before its work arrived, and half of a 24-node network
  stayed on the losing side for good. The same bug was behind the splits
  milestone 7 saw with K = 1 or 2: with it fixed, every K tried agrees.
- Larger networks change little else: at 64 nodes the attack numbers are
  close to those at 16, and loads of 64 and 128 nodes settle as fast.

These are measurements of this model, not of a real network: see
[Limits](#limits).

## Agreement, conservation and partitions

`poon sim load -runs 30` and `poon sim partition -runs 30`:

- Every node ended on the same tip with the same weight.
- Every claimed mint's portions summed to its supply.
- The heavier side of each partition won.
- Every losing-side payment that conflicted with no other payment was
  selected again after a rebuild.

Before decision D72's fix, 1 run in 30 lost such a payment. These numbers
are the same, to the last payment, on the milestone 8 code: the runs are
deterministic, and D79 changes nothing at this size.

## Attacks with a head start

`poon sim attack -runs 30 -share Q -threshold T`, with a 2 s head start and
60 s to keep trying after the goods are released. The attacker's share is
of all the network's work, all of it put on the attacked pool. Each cell
is the share of released payments it reversed. The last column is the
mean time to reach the threshold.

| Threshold | 10% | 20% | 30% | 50% | Mean release |
| --- | --- | --- | --- | --- | --- |
| 1,000 | 0.57 | 0.80 | 0.93 | 0.97 | 1.6 s |
| 10,000 | 0.27 | 0.67 | 0.87 | 0.93 | 6.8 s |
| 100,000 | 0.20 | 0.43 | 0.67 | 0.87 | 41 s |
| 1,000,000 | 0.03 | 0.10 | 0.23 | 0.87 | 4.7 min |

How to read it:

- Credit is heavy-tailed: one lucky layer can be worth more than many
  ordinary ones (the design's "variance"). So even a small attacker
  sometimes wins against a low threshold.
- Each tenfold rise in threshold takes several times longer to reach and
  cuts a weak attacker's chances sharply.
- Against an attacker with half the work, no threshold here is enough.
  That matches the design: weight is contestable by anyone with more work.
- These attackers may try for a full minute after release. A shorter
  `-give-up` lowers every number.

## Sizing sweep

`poon sim sizing -runs 30 -share 0.3 -threshold 5000`: the same 8 mints,
payment rate and total builder power, with the pool size swept.

| Pool size | Pools | Settled | Mean wait | Sibling share | Attack won |
| --- | --- | --- | --- | --- | --- |
| 1 | 8 | 175 | 852 ms | 0.000 | 0.97 |
| 2 | 4 | 189 | 823 ms | 0.000 | 0.90 |
| 4 | 2 | 184 | 889 ms | 0.004 | 0.83 |
| 8 | 1 | 184 | 872 ms | 0.000 | 0.53 |

How to read it:

- With builders' power split across pools, a small pool gets a small
  slice of the defence, while an attacker can put everything on it.
- At this load, wait and sibling work barely change with the pool size.
  The contention side of the trade-off needs a busier network to show:
  try `-pay-every 50`.

## Latency and sibling work

`poon sim load -runs 5 -pay-every 100 -min-latency A -max-latency B`:

| Link delay | Mean wait | Sibling share | Settled of sent |
| --- | --- | --- | --- |
| 10–50 ms | 0.7 s | 0.000 | 2632 / 2705 |
| 20–200 ms | 0.9 s | 0.004 | 2436 / 2562 |
| 200–1000 ms | 2.1 s | 0.089 | 1758 / 2229 |
| 1–3 s | 3.7 s | 0.351 | 812 / 1873 |

Slow links make builders bundle and work on entries that lose, and fewer
payments settle in the same time. This is the "sibling entries under load"
cost, and it grows quickly once delays approach the build period (2 s).

## K under partitions

`poon sim partition -runs 20 -k K`, before and after decision D79:

| K | Agreed (M7) | Uncontested lost (M7) | Agreed (M8) | Uncontested lost (M8) |
| --- | --- | --- | --- | --- |
| 1 | 17 / 20 | 101 | 20 / 20 | 0 |
| 2 | 17 / 20 | 101 | 20 / 20 | 0 |
| 8 (default) | 20 / 20 | 0 | 20 / 20 | 0 |

Milestone 7 read the first two rows as a cost of a small K. They were a
bug: a node catching up takes the other side's ledger one entry at a time,
and its first entries alone were lighter than the K branches it kept, so
each was pruned on arrival and the next had no parent. A small K made that
likely. With the newest entry kept while it is a leaf (D79), every K
tried agrees. This is the design's "agreement after drops" test. K stays 8
(D74).

## Larger networks

Milestone 8 ran the scenarios at more nodes, with a quarter of them
building and the attacker's share still of all work:

| Run | Result |
| --- | --- |
| `load -nodes 64 -builders 16 -runs 10 -duration 300000` | 10 / 10 agree and conserve; 9768 of 9875 payments settled; mean wait 0.73 s; sibling share 0.004 |
| `load -nodes 128 -builders 32 -degree 3 -runs 3 -duration 300000` | 3 / 3 agree and conserve; 2940 of 2971 settled; mean wait 0.63 s; sibling share 0.005 |
| `partition -nodes 32 -builders 8 -runs 20 -split 60000` | 20 / 20 heavier side won and agree; 1198 carried, 0 uncontested lost |
| `partition -nodes 32 -builders 8 -runs 10 -split 120000` | 10 / 10; 1314 carried, 0 lost |
| `partition -nodes 64 -builders 16 -runs 10 -split 120000` | 10 / 10; 1321 carried, 0 lost |
| `partition -nodes 64 -builders 16 -runs 5 -split 60000 -k 1` | 5 / 5; 342 carried, 0 lost |
| `attack -nodes 64 -builders 16 -runs 20`, share 10% / 30%, threshold 10,000 | won 0.25 / 0.70 (0.27 / 0.87 at 16 nodes) |
| the same at threshold 100,000 | won 0.20 / 0.75 (0.20 / 0.67 at 16 nodes) |

Before D79, the 64-node partition did not finish in 45 minutes, and a
24-node one split for 60 seconds never agreed. With it, each run takes
seconds. More nodes spread the same work over more paths, so a payment
reaches the threshold sooner at 64 nodes (2.0 s against 6.8 s for 10,000),
which gives an attacker less time.

## Limits

- **The network is a model.** It has fixed latencies and flooding, and
  catch-up is simplified. There is no bandwidth limit, no peer ranking and
  no chasing (`pkg/p2p` has those and tests them over real connections).
- **Builders are uniform.** Each has fixed tries per period. Real power is
  uneven.
- **Runs are short.** They cover minutes of virtual time, on networks of
  at most 128 nodes, on one small series. Numbers for a real series need
  its own parameters: `pkg/sim` takes any series.

The provisional defaults of D12 and D62 are kept. Since D79, the K
measurement no longer favours one K over another. The peer management defaults (D62) are not exercised here,
since the simulator does not model peer ranking.
