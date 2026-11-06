# ORANGE-041: Round 31 measured; the snapshot pause after D182, the per-start check, and port mapping

| Field | Value |
| --- | --- |
| Status | proposed |
| Based on | `main` @ `bf23ddb` |
| Area | `pkg/node`, `pkg/ledger` (snapshots, D178, D181, D182); `pkg/portmap`, `pkg/p2p` (port mapping, D182) |
| Effort | measurements only; two proposals |
| Decided by | Grey; Black in Part D for Proposal 1 |

## Summary

D182 gathers a snapshot in one walk and spaces running snapshots at
`max(snapshot_every, records / 10)`. D181 (RED-088) checks every restored
pool against the snapshot's fingerprint. D182 also adds opt-in port
mapping. I measured them all against Round 30 (ORANGE-040), on the same
machine and with the same histories. Each history has four transactions
and a layer per entry.

**1. The pause for one snapshot is 10 to 15% shorter. It is still about
6 s at a million entries.**

| entries (Ed25519) | gather under the lock, R30 | R31 |
| --- | --- | --- |
| 10,000 | 23–33 ms | 26–30 ms |
| 100,000 | 375–439 ms | 353–373 ms |
| 1,000,000 | 6.4–7.8 s | 5.5–6.5 s |

In a profile at 100,000 entries, the one walk (`SnapshotPrint`) is 95%
of the gather:
- the records are 41%;
- `tidsByNode`, now run once, is 29%;
- the fingerprint is 24%.

What is left still grows with the history, because the gather still
holds the node's lock (ORANGE-040 Proposal 3).

**2. The new spacing takes far fewer snapshots. Building a million
entries was a third faster.**

During a build, every pending transaction is also a record, so these
histories write about six records an entry.

| entries | running snapshots, R30 spacing | R31 | build time, R30 → R31 |
| --- | --- | --- | --- |
| 10,000 | 1 | 1 | 4.9 → 5.6 s |
| 100,000 | 12 | 11 | 56 → 60 s |
| 1,000,000 | about 120 | 33 | 17 min 19 s → 11 min 24 s |

The six minutes saved at a million entries are about 87 fewer gathers of
3 to 6 s each. Below 500,000 records the spacing is
`snapshot_every`, as before.

**3. A start from a snapshot is 25 to 50% slower than in Round 30,
because of the new per-start check.**

| owners | entries | full replay (R31) | from snapshot, R30 | from snapshot, R31 |
| --- | --- | --- | --- | --- |
| Ed25519 | 10,000 | 1.23 s | 0.20 s | 0.24–0.27 s |
| Ed25519 | 100,000 | 13.4 s | 0.88–1.17 s | 1.26–1.28 s |
| Ed25519 | 1,000,000 | 152 s | 9.8 s | 14.6–16.1 s |
| ML-DSA-65 | 10,000 | 4.3 s | 0.42–0.46 s | 0.50–0.73 s |

Every start now takes the restored pool's `Fingerprint`, which costs
3.4–3.6 s at a million entries. That accounts for most of the added time.
A snapshot start is still 10 times faster than a full replay. The full
replays here were also 10 to 20% slower than in Round 30, so part of the
gap is the machine.

The other figures are as in Round 30:
- the snapshot is 323 bytes per held entry;
- the heap after a start at a million entries is 1,754 MiB;
- writing outside the lock takes 0.45 to 0.54 s at a million entries;
- a clean stop at a million entries took 27 s, since it gathers under
  the lock too.

**4. Port mapping costs a few milliseconds when the router answers, and
up to 6 s of waiting when it drops the first two methods.**

These are loopback fakes of each kind of router, with the default
3-second wait per method:

| router | method asked | map | renew | unmap |
| --- | --- | --- | --- | --- |
| PCP | `pcp` or `auto` | 0.1–0.3 ms | 0.1 ms | 0.1 ms |
| NAT-PMP only | `auto` | 0.2 ms | 0.1 ms | 0.1 ms |
| UPnP only | `upnp` | 2.1 ms | 1.0 ms | 0.4 ms |
| UPnP only, port 5351 refused | `auto` | 2.2 ms | 1.5 ms | 0.5 ms |
| UPnP only, port 5351 dropped | `auto` | 6.0 s | 1.6 ms | 0.5 ms |
| nothing answers | `auto` | fails after 9.0 s | | |

- On a real network each request adds the round trip to the router,
  usually under a few milliseconds.
- `auto` tries PCP and then NAT-PMP, which both use port 5351. A router
  with UPnP only that silently drops that port costs 3 s for each before
  UPnP is tried. That happens on each first try and each retry, but not
  on a renewal, which keeps the method that worked.
- All of this runs in the mapping goroutine and holds no node lock. The
  only cost is that the address is advertised 6 s later.

**The dial-back** to the mapped address is one TCP connection and one
hello, which takes a round trip when the router can loop it back. When it
cannot:
- a refused dial fails at once;
- a dropped one waits for the 10-second dial timeout.

Either way the mapping is then removed, within 3 s, and nothing is
advertised.

**The lease and retry pacing fit.**
- **Lease (3,600 s):** a renewal every 30 minutes is one small request. A
  crash leaves a stale mapping for at most an hour. UPnP routers that
  grant only permanent leases are the exception, as D182 says.
- **Retries:** they start at 1 minute and double up to 30 minutes. A
  router that cannot loop back costs one map, one dial of up to 10 s and
  one unmap every 30 minutes, indefinitely. That is cheap, but it adds and
  removes the mapping each time. `poon doctor` already tells the owner to
  set the address by hand, so nothing more is needed.

## Proposals

**Proposal 1: make the fingerprint cheaper, for both the gather and the
start.** The fingerprint is now about a quarter of the gather and about a
third of a snapshot start at a million entries. It is local to the node,
because a snapshot from another build is never used. It could be computed
over the sections at once: the nodes, each range of the chain's tids, and
the unspent portions, each hashed on its own core and then hashed
together. On 4 cores that should cut it by two to three times. The check
it gives (RED-088) stays the same. Black should confirm it is as
sensitive as today.

**Proposal 2: try PCP and NAT-PMP together under `auto`.** Send both
requests on port 5351 at once, and use whichever answers. This takes the
waiting before UPnP from 6 s to 3 s on routers that drop that port, and
removes nothing. It is optional: the wait is only a delay before the
address is advertised.

The larger pause in Section 1 still needs ORANGE-040 Proposal 3, which
gathers outside the node's lock.

## Measurements

The machine has 4 vCPUs (Xeon with SHA-NI, AVX2 and AVX-512), has 15 GiB
of memory and runs Go 1.27.3. The harnesses were test files outside the
commit.

- **Snapshots:** the ORANGE-040 harness, unchanged except that it counts
  the running snapshots written during the build. Its setup:
  - the example series, pool 1, four mint chains;
  - each entry spends each mint once and gets a 16-try layer;
  - `entry_cache_mib` 1 and `no_sync`, with the other settings at their
    defaults.

  The runs:
  - starts: from the snapshot, then with the snapshot removed (a full
    replay), then from the snapshot again;
  - gather: `n.collect` under `n.mu`, three times;
  - the Round 30 figures are from ORANGE-040, on `cf8c859`;
  - profile: five gathers on the 100,000-entry node.
- **Port mapping:** `portmap.Client.Map`, then a renewal and an `Unmap`,
  against the package's own test fakes on 127.0.0.1:
  - a PCP gateway, a NAT-PMP-only gateway and a UPnP IGD;
  - a silent UDP socket for a router that drops port 5351;
  - a closed port for one that refuses it.

  Each map figure is the mean of five, with the default 3-second
  timeout.

## What changed

Only this document and its row in the proposals README.

## Consensus impact

None. This PR changes no code.
- Proposal 1 changes how the node's own snapshot fingerprint is computed,
  not what it covers. Snapshots are never sent or taken from peers.
- Proposal 2 changes only how a node asks its router for a mapping.
- Neither changes a verdict or adds a depth limit.
