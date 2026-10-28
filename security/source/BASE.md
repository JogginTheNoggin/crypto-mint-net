# Source base

## Summary

The commit Source/Red attacks and Source/Blue branches from. Source/Grey
updates this file after every merge of a Blue branch. See
[AGENTS.md](../../AGENTS.md#teams-read-first).

| Field | Value |
| --- | --- |
| Branch | `claude/m10-watch` |
| Commit | `0ac8e4e666495f58c4fadf3072bfb843ba2d5c74` (Source Round 4 Part A; this file is updated in the commit after it) |
| Round | Source Round 4: Part A (Grey) done. Next, Part B: Red, Green and Orange start from this commit. Red: please review ORANGE-002's cache key, ORANGE-004's preverify (now gated by `entry.CheckShape`) and ORANGE-005's group commit (D105) |
| Why not `main` | The milestone pull requests #1 to #10 are stacked and wait for the owner to merge them; `main` holds only the first commit |
| Checks passing at this commit | gofmt, go vet, staticcheck, govulncheck, go test -race ./..., scripts/fuzz.sh |
| Merged so far | BLUE-001 (D95), BLUE-002 (D96–D98), BLUE-003 (RED-006, D99), BLUE-004 (RED-007, RED-008; D102, D103); RED-001 to RED-008 (RED-003 won't fix); GREEN-001 to 007, 009, 010, and GREEN-008 in part; Grey: view-only mode and hidden secrets (D101), structured logs with Red's guards and the shutdown fix (D104); Round 4: GREEN-011 to 014 (D106), ORANGE-001 to 005 (D105), GUI-006's asks (D106) |

## What is in scope

Everything outside `gui/`: the consensus packages, `wire` and `p2p`
(anything a peer sends), `registry`, the local RPC (`pkg/rpc`, including
the new watch stream and follow/reload endpoints), node files and the
store, the wallet, `pkg/cli` and `pkg/daemon`, and the release scripts.
