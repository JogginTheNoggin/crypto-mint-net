# RED-091: A mapping the router already made is left behind when a later step of the same request fails, and a permanent UPnP lease then stays for ever

| Field | Value |
| --- | --- |
| Severity | Low to medium (an open port the owner did not know of; for a router that grants only permanent leases it lasts until someone removes it) |
| Status | fixed |
| Found | Source Round 31 Part B, on `main` @ `bf23ddb` (D182) |
| Area | `pkg/portmap/upnp.go` (`upnpMap`), `pkg/portmap/pmp.go` (`pcpMap`, `pcpReply`), `pkg/portmap/portmap.go` (`Map`), `pkg/p2p/portmap.go` (`portMapLoop`) |
| For Black | Not a rule. |
| Fixed by | `claude/project-thread-ggo8dt` @ 012719f (BLUE-032) |

## What is wrong

D182: a mapping is "removed at a clean stop". The node removes what `Map`
returned. When `Map` returns an error it returns no mapping, so nothing is
removed, even if the router created one on the way:

- **UPnP.** `upnpMap` calls `AddPortMapping` first and `GetExternalIPAddress`
  after. If the second call fails (a fault, a reply that is not an IPv4
  address, the context cancelled between the two calls), `upnpMap` returns an
  error and the mapping stays at the router. A router that grants only
  permanent leases (error 725, the case D182 handles by asking for lease 0)
  keeps it until someone deletes it, through a crash, a reload or a stop. For
  other routers it lasts until the lease ends (3,600 s by default).
- **PCP.** `pcpReply` rejects a reply whose external address is not IPv4 or
  whose port is 0 ("granted no usable address") after the router has created
  the mapping, and nothing sends the removal. A PCP router on a network with
  IPv6 can answer with a native IPv6 address.
- **Auto.** `Map` then goes on to the next method, so a failed PCP mapping can
  be followed by a NAT-PMP or UPnP one, and only the last is known to the node.
- **Cancelled.** A reload or stop while a `Map` call is waiting for the
  router's answer returns with the request sent and possibly granted.

## How I checked it

`pkg/portmap` tests (throwaway, removed), the existing UDP and HTTP fakes.

UPnP router that answers the first `AddPortMapping` with 725, accepts the
second with lease 0, then returns 500 for `GetExternalIPAddress`:

```
Map error: portmap: upnp: GetExternalIPAddress: 500 Internal Server Error
router saw: [AddPortMapping AddPortMapping GetExternalIPAddress]; mapping still at the router: true
```

No `DeletePortMapping` is sent.

PCP router that creates the mapping and answers with the external address
2001:db8::1:

```
Map error: portmap: pcp: granted no usable address ([2001:db8::1]:8646)
requests the router saw: 1, lifetimes asked [3600] (none is a removal)
```

## What could happen

- A port stays open to the internet after the node was stopped, with no
  record in `poon status` (the status shows a failure, not a mapping). With
  `port_map` an owner has agreed to a mapping while the node runs, not after.
- Each retry (every 1 to 30 minutes) asks again, so a router that fails the
  second step every time is asked to add the same mapping again and again.

## Suggested fix (Blue's call)

- In `upnpMap`, ask for the external address first (it needs no mapping), or
  delete the mapping when any step after `AddPortMapping` fails.
- In `pcpMap`, send the removal (same nonce, lifetime 0) when the reply
  grants a mapping the node cannot use. In `Map`, remove what a method made
  before trying the next.
- On cancel, make a short best-effort removal with a fresh context, as the
  clean stop does, for a request that was sent.
- Test: the two fakes above must see a removal request.

## Not tested

A real router; the effect of the removal on routers that treat a PCP lifetime
of 0 for an unknown nonce as an error.

## Resolution

Fixed in BLUE-032 (D183). UPnP asks for the external address before
`AddPortMapping`, so Red's 725 router now sees no mapping request at all.
A PCP mapping granted with an unusable address is removed with the same
nonce. A request whose answer was not read (the caller's context ended,
or a UPnP connection failed) gets a best-effort removal of at most 1 s
with its own context. Each method cleans up after itself, so `auto`
leaves nothing when it moves on. Tests:
`TestBlue032UPnPExternalFailsLeavesNoMapping`,
`TestBlue032UPnPCancelRemoves`, `TestBlue032PCPUnusableRemoved`,
`TestBlue032CancelSendsRemoval`.
