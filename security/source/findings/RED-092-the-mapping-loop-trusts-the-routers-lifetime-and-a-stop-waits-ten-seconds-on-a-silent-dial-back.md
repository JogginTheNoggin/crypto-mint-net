# RED-092: The mapping loop renews at half of whatever lifetime the router states, however long or short, and a stop waits up to 10 s on a dial-back that says nothing

| Field | Value |
| --- | --- |
| Severity | Low |
| Status | fixed |
| Found | Source Round 31 Part B, on `main` @ `bf23ddb` (D182) |
| Area | `pkg/p2p/portmap.go` (`holdMapping`, `confirmSelf`), `pkg/portmap` (`pcpReply`, `pmpMap`) |
| For Black | Not a rule. |
| Fixed by | `claude/project-thread-ggo8dt` @ 012719f (BLUE-032) |

## What is wrong

1. **Renewal follows the router.** `holdMapping` renews at `m.Lifetime / 2`,
   not sooner than 30 s. A router may state a longer lifetime than the node
   asked for (the node asks 3,600 s; PCP and NAT-PMP let the router choose;
   a `uint32` of seconds is up to 136 years). The node then does not renew, and
   does not look at the router's external address again, until half of that
   has passed. If the ISP gives the router a new address in between, the node
   goes on offering the old one. A router that states a very short lifetime
   (1 or 2 s) is renewed every 30 s, so the mapping is missing most of the
   time while the node says it is advertised.
2. **PCP lifetime 0.** `pcpReply` takes a reply with lifetime 0 as a
   mapping. In PCP that means the mapping was removed; the node's own
   `Lifetime == 0` means "until removed" (UPnP permanent), so it renews at
   `PortMapLease / 2` as if it had a permanent lease.
3. **A silent dial-back holds a stop.** `confirmSelf` dials with the context
   but then writes and reads under `helloTimeout` (10 s) with no way for the
   context to end the read. If the address accepts and does not answer, a
   stop or reload waits for that read.

## How I checked it

(1) and (2) by reading. (3) `pkg/p2p` test (throwaway, removed): the fake
router reports a TCP listener that accepts and never writes; after the node
had started the dial-back, stop was called:

```
stop took 9.7s; unmaps [127.0.0.1:43013]
```

The unmap did follow, so nothing is left behind; the stop is slow.

## Suggested fix (Blue's call)

- Renew at `min(lifetime, PortMapLease) / 2`, and treat a lifetime under, say,
  `pmMinRenew` as a failure to keep a mapping (show `failed`, do not
  advertise). Re-ask the external address at each renewal and when it changes
  confirm again (already so for a changed `External`).
- Treat a PCP reply with lifetime 0 as refused.
- In `confirmSelf`, close the connection when the context ends (the same
  `context.AfterFunc` pattern `exchange` uses), so a stop does not wait.

## Not tested

A router that really states a long lifetime; a change of external address
between renewals.

## Resolution

Fixed in BLUE-032 (D183). The renewal comes at half of the granted
lifetime or of `p2p.port_map_seconds`, whichever is shorter, never sooner
than 30 s (`renewAfter`). A lifetime of 30 s or less is refused (`failed`,
removed). A PCP reply with lifetime 0 is refused. The dial-back's dial,
write and wait end with the run's context, so a stop returns at once and
still removes the mapping. Tests: `TestBlue032RenewalCappedByLease`,
`TestBlue032ShortLifetimeRefused`, `TestBlue032PCPLifetimeZeroRefused`,
`TestBlue032StopDoesNotWaitOnSilentDialBack`.
