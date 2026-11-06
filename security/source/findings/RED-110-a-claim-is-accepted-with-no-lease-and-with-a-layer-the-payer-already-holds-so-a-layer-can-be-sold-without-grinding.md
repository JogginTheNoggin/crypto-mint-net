# RED-110: A claim is accepted with no lease and for a layer the payer already holds, so an offer can be filled without grinding

| Field | Value |
| --- | --- |
| Severity | Medium |
| Status | open |
| Found | Source Round 36 Part B, on `main` @ `6b56136` (D196) |
| Area | `pkg/bounty/book.go` (`Claim`), `pkg/p2p/bounty.go` (`handleBountyClaim`) |
| For Black | Not a rule. |

## What is wrong

`Book.Claim` accepts a claim when the nonce is new *for that offer*, the offer
is open with room, and the strength at or above the target. It does not require
a lease: the cases are `i >= 0 && PayTo differs` (mismatch) and
`i < 0 && free() <= 0` (reserved), and an offer with a free slot goes on to
`accepted` for any peer. It also does not ask whether the layer is new. The
caller then gives it to the engine (`HandleLayerFrom`), whose "already known"
is logged at debug level and changes nothing.

So a peer that has not ground anything can take any layer of the entry that
is public (the ledger's own, from a watch node or an export) at or over the
target and claim it with its own `pay_to`; the payer owes the price. The lease
protocol, the rate statement and the "one grinder at a time" rule play no part.
The grinder also earns more leases for the layers it "filled" (`filled++`).

The payer pays by hand, so the loss needs `poo bounty pay`; nothing in the
command or in `poo bounty list` says that the layer was already held.

## How I checked it

- `pkg/bounty` test (throwaway, removed; copy kept with Red's notes): a book
  that posts an offer for one layer; `Claim("nobody-asked-for-a-lease", …,
  strength 24)` returned `accepted`.
- The second offer in RED-109's run is the same thing with a real node: the
  layer was already in the payer's engine (offer A's) and the claim on offer
  B was accepted and recorded as owed.

I did not take a layer from another node's ledger with a second real node.

## Suggested fix (Blue's call)

- Require a lease for a claim; a claim without one is `reserved` or `closed`.
- Before `accepted`, ask the engine whether the layer is new; if it is known,
  answer with a new outcome (`known`) and owe nothing. Keep the engine's
  answer in the claim record, and show it in `poo bounty list` and
  `poo bounty pay`.
- Optionally, the offer may say the layer must be one the payer's node did not
  hold when the offer was posted.

## Not tested

A claimant on another node taking a layer from the ledger it holds.
