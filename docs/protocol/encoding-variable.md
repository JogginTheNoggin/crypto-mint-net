# Byte formats for a variable supply series (format version 2)

## Summary

These are the bytes of a variable supply series (PoO section 16.8, decision
D163): its parameters, its part ids and the release of a part. Every form
not listed here is the one in [encoding.md](encoding.md), which stays
frozen (byte form v1, D76). A fixed series is always written in format
version 1, so its bytes and its series_id never change. The vectors are in
[`pkg/vectors/testdata/variable.json`](../../pkg/vectors/testdata/variable.json).

## Series parameters

```
U8   format version (2)
...  every field of format version 1, in its order, from the hash number
     to the issuer address
U8   supply type (1 = variable; any other value is refused)
U64  part size (M)
U64  largest number of parts (0 = no limit)
```

- `series_id = H("PoO-series" ∥ params)`, as for version 1.
- A variable series must name an issuer. Its part size is at least 1, and
  N and M are whole multiples of the pool size. The serials of its
  largest number of parts fit in 64 bits.
- Version 2 is only ever a variable series. A fixed series under version 2
  is refused, so each series has exactly one byte form.
- A node that does not know version 2 reports the series as unreadable
  (PoO section 16.7).

## Parts

Part 1 holds serials 0 to N − 1. Part k, for k ≥ 2, holds serials
N + (k − 2)·M to N + (k − 1)·M − 1. Pool k holds serials k·P to
(k + 1)·P − 1, and every pool sits inside one part.

```
part_id(1) = H("PoO-part" ∥ series_id ∥ U64(1) ∥ series_id)
part_id(k) = H("PoO-part" ∥ series_id ∥ U64(k) ∥ part_id(k − 1))
```

## Release

```
Hash series_id
U64  part (k ≥ 2)
Hash previous            part_id(k − 1)
Witness                  the issuer's, as in encoding.md
```

- The part's id is computed from the release's own fields:
  `H("PoO-part" ∥ series_id ∥ U64(k) ∥ previous)`. The witness is not
  part of it.
- The witness satisfies the series' issuer address for the message of the
  pseudo-tid `H("PoO-part-v1" ∥ part_id(k))`. That message is
  `"PoO-witness-v1" ∥ pseudo-tid`, as for every witness.
- A release is valid when all three of these hold: its part is within the
  largest number of parts, `previous` is the real `part_id(k − 1)`, and
  its witness checks.

## Transactions

A transaction on a variable series names the mint's part between the
series and the serial (D166). Everything else is as in
[encoding.md](encoding.md):

```
Hash series_id
U64  part                (variable series only; PartOf(serial), from 1)
U64  serial
...                      inputs, outputs and witnesses as in encoding.md
```

- A fixed series' transactions have no part field. Their bytes and tids
  are those of format version 1.
- A reader knows which form it has from the series id: the series is
  followed, so it is known before its transactions are read.
- The part must be the one whose range holds the serial. A transaction
  whose part is not released at the node waits: it is not invalid.

## Test vectors

This table gives the example series of the variable supply document:
N = 50,000, M = 10,000, S = 1 and P = 1,000. Its issuer is the test key
`issuer`, and the hash, types and algorithms are those of
`poo series example`.

| Value | Hex |
| --- | --- |
| series_id | `3bed1a7d65e4e3d0b153ea881094ac8ea9e1a448c94a63845f0502398a466253` |
| part_id(1) | `51c699951f1b1b395f440a9aba4a3d5dc630ac9e036ee275427a4dbac8db0877` |
| part_id(2) | `59b0cad314831c3ec3f99601a573b47f934d538f8f1c845b7272a138cfe596f2` |
| part_id(3) | `88dc39ab091254898af680a132db5c8eb00832368cd1639a4af19efe1ddb0b85` |
| release message of part 2 | `468897668eedeeb1e8641fb391274494e7acdd5025528f5c3b935c026edd183d` |

The JSON file also holds the series' full encoding, the release's encoding
and the issuer witness, and a claim of serial 50,000 (the first mint of part
2) with its body, tid and encoding (D166).
