# BLUE-032: The dial-back proves it reached this node; a failed mapping request leaves nothing at the router; renewal and stop no longer follow the router

| Field | Value |
| --- | --- |
| Fixes | RED-090 (medium), RED-091 (low), RED-092 (low) |
| Branch | `claude/project-thread-ggo8dt` (PR against `main`) |
| Base | `main` @ `4c0f708` (Source Round 31 Part C), with GREEN-056 (#234) merged in |
| Decisions | D183 |
| With | GREEN-056 (PR #234), carried: it changes the same `pkg/p2p/portmap.go` (`holdMapping` returns whether a mapping was held). |
| Review | No consensus change. For Black: port mapping decides only what a node tells peers about itself and whether other nodes can dial it, never which ledger is valid. |

## Summary

- **RED-090.**
  - `confirmSelf` sends a hello whose `Agent` carries a random token.
  - Only this node's listener recording that token (`sawSelfProbe`, in
    the handshake's self check) confirms the address.
  - The far end's answer can only end the wait early with a failure.
  - The wire format is unchanged.
- **RED-091.**
  - UPnP asks for the external address before mapping.
  - An unusable PCP grant is removed.
  - A request whose answer was not read gets a best-effort removal of
    at most 1 s.
- **RED-092.**
  - The renewal comes at half of the lifetime or the lease, whichever is
    shorter.
  - A lifetime of 30 s or less is refused, and so is a PCP lifetime of 0.
  - The dial-back ends with the run's context.

## Consensus

None.

## Tests

- `pkg/p2p/blue032_test.go`: `TestBlue032DialBackNeedsOwnListener`, `TestBlue032StopDoesNotWaitOnSilentDialBack`, `TestBlue032ShortLifetimeRefused`, `TestBlue032RenewalCappedByLease`
- `pkg/portmap/blue032_test.go`: `TestBlue032UPnPExternalFailsLeavesNoMapping`, `TestBlue032UPnPCancelRemoves`, `TestBlue032PCPUnusableRemoved`, `TestBlue032PCPLifetimeZeroRefused`, `TestBlue032CancelSendsRemoval`
- Each fails on the code before the fix.
