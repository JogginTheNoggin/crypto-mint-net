# RED-113: The acks a held offer keeps, and the per-id records, have no bound, and an ack for a claim nobody made is kept

| Field | Value |
| --- | --- |
| Severity | Low |
| Status | fixed |
| Found | Source Round 36 Part B, on `main` @ `6b56136` (D196) |
| Area | `pkg/bounty/book.go` (`Acked`, `Held.Acks`, `payers`, `grinders`) |
| Fixed by | `claude/project-thread-ggo8dt` @ afd0dc1 (BLUE-037) |
| For Black | Not a rule. |

## What is wrong

`Acked` appends every ack from the offer's sender to `Held.Acks`, with its
up-to-16 tids, whether or not this node has made a claim; `Held.Acks` is only
dropped with the offer, and an offer with `open_seconds` 0 never expires
(it leaves only when the held list is full). `payers[peer]` and
`grinders[peer]` get an entry per id and never lose one. `HeldOffers()` copies
every ack on each call (`poon bounty list`, the node's status view).

At the budget (120 discovery-class messages a minute per address) one sender
adds about 7,200 acks an hour; 24 inbound peers 170,000.

## How I checked it

`pkg/bounty` test (throwaway, removed; copy kept with Red's notes): 20,000 acks
of 16 tids, each decoded from bytes as a peer's would be, put to one held offer:
heap +11 MiB (about 570 bytes each); `HeldOffers()` with them took 0.9 ms.
Extrapolated to a day at the budget from one address: about 100 MiB. Not run
at that length, and not through the wire.

## Suggested fix (Blue's call)

- Keep an ack only if it answers a take or claim this node made and is waiting
  for, or keep the last few (for example 16) per held offer.
- Bound `payers` and `grinders` (drop the oldest, or those not seen for a
  day).

## Not tested

The same through the real handler; the effect on a node with `held_offers`
4096.

## Resolution

Fixed in BLUE-037 (D197). Each held offer keeps its newest 16 acks, and payers and grinders are capped at 4,096 records each. Test: `TestBlue037HeldOfferKeepsPayerAndBoundsAcks`.
