# Byte formats (byte form v1)

## Summary

This document defines the exact bytes of every object the node hashes or
sends: series parameters, policies and addresses, witnesses, transactions,
entries and layers, and how each id is computed from them. An independent
implementation that follows it must reproduce the published test vectors in
[`pkg/vectors/testdata/vectors.json`](../../pkg/vectors/testdata/vectors.json).
Changing any form here changes every id in every series, so a change needs a
new entry in [decisions.md](../decisions.md) and regenerated vectors. The
rationale for the forms is in decisions D2 to D9, and every primitive is
listed in [../cryptography.md](../cryptography.md). As of release 0.1.0, byte
form v1 is **stable**: it will not change, and a test fails if its vectors
do ([Stability](#stability)). A variable supply series is written in format
version 2, which [encoding-variable.md](encoding-variable.md) defines
beside this document (D163). Every fixed series stays in version 1.

## Stability

Byte form v1 is frozen (decision D76). Everything in this document is fixed
for every series whose parameters say format version 1:

- No form here is edited, reordered or given a new meaning. A fix or an
  extension is a new format version, with its own document and vectors,
  and a series opts in through the format version byte of its parameters.
  Series made under v1 keep v1 forever, since their ids depend on it.
- `TestByteFormV1Frozen` in `pkg/vectors` checks the SHA-256 of
  `vectors.json`. Regenerating the vectors with `-update` does not get past
  it.
- What may still change without a new version: limits a node applies before
  decoding (they decide what a node accepts, not what bytes mean), error
  messages, and this document's wording where it does not change a byte.

## Primitives

| Name | Bytes |
| --- | --- |
| `U8`, `U16`, `U32`, `U64` | Unsigned integer, big-endian, fixed width |
| `Bool` | One byte, `0x00` or `0x01`; any other value is refused |
| `Hash` | 32 raw bytes |
| `Bytes` | `U32` length, then that many bytes |
| `List<T>` | `U32` count, then each item |
| `∥` | Concatenation |

`H` is the hash named by the series (1 SHA-256 to 5 BLAKE3, listed in [../cryptography.md](../cryptography.md); all have 32-byte output). `H(a ∥ b ∥ …)` hashes the
concatenation. Tags are ASCII without a terminator. Decoders refuse trailing
bytes and lengths or counts above the documented limits.

## Series parameters

```
U8   format version (1)
U32  hash number
U64  mints (N)
U64  supply per mint (S)
U64  pool size (P)
U16  minimum layer strength
U16  minimum entry strength
Bool entry hash counts as credit
List<U32> allowed policy types
List<U32> allowed key algorithms
U32  witness ceiling (0 = none)
Bool has issuer
[Hash issuer address]        only when "has issuer" is 1
```

- `series_id = H("PoON-series" ∥ params)`
- Pool k holds serials `k·P` to `min((k+1)·P, N) − 1`.
- `pool_id(k) = H("PoON-pool" ∥ series_id ∥ U64(k))`. It is the parent of the
  first entry of pool k.

## Policies and addresses

```
policy  = U32(4) ∥ U32(type) ∥ Bytes(params)
address = H("PoON-address" ∥ policy)
```

The type is written as a 4-byte field preceded by its length, 4.

Threshold params (type 1):

```
U16  K
List<{U32 algorithm, Bytes public key}>   1 to 16 keys, no repeats
```

Threshold witness data:

```
List<{U16 key index, Bytes signature}>    exactly K, indices strictly increasing
```

Hashlock params (type 2):

```
U32  hash number            any hash of the hash list, not only the series'
Hash digest                 32 bytes
```

Hashlock witness data: `Bytes(preimage)`, at most 1024 bytes, whose hash is
the digest.

K-of-policies params (type 3):

```
U16  K
List<policy>                1 to 16 policies, no repeats, each in the policy
                            form above, nested at most 4 deep
```

K-of-policies witness data:

```
List<{U16 policy index, Bytes data}>   exactly K, indices strictly increasing
```

Each part's data is the witness data of the policy at its index.

The key algorithms (1 Ed25519 to 11 SLH-DSA-SHA2-256s), their key and
signature forms, and the hash list are in [../cryptography.md](../cryptography.md).
Each public key and signature has its algorithm's exact length, and
threshold data is read against its params, so each signature is read at
its key's exact size (D170). There is no other length limit: `params` and
`data` are bounded by the series' witness ceiling and by the bytes
received.

## Witnesses

```
witness = policy ∥ Bytes(data)
message = "PoON-witness-v1" ∥ tid
```

Each signature in the witness data signs `message`. The witness ceiling of a
series limits the size of `witness`.

The issuer witness of a series signs the message for the pseudo-tid
`H("PoON-issuer-v1" ∥ series_id)`.

## Transactions

```
body =
  Hash series_id
  U64  serial
  List<{Hash tid, U32 output index}>   inputs (empty for a claim)
  List<{Hash address, U64 amount}>     outputs

tx = body ∥ List<witness>

tid = H("PoON-tx" ∥ body)
```

Witnesses are outside the tid. A claim has no inputs, outputs summing to the
supply, and one witness per output proving control of that output's address.
A spend has one witness per input, in input order, each proving control of
the address that holds the spent portion.

## Entries

```
entry = Hash parent ∥ U64 nonce ∥ List<tx>
```

Transactions are in canonical order: by serial, then by tid, strictly
increasing.

```
entry hash = H("PoON-entry" ∥ parent ∥ tid₁ ∥ … ∥ tidₙ [∥ U64 nonce])
```

The nonce is part of the hash only when the series' minimum entry strength
is above 0. Otherwise the wire field must be 0.

## Layers

```
layer      = Hash entry hash ∥ U64 nonce        (40 bytes)
layer hash = H(entry hash ∥ U64 nonce)
strength   = leading zero bits of the layer hash
credit     = 2^strength
```

A layer whose strength is below the series' minimum layer strength is
invalid.

## Test vectors

`go test ./pkg/vectors` recomputes these from the example series
(`poon series example`) and keys whose Ed25519 seeds are
`SHA-256("PoON-test-key:" ∥ label)`:

| Vector | Value |
| --- | --- |
| series_id | `9d7f095da805d59ec8aa667a24ce07d618dd3dbdd95a8a7d97318f5bcfbbe3f1` |
| pool 1 id | `e4016d16603ba8217dfedf52cc5bf05ac87c1d2d1a60400e029a55e7ed973566` |
| alice's address | `e3a25c3e0a1b0830c33d3b9371924a24a8d23ccbdcc6b9b025393d3e29c84e2a` |
| claim of serial 4 to alice | `9a0afc5e0497f9fbad66f8d95c3770f00a3329b415d911a2e3084632594f0bd0` |
| entry 1 (that claim) | `9254af12b08451267a6425371cc7331d9e52390fe870ea50ed40682f3ea72490` |
| best layer on entry 1, nonces 0–3999 | nonce 1649, strength 12 |

The JSON file also holds the full encodings of the series, alice's policy
and the claim.
