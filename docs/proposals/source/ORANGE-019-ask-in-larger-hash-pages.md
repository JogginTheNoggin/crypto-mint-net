# ORANGE-019: Ask for hashes in pages of 4,096, as peers already serve

| Field | Value |
| --- | --- |
| Status | proposed |
| Based on | `claude/m10-watch` @ `1ae4d46` |
| Area | `pkg/p2p` (`DefaultConfig().Limits.HashLimit`), `docs/modules/nodepolicy.md` |
| Effort | one line |
| Decided by | Grey, with Red for D120's pacing (Black reviews in Part D) |

## Summary

With ORANGE-018 adopted (D124), a 100,000-entry bare run is reached
instead of never. It still takes 2 minutes, and so does
`TestHeavierLedgerLongBareRun` in CI. The chase spends almost all of it
waiting:
- The chase asks for 256 hashes a page (`hash_limit`), and checks each
  page with one hash request and one layers request.
- 100,000 entries take 985 requests. At the pace of 480 a minute, that
  is 2 minutes, plus D120's 5 s pause every 64 weightless pages.
- The CPU used is 6 s of the 122 s.

Peers already serve 4,096 hashes a page (`ServeHashes`). The asker just
never asks for more than 256. Asking for 4,096:

| Case | `hash_limit` 256 (now) | 4,096 |
| --- | --- | --- |
| 100,000 bare entries | 2 m 2 s, 985 requests | 1.0 s, 155 requests |
| 300,000 bare entries | (about 6 min, not run) | 9.7 s, 448 requests |
| Layers 100,000 entries deep (B1 top-up) | 60 s, 786 requests | 0.14 s, 54 requests |
| `TestHeavierLedgerLongBareRun` | 124 s | 2.2 s |

A page of 4,096 hashes is 128 KB, inside one 1 MiB chunk.

No consensus change. It only changes how many hashes a node asks for at
a time; peers cap it at `ServeHashes` as before.

## For Red: D120's pacing

D120 pauses a chase after `WeightlessPages` (64) pages in a row with no
work. Since a page is now 16 times longer, the pause comes after 262,144
weightless hashes instead of 16,384. Two ways to read D120:

| Choice | 100,000 bare entries | Weightless hashes per pause |
| --- | --- | --- |
| Keep 64 pages (this prototype) | 1.0 s | 262,144 |
| Keep 16,384 hashes (`WeightlessPages` 4) | 31 s | 16,384 |

What a weightless page costs the asker, measured:
- **Memory:** one page, 128 KB instead of 8 KB.
- **CPU:** too little to show in the profile. In the 300,000-entry run,
  the CPU is in taking the entries (`HandleEntryFrom`, 45%), not in
  checking the pages.
- **Requests:** a liar's listing is also held to 480 requests a minute by
  `RequestPace`, so 240 pages, about 1 M hashes, a minute at most.

My suggestion is to keep 64 pages. The cost per page stays small, and
the request pace already bounds the rate. The choice is Red's.

## Measurements

Same machine as ORANGE-012: 4 vCPU Xeon at 2.10 GHz, Go 1.27.3.

How they were taken:
- A test-only harness with two `startNode` nodes and default settings,
  apart from b's `HashLimit`.
- a builds N bare entries with one layer on the last, as
  `TestHeavierLedgerLongBareRun` does.
- The timer runs from `connect` to b holding a's tip and weight, polled
  with `Engine.Tip`.
- CPU is the process's user and system time (both nodes) over that span.

| Case | `hash_limit` | `WeightlessPages` | Time | CPU | Requests b sent | Pauses (weightless, request pace) |
| --- | --- | --- | --- | --- | --- | --- |
| 100,000 bare | 256 | 64 | 2 m 1.9 s | 6.0 s | 985 | 5, 19 |
| 100,000 bare | 1,024 | 64 | 6.0 s | 1.5 s | 301 | 1, 0 |
| 100,000 bare | 4,096 | 64 | 1.0 s | 1.4 s | 155 | 0, 0 |
| 100,000 bare | 4,096 | 4 | 31.1 s | 2.3 s | 155 | 6, 0 |
| 300,000 bare | 4,096 | 64 | 9.7 s | 6.1 s | 448 | 1, 0 |
| Top-up 100,000 deep | 256 | 64 | 60.3 s | 2.2 s | 786 | |
| Top-up 100,000 deep | 4,096 | 64 | 0.14 s | 0.23 s | 54 | |

The CPU at 256 is higher because the node fetches the entries over many
more slices.

## Idea

- `p2p.DefaultConfig().Limits.HashLimit` goes from 256 to 4,096, the
  same as `ServeHashes`. A node policy's `p2p.hash_limit` still sets it.
- The example in `docs/modules/nodepolicy.md` shows 4,096.
- Nothing else changes. The same value already sets the size of
  `findCommon`'s first page, of `checkWork`'s pages and of
  `topUpLayers`' pages, each answered by one layers request.

## Cost and risk

- A page holds up to 128 KB of hashes, and a layers request for it is
  the same size.
- The suite passes with the new default, and so do
  `TestHeavierLedgerHuge`, `TestHeavierLedgerLongBareRun` (2.2 s) and
  `GOEXPERIMENT=simd`. Tests that set a small `HashLimit` keep it.

## Prototype

On `claude/source-orange-r9`; see the commit "ORANGE-019".
