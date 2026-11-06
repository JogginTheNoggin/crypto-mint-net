# ORANGE-045: Round 35 measured; `poon sign` signed each input apart, and skipped header keys

| Field | Value |
| --- | --- |
| Status | proposed |
| Based on | `main` @ `866a4ac` |
| Area | `pkg/wallet` (`SignWith`, `addSigs`; `poon sign`, D193), `pkg/cli` (ledger header, D192 W-20) |
| Effort | one small change and one test; two proposals |
| Decided by | Grey; Black in Part D for the signing change |

## Summary

D193 adds `poon sign`, which signs an unsigned payment with a key kept
off the node. D192 lets `poon ledger import` skip up to 64 header keys it
does not know. I measured both.

**1. `poon sign` signed the same message once for every input. It now
signs once per transaction, four times faster at four inputs a
transaction. Fixed here.**

Every input of a transaction is signed over the same message, its tid.
`SignWith` still called `Sign` for each input the key owns. The wallet's
own `addSigs` did the same, and also derived the key again each time.

| key | legs × inputs | sign, before | with the fix | file, unsigned → signed |
| --- | --- | --- | --- | --- |
| Ed25519 | 256 × 4 | 37 ms | 21 ms | 0.30 → 0.55 MB |
| Ed25519 | 64 × 32 | 69 ms | 19 ms | 0.49 → 0.98 MB |
| ML-DSA-65 | 256 × 4 | 1.03 s | 0.38 s | 4.2 → 11.1 MB |
| ML-DSA-65 | 64 × 32 | 1.82 s | 0.41 s | 8.4 → 22.1 MB |
| ML-DSA-87 | 256 × 4 | 1.12 s | 0.47 s | 5.5 → 15.1 MB |
| ML-DSA-87 | 64 × 32 | 2.33 s | 0.68 s | 11.0 → 30.2 MB |
| SLH-DSA-128s | 16 × 4 | 22.0 s | 5.6 s | 0.02 → 1.0 MB |
| SLH-DSA-256s | 16 × 4 | 43.5 s | 10.5 s | 0.02 → 3.8 MB |

- One signature costs about 0.35 s with SLH-DSA-128s and 0.68 s with
  SLH-DSA-256s. Before the fix, a 64-input payment with SLH-DSA-256s took
  43 s on one core. Now a transaction costs that once, whatever its
  inputs.
- With ML-DSA, most of what is left is reading and writing the file. Each
  input repeats its policy in hex (3.9 KB for an ML-DSA-65 key), and each
  signature is hex (6.6 KB).
- `-show` takes 1–77 ms. With SLH-DSA it is about 50 ms, which is making
  the key.
- Peak heap is about four times the signed file: 131 MiB for the 30 MB
  ML-DSA-87 payment.

**The fix.**
- `SignWith` keeps one signature for each leg and gives it to every input
  of that leg the key owns.
- `addSigs` keeps one signature for each wallet key in a leg, so it also
  derives each key once.

A transaction carries the same witness bytes it would have carried before
when the scheme is deterministic (Ed25519). For a hedged scheme (ML-DSA,
SLH-DSA), the inputs now share one valid signature instead of each having
a different valid one. Either way, each input's witness satisfies its
policy for that tid, as before.

A side effect helps the node too. Below 64 witnesses, the node remembers
each witness that passed under the witness and its tid (`CheckCached`).
So a transaction whose inputs share a signature now costs the node one
signature check per key, not one per input.

**Test.** `TestSignWithOncePerLeg` (pkg/wallet) uses Ed25519 and ML-DSA-65:
- 3 legs of 4 inputs, one of them a 2-of-2 shared with another key;
- the key signs 12 inputs with 3 signings;
- after the second key signs, every witness of every finished leg
  satisfies its policy.

I checked that the test fails without the fix. `TestColdKeyPayment` and
the wallet tests pass unchanged.

**2. Skipping 64 unknown header keys costs at most 15 ms.**

| header | bytes | read |
| --- | --- | --- |
| known keys only | 130 | 3.4 µs |
| 64 unknown keys of 8 bytes | 1.3 KB | 34 µs |
| 64 unknown keys, each key and string value near 4 KB | 512 KB | 1.4 ms |
| the same, each value nested arrays `[[[…]]]` | 512 KB | 15 ms |

The peak heap was 2 to 3 MiB in every case. The 4 KiB bound per key and
per value holds it there. A 65th key is refused before it is read.
Nothing to do.

## Proposals

**Proposal 1: sign the legs on every core.** The legs are independent.
`SignWith` could hand them to `GOMAXPROCS` workers, which on 4 cores
would take SLH-DSA-128s at 16 legs from 5.6 s to about 1.5 s. Each
`policy.Signer` would first need to be checked as safe for concurrent
use. Ed25519 and ML-DSA keep only read-only state, but each SLH-DSA
signer needs a look.

**Proposal 2: keep the policy once per payment file.** Each input of an
unsigned payment repeats its policy in hex: 3.9 KB per input for an
ML-DSA-65 key, and more for a multi-key policy. A list of policies, with
each input naming one by index, would make the unsigned file about half
as large for post-quantum keys. That changes the file format, so White
should say how an older `poon sign` would read it. It is optional: today's
files stay well under the 64 MiB that `poon sign -` reads from standard
input.

Parked: the wallet's `pay` path end to end, which needs a node. The
saving is the same per transaction, plus one key derivation (about 50 ms
with SLH-DSA) per input.

## Measurements

The machine has 4 vCPUs (Xeon with SHA-NI, AVX2 and AVX-512), has 15 GiB
of memory and runs Go 1.27.3. The harness was an internal test in
`pkg/cli`, outside the commit.

- **`poon sign`:**
  - Unsigned payments made in the test: L legs, each a transaction with
    I inputs.
  - Every input is owned by one single-key address of the algorithm.
  - The series hash is SHA-512/256.
  - `signCmd` ran on the file with `-key-file` and `-alg`, end to end
    (read, sign, write back), timed once.
  - Then `-show` ran on the signed file.
  - Peak heap is `HeapInuse`, sampled every 2 ms against the heap after
    two GCs.
  - "Before" is `main` at `866a4ac` and "with the fix" is this branch, on
    the same files.
- **Ledger header:** `readLedgerFile` on an in-memory header with N
  unknown keys and `"entries": []`, 200 times each.

## What changed

- `pkg/wallet/pay.go`:
  - `SignWith` signs each leg once for the key and gives that signature
    to every input of the leg the key owns;
  - `addSigs` does the same for each wallet key, deriving it once.
- Test: `pkg/wallet/orange045_test.go`, `TestSignWithOncePerLeg`.

## Consensus impact

None.
- Each input still carries a witness that satisfies its policy for the
  transaction's tid. The tid, the inputs, the outputs and the witness
  layout are the same.
- No rule asks that the inputs' signatures differ. The node already takes
  the same witness on several inputs, and checks each one against its
  own input's address.
- Proposal 1 changes only how fast a client signs. Proposal 2 changes
  only a client's file.
- No verdict changes and no depth limit is added.
