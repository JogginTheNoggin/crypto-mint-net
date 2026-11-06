# RED-109: `poo bounty work` always grinds from nonce 0, so every grinder finds the same layer: an offer for two layers cannot be filled, and a second offer on the same entry is paid for the same layer

| Field | Value |
| --- | --- |
| Severity | Medium (the payer's money, when the payer pays; the grinder's time) |
| Status | fixed |
| Found | Source Round 36 Part B, on `main` @ `6b56136` (D196) |
| Area | `pkg/rpc/bounty.go` (`bountyWork`), `pkg/cli/bounty.go` (`bountyPay`), `pkg/p2p/bounty.go` (`handleBountyClaim`) |
| Fixed by | `claude/project-thread-ggo8dt` @ HASH (BLUE-037) |
| For Black | Not a rule: a layer is still an ordinary layer; nothing about what a node accepts changes. |

## What is wrong

`bountyWork` grinds `layer.GrindHash(id, entry, start, 1<<20, workers)` with
`start` beginning at 0, and stops after the first block of 1,048,576 tries
that holds a layer at or over the target. `GrindHash` returns the best layer in
the block, so the answer depends only on the entry and the block: **every
grinder, and every call, finds the same nonce.** Three results:

1. **An offer for two or more layers cannot be filled by the stock tool.** The
   second layer a grinder (or a second grinder) hands in has the same nonce
   as the first; `Book.Claim` answers `repeat`, the lease (here 60 s) is
   spent, and a million tries are wasted.
2. **A second offer on the same entry is accepted with the nonce the payer
   already holds.** The claim is "accepted", recorded as owed, and the
   engine's answer to the layer ("known") is only logged at debug level
   (`handleBountyClaim`). `poo bounty pay` then pays the amount without
   looking at whether the layer was new.
3. If ordinary grinders also start at nonce 0, a layer already in the ledger is
   found again here and sold to the payer a second time (not checked).

## How I checked it

`pkg/cli` test (throwaway, removed; copy kept with Red's notes) on the same
two-node setup as `TestBountyMarket`: payer posts offer A (strength 8, count 2,
amount 10); the grinder works it twice; the payer posts offer B (count 1, same
entry); the grinder works it.

```
offer A (count 2) work 1: outcome=accepted nonce=227480 strength=19 tried=1048576
offer A (count 2) work 2: grant=60s outcome=repeat nonce=227480 tried=1048576
offer B (count 1, same entry) work: outcome=accepted nonce=227480
payer owes: offer 670870f5 claim nonce=227480
payer owes: offer 5004e294 claim nonce=227480
```

`poo bounty pay` paid the first claim. Paying the second failed only because the
test wallet's one coin was still in flight ("0 held, 10 asked"); I did not run
it further, so the second payment is from the code, not a run. The returned
strength was 19, not 8: the grinder gives the best layer of the block, which
is fine for the payer but is also what makes the nonce fixed.

## Suggested fix (Blue's call)

- Start each `work` call at a random nonce (and make the nonce part of the
  claim). Two grinders then find different layers.
- In `handleBountyClaim`, ask the engine whether the layer is new for that
  entry before answering `accepted`. A known layer gets its own outcome (not
  owed), and the engine's answer is part of what the payer reads in
  `poo bounty list`.
- `poo bounty pay` should refuse (or warn and need `-force`) when two owed claims
  on the same entry have the same nonce.

## Not tested

Two grinders at once; whether the ordinary `poo grind` starts at 0 too.

## Resolution

Fixed in BLUE-037 (D197). `poo bounty work` grinds from a random nonce, and a layer the payer holds is answered `known` and not owed (RED-110). Test: `TestBountyMarket` fills a two-layer offer with two work calls.
