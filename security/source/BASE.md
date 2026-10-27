# Source base

## Summary

The commit Source/Red attacks and Source/Blue branches from. Source/Grey
updates this file after every merge of a Blue branch. See
[AGENTS.md](../../AGENTS.md#teams-read-first).

| Field | Value |
| --- | --- |
| Branch | `claude/m10-watch` |
| Commit | `6278ae910ee1613ad099e73bda218eb6f80df61e` (M10 with BLUE-001, RED-001 to 003 and GREEN-001, 002 merged; this file is updated in the commit after it) |
| Why not `main` | The milestone pull requests #1 to #10 are stacked and wait for the owner to merge them; `main` holds only the first commit |
| Checks passing at this commit | gofmt, go vet, staticcheck, govulncheck, go test -race ./... |
| Merged so far | BLUE-001 (Argon2id wallets, D95); Red findings RED-001 to RED-003; Green GREEN-001 (help and typos), GREEN-002 (metrics) |

## What is in scope

Everything outside `gui/`: the consensus packages, `wire` and `p2p`
(anything a peer sends), `registry`, the local RPC (`pkg/rpc`, including
the new watch stream and follow/reload endpoints), node files and the
store, the wallet, `pkg/cli` and `pkg/daemon`, and the release scripts.
