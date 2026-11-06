# ORANGE-048: Round 38 measured; the start and the wallet no longer grow slower with history

| Field | Value |
| --- | --- |
| Status | proposed |
| Based on | `main` @ `9c4dc92` |
| Area | `pkg/bounty` (`done_size`, D200), `pkg/wallet` and `pkg/keepjson` (`decodeFile`, `encodeFile`, `SplitValid`) |
| Effort | measurements only; no proposals |
| Decided by | Grey |

## Summary

D200 built ORANGE-047's three proposals:
- P1: a start reads the done file only past `done_size`;
- P2: `SplitValid` and `JoinValid` skip a check the caller already made;
- P3: `wallet.Open` and `Save` decode and encode the file directly.

BASE.md asked for the start time with a large done file, and wallet
open and save at 26.7 MB.

**1. A start costs a fraction of a millisecond at any history.**

The book has 16 open offers. Each finished offer in the done file has 16
paid claims, as in ORANGE-046 and ORANGE-047.

| finished offers | done file | Round 37 start | Round 38 start | Round 38, no `done_size` |
| --- | --- | --- | --- | --- |
| 0 | 0 | 0 | 0.19–0.23 ms | 0.19–0.25 ms |
| 2,560 | 11.4 MB | 44–65 ms | 0.19–0.33 ms | 4.9 ms |
| 25,600 | 114 MB | 460–475 ms | 0.15–0.19 ms | 56 ms |
| 256,000 | 1.14 GB | | 0.18–0.44 ms | 672 ms |

- With `done_size`, the start reads the done file's first line and then
  only the bytes past that size. That is the last append, if a crash
  came before the save. History no longer matters.
- Without `done_size`, the whole file is read. That happens with a
  `bounties.json` from Round 37, or with a size the file does not reach.
  It now takes the id from each line's prefix, without decoding the line
  as JSON, which is eight times faster than Round 37's full decode. It
  happens once, since the next save writes the size.
- These figures are with the done file in the page cache, as it is right
  after the node writes it. A cold read of a large done file without
  `done_size` would add the disk's read time, also once.

**2. Opening a 26.7 MB wallet is now 1.3 times a plain decode, down from
2. Saving costs what a plain encode and write cost.**

| step (10,000 keys, 10,000 payments, 26.7 MB) | Round 37 | Round 38 |
| --- | --- | --- |
| decode (`decodeFile`, before: `File.UnmarshalJSON`) | 207 ms | 128–136 ms |
| plain decode of the same fields, for comparison | 101 ms | 95–101 ms |
| `wallet.Open` (read, decode, checks) | | 131–135 ms |
| encode (`encodeFile`, before: `MarshalIndent` of `File`) | 332 ms | 140–152 ms |
| plain `MarshalIndent` of the same fields | 170 ms | 142–143 ms |
| `wallet.Save` (encode, lock, read and compare, synced write) | | 208–240 ms |

- What is left on open is `keepjson`'s one top-level scan, about 31 ms
  at 26.7 MB. It no longer validates the bytes a third time.
- `Save` is the encode (about 145 ms), reading the current file to check
  nobody else changed it (about 10 ms), and the synced write (about
  65 ms). Encoding is now no slower than a plain `MarshalIndent`.
- At 267 MB (100,000 keys and 100,000 payments), open was 2.1 s against
  2.2 s for a plain read, and encode 1.2 s against 1.3 s. That is the
  same cost, within noise.
- The file's bytes are unchanged. For the wallets with no kept keys,
  the harness saved a wallet read from `encodeFile`'s own output and the
  file came back byte for byte at every size. D200's
  `TestOrange047FileBytesUnchanged` checks the case with kept keys.

## Proposals

None. Both costs now follow the work, not the history. One note for
later: the done file only grows, about 4.5 KB per finished offer, so
1.1 GB after 256,000. Nothing reads it whole any more, so it costs disk
space only. An owner may move it aside once its offers are no longer of
interest; the node does not need it except to check the last append.

## Measurements

The machine has 4 vCPUs (Xeon with SHA-NI, AVX2 and AVX-512), has 15 GiB
of memory and runs Go 1.27.3. The harnesses were internal test files
outside the commit.

- **Start:**
  - A done file of N finished offers, written and synced as `archive`
    writes it.
  - `Open`, then one offer posted, leased, claimed and paid, so `Paid`
    moves it to the done file and `bounties.json` records `done_size`.
  - Then 16 open offers.
  - `Open` five times. Then the same, with `done_size` taken out of
    `bounties.json`.
  - Two runs.
- **Wallet:**
  - The ORANGE-046 wallet: K keys and S payments of two legs, each leg
    with a 300-byte transaction and two inputs.
  - Written with `encodeFile`.
  - `Open`, `Save`, `decodeFile` and `encodeFile` timed ten times each,
    against `json.Unmarshal` and `json.MarshalIndent` on a copy of the
    fields without the methods.
  - Two runs.
  - A first run that timed a 267 MB wallet in the same process first gave
    slower `Save` figures for the smaller wallets (up to 600 ms). That is
    GC after the large one, so those runs were redone alone.

## What changed

Only this document and its row in the proposals README.

## Consensus impact

None. This PR changes no code. It measures how a node reads its own
files.
