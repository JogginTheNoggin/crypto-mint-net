# BLUE-038: Bounty offers bounded in bytes; held offers and leases per address and /64; the done file read in bounded pieces

| Field | Value |
| --- | --- |
| Fixes | RED-115 (medium), RED-116 (low, in part), RED-117 (low, in part) |
| Branch | `claude/project-thread-ggo8dt` (PR against `main`) |
| Base | `main` @ `bf90ffd` (Source Round 37 Part C), with GREEN-062 (#314) merged in |
| Decisions | D199 |
| With | GREEN-062 (#314), carried: it changes the same `book.go` and `p2p/bounty.go`. ORANGE-047 (#313) is docs only; its proposals are for Grey (proposal 3 with White). |
| Review | No consensus change: the market is outside the ledger (D196). For White: an offer over 1,024 bytes is refused even alone (a list already had that bound); held offers gain `addr`. |

## Summary

- **RED-115.** `MaxOfferBytes` 1,024 for one offer as for a list item.
- **RED-116.** Held offers bounded per address; IPv6 keyed by /64;
  leased offers kept on eviction; least-earning grinder record dropped.
- **RED-117.** Done lines read only to the id; drops reported.

## Consensus

None.

## Tests

- `pkg/bounty/blue038_test.go`: `TestBlue038OfferBytesBounded`, `TestBlue038HeldPerAddress`, `TestBlue038DoneFileBounded`
- `pkg/p2p/blue038_internal_test.go`: `TestBlue038AddressKey`
- The p2p test fails on the code before the fix, and the bounty tests fail to build on it (`MaxOfferBytes`, `ReceiveFrom`, `Held.Addr`).
