# Byte formats for a variable supply series (format version 2)

## Summary

These are the bytes of a variable supply series (PoON section 16.8, decision
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

- `series_id = H("PoON-series" ∥ params)`, as for version 1.
- A variable series must name an issuer. Its part size is at least 1, and
  N and M are whole multiples of the pool size. The serials of its
  largest number of parts fit in 64 bits.
- Version 2 is only ever a variable series. A fixed series under version 2
  is refused, so each series has exactly one byte form.
- A node that does not know version 2 reports the series as unreadable
  (PoON section 16.7).

## Parts

Part 1 holds serials 0 to N − 1. Part k, for k ≥ 2, holds serials
N + (k − 2)·M to N + (k − 1)·M − 1. Pool k holds serials k·P to
(k + 1)·P − 1, and every pool sits inside one part.

```
part_id(1) = H("PoON-part" ∥ series_id ∥ U64(1) ∥ series_id)
part_id(k) = H("PoON-part" ∥ series_id ∥ U64(k) ∥ part_id(k − 1))
```

## Release

```
Hash series_id
U64  part (k ≥ 2)
Hash previous            part_id(k − 1)
Witness                  the issuer's, as in encoding.md
```

- The part's id is computed from the release's own fields:
  `H("PoON-part" ∥ series_id ∥ U64(k) ∥ previous)`. The witness is not
  part of it.
- The witness satisfies the series' issuer address for the message of the
  pseudo-tid `H("PoON-part-v1" ∥ part_id(k))`. That message is
  `"PoON-witness-v1" ∥ pseudo-tid`, as for every witness.
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
`poon series example`.

| Value | Hex |
| --- | --- |
| series_id | `86314ce1d77609e1b83ea4d342a651051773def0f5598dfcdc656c33dff2daf7` |
| part_id(1) | `e1983895cf59cc332fc0c77e5811388c160581180c480538200575eff2f158ed` |
| part_id(2) | `c576cfc4061560958a67c84cbc3d3e69b605a1dcf808f896f73e122306fb1785` |
| part_id(3) | `52543ff0bb911e8f24e1be69daf1dcc7a85ea4c99dbcbf866b22ce3e625485d1` |
| release message of part 2 | `13a997787bac5d4e64c82c468ab0db9a8b6536bc08347c2bd0c8c3d60cabeb54` |

The JSON file also holds the series' full encoding, the release's encoding
and the issuer witness, and a claim of serial 50,000 (the first mint of part
2) with its body, tid and encoding (D166).
