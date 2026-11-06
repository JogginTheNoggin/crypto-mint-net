# RED-112: Lease limits are per node id and the stated rate is not checked, so a few connections hide every offer for an hour

| Field | Value |
| --- | --- |
| Severity | Medium |
| Status | fixed |
| Found | Source Round 36 Part B, on `main` @ `6b56136` (D196) |
| Area | `pkg/bounty/book.go` (`Take`, `Listable`), `pkg/p2p/bounty.go` |
| Fixed by | `claude/project-thread-ggo8dt` @ afd0dc1 (BLUE-037) |
| For Black | Not a rule. |

## What is wrong

A grinder's rate is a number it states in the take. `Take` believes it: the
lease lasts three times the expected time at that rate, up to the ceiling
(3,600 s by default), and the extension adds the same again. A leased offer is not
listed (`Listable` skips an offer with no free slot), so while it is leased
only the holder can see it.

The limits meant to stop one grinder from holding many offers
(`max_leases_per_grinder`, "earn one more per layer handed in") are kept
per **node id**, and a node id costs nothing (RED-111). One connection per id,
at most the inbound cap (24 by default), so a peer holds as many leases as
it has connections: an offer for one layer is blocked by one connection.

The lease is not worked: no claim is needed to keep it, and nothing makes the
holder prove it grinds. It lasts for the ceiling, then (with its one
extension) twice that.

Each lease also rewrites `bounties.json` (`save()` in `Take`, under the book's
lock); not measured.

## How I checked it

`pkg/bounty` test (throwaway, removed; copy kept with Red's notes): eight
offers (strength 20, one layer each); eight different peer ids each take one,
stating a rate of 2^20/3000 tries a second (the slowest that passes the
"covers another lease" check):

```
offer 0: lease 3600 s
offer 1: lease 3600 s
listable after 8 takes (one per id): 0 of 8
an honest grinder at 2^30/s: seconds=0 reason="leased or filled"
sybil-0 extends: 7200 s
```

At the node level a node id is whatever the hello says (the raw peers in
RED-111 choose it), so the eight ids are eight connections.

## Suggested fix (Blue's call)

- A lease per **address** (the connection's IP) as well as per id, or a
  limit on takes per address that are not followed by a claim.
- A lease the holder has not used (no claim, no progress message) ends
  early, and a grinder that lets leases lapse (`grinder.expired`, already
  counted) gets fewer.
- Keep a listed offer listed while it is leased, marked "leased", so a payer
  and other grinders can see that it is not dead.

## Not tested

The cost of the saves; the inbound cap's effect at a node with many offers.

## Resolution

Fixed in part in BLUE-037 (D197). The leases from one network address are limited to `max_leases_per_grinder` in all, whatever node ids it uses. Listing leased offers and ending unused leases early are not taken. Test: `TestBlue037LeasesPerAddress`.
