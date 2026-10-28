# Source base

## Summary

The commit Source/Red attacks and Source/Blue branches from. Source/Grey
updates this file after every merge of a Blue branch. See
[AGENTS.md](../../AGENTS.md#teams-read-first).

| Field | Value |
| --- | --- |
| Branch | `claude/m10-watch` |
| Commit | `c71f7748ab6942d9cd76017a7779700536bab79b` (Source Round 2 Part A; this file is updated in the commit after it) |
| Round | Source Round 2: Part A (Grey) done. Next, Part B: Red and Green start from this commit |
| Why not `main` | The milestone pull requests #1 to #10 are stacked and wait for the owner to merge them; `main` holds only the first commit |
| Checks passing at this commit | gofmt, go vet, staticcheck, govulncheck, go test -race ./... |
| Merged so far | BLUE-001 (D95), BLUE-002 (D96–D98), BLUE-003 (RED-006, D99); RED-001 to RED-006 (RED-003 won't fix); GREEN-001 to 005, GREEN-007 (D100); Grey: view-only mode and hidden secrets (D101); RED-007 filed, open (carries to Part C) |

## What is in scope

Everything outside `gui/`: the consensus packages, `wire` and `p2p`
(anything a peer sends), `registry`, the local RPC (`pkg/rpc`, including
the new watch stream and follow/reload endpoints), node files and the
store, the wallet, `pkg/cli` and `pkg/daemon`, and the release scripts.
