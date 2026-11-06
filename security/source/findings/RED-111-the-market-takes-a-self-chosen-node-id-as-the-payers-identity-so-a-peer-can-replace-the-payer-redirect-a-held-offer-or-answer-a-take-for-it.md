# RED-111: The market takes a self-chosen node id as the payer's identity: a peer can replace the payer, move a held offer to itself, or answer a take for it

| Field | Value |
| --- | --- |
| Severity | Low (phase 1: offers are not signed; the parts under (b) and (c) are cheap to close now) |
| Status | open |
| Found | Source Round 36 Part B, on `main` @ `6b56136` (D196) |
| Area | `pkg/p2p/peer.go` (hello, duplicate ids), `pkg/p2p/bounty.go` (`handleBounty`), `pkg/bounty/book.go` (`Receive`, `Granted`, `Acked`) |
| For Black | Not a rule. |

## What is wrong

A held offer remembers the node id of the peer that sent it (`From`), and
`take`, `claim`, `grant` and `ack` are accepted or sent by that id. The id is
the 32 bytes a peer writes in its hello; nothing proves them. Three ways in:

- **(a) A second connection with the payer's id replaces the payer's.**
  `handshake` keeps the new connection when both are on the same side
  (`keepNew`: "the old one is stale"). The grinder's `bountyPeer(From)` is then
  the attacker, who answers the take (and the book accepts it: `h.From ==
  peer`), receives the claim with the nonce, and never pays.
- **(b) An offer sent again by another peer moves.** `Receive` on an offer the
  book holds sets `h.From = peer`, whoever sends it. The offer's id is the hash of
  its bytes, so anyone who saw it (every peer it was posted to, or the list)
  can take it over with one message, with no id trick.
- **(c) A grant is handed to the waiting call from any peer.** `handleBounty`
  looks up `bwait.grants[g.Offer]` and sends to its channel before (and
  whether or not) the sender is the offer's `From`. The book's own `Granted`
  does check the sender and records no lease, but the waiting call has
  already been answered, so a third peer can answer a `poo bounty work` that
  waits for the payer and the call grinds for as long as the attacker named
  (at most the call's four minutes).

All of these end in the grinder working for someone who will not pay. Phase 2
(signed offers) is meant to stop it; (b) and (c) are small changes now.

## How I checked it

`pkg/p2p` and `pkg/bounty` tests (throwaway, removed; copies kept with Red's
notes), a grinder node with the market on and raw peers that list `bounty`:

- (a) a payer connects, sends an offer; a second raw peer connects with the
  *same node id*. The real payer's connection reads EOF; `TakeBounty`
  sends its take to the new one, which answers a grant of 3,600 s; the book
  records the lease: `&{PayTo:0100… Until:… (+1 h)}`.
- (b) book: `Receive("payer", o)`, `Receive("mallory", o)`: held offer now
  `From "mallory"`.
- (c) the payer connected and sent the offer; a second peer, with its own id,
  sent a grant while `TakeBounty` waited: the call returned "grant 3600 s,
  err nil" with the payer silent. The book's lease record stayed empty.

## Suggested fix (Blue's call)

- (b) Do not change `From` when an offer is received again; if another peer
  sends it, keep the first and ignore (or count) the second.
- (c) Deliver to the waiter only when the sender is the held offer's `From`;
  the same for acks.
- (a) is the hello (not the market): the node id is a claim. Until offers are
  signed (phase 2), say in the docs that in phase 1 the payer is whoever holds
  the connection with that id, and that a grinder should only work offers it
  has reason to trust.

## Not tested

Impersonating a grinder at the payer (a lease is keyed by its id as well); the
other direction of `keepNew` (the node is the dialler: the attacker wins only
when its id is the lower one, from the code).
