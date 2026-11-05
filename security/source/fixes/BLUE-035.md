# BLUE-035: Hello frames bounded and capped, queues bounded in bytes, import reads bounded

| Field | Value |
| --- | --- |
| Fixes | RED-100 (medium), RED-101 (low), RED-102 (low), RED-103 (low, in part) |
| Branch | `claude/project-thread-ggo8dt` (PR against `main`) |
| Base | `main` @ `f1ac406` (Source Round 34 Part C), with GREEN-059 (#277) merged in |
| Decisions | D191 |
| With | GREEN-059 (PR #277), carried. ORANGE-044 (PR #276): P1's handshake count and P2's byte cap are built here; P3, P4 and a per-address handshake cap are left for Grey. |
| Review | No consensus change. For Black: frame and queue limits and tool input checks only; no entry is refused by size, and a ledger of any weight is still accepted. For Black and White: a discovery frame limit (RED-103's second fix) is not taken because D190 tests a 2 MiB ping at the limits; it would change wire behaviour frozen at the tag. |

## Summary

- **RED-100.**
  - `ReadFrame` allocates as bytes arrive.
  - The hello is read under `wire.MaxHelloFrame` (3 MiB).
  - Connections still saying hello count against `MaxInbound`.
- **RED-101.** `decodeImport` stops reading at the batch bound.
- **RED-102.** Ledger file headers: known keys only, 4 KiB each, once each;
  early entries spooled to a temporary file.
- **RED-103.** Inbox queues bounded in bytes (32 MiB each); no discovery
  frame limit.

## Consensus

None.

## Tests

- `pkg/wire/blue035_test.go`: `TestBlue035FrameMemoryFollowsBytes`
- `pkg/p2p/blue035_test.go`: `TestBlue035HelloBoundAndCap`
- `pkg/p2p/blue035_internal_test.go`: `TestBlue035InboxBoundInBytes`
- `pkg/rpc/blue035_internal_test.go`: `TestBlue035ImportReadStops`
- `pkg/cli/blue035_test.go`: `TestBlue035LedgerHeaderBounded`
- The frame, hello, import-read and header tests fail on the code before the fix.
