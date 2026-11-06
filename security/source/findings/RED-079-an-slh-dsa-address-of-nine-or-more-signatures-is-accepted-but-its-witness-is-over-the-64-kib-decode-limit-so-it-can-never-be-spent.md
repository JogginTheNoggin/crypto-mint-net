# RED-079: An SLH-DSA address that needs 9 or more signatures is accepted, but its witness is over the 64 KiB decode limit, so it can never be spent

| Field | Value |
| --- | --- |
| Severity | Medium |
| Status | fixed |
| Found | Source Round 26 Part B, on `main` @ `439b369` (D168) |
| Area | `pkg/policy` (`slhdsa.go`, `MaxDataSize`, `Registry.CheckPolicy`), `pkg/wallet` (`CheckPolicy`) |
| Related | RED-077 (same class, ML-DSA-87; still open) |
| For Black | The decode limits are consensus; I do not ask to change them. The ask is that no tool offers an address that cannot be spent. |
| Fixed by | `claude/project-thread-ggo8dt` @ `a117710` (BLUE-027) |

## What is wrong

SLH-DSA-SHA2-128s signatures are 7,856 bytes. Threshold witness data is
`4 + k × (6 + 7,856)`. The decoder reads it with `VarBytes(MaxDataSize)`,
64 KiB (65,536). So k = 8 is the largest that decodes; k = 9 to 16 (the
threshold type allows up to 16 keys) does not, and a witness that does not
decode is invalid at every node.

`slhdsa.go` says "a witness holds up to 8 of them within MaxDataSize". That
is true of the decoder, but nothing enforces it where an address is made:
`Registry.CheckPolicy` and `wallet.CheckPolicy` accept a 9-of-10 policy, the
wallet can give out the address, and funds sent to it can never leave. This is
RED-077 again, and SLH-DSA moves the line from 14 signatures to 8.

## How I checked it

A test in `pkg/policy` (removed after) built threshold witnesses of ten
SLH-DSA keys with `SignThreshold`, encoded them and decoded them back:

```
k=1   data  7,866 B   CheckPolicy ok   decode ok   check ok  (0.33 ms)
k=8   data 62,900 B   CheckPolicy ok   decode ok   check ok  (2.2 ms, 2.4 s to sign)
k=9   data 70,762 B   CheckPolicy ok   decode: encoding: length 70762 above limit 65536
k=10  data 78,624 B   CheckPolicy ok   decode: encoding: length 78624 above limit 65536
```

A mixed set is worse: any combination whose signatures add up past 64 KiB
fails the same way (for example 6 SLH-DSA and 4 ML-DSA-87).

## What could happen

Someone sets up a custody address of "5 of 12" SLH-DSA keys (a normal
quantum-resistant custody choice) and funds it. Spending needs a 5-signature
witness, which is fine; but "9 of 12" cannot be spent at all, and a wallet
showing the address as valid gives no warning. Because the address is a hash
of the policy, no node can refuse the funding.

## Suggested fix (Blue's call)

The one fix in RED-077 covers this: `policy.CheckSpendable(rules, p)`, which
sums the worst-case witness data of the policy (k signatures of the largest
size among its keys, or the k largest) and refuses a policy whose data is
over `MaxDataSize` or whose witness is over the series' `max_witness`. Call it
from `Registry.CheckPolicy`, show the limit in `poo crypto`, and name SLH-DSA
in the doc comment as one of the algorithms that reach it.
Test: a 9-of-10 SLH-DSA policy and a mixed one are refused; 8-of-10 passes.

## Not checked

The 192-bit and 256-bit sets are not added (their signatures are over
`MaxSigSize`, 8,192). A 16,224-byte signature in a witness is refused by the
decoder with `encoding: length ... above limit`, the same as for any
oversize field.

## Resolution

Fixed in BLUE-027 (D169) by the same check as RED-077: `CheckSpendable` in `CheckPolicy`. A 9-of-10 SLH-DSA-SHA2-128s policy, and a mix whose K smallest signatures add up past 64 KiB, are refused where the address is made. An 8-of-10 policy passes. The `slhdsa.go` comment names the check. `poo crypto` shows 8 for SLH-DSA-SHA2-128s. Test: `TestCheckSpendable`.
