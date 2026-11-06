# RED-077: A 15- or 16-of-n ML-DSA-87 address is accepted, but its witness is over the 64 KiB decode limit, so it can never be spent

| Field | Value |
| --- | --- |
| Severity | Medium (high for a reserve that offers ML-DSA-87 custody) |
| Status | fixed |
| Found | Review of the Reserve Currency Default Series doc, on `main` @ `e93eb29` |
| Area | `pkg/policy` (`MaxDataSize`, `MaxParamsSize`, `DecodeWitness`, `Registry.CheckPolicy`), `pkg/wallet` (`CheckPolicy`, shared policies), `pkg/cli` (`wallet policy`) |
| For Black | The decode limits are consensus (a witness that does not decode is invalid everywhere). I do not ask to change them; that needs jerry. The ask is that nothing offers an address that can never satisfy them. |
| Fixed by | `claude/project-thread-ggo8dt` @ `a117710` (BLUE-027) |

## What is wrong

`policy.MaxDataSize` and `MaxParamsSize` are 64 KiB (65,536 bytes) each.
`DecodeWitness` reads a witness's data with `VarBytes(MaxDataSize)`. A
threshold witness's data is `4 + k × (2 + 4 + signature size)` bytes. For
ML-DSA-87 (signature 4,627 bytes) that is 64,866 bytes for k = 14 and 69,499
for k = 15, so a witness with 15 or 16 ML-DSA-87 signatures does not decode:
it is invalid at every node, whatever the series' `max_witness`.

Nothing refuses to create or fund such an address. The address is a hash of
the policy, so a node cannot know the policy until it is spent. The only
places that see the policy first are the wallet, the CLI and the GUI, and
their check (`Registry.CheckPolicy`, `wallet.CheckPolicy`) looks only at the
policy type and the key algorithms the series allows.

The Reserve Currency doc says the 128 KiB ceiling "fits a 16-of-16 ML-DSA-87
custody witness (about 116 KB)". The ceiling does, the decoder does not.

## How I checked it

A test in `pkg/policy` (removed after) built k-of-k threshold witnesses,
encoded them and decoded them back:

```
ml-dsa-65  k=n=16  witness=84,426 B   decode ok   verify ok  (about 3 ms)
ml-dsa-87  k=n=14  witness=101,288 B  decode ok   verify ok  (about 3.7 ms)
ml-dsa-87  k=n=15  witness=108,521 B  decode err: encoding: length 69499 above limit 65536
ml-dsa-87  k=n=16  witness=115,754 B  decode err: encoding: length 74132 above limit 65536
```

With a wallet on a node running the reserve series (SHA-512/256, key algs
1, 8, 9, types 1, 2, `max_witness` 131072), `poon wallet policy -k 14`, `-k 15`
and `-k 16` over 16 ML-DSA-87 keys all print an address, exit 0.

## What could happen

An institution that sets up the custody the doc recommends for the largest
balances (ML-DSA-87, 15 or 16 of 16) and pays it a mint gets a balance that
no set of signatures can move, and nothing says so until the first spend
fails. At 10^14 units a mint this is one of the larger losses the system
allows. The reserve is also one for which the doc points people at ML-DSA-87.

## Suggested fix (Blue's call)

- A function `policy.CheckSpendable(rules, p)` that works out the largest
  witness the policy can produce (params size, and for threshold
  `4 + k × (6 + signature size)` with the largest signatures it could use,
  and for a hashlock its preimage) and returns an error if the data is over
  `MaxDataSize` or the params over `MaxParamsSize`. Call it from
  `Registry.CheckPolicy` (so `wallet policy`, `wallet.CheckPolicy` and the
  payer's check before paying an address with its policy all use it).
  Say the limit in the error: "at most 14 ML-DSA-87 signatures fit".
- Show the limit in `poon crypto` next to each algorithm (the largest k that
  fits), and in the docs.
- Do not raise the decode limits to fit: that is a consensus change.
- For the default series doc: say ML-DSA-87 custody is at most 14 of 16, or
  use ML-DSA-65 (16 of 16 fits at 84 KB).
- A test: a 15-of-15 ML-DSA-87 policy is refused by the wallet; 14-of-16 is
  not.

## Not tested

A 15-of-n spend through a node (the decode limit fails it at the wire in the
same way); hybrid mixes of Ed25519 with ML-DSA-87 (they fit when the total is
under 64 KiB).

## Resolution

Fixed in BLUE-027 (D169), together with RED-079.

- `policy.Registry.CheckSpendable` adds up the least witness data a policy can be satisfied with (K of the smallest signatures among a threshold's keys, the K cheapest parts of a k-of-policies, an empty hashlock preimage). It refuses the policy (`ErrUnspendable`) when that is over `MaxDataSize`, when the params are over `MaxParamsSize`, or when the witness is over the series' `max_witness`. The error says how many signatures fit ("at most 14 of these keys' signatures fit").
- `Registry.CheckPolicy` runs it, so `poon wallet policy`, `wallet.CheckPolicy`, a payment request and a payer's check before paying an address with its policy all refuse a 15- or 16-of-n ML-DSA-87 policy.
- `poon crypto` prints `max_signatures_per_witness` for each algorithm (ML-DSA-87: 14).
- The decode limits are unchanged. The check is never part of checking a witness, and every witness that decodes already passes it.
- The Reserve Currency Default Series document (project knowledge, not the repository) still says 16-of-16 ML-DSA-87 fits. I've passed that to the coordinator for jerry, since documents are his to change.
- Test: `TestCheckSpendable` (14 of 16 passes, 15 of 16 is refused, 16 of 16 ML-DSA-65 passes).
