# ORANGE-049: Round 39 measured; a bounty list filtered by strength costs at most 30 µs

| Field | Value |
| --- | --- |
| Status | proposed |
| Based on | `main` @ `fa8dad7` |
| Area | `pkg/bounty` (`Listable`, `ReceiveFrom`; max strength, D202), `pkg/p2p` (bounty list request 71) |
| Effort | measurements only; no proposals |
| Decided by | Grey |

## Summary

D202 lets a grinder state the strongest offer it works. A payer lists
only offers at or under it, and a grinder drops a listed offer over its
own `bounty.max_strength`. BASE.md asked for the list reply with many
offers filtered by strength.

**1. Answering a list request costs 10 to 30 µs at the most open offers
a payer can have.**

The payer has 256 open offers, the most `bounty.max_open` allows. The
time is one answer: decode the request, `Listable`, and `EncodeList`.

| offers' strengths, in posting order | max 255 (any) | max 40 | max 21 | max 19 (none match) |
| --- | --- | --- | --- | --- |
| 20 to 83, cycling | 25 µs, 64 listed | 31 µs, 64 | 17 µs, 8 | 10 µs, 0 |
| 20 to 83, rising | 27 µs, 64 | 24 µs, 64 | 15 µs, 8 | 11 µs, 0 |
| 83 to 20, falling (matches last) | 24 µs, 64 | 27 µs, 64 | 15 µs, 8 | 10 µs, 0 |
| 20 to 83, random | 25 µs, 64 | 29 µs, 64 | 18 µs, 14 | 10 µs, 0 |
| 52 to 21, falling, first 192 leased | 31 µs, 64 | 30 µs, 64 | 17 µs, 8 | 13 µs, 0 |
| 16 open (the default `max_open`) | 5.6 µs, 16 | 5.2 µs, 16 | 1.4 µs, 2 | 0.8 µs, 0 |

- A request that matches nothing walks all 256 offers and costs about
  10 µs. About 5.5 µs of that is `expire`, which walks the payer's own
  offers on every call; the rest is the walk itself.
- A full list of 64 adds copying the 64 offers (about 4 to 8 µs) and
  encoding them (about 10 µs). It is 6,404 bytes, as in ORANGE-046.
- The filter is one comparison per offer, so the order of the offers and
  how many are filtered out change the cost by a few microseconds only.
  The worst case is the list that fills to 64 late, about 31 µs.
- List requests are counted with discovery, 120 a minute per peer. At 32
  peers each asking 120 times a minute, the payer spends about 0.1 s a
  minute answering, 0.2% of its inbox worker.

**2. On the grinder, an offer over its max strength costs almost
nothing.**

A list of 64 from a new peer, on a grinder with `bounty.max_strength`
40, taken in with `DecodeList` and `ReceiveFrom` for each offer:

| held offers | none over 40 | 32 over 40 | all 64 over 40 |
| --- | --- | --- | --- |
| 256 | 97 µs | 51 µs | 13 µs |
| 4,096 | 417 µs | 242 µs | 17 µs |

An offer over the max is dropped before `expire` and before the held
book is touched, so a payer that ignores the asker's max strength only
costs the grinder the decode. The 4,096 figure with none over is the
`dropHeld(0)` cost of ORANGE-047, unchanged.

## Proposals

None. Both sides follow the number of offers, which `max_open` (at most
256) and `held_offers` bound, not the strengths asked for.

## Measurements

The machine has 4 vCPUs (Xeon with SHA-NI, AVX2 and AVX-512), has 15 GiB
of memory and runs Go 1.27.3. The harnesses were internal test files
outside the commit.

- **The payer:**
  - A book with `post` on, `max_open` 256 and a file in a temporary
    directory, given 256 one-layer offers of the strengths in the table.
  - For the leased row, the first 192 offers each leased by its own
    grinder, so they are not listed.
  - Each figure is the mean of 20,000 answers, with
    `DecodeListReq`, `Listable` and `EncodeList`.
  - The split into `expire`, `Listable` and `EncodeList` was timed apart,
    50,000 times each, in two runs.
- **The grinder:** a book with `take` on, `max_strength` 40, 64 offers
  per peer, filled to `held_offers` with offers of strength 30 from
  peers of 64 each, then given 200 lists of 64 new offers from 200 new
  peers and addresses, with the stated number at strength 50.

## What changed

Only this document and its row in the proposals README.

## Consensus impact

None. This PR changes no code. It measures how a node answers and takes
in bounty lists, which are node-to-node messages outside the ledger. No
verdict changes and no depth limit is added.
