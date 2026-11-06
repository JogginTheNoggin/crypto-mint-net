# Cryptography

## Summary

This is the one place that lists every hash, signature scheme, address
policy type, key derivation, cipher and random source the node uses, with
their sizes, standards, where each is used and why it was chosen.

- **A mint chooses its own.** A series names its hash, the signature
  algorithms it allows and the address policy types it allows, by number,
  when it is published. The node ships five hashes, eleven signature
  algorithms and three policy types (decisions D80 to D82, D168, D170). A series picks
  any hash and any subset of the others; `poon crypto` lists them and
  `poon series new` builds checked params from their names.
- **Hashes** (all 32-byte output): SHA-256, SHA-512/256, SHA3-256,
  BLAKE2b-256, BLAKE3.
- **Signatures:** Ed25519; ECDSA over P-256, P-384 and P-521; ECDSA and
  BIP-340 Schnorr over secp256k1; the post-quantum ML-DSA-44, -65 and
  -87; and the hash-based post-quantum SLH-DSA-SHA2-128s and
  SLH-DSA-SHA2-256s (D168, D170).
- **Address policy types:** threshold (K of up to 16 keys), hashlock (show
  a preimage) and k-of-policies (K of up to 16 policies, nested up to 4
  deep), so a series can allow joint accounts, mixed classical and
  post-quantum keys, and the claim side of atomic swaps.
- **Not offered, on purpose:** RSA, SLH-DSA's 192-bit and "f" sets, Ed448, and timelocks. Each is
  explained under [Advised against or deferred](#advised-against-or-deferred).
- **Off consensus**, the node uses HMAC-SHA256 for wallet keys, Argon2id
  (PBKDF2-SHA256 for older wallets) for the passphrase, HKDF-SHA512 to turn a seed into a key of any algorithm,
  AES-256-GCM to seal the wallet seed, CRC-32C to find torn writes in its
  log, and the operating system's random number generator.
- **Where the code comes from:** the Go standard library for everything
  except BLAKE2b and Argon2id (`golang.org/x/crypto`, maintained by the Go team), BLAKE3
  (`zeebo/blake3`), SLH-DSA (`cloudflare/circl`, Cloudflare's audited
  post-quantum library) and secp256k1 (`decred/dcrd` and `btcsuite/btcd`, the
  libraries of two production Bitcoin-family nodes). No primitive is
  implemented by this project (decision D83).

## Supported

Numbers are never reused or renumbered (decision D7). A series that names
a number this node does not know is unreadable here, and is refused, not
misread.

### Hash functions

| Number | Name | Output | Standard | Package |
| --- | --- | --- | --- | --- |
| 1 | `sha256` | 32 bytes | FIPS 180-4 | `crypto/sha256` |
| 2 | `sha512-256` | 32 bytes | FIPS 180-4 | `crypto/sha512` |
| 3 | `sha3-256` | 32 bytes | FIPS 202 | `crypto/sha3` |
| 4 | `blake2b-256` | 32 bytes | RFC 7693 | `golang.org/x/crypto/blake2b` |
| 5 | `blake3` | 32 bytes | BLAKE3 specification | `github.com/zeebo/blake3` |

The series' hash is used for everything in the series: ids, addresses, and
the work in layers. Every hash has a 32-byte output, so ids, strength (at
most 256) and the byte form are the same whatever the choice (decision
D80). `pkg/hash` checks each against a published known answer.

How to choose: SHA-256 has hardware support on most processors and is the
default. SHA-512/256 is faster in software on 64-bit machines. SHA3-256 is
built differently from SHA-2, so a break of one is unlikely to touch the
other. BLAKE2b and BLAKE3 are the fastest in software. Because layer work
is hashing, a hash much faster on special hardware than on ordinary
machines favours whoever owns that hardware; a mint that wants work to
stay with ordinary machines should weigh that.

### Signature algorithms

| Number | Name | Public key | Signature | Standard | Package |
| --- | --- | --- | --- | --- | --- |
| 1 | `ed25519` | 32 | 64 | RFC 8032 | `crypto/ed25519` |
| 2 | `ecdsa-p256-sha256` | 65 | 64 | FIPS 186-5, SEC 1 | `crypto/ecdsa` |
| 3 | `ecdsa-p384-sha384` | 97 | 96 | FIPS 186-5, SEC 1 | `crypto/ecdsa` |
| 4 | `ecdsa-p521-sha512` | 133 | 132 | FIPS 186-5, SEC 1 | `crypto/ecdsa` |
| 5 | `ecdsa-secp256k1-sha256` | 33 | 64 | SEC 2, as Bitcoin uses it | `decred/dcrd/dcrec/secp256k1/v4` |
| 6 | `schnorr-secp256k1-bip340` | 32 | 64 | BIP-340 | `btcsuite/btcd/btcec/v2/schnorr` |
| 7 | `ml-dsa-44` | 1312 | 2420 | FIPS 204 | `crypto/mldsa` |
| 8 | `ml-dsa-65` | 1952 | 3309 | FIPS 204 | `crypto/mldsa` |
| 9 | `ml-dsa-87` | 2592 | 4627 | FIPS 204 | `crypto/mldsa` |
| 10 | `slh-dsa-sha2-128s` | 32 | 7856 | FIPS 205 | `github.com/cloudflare/circl/sign/slhdsa` |
| 11 | `slh-dsa-sha2-256s` | 64 | 29792 | FIPS 205 | `github.com/cloudflare/circl/sign/slhdsa` |

Sizes are in bytes. Every signature signs the witness message
`"PoON-witness-v1" ∥ tid` (decision D9), as each scheme defines:

- **Ed25519** signs the message itself. Verification rejects
  non-canonical signatures and encodings.
- **ECDSA over the NIST curves** signs the curve's paired SHA-2 hash of the
  message. A public key is the uncompressed SEC 1 point; a signature is
  r ∥ s, each the size of a scalar, with s in the lower half of the group
  order. The upper-half twin of a valid signature is refused, so each
  signature has one encoding, which a consensus system needs.
- **ECDSA over secp256k1** signs SHA-256 of the message, with a 33-byte
  compressed public key, the same r ∥ s form and the same lower-half rule.
  Signing uses RFC 6979 nonces.
- **BIP-340 Schnorr** signs SHA-256 of the message (BIP-340 signs 32-byte
  messages) with a 32-byte x-only public key. Verification parses r and s
  strictly.
- **ML-DSA** (the lattice scheme NIST standardised in 2024 for
  resistance to quantum computers) signs the message itself, with an empty
  context string. Signing is hedged: fresh randomness mixed with the key.
- **SLH-DSA** (FIPS 205, the stateless hash-based scheme once called
  SPHINCS+) signs the message itself with an empty context string, in the
  pure form (no pre-hash). Signing is randomised. Its security rests only
  on SHA-256, the most conservative post-quantum choice, at the price of a
  7,856-byte signature and a slow signer: about 0.27 s to sign and 0.28 ms
  to verify here (Ed25519: 40 µs, ML-DSA-65: 150 µs). A key comes from a
  seed as every other algorithm's does: HKDF-SHA512 with the info
  `"PoON-key-v1 slh-dsa-sha2-128s 0"` gives the three 16-byte seeds FIPS
  205's key generation takes. SLH-DSA-SHA2-256s (number 11, D170) is NIST
  category 5, the level of ML-DSA-87: 64-byte keys, 29,792-byte
  signatures, about 0.59 s to sign and 0.77 ms to verify, and three
  32-byte seeds from info `"PoON-key-v1 slh-dsa-sha2-256s 0"`.

How to choose: Ed25519 is the default, small and fast. P-256 suits
holders whose keys live in hardware tokens, phones and cloud key services,
which mostly support NIST curves. secp256k1 lets holders reuse Bitcoin
tooling and hardware wallets. ML-DSA resists a quantum computer, at the
price of much larger keys and signatures: ML-DSA-44 is enough for most
uses, ML-DSA-65 and -87 trade size for margin. SLH-DSA-SHA2-128s suits a key
that signs rarely and must last, such as an issuer's or a reserve's cold
key: it trusts only the hash. A series can allow several,
and an any-of policy lets a holder keep a post-quantum key beside an
elliptic-curve one (see below).

Size limits (D170): there is no size limit shared by every series. A
witness is bounded by its type's declared bounds (below), by its
algorithms' exact key and signature sizes (each signature is read at its
key's exact size), and by its series' witness ceiling (`max_witness`),
the only overall limit, checked before any hash or signature. A series
with no ceiling is valid, but `poon series new` and `poon init` refuse one
(or one above 4 MiB) without `-any-ceiling`, and a node may decline to
add one (`series_ceiling`). An address whose largest witness does not fit
its series' ceiling cannot be made (`CheckSpendable`). Under the reserve
series' 128 KiB ceiling the largest k-of-k is:

| Algorithm | Signature | Largest k under 128 KiB |
| --- | --- | --- |
| Ed25519 and the elliptic-curve algorithms | 64 to 132 B | 16 (the threshold type's bound) |
| ML-DSA-65 | 3,309 B | 16 (about 84 KB) |
| ML-DSA-87 | 4,627 B | 16 (115,754 B) |
| SLH-DSA-SHA2-128s | 7,856 B | 16 (about 126.5 KB) |
| SLH-DSA-SHA2-256s | 29,792 B | 4 (119,506 B) |

`poon crypto -max-witness N` gives the same for any ceiling.

### Address policy types

| Number | Name | Satisfied by |
| --- | --- | --- |
| 1 | `threshold` | K valid signatures from up to 16 listed keys, each a listed algorithm, in strictly increasing key order (decision D7) |
| 2 | `hashlock` | A preimage, at most 1024 bytes, whose hash under a named hash function is a listed digest |
| 3 | `k-of-policies` | Satisfying exactly K of up to 16 listed policies, given in strictly increasing order, nested at most 4 deep |

A single-key address is a threshold policy with K = 1 and one key. The
byte forms are in [protocol/encoding.md](protocol/encoding.md#policies-and-addresses).

- A **hashlock** names its own hash, independent of the series' hash, so
  it can match a lock made on another chain (SHA-256 for Bitcoin's).
- A **k-of-policies** policy holds whole policies, so it can say "Alice's
  key and the preimage" (`AllOf`), "the Ed25519 key or the ML-DSA key"
  (`AnyOf`), or "two of these three groups". The series' rules apply at
  every depth: every policy type and every key algorithm inside must be
  one the series allows. Its cost is bounded by what it lists: each listed
  policy is checked at most once.

Examples and their addresses under every hash are pinned in
`pkg/policy/testdata/known.json`.

## Advised against or deferred

| Option | Why not |
| --- | --- |
| A hashlock on its own | The preimage is public once a spend is seen, so anyone watching the network can copy it into a spend of their own and race the owner. Use a hashlock only inside a k-of-policies policy beside a key. The code allows it alone, since a series may want it for a puzzle, but nobody should keep value behind one. |
| Timelocks (hash time-locked contracts, refunds after a deadline) | Validity in this design depends only on the transaction and the pool state. Nothing in consensus knows the time or a height, and each pool's ledger grows on its own. A timelock needs an agreed clock first; adding one is a design question, not a policy type. Without it, the refund branch of a swap cannot be expressed. |
| RSA | Keys and signatures of 256 to 512 bytes for security no better than P-256's, many encodings and padding schemes to get wrong, and no advantage here. |
| SLH-DSA's other sets (SHA2-192s, SHAKE, and the fast "f" sets) | Left out on purpose (Red, 2026-11-04): 128s and 256s already cover small and strong, and every algorithm a series lists is permanent. There is no size reason any more (D170). SHA2-128s (number 10) and SHA2-256s (number 11) are offered. |
| Ed448 | Not in Go's standard library, and the third-party packages are less used than the ones chosen here. P-521 or ML-DSA cover the need for a larger margin. |
| Falcon (FN-DSA) | Smaller than ML-DSA, but its standard (FIPS 206) is not final, and correct constant-time signing is hard. A candidate once it is. |
| Hashes with output other than 32 bytes, MD5, SHA-1 | Ids are 32 bytes throughout. MD5 and SHA-1 are broken. |

## Where each is used

### In consensus (every node computes these the same way)

All of these are the series' hash `H`, with an ASCII domain tag in front
so that no two kinds of object can share a hash (decision D3). The exact
byte forms are in [protocol/encoding.md](protocol/encoding.md).

| Value | Computed as |
| --- | --- |
| Series id | `H("PoON-series" ∥ params)` |
| Pool id | `H("PoON-pool" ∥ series_id ∥ U64 k)` |
| Address | `H("PoON-address" ∥ policy)` |
| Transaction id | `H("PoON-tx" ∥ body)`, witnesses left out |
| Entry hash | `H("PoON-entry" ∥ parent ∥ tids… [∥ U64 nonce])` |
| Layer hash | `H(entry hash ∥ U64 nonce)`, with no tag: it is the work (decision D6) |
| Strength and credit | Leading zero bits of a layer hash; credit is 2^strength |
| Signed message | `"PoON-witness-v1" ∥ tid` (decision D9) |
| Issuer message | `"PoON-witness-v1" ∥ H("PoON-issuer-v1" ∥ series_id)` (decision D8) |

Signatures and hashlocks appear only in witnesses: one witness per output
of a claim and one per input of a spend, each satisfying the policy of the
address it proves control of.

### In the network (not consensus)

| Use | Primitive |
| --- | --- |
| Request ids | `SHA-256("PoON-request-v1" ∥ U16 type ∥ section ∥ body)`, SHA-256 whatever the series' hash (decision D31) |
| Node ids, request nonces | 32 and 8 random bytes from `crypto/rand` |

Connections are plain TCP with no encryption or authentication (decision
D30). Nothing secret crosses the wire, and every item is checked on
arrival, so the protocol does not depend on the transport. See
[security.md](security.md) for what that leaves open.

### Keys from a seed

Every key of every algorithm comes from a 32-byte seed
(`policy.NewSigner`, decision D81), so a holder only needs to keep the
seed:

- **Ed25519** uses the seed as its RFC 8032 seed.
- **ML-DSA** uses the seed as its FIPS 204 key-generation seed.
- **ECDSA and Schnorr** expand the seed with HKDF-SHA512 (RFC 5869), info
  `"PoON-key-v1 <name> <attempt>"`, into a candidate private scalar, and
  take the first attempt that is a valid scalar (almost always the first;
  for P-521 the top bits are cleared first).

The same seed under two algorithms gives unrelated keys.

### In the wallet

| Use | Primitive | Parameters |
| --- | --- | --- |
| Seed | `crypto/rand` | 32 bytes |
| Key *i* | The wallet's algorithm's key from seed `HMAC-SHA256(seed, "PoON-wallet-<name>-v1" ∥ U32 i)` | decisions D63, D81; for Ed25519 the label is `PoON-wallet-ed25519-v1`, as before |
| Passphrase key | Argon2id (`x/crypto/argon2`) | 3 passes, 64 MiB, 4 lanes, 16-byte random salt, 32-byte key (decision D95) |
| Passphrase key, older wallets | PBKDF2-HMAC-SHA256 | 600,000 iterations, 16-byte random salt, 32-byte key (decision D64); still opened, moved to Argon2id by a new passphrase |
| Sealed seed | AES-256-GCM | 12-byte random nonce, 16-byte tag, associated data `"PoON-wallet-seed-v1"` |

A wallet's keys are all of one algorithm, chosen when it is created
(`poon wallet create -alg NAME`, default Ed25519). The wallet file names its
algorithm, key derivation and work factor, so wallets made before there
was a choice read unchanged.

### On the node's own machine

| Use | Primitive |
| --- | --- |
| Log record checksums | CRC-32C (Castagnoli), to find a torn write, not to resist tampering: the log is replayed through the engine, so nothing on disk is trusted (decision D21) |
| RPC token | 32 bytes from `crypto/rand`, hex, compared in constant time (decision D26) |
| Release checksums | SHA-256, in `SHA256SUMS` (decision D78) |

## Why these

- **Standard, widely reviewed schemes only.** Each is a published
  standard with independent implementations and years of analysis, or
  (ML-DSA) the result of NIST's multi-year post-quantum competition.
- **One encoding per value.** Every verifier here refuses the second
  encoding of a valid signature (low-S for ECDSA, strict parsing for
  Schnorr and Ed25519), so a re-encoded witness cannot be used to make two
  transactions look different. The fuzz targets check it.
- **Few, maintained dependencies** (decision D83). The standard library
  covers six of the nine algorithms and three of the five hashes. The rest
  come from the Go team's own `x/crypto`, and from the secp256k1 code that
  the `dcrd` and `btcd` nodes run in production. `govulncheck` runs on
  every release.
- **Numbered, per series.** A series fixes its choices when it is
  published, and its ids depend on them forever (decision D76). A new
  scheme is a new number that new series can choose; old series keep
  working unchanged.

## Adding a primitive

- **A hash:** add a number, name and constructor to `known` in `pkg/hash`,
  with a known-answer test. Its output must be 32 bytes.
- **A signature algorithm:** implement `policy.KeyAlg`, register its number
  in `Default`, and add a case to `policy.NewSigner` so wallets can derive
  its keys. `TestEveryAlgorithm`, the wallet's `TestEveryKeyAlgorithm`, the
  known-answer file and `FuzzVerify` then cover it.
- **A policy type:** implement `policy.Type` (and `policy.Composite` if it
  holds other policies) and register its number. Declare its cost bound.
- In each case, record the choice in [decisions.md](decisions.md) and
  never reuse a number.
