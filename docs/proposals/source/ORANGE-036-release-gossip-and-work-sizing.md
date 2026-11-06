# ORANGE-036: Round 26 measured; release gossip, in-place limits, SLH-DSA and work sizing

| Field | Value |
| --- | --- |
| Status | proposed |
| Based on | `main` @ `439b369` |
| Area | `pkg/series`, `pkg/node`, `pkg/p2p`, `pkg/nodepolicy`, `pkg/plain` |
| Effort | three small changes and three tests; one follow-up for Grey |
| Decided by | Grey; Black in Part D |

## Summary

D168 passes releases between peers, applies some limits in place, adds
SLH-DSA-SHA2-128s and sizes work tries from the series' floor. I measured
each one.

**1. Taking one release checked its witness about seven times. Fixed
here.**

`node.AddRelease` checks the release, then:
- `Policy.AddRelease` builds the series from node.json, which checks the
  kept release, then checks the new one twice (`CheckRelease`, then
  `Series.AddRelease`);
- the engine's series checks it again;
- the running policy repeats the `Policy.AddRelease` step.

That is about eight witness verifies for a new release and seven for a
known one. Every other variable series listed before it in node.json adds
two or three more, because the loop builds each one until it finds the
match. With an ML-DSA-87 or SLH-DSA issuer and eight other series, a known
release cost 11 to 12 ms under the policy lock.

The fix is a small memory in `pkg/series`. It holds the last four releases
of each series that `CheckRelease` checked in full, byte for byte, with the
registry that checked them. Whether a release checks depends only on:
- the series, which its id fixes;
- the registry;
- the release's own bytes.

So the same bytes give the same answer again, and `CheckRelease` returns
it with no verify.

Only releases that checked are held. A forged one, even with the same
part and previous id, has other bytes and is checked in full every time,
so RED-074 still holds. A series built with another registry checks for
itself.

**2. Several peers passing the same release at once were each credited,
and each passed it on again. Fixed here.**

All of the copies get past the host's known-part check before any of them
is taken. `node.AddRelease` then serialises them, and only the first is
taken. `handleRelease` did not look at whether its call took the release,
so each copy was:
- credited as a first delivery;
- logged as "release kept";
- announced to every peer again.

`ReleaseAdded` now carries `Taken` (not printed), which is true when this
call added the release to the series the engine runs. `handleRelease`
drops a copy that was neither saved nor taken, as it drops a known part.

**3. node.json's text check searched the Unicode tables for every
character of every witness. Fixed here.**

`nodepolicy.Load` checks every string in node.json with `plain.Text`.
With 8 KB witnesses written as hex, that was most of what was left of a
known release's cost. `plain.Unsafe` now answers ASCII directly: only the
control bytes below space and DEL are unsafe there. A test checks all 128
ASCII values against the tables.

**4. In-place `-set` is milliseconds, against seconds for a reload. No
change needed.**

Each in-place change costs 4 to 8 ms, most of it saving node.json. The one
exception is shrinking a full book of 65,536 addresses to the floor of
256, which costs 169 ms. A reload of a node with 100,000 stored entries
was 5.4 to 6.3 s in Round 23 (ORANGE-033), and the in-place path touches
no stored entry.

**5. SLH-DSA-128s verifies as fast as ML-DSA-87 but signs a thousand times
slower. No change needed.**

| | verify, 1 core | verifies a second, 4 cores | sign, 1 core | signature |
| --- | --- | --- | --- | --- |
| Ed25519 | 65 µs | 51,600 | 26 µs | 64 B |
| ML-DSA-65 | 222 µs | 16,900 | 0.71 ms | 3,309 B |
| ML-DSA-87 | 337 µs | 9,600 | 0.79 ms | 4,627 B |
| SLH-DSA-SHA2-128s | 332 µs | 12,900 | 346 ms | 7,856 B |

A node checks SLH-DSA at the ML-DSA-87 rate, so the default series' load
estimates hold. Signing takes a third of a second, which only the wallet
pays: it is fine for an issuer's release or a cold key, but slow for a
k-of-n spend. Each signature is 1.7 times the size of ML-DSA-87's, so
storage and bandwidth per spend grow by the same factor.

**6. The work sizing at floors 20 and 24 is sound. One follow-up for higher
floors.**

`poon work`'s default of 2^(floor+2) tries finds a layer at the floor 98.2%
of the time. The 2^(floor+4) cap is reached only with `-tries`.

At floor 24, the default search takes:
- SHA-256: 2.1 s on 4 cores, 8.1 s on 1 core;
- SHA-512/256: 5.9 s on 4 cores, 19 s on 1 core.

At the cap of 2^28 tries, SHA-512/256 on one core takes 76 s, within the
client's 5 minute timeout.

The cap grows with the floor, though, and the timeout does not. A
request at the cap passes 5 minutes on 1 core:
- from floor 26 with SHA-512/256 (2^30 tries, 305 s);
- from floor 28 with SHA-256 (2^32 tries, 519 s).

Below the floors proposed, nothing is needed. If a series ever sets
floors that high, `poon work -tries` at the cap should send `for` with a
time limit, or the client's timeout should follow the tries. I leave
that to Grey.

## Measurements

The machine has 4 vCPUs (Xeon with SHA-NI, AVX2 and AVX-512), and runs Go
1.27.3. The harnesses were test files outside the commit.

### Taking a release (`node.AddRelease`)

The node lists one variable series (part 1 of 8 mints, then parts of 4)
and, in the "8 others" rows, eight more variable series, each with a kept
release. Each figure is the mean of 12 releases:
- "new" is parts 2 to 13, and includes saving node.json, about 1.8 ms;
- "known" is the newest part again, which is the command path, since the
  peer path skips a known part before this call;
- "8 at once" is one new part handed in by 8 goroutines at once.

| issuer | others | new, before → after | known, before → after | 8 at once, before → after |
| --- | --- | --- | --- | --- |
| Ed25519 | 0 | 3.24 → 1.82 ms | 733 → 115 µs | 9.0 → 4.6 ms |
| Ed25519 | 8 | 5.55 → 3.49 ms | 2.45 → 0.47 ms | 22.0 → 7.1 ms |
| ML-DSA-87 | 0 | 7.23 → 3.47 ms | 3.07 → 0.31 ms | 27.8 → 5.9 ms |
| ML-DSA-87 | 8 | 22.95 → 9.30 ms | 11.14 → 2.20 ms | 117.8 → 29.3 ms |
| SLH-DSA-128s | 0 | 5.79 → 3.19 ms | 2.43 → 0.30 ms | 19.0 → 7.8 ms |
| SLH-DSA-128s | 8 | 22.25 → 9.15 ms | 12.36 → 2.39 ms | 108.9 → 33.2 ms |

One full `CheckRelease` (witness and previous id) costs 113 µs with Ed25519,
467 µs with ML-DSA-87 and 315 µs with SLH-DSA, so before the change a known
release cost 6.5 to 7.7 checks, and 28 to 35 with eight other series. After
it, what is left is reading node.json: parsing the JSON and checking its
text, which grows with the witnesses it holds.

With the memory in and before the ASCII change, `plain.Text` was 54% of `node.AddRelease`'s CPU
in a profile of these runs, and JSON decoding was 24%.

`TestSameReleaseAtOnceTakenOnce` hands each of parts 2 to 5 to 8
goroutines at once and checks that exactly one call takes it and saves
it.

### In-place limits

These are `poon policy limits -set` calls through the RPC, on a node with a
running p2p host. Each figure includes loading and saving node.json.

| change | time |
| --- | --- |
| book limits to the ceilings (book empty) | 6.4 ms |
| `p2p.retry_seconds`, book of 65,536 | 8.1 ms |
| `tracking.log_size` | 4.0 ms |
| `seen_size` to 2^22 | 3.7 ms |
| `seen_size` to 1,024 | 4.1 ms |
| book limits to the floors, 65,536 → 256 addresses | 169 ms |
| back to the defaults | 8.4 ms |

### Work tries

These are grinding rates through `node.WorkWith`, 2^22 tries each:

| hash | 1 worker | 4 workers |
| --- | --- | --- |
| SHA-256 | 8.27 M/s | 31.3 M/s |
| SHA-512/256 | 3.52 M/s | 11.3 M/s |

These are the times to run each budget through:

| floor | budget | tries | SHA-256, 4 / 1 cores | SHA-512/256, 4 / 1 cores | finds the floor |
| --- | --- | --- | --- | --- | --- |
| 20 | default 2^(f+2) | 4.2 M | 0.13 / 0.51 s | 0.37 / 1.2 s | 98.2% |
| 20 | cap 2^24 | 16.8 M | 0.54 / 2.0 s | 1.5 / 4.8 s | 99.99999% |
| 24 | default 2^(f+2) | 67 M | 2.1 / 8.1 s | 5.9 / 19 s | 98.2% |
| 24 | cap 2^(f+4) | 268 M | 8.6 / 32 s | 24 / 76 s | 99.99999% |

The chance of a layer at the floor in T tries is 1 − e^(−T / 2^f).

### SLH-DSA

`BenchmarkVerify`-style loops over each algorithm's `Verify` and `Sign`,
`-benchtime 2s -cpu 1`. The 4-core rate is 4 goroutines verifying the
same signature 2,000 times each.

## What changed

- `pkg/series`: `CheckRelease` answers a release it checked in full before
  from a per-series memory of 4, keyed by registry and bytes
  (`wasVerified`, `noteVerified`).
- `pkg/node`: `AddRelease` sets `ReleaseAdded.Taken`.
- `pkg/nodepolicy`: `ReleaseAdded.Taken`, not printed.
- `pkg/p2p`: `handleRelease` drops a copy that was neither saved nor
  taken.
- `pkg/plain`: `Unsafe` answers ASCII without the tables.
- Tests: `TestReleaseCheckedOnce` (pkg/series),
  `TestSameReleaseAtOnceTakenOnce` (pkg/node), `TestUnsafeASCII`
  (pkg/plain).

## Consensus impact

None. Whether a release checks is unchanged: the memory holds only
releases that checked, and a release is answered from it only when its
bytes and the registry are the same. Which peer gets credit and which
copy is passed on changes nothing a node accepts. `plain.Unsafe` gives the
same answer for every rune.
