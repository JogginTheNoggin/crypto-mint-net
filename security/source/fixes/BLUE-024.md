# BLUE-024: The node proves itself before the token is sent; the owner's add always finds a place; names count where they lead; builds on a parent are linear and check the pool

| Field | Value |
| --- | --- |
| Fixes | RED-066 (medium), RED-067 (medium), RED-068 (low), RED-069 (low) |
| Branch | `claude/project-thread-ggo8dt` (PR against `main`) |
| Base | `main` @ `215d428` (Source Round 23 Part C), with ORANGE-033 (#143) merged in |
| Code | `e8f71ff` |
| Decisions | D162 |
| With | ORANGE-033 (PR #143): its set-based conflict check is half of RED-068, so the PR carries its commit |
| Review | No consensus change. For Black: `CheckPendingOn` skips the witness check that the pending pool already did; `AddEntry` still checks the entry in full. |

## Summary

- **RED-066.**
  - `Dial` sends the token only after `GET /v1/hello` proves the server
    knows it: an HMAC bound to the address in `rpc.addr` and a fresh
    nonce.
  - A stranger on a stale port gets nothing, and the command falls back
    as when no node runs.
- **RED-067.**
  - The owner's add (hand, `p2p.peers`) takes the place of a reached
    learned record when nothing else gives way. A peer in use now is
    never taken.
  - A configured peer that is refused is logged.
  - A host name the node reaches counts in the range it led to
    (`Record.Via`), so one server behind many names holds at most
    `book_per_range` places.
- **RED-068.**
  - ORANGE-033's sets make the conflict check linear.
  - `CheckPendingOn` does not check the pending pool's witnesses again:
    20,000 transactions take about 30 ms (it was 1.3 s).
- **RED-069.** A transaction on another pool's mint is refused with
  `ErrWrongPool` before the grind.

## Consensus

None.

- The RPC proof and the peer book are local.
- `CheckPendingOn` gives `CheckOn`'s result. A witness satisfies an
  address, and an input's owner is the address of the output it spends,
  whatever the parent. Its result only chooses what this node puts in its
  own entry, and `AddEntry` checks that entry in full.
- `ErrWrongPool` is `AddEntry`'s own refusal, given earlier.

## Tests

- `pkg/rpc`:
  - `TestDialSendsNoTokenToAStranger`
  - `TestDialProvesTheNode`
- `pkg/peers`:
  - `TestNamesForOneMachineShareARange`
  - `TestOwnerAddTakesAReachedPlace`
  - `TestGiveWayQueuesStayBounded` (now with the reached heap)
- `pkg/ledger`:
  - `TestCheckPendingOnIsLinear`
  - `TestCheckPendingOnMatchesCheckOn`
- `pkg/engine`:
  - `TestBuildOnRefusesAnotherPool`
- `pkg/cli`:
  - `TestDoctorStoppedNode` (a stale `rpc.addr` is still named left over)
