# Cryptography

## Summary

This is the one place that lists every hash, signature scheme, key
derivation, cipher and random source the node uses, with their sizes,
where each is used and why it was chosen. Two tables matter most:

- **Consensus** uses exactly two primitives. **SHA-256** hashes every id
  (series, pools, addresses, transactions, entries, layers) and measures
  the work in layers. **Ed25519** signs witnesses. Each series names its
  hash and the signature algorithms it allows by number, so new ones can
  be added later without changing existing series.
- **Off consensus**, the node also uses HMAC-SHA256 and PBKDF2-SHA256 for
  wallet keys, AES-256-GCM to seal the wallet seed, CRC-32C to detect torn
  writes in its log, and the operating system's random number generator
  for seeds, tokens and nonces.

Everything comes from the Go standard library (decision D1). No primitive
is implemented by this project.

## Supported today

### Hash functions

| Number | Name | Output | Standard | Package |
| --- | --- | --- | --- | --- |
| 1 | SHA-256 | 32 bytes | FIPS 180-4 | `crypto/sha256` |

A series names its hash by number in its parameters (`U32 hash number`,
[protocol/encoding.md](protocol/encoding.md)). A series naming any other
number is refused. Every consensus hash goes through `pkg/hash`, so no
other package hard-codes SHA-256 (decision D3).

### Signature algorithms

| Number | Name | Public key | Signature | Standard | Package |
| --- | --- | --- | --- | --- | --- |
| 1 | Ed25519 | 32 bytes | 64 bytes | RFC 8032 | `crypto/ed25519` |

A series lists the algorithms it allows (`List<U32> allowed key
algorithms`). A key of an algorithm the series does not allow makes the
policy invalid. Verification uses the standard library's
`ed25519.Verify`, which rejects non-canonical signatures and encodings.

### Policy types

| Number | Name | What it checks |
| --- | --- | --- |
| 1 | Threshold | K valid signatures from up to 16 listed keys, each a listed algorithm, in strictly increasing key order (decision D7) |

A single-key address is a threshold policy with K = 1 and one key.
Type and algorithm numbers are never reused (decision D7).

## Where each is used

### In consensus (every node computes these the same way)

All of these are the series' hash `H` (today SHA-256), with an ASCII
domain tag in front so that no two kinds of object can share a hash
(decision D3). The exact byte forms are in
[protocol/encoding.md](protocol/encoding.md).

| Value | Computed as |
| --- | --- |
| Series id | `H("PoO-series" ∥ params)` |
| Pool id | `H("PoO-pool" ∥ series_id ∥ U64 k)` |
| Address | `H("PoO-address" ∥ policy)` |
| Transaction id | `H("PoO-tx" ∥ body)`, witnesses left out |
| Entry hash | `H("PoO-entry" ∥ parent ∥ tids… [∥ U64 nonce])` |
| Layer hash | `H(entry hash ∥ U64 nonce)`, with no tag: it is the work (decision D6) |
| Strength and credit | Leading zero bits of a layer hash; credit is 2^strength |
| Signed message | `"PoO-witness-v1" ∥ tid` (decision D9) |
| Issuer message | `"PoO-witness-v1" ∥ H("PoO-issuer-v1" ∥ series_id)` (decision D8) |

Signatures (Ed25519) appear only in witnesses: one witness per output of a
claim and one per input of a spend, each satisfying the policy of the
address it proves control of.

### In the network (not consensus)

| Use | Primitive |
| --- | --- |
| Request ids | `SHA-256("PoO-request-v1" ∥ U16 type ∥ section ∥ body)`, SHA-256 whatever the series' hash (decision D31) |
| Node ids, request nonces | 32 and 8 random bytes from `crypto/rand` |

Connections are plain TCP with no encryption or authentication (decision
D30). Nothing secret crosses the wire, and every item is checked on
arrival, so the protocol does not depend on the transport. See
[security.md](security.md) for what that leaves open.

### In the wallet

| Use | Primitive | Parameters |
| --- | --- | --- |
| Seed | `crypto/rand` | 32 bytes |
| Key *i* | Ed25519 key whose seed is `HMAC-SHA256(seed, "PoO-wallet-ed25519-v1" ∥ U32 i)` | decision D63 |
| Passphrase key | PBKDF2-HMAC-SHA256 | 600,000 iterations by default, 16-byte random salt, 32-byte key (decision D64) |
| Sealed seed | AES-256-GCM | 12-byte random nonce, 16-byte tag, associated data `"PoO-wallet-seed-v1"` |

The wallet file names its key derivation and work factor, so a stronger
derivation can be added without breaking existing wallets.

### On the node's own machine

| Use | Primitive |
| --- | --- |
| Log record checksums | CRC-32C (Castagnoli), to find a torn write, not to resist tampering: the log is replayed through the engine, so nothing on disk is trusted (decision D21) |
| RPC token | 32 bytes from `crypto/rand`, hex, compared in constant time (decision D26) |
| Release checksums | SHA-256, in `SHA256SUMS` (decision D78) |

## Why these

- **SHA-256** is the most widely implemented and reviewed 256-bit hash. It
  has hardware support on most processors, which matters because layer
  work is hashing, and a hash that is fast for everyone keeps work fair.
  Its 32-byte output gives 128-bit collision resistance.
- **Ed25519** has small keys and signatures, fast verification, and no
  per-signature randomness to get wrong. Its encodings are canonical,
  which a consensus system needs. It needs no parameters chosen by the
  project.
- **Both are in Go's standard library**, so the node has no third-party
  cryptographic code to audit or pin (decision D1).
- **Numbered, per series.** A series fixes its hash and algorithms when it
  is published, and its ids depend on them forever (decision D76). A new
  hash or signature scheme is a new number that new series can choose;
  old series keep working unchanged.

## Adding a primitive

- **A hash:** add a number and a function in `pkg/hash` (`Lookup`), with a
  known-answer test. Only new series can name it.
- **A signature algorithm:** implement `policy.KeyAlg` and register its
  number (see [modules/policy.md](modules/policy.md)), with test vectors.
  For the wallet, derive its keys under a new label (see
  [modules/wallet.md](modules/wallet.md)).
- **A policy type:** implement `policy.Type` and register its number.
- In each case, record the choice in [decisions.md](decisions.md) and
  never reuse a number.

Post-quantum signatures are the likeliest addition. Ed25519, like every
elliptic-curve scheme, would fall to a large quantum computer. A series
can allow a new algorithm alongside Ed25519, and since addresses are
hashes of policies, holders can move to policies that use it.
