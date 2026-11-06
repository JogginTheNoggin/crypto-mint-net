# ORANGE-046: Round 36 measured; the bounty market's messages are cheap, but its file only grows

| Field | Value |
| --- | --- |
| Status | proposed |
| Based on | `main` @ `6b56136` |
| Area | `pkg/bounty`, `pkg/p2p` (bounty messages, D196), `pkg/rpc` (`poon bounty work`), `pkg/keepjson` and `pkg/wallet` (D195) |
| Effort | measurements only; four proposals |
| Decided by | Grey; White in Part D for Proposals 1 and 3 |

## Summary

D196 adds the bounty market, where a node pays peers to grind layers on
its entries. D195 keeps the fields a later release adds to the wallet
and payment files (`keepjson`). BASE.md asked for the per-peer cost of
bounty messages, the rate `poon bounty work` states, and `keepjson` on a
large wallet.

**1. A full list of 64 offers is 6.4 KB and decodes in 9 µs. Taking it
in costs 0.4 ms, or 8 ms when 4,096 offers are held.**

| step | cost |
| --- | --- |
| encode a list of 64 offers | 9.7 µs |
| decode it | 9.3 µs |
| take the 64 in (`Book.Receive`), 256 held (the default) | 0.38 ms |
| the same, 4,096 held (the most `held_offers` allows) | 8.4 ms |

- `Receive` runs `expire` over every held offer, and counts the
  sender's offers by walking them all. It does that once for each of the
  64 offers.
- Bounty messages are counted with discovery: 120 a minute per peer.
  They are taken by the node's one inbox worker, the same one that takes
  transactions, entries and layers.
- A node may send a list at any time, not only in answer to an ask. At
  4,096 held, 32 peers each sending 120 lists a minute would take about
  half that worker's time. At the default of 256 it is about 2%.

**2. A claim check costs under a microsecond. Accepting a claim costs a
rewrite of `bounties.json`.**

Checking a claimed layer is one hash, whatever the offer's strength:
0.19 µs with SHA-256, 0.42 µs with SHA-512/256 and 0.58 µs with
SHA3-256. A claim below the target is refused after that one hash.

An accepted claim, a new lease and a lease's extension each write the
whole `bounties.json`, synced to disk, under the book's lock, in the
inbox worker.

**3. `bounties.json` never drops an offer. Each lease or claim gets
slower as old offers pile up.**

A closed offer stays in the file even after every layer is paid, and
nothing removes it. Here each old offer had 16 layers handed in and paid:

| old offers in the file | file | new lease | accepted claim | `Open` at start |
| --- | --- | --- | --- | --- |
| 0 | 0 MB | 0.97 ms | 0.97 ms | 0 ms |
| 256 | 1.6 MB | 18 ms | 19 ms | 8 ms |
| 2,560 | 15.9 MB | 148 ms | 172 ms | 107 ms |
| 25,600 | 159 MB | 2.0 s | 1.9 s | 0.78 s |

- That is about 6 KB for each finished offer.
- A node that posts 16 offers a day has 2,560 after about five months.
  From then on, each lease or claim stops the inbox worker for about
  0.15 s, and so stops every peer's transactions, entries and layers for
  that long.
- Every `expire` also walks every old offer. A refused claim costs 31 µs
  at 2,560 and 0.75 ms at 25,600.
- `max_open` bounds only the open offers, and `budget` only what is owed.
  Neither bounds the history.

**4. `poon bounty work` states a rate 5 to 15% below `poon grind bench`.
That is safe.**

| hash | rate stated (`bountyRate`) | `bountyWork`'s grinding | `poon grind bench` |
| --- | --- | --- | --- |
| SHA-256 | 25.1–26.8 M/s | 26.3–26.4 M/s | 28.6–29.5 M/s |
| SHA-512/256 | 8.4–10.6 M/s | 9.4–10.2 M/s | 10.5–11.2 M/s |
| SHA3-256 | 5.1–6.9 M/s | 6.3–6.9 M/s | 6.5–7.4 M/s |
| BLAKE2b-256 | 13.3–13.8 M/s | 13.8–14.6 M/s | 15.0–15.6 M/s |
| BLAKE3 | 22.2–22.7 M/s | 22.2–24.8 M/s | 24.8–25.7 M/s |

All use 3 threads on this 4-core machine. The rate is measured over
250 ms in 65,536-try calls, and grinding runs in 1 MiB calls. Both start
their workers again for each call. Bench runs one search for its whole
time, so it is a little faster. A rate stated low only makes the lease a
little longer, which is about three times the expected time anyway.
Nothing to do.

**5. `keepjson` doubles the time to read a wallet. Writing a very large
one takes up to four times as long.**

| wallet (keys, sent payments) | file | read, plain → with keepjson | write, plain → with keepjson |
| --- | --- | --- | --- |
| 100, 100 | 0.3 MB | 1 → 2 ms | 1 → 2 ms |
| 10,000, 10,000 | 26.7 MB | 97 → 215 ms | 190 → 215 ms |
| 10,000, 10,000, 10 unknown keys | 26.7 MB | 113 → 208 ms | 119 → 289 ms |
| 100,000, 100,000 | 267 MB | 0.99 → 2.8 s | 1.15 → 4.8 s |

- `Split` decodes the whole file a second time into a map, which copies
  every value, only to find the top-level keys.
- On writing, `File.MarshalJSON` makes the compact form, and
  `MarshalIndent` then checks and indents it again. When keys are kept,
  `Join` also decodes the whole output into a map to see which keys it
  already has.
- The wallet is read and written on most wallet commands, so a wallet of
  10,000 payments now spends about 0.1 to 0.2 s more on each.

## Proposals

**Proposal 1: keep `bounties.json` to what is still owed.** When an
offer is closed and every accepted layer is paid, move it out of
`bounties.json`, for example to an append-only `bounties-done.jsonl`
that is written once per offer and read only by `poon bounty list -all`.
Then the file, each save, each `expire` and the start depend on the open
and unpaid offers, which `max_open` and `budget` bound. White should say
how an older release reads a book that no longer holds an offer it
remembers. It does not need to, since a paid closed offer needs nothing
more.

**Proposal 2: take the save out of the inbox worker.** Mark the book
changed under its lock, and have one goroutine write it, at most once a
second, from a copy. A layer is accepted only after the claim is written
today, so the claim's ack would wait for that write and the worker would
not. With Proposal 1 this matters less, but a save stays a disk sync.

**Proposal 3: make `Receive` cost the same at any `held_offers`.** Keep a
count of offers per peer, and expire held offers at most once a second,
not on every offer. A list of 64 at 4,096 held would go from 8.4 ms to
about what it costs at 256.

**Proposal 4: let `keepjson` find the top-level keys without decoding
every value.** Walk the object with `json.Decoder.Token`, skipping each
known key's value and keeping only the unknown ones. Do the same in
`Join`. That should take most of Section 5's extra time away, with the
same bytes in and out. White should check that it treats a key's case as
`encoding/json` does, as `Split` does now.

## Measurements

The machine has 4 vCPUs (Xeon with SHA-NI, AVX2 and AVX-512), has 15 GiB
of memory and runs Go 1.27.3. The harnesses were internal test files
outside the commit.

- **Lists:** 64 offers with random entries and salts. `EncodeList` and
  `DecodeList` ran 20,000 times. For `Receive`, a book with `take` on was
  filled to `held_offers` from peers of 64 offers each, then given 50
  lists of 64 new offers from 50 new peers.
- **Claim check:** `Layer.Strength` with each hash, a million times.
- **`bounties.json`:** a book with `post` on and a file in a temporary
  directory, synced as the node writes it. It was given N closed offers
  with 16 paid claims each, then 16 open offers. Each figure is the mean
  of 16 `Take`s, 16 accepted `Claim`s, 200 `Listable`s and 200 claims on
  an unknown offer, and one `Open` of the file.
- **Rate:** `bountyRate` and `bountyWork`'s loop of 1 MiB `GrindHash`
  calls for 2 s, against `grind.RunBench` for 2 s with the plan
  `grind.Default.Choose` picks, three times each per hash.
- **`keepjson`:** a wallet `File` with K keys and S sent payments of two
  legs. Each leg had a 300-byte transaction and two inputs. Optionally
  there were 10 unknown top-level keys. `json.Unmarshal` and
  `json.MarshalIndent` ran on `File` and on a copy of its fields without
  the methods, 5 to 50 times each.

## What changed

Only this document and its row in the proposals README.

## Consensus impact

None. This PR changes no code.
- Proposals 1 to 3 change how a node keeps and writes its own market
  book. Proposal 4 changes how files are read and written, with the same
  bytes.
- A bounty layer still enters only through the ordinary layer path, and
  a payment is an ordinary transaction.
- No verdict changes and no depth limit is added.
