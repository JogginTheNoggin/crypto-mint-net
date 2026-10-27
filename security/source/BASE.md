# Source base

## Summary

The commit Source/Red attacks and Source/Blue branches from. Source/Grey
updates this file after every merge of a Blue branch. See
[AGENTS.md](../../AGENTS.md#teams-read-first).

| Field | Value |
| --- | --- |
| Branch | `claude/m10-watch` |
| Commit | `a740841e53101606e7249af2539b67616b876668` (M10 with BLUE-001, BLUE-002, RED-001 to 006, GREEN-001, 002 and 004 merged; this file is updated in the commit after it) |
| Why not `main` | The milestone pull requests #1 to #10 are stacked and wait for the owner to merge them; `main` holds only the first commit |
| Checks passing at this commit | gofmt, go vet, staticcheck, govulncheck, go test -race ./... |
| Merged so far | BLUE-001 (D95), BLUE-002 (RED-001, 002, 004, 005; D96–D98); Red findings RED-001 to RED-006 (RED-003 won't fix, accepted by Grey; RED-006 open); Green GREEN-001, 002, 004 |

## What is in scope

Everything outside `gui/`: the consensus packages, `wire` and `p2p`
(anything a peer sends), `registry`, the local RPC (`pkg/rpc`, including
the new watch stream and follow/reload endpoints), node files and the
store, the wallet, `pkg/cli` and `pkg/daemon`, and the release scripts.
