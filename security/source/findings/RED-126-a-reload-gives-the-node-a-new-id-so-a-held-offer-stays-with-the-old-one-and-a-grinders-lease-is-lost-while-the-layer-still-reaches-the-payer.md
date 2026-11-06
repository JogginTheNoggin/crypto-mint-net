# RED-126: A reload gives the node a new id, so a held offer stays with the old one and a grinder's lease is lost while the layer still reaches the payer

| Field | Value |
| --- | --- |
| Severity | Low (a grinder's work is lost without pay and an offer is stuck until it ends; it needs a reload of either node at the wrong time, and `poon policy limits -set` is one) |
| Status | open |
| Found | Source Round 41 Part B, on `main` @ `6cf8002`, from jerry's test findings item 7 (D196 and later; `pkg/p2p/host.go`, `pkg/bounty/book.go`) |
| Area | `pkg/p2p/host.go` (the node id is `rand.Read` at each `Start`), `pkg/bounty/book.go` (`ReceiveFrom`, `Claim`, `lease`), `pkg/p2p/bounty.go` (`handleBountyClaim`) |
| Fixed by | |
| For Black | Not a rule. |

## What is wrong

jerry asked whether a reload resetting the node id matters (item 7). The id is made fresh in `Host.Start`, and a reload starts a new host, so the id changes (his d6acb85d to d5652e54). The market keys three things by that id, and they do not survive it:

1. **The payer reloads.** Its offers are on disk, its id is new. A grinder that holds the offer asks again and the payer lists it, but `ReceiveFrom` returns early for an offer already held ("kept from the peer that sent it first", RED-111), so `Held.From` stays the old id. `TakeBounty` then looks for the old id's connection, finds none (`ErrPayerGone`), and the offer cannot be taken until it ends or is pushed out. For an offer open for hours this is most of its life.
2. **The grinder reloads while it holds a lease.** The lease is by the grinder's id (and address). The claim then comes from the new id and is refused: `Reserved` while the old lease is still alive (the offer has no free layer), `NoLease` after. The claim path still hands a layer at or above the series floor to the engine "whatever the market made of it" (`handleBountyClaim`), so the payer's node keeps the layer and does not pay it.
3. **Records by id** (`payers`, `grinders`, peer tracking) restart empty, as jerry saw.

`poon policy limits -set` reloads the node, and jerry's own runs did it between steps, so a limit changed during `bounty work` loses the work.

## How I checked it

`pkg/bounty` test (throwaway, removed; copy kept with Red's notes):

```
held offer's payer after the payer reloaded and listed it again: "payer-old-id"
lease for grinder-old-id: 60 seconds
claim of a strength-25 layer (offer asks 20) from grinder-new-id: outcome 6 (Reserved)
```

Case 1 is the first line plus `TakeBounty`'s lookup (from the code; not run through the host). The reload changing the id I confirmed in `host.go:577`.

## Suggested fix (Blue's call)

- Keep the node id across reloads: make it once per process (or once per node directory, in a file with the other state), not once per `Host`. It is not an identity (RED-114 says so); it only has to be the same while the node is the same.
- In `ReceiveFrom`, when the offer is held from an id that is no longer connected, and the same address (or a connected peer) lists it again, move it to the new sender.
- In `Claim`, when the claimant has no lease but a lease on the offer was granted to the same address and payout address within its time, take it as that lease.
- A claim that is refused for a missing lease should not be worth the layer to the payer: either pay it, or do not hand the layer to the engine (the layer is still valid weight, so paying it is better).

## Not tested

The two reloads through real hosts; the `payers` records.
