# RED-090: The dial-back that confirms a mapped address accepts any host that sends the node's own hello back, so a lying router (or a forged NAT-PMP reply) makes the node advertise an address that is not its own

| Field | Value |
| --- | --- |
| Severity | Medium (the check D182 relies on to keep a router's claim from becoming the node's advertised address does not check anything; the attacker must be the router or win a race against it) |
| Status | fixed |
| Found | Source Round 31 Part B, on `main` @ `bf23ddb` (D182) |
| Area | `pkg/p2p/portmap.go` (`confirmSelf`, `holdMapping`), `pkg/p2p/discovery.go` (`contact`) |
| For Black | Node policy only; no rule is touched. It decides what a node tells peers about itself, never what is valid. |
| Fixed by | `claude/project-thread-ggo8dt` @ 012719f (BLUE-032) |

## What is wrong

D182: "The advertised address comes from the router, never from peers. The
node then dials it and must get its own hello back (its own node id) before
it offers the address to peers." `confirmSelf` does this:

1. dial the address the router reported;
2. **send our own hello**, which carries our own `NodeID`;
3. read one frame, require it to be a hello, and require
   `theirs.NodeID == h.id`.

The test in step 3 compares the reply with a value we just told the other
end. A host that reads our hello and sends it back, or any plain TCP echo
service, passes. Nothing in the exchange proves the connection reached this
node's listener.

## How I checked it

`pkg/p2p` test (throwaway, removed), the existing `startMapped` harness with
`AllowLocalPeers` on (the harness runs on loopback; the public-address check
is a separate step and unchanged). The fake router reports as external
address a **bare TCP echo server** (`io.Copy(c, c)`) that has nothing to do
with the node, which listens on another port:

```
node listens on 127.0.0.1:40585; a bare echo server at 127.0.0.1:40451 was confirmed:
state=advertised advertised=127.0.0.1:40451
```

`contact()` then offers `127.0.0.1:40451` to peers, and `Listed()` holds it.

## What could happen

- **A hostile or compromised router**, or a router reachable through a VPN
  whose provider is not trusted, reports an address it controls. The node
  confirms it and tells peers to connect there. Peers that act on it connect to
  the attacker, who can answer as a node, so it is a way to steer a node's
  inbound peers (a piece of an eclipse) and to learn who is looking for it.
  The router could already read the node's traffic, but the point of the
  dial-back was to stop the router's claim from reaching other nodes.
- **A forged NAT-PMP reply.** NAT-PMP replies carry no nonce, and the
  external-address reply and the mapping reply each only need the gateway's
  source address and the exchange's ephemeral port. A host on the same
  network that can send packets with the router's source address can flood
  the 28,000 or so ports in the ephemeral range. It has to beat the real
  router's answer, so it works best against a slow or busy router; I did not
  build this race. PCP is safer: its 96-bit nonce is not guessable by a host
  that cannot see the request.
- The same dial also makes the node open a connection to an address chosen by
  the router (a public address, since `publicIP` is checked first) and send it
  a hello frame. That is a small outbound-connection primitive for whoever
  controls the answer.

## Suggested fix (Blue's call)

- Make the dial-back prove the connection reached the node's own listener,
  not just that the other end knows its id. `confirmSelf` puts a random token
  in the hello it sends (for example in the `Agent` string, or an optional
  field if the wire format allows one). The listener, when an inbound hello
  carries this node's own id and a token it has pending, records that it saw
  the token. `confirmSelf` succeeds only when its token was seen by the
  listener. An echo server or a relay that does not reach the node cannot
  produce that.
- Keep the router-reported address out of anything else until that holds
  (already so), and keep the unconfirmed-and-removed behaviour.
- Test: the echo server above; a server that replies with a hello copied from
  the node's own; and a real hairpin (the existing confirmed test) still
  passing.

## Not tested

A forged NAT-PMP answer racing a real router; a real hairpin through a
router; the public-address check against a router that reports a public
address it does not own (that is this finding).

## Resolution

Fixed in BLUE-032 (D183). `confirmSelf` sends a hello whose `Agent`
carries a fresh random 128-bit token. The listener's handshake records an
inbound hello with this node's own id and a pending token
(`sawSelfProbe`), and only that record confirms the address. The far
end's answer can only end the wait early with a failure. The echo server
and a server that answers with a copy of the node's hello are now
`unconfirmed`, and the mapping is removed. A host that relays to this
node's real listener still passes; connections through it do reach this
node. Test: `TestBlue032DialBackNeedsOwnListener`.
