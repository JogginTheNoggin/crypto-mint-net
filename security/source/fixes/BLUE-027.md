# BLUE-027: No address that can never be spent; release bursts coalesced; hand edits wait for the reload; large work requests bounded in time

| Field | Value |
| --- | --- |
| Fixes | RED-077 (medium), RED-078 (medium), RED-079 (medium), RED-080 (low), RED-081 (low) |
| Branch | `claude/project-thread-ggo8dt` (PR against `main`) |
| Base | `main` @ `439b369` (Source Round 26 Part C), with ORANGE-036 (#178) merged in |
| Decisions | D169 |
| With | ORANGE-036 (PR #178). Its rule of crediting and passing on only the copy taken is the start of RED-078's fix, so the PR carries its commit. |
| Review | No consensus change. For Black: (1) `CheckSpendable` runs only where an address is made or paid, never when a witness is checked, and it refuses only policies no decodable witness could satisfy. The decode limits are unchanged. (2) Coalescing changes when a release is taken, never whether it is valid. |

## Summary

- **RED-077 and RED-079.**
  - `Registry.CheckSpendable` adds up the least witness data a policy can
    be satisfied with, and refuses the policy when that data is over
    `MaxDataSize`, its params are over `MaxParamsSize`, or its witness is
    over `max_witness`.
  - `CheckPolicy` runs it, so the wallet, payment requests and payers
    refuse such a policy.
  - `poo crypto` shows `max_signatures_per_witness` (ML-DSA-87: 14,
    SLH-DSA-SHA2-128s: 8).
- **RED-078.**
  - A release from a peer has its witness checked on arrival.
  - The first release of a series is taken at once. Releases that arrive
    in the next 2 s wait, and only the highest is taken.
  - A copy of the part the node already holds is not counted as a repeat.
  - A replay of parts 2 to 200 now makes about two takes, where it made
    199.
- **RED-080.**
  - `SameButInPlace` compares `grind` and `p2p.peers`.
  - The RPC edits that apply them write the running policy
    (`node.UpdateRunning`).
- **RED-081.**
  - More than 2^24 tries with no `for` stop after 4 minutes, and the
    reply says `stopped_on_time`.

## Consensus

None.

- Every witness that decodes and fits already passes `CheckSpendable`,
  and it is not called on the validation path.
- A release held in the window leaves its part waiting, which is missing
  information. The highest release is still checked in full.

## Tests

- `pkg/policy/blue027_test.go`: `TestCheckSpendable`
- `pkg/p2p/blue027_test.go`: `TestReleaseReplayIsCoalesced`
- `pkg/rpc/inplace_test.go`: `TestInPlaceLeavesHandEditsToTheReload`
- `pkg/rpc/blue027_test.go`: `TestLargeWorkStopsOnTime`
