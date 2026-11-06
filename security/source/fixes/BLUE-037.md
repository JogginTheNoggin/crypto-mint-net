# BLUE-037: Bounty grinders start at random; claims need a lease and a new layer; leases per address; the payer fixed; bounded records; bounties.json checked

| Field | Value |
| --- | --- |
| Fixes | RED-109, RED-110, RED-112 (medium), RED-111, RED-113, RED-114 (low); RED-111, RED-112 and RED-114 in part |
| Branch | `claude/project-thread-ggo8dt` (PR against `main`) |
| Base | `main` @ `d25f777` (Source Round 36 Part C), with GREEN-061 (#299) merged in |
| Code | `afd0dc1` |
| Decisions | D197 |
| With | GREEN-061 (#299), carried: it changes the same `pkg/p2p/bounty.go`. ORANGE-046 (#301) is docs only; its proposals are for Grey. |
| Review | No consensus change: the market is outside the ledger (D196). For White: two new ack outcomes (9 `known`, 10 `no lease`) that an older build names "outcome N"; leases in `bounties.json` gain `addr`. |

## Summary

- **RED-109.** Random start nonce in `poo bounty work`.
- **RED-110.** Lease required; `known` for a layer the pool holds.
- **RED-111.** (b) and (c) fixed; (a) is phase 2, now in the docs.
- **RED-112.** Leases limited per network address.
- **RED-113.** 16 acks per held offer; 4,096 peer records.
- **RED-114.** `Open` checks offers and claims; `Owed` saturates.

## Consensus

None.

## Tests

- `pkg/bounty/blue037_test.go`: `TestBlue037ClaimNeedsLeaseAndNewLayer`, `TestBlue037LeasesPerAddress`, `TestBlue037HeldOfferKeepsPayerAndBoundsAcks`, `TestBlue037FileChecked`
- `pkg/p2p/blue037_internal_test.go`: `TestBlue037AnswersOnlyFromThePayer`
- `pkg/cli/bounty_test.go`: `TestBountyMarket` (two-layer offer)
- The work test and the payer-answer test fail on the code before the fix, and the book tests fail to build on it (the new outcomes, `TakeFrom` and `HeldFrom`).
