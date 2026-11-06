# RED-116: A few addresses and a few node ids still hide every offer and fill the held list, and the grinder records drop earned leases

| Field | Value |
| --- | --- |
| Severity | Low |
| Status | fixed |
| Found | Source Round 37 Part B, on `main` @ `bf90ffd` (D198; follow-up to RED-111 and RED-112) |
| Area | `pkg/bounty/book.go` (`TakeFrom`, `Receive`, `forgetGrinder`), `pkg/p2p/bounty.go` (`hostOf`) |
| Fixed by | `claude/project-thread-ggo8dt` @ HASH (BLUE-038) |
| For Black | Not a rule. |

## What is wrong

RED-112's fix limits leases per network address (`TakeFrom`: `fromAddr >=
max_leases_per_grinder`, 4 by default). What is left:

- **Leases.** The cap is four leases per host, so *k* addresses hold 4*k* offers. An
  attacker with four addresses (or an IPv6 /64, where every address is a different
  host string: `hostOf` is the exact host) holds all 16 offers of a default payer
  for an hour, as before. The stated rate is still a statement.
- **The held list.** `Receive` limits offers per node id (64) and in all
  (256), and a node id is a self-chosen value. Four ids from one address, within
  the discovery budget (a 64-offer list counts as one message), replace the whole
  held list, oldest first, so the legitimate offers a taker held are gone and
  its `poo bounty list` shows only the junk. They are junk by the market's own checks
  (a followed series, any entry, a price over `min_price`).
- **Earned leases.** When the grinder records reach `maxPeerRecords`,
  `forgetGrinder` drops *any* record with no lease, including one with `filled > 0`,
  so a grinder that earned more leases starts again from one. Each new
  id can force it.

## How I checked it

`pkg/bounty` tests (throwaway, removed; copy kept with Red's notes):

```
4 addresses x 4 ids: 16 leases of 16; listable now: 0
256 junk offers from 4 ids; held now 256; the legitimate offer is still held: false
```

The IPv6 point and `forgetGrinder` are from the code; I did not fill the grinder
records.

## Suggested fix (Blue's call)

- Count held offers per address as well as per id, and keep an offer the node
  is working (leased, or asked about) when it evicts.
- For leases, key the address by IPv4 address or IPv6 /64, and end a lease
  that was not used (no claim) early.
- `forgetGrinder` should drop the record with the smallest `filled`, not any.

## Not tested

IPv6; a full grinder table.

## Resolution

Fixed in part in BLUE-038 (D199). Offers held from one address are bounded as one peer's are. The address key is the IPv4 address or the IPv6 /64, for offers and leases. An offer this node has a lease on is kept when the held list is full, and `forgetGrinder` drops the record that earned least. Ending unused leases early is not taken. Test: `TestBlue038HeldPerAddress`, `TestBlue038AddressKey`.
