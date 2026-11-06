# RED-070: The part-id cache holds every id up to k, so one release of a huge part takes all the memory

| Field | Value |
| --- | --- |
| Severity | Medium (phase 2 is where it reaches the network; today it is the issuer's own tools and any file they sign) |
| Status | fixed |
| Found | Source Round 24 Part B, on `main` @ `08404df` (D163) |
| Area | `pkg/series` (`PartID`, `partChain`, `CheckRelease`, `Unsigned`), `pkg/cli` (`poon series parts`, `poon series release`) |
| Raised by | Red, and Source Green saw it too (measured with the poon binary) |
| For Black | **Read the note on node policy below.** The fix is memory and speed, not a refusal. A node policy that refuses a part number it will not work out would let two nodes that hold the same release disagree about whether it is valid, which is a hard fork by the owner's definition. |
| Fixed by | `claude/project-thread-ggo8dt` @ `c05286b` (BLUE-025) |

## What is wrong

`part_id(k)` is a chain: `H("PoO-part" ∥ series_id ∥ k ∥ part_id(k − 1))`. D163's
`Series.PartID(k)` works it out from part 1 and keeps every id in one
slice (`partChain.ids`, 32 bytes each, grown by `append`), all while holding
the cache's mutex:

```go
c.mu.Lock(); defer c.mu.Unlock()
for uint64(len(c.ids)) < k { ... c.ids = append(c.ids, s.partID(n, prev)) }
```

So part k costs k hashes, **k × 32 bytes of memory (up to twice that while
`append` doubles)**, and every other `PartID` call waits behind it. A
series with no `max_parts` allows any k up to 2⁶⁴ / M.

`CheckRelease` checks the issuer's witness first, so only the issuer can
make it work out a long chain (the code says so). That is enough:

- The issuer is the one party a follower of the series does not control. A
  careless or hostile issuer signs a release of part 2³⁶ and publishes it.
- In phase 2 a node learns releases from peers and from files. One valid
  release of a huge part makes every follower run out of memory, not just
  spend time.

## How I checked it

A variable series (40 mints, pools of 4, parts of 8, no limit), built with
`poon series new -variable`.

1. A local call: `poon series parts v.json -from 30000000 -to 30000001`.
   After 10 seconds the process held 1.5 GB resident and was still growing
   (3.1 GB virtual). Source Green measured 2.9 GB and 23 s at part 20,000,000.
2. A release signed by the issuer's key for part 2³⁶ (a file of 459 bytes,
   built with the series package and the issuer's seed), then
   `poon series release v.json -check huge-release.json`:
   1.5 GB resident after 8 s, 2.7 GB after 14 s, still growing. It is
   killed by the memory limit before it ends (2³⁶ ids are 2 TB).

`poon series release v.json -part 4000000000` (the issuer signing, by
mistake) does the same through `Unsigned`.

## What could happen

- Phase 2, a follower node: one issuer-signed release of a huge part, from
  a peer or a file, takes all memory of every node that follows the series.
- Phase 2, a transaction that names a mint in part k makes the node work out
  `part_id(k)` before it can check anything, unless it first checks that a
  release of part k (or later) is known. Anyone can sign a transaction
  naming serial 2⁶⁴ − 1. The order of those checks matters (see below).
- Today: a person checking a release file from an issuer, or listing parts
  far out, loses their machine's memory.

## Suggested fix (Blue's call)

1. **Do not keep every id.** Keep a checkpoint every 4096th part and the
   newest id, and work out the rest from the nearest checkpoint below. The
   ids are the same; memory is `k / 4096 × 32` bytes. Better for a single
   `PartID(k)` that is far out: stream the chain with a running hash and
   keep nothing but the checkpoints it passes.
2. **Do not hold the mutex for the whole chain.** Work from a checkpoint
   under a short lock, or let each caller hash its own stretch, so a long
   request does not stop every other check.
3. **Keep the work of a release honest to its cost.** A release of part k
   proves every part before it, so a new node must work out k hashes once:
   that is time (2²⁸ hashes take about a minute), not memory, and it is fine.
4. **Phase 2 order of checks** (note for Blue's and Green's design): a
   transaction naming a part must be checked against a *known release*
   before `PartID` is asked for it; a part no known release reaches is
   "not released yet", answered with no hashing.
5. **No refusal by size.** A node policy cap on how far to work out ids
   (the design note foresees one) must not be a rule that makes the node
   refuse a valid release: nodes with the same information would then
   disagree, which the owner's definition calls a hard fork. A cap may
   only make a node *slow* (work in the background, answer "not yet known"
   until it is done) and never answer "invalid". Black should review any
   such setting.

Test: `PartID` of a part 50 million out stays under, say, 16 MB of heap and
returns the same ids as the cached version for k up to 100,000.

## Not tested

Phase 2 (nothing in the node reads a release yet; a variable series is not
followed). The speed of a hash chain of 2⁴⁰ steps (hours; no memory).

## Resolution

Fixed (D165). It builds on ORANGE-034 (#157, merged into this branch) and
follows Red's points 1, 2 and 5.

- Orange's marks every 1,024 parts keep the same ids in k/1,024 of the
  memory. That is still 2 GiB at part 2³⁶, so the chain now also thins
  its marks. When there would be more than 65,536, it drops every other
  one and doubles the stride.
  - Its memory stays under 2 MiB whatever part is asked for.
  - Part k up to the newest costs at most one stride of hashes from its
    mark.
  - The last answer is kept, so parts listed in order cost one hash each.
- The lock is held for 16,384 hashes at a time. A call far out lets
  others in between, so a part already known is answered while a long
  call runs.
- No refusal by size. A release of part k costs k hashes once: time, not
  memory. Point 4 (check a known release before `PartID` in phase 2) is
  written into D165 for the phase 2 design, since phase 2 is not built.
- Tests:
  - `TestPartChainBoundedAndExact`: small strides, so the marks thin out.
    Every id equals the naive chain, backwards, in order and from 8 callers
    at once, and also on a fresh chain.
  - `TestPartIDFarOutInBoundedMemory`: part 8 million with heap growth
    under 16 MiB (the old chain held 256 MB), and a known part answered
    within 100 ms while a call for part 5 million runs.
