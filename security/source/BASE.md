# Source base

## Summary

The commit Source/Red attacks and Source/Blue branches from. Source/Grey
updates this file after every merge of a Blue branch. See
[AGENTS.md](../../AGENTS.md#teams-read-first).

| Field | Value |
| --- | --- |
| Branch | `claude/m10-watch` |
| Commit | `afd779f` (Source Round 8 Part A; this file is updated in the commit after it) |
| Round | Source Round 8: Part A (Grey) done. Next, Part B: Red, Green and Orange start from this commit, then Blue (C) and Black (D). Red: please test C-1 (a long branch while the node's own side keeps adding layers), whether unlisted senders can fill the 256 progress places (C-2), the held-branch walk and `topUpLayers` (B1), layers-from paging (C-3, wire 27/52/53), and budgets that count only what they take. Black: D121 and D122 claim no consensus change |
| Why not `main` | The milestone pull requests #1 to #10 are stacked and wait for the owner to merge them; `main` holds only the first commit |
| Checks passing at this commit | gofmt, go vet, staticcheck, govulncheck, go test -race ./..., GOEXPERIMENT=simd tests, scripts/fuzz.sh, TestHeavierLedgerHuge, windows and darwin builds |
| Merged so far | BLUE-001 (D95), BLUE-002 (D96–D98), BLUE-003 (RED-006, D99), BLUE-004 (RED-007, RED-008; D102, D103); RED-001 to RED-008 (RED-003 won't fix); GREEN-001 to 007, 009, 010, and GREEN-008 in part; Grey: view-only mode and hidden secrets (D101), structured logs with Red's guards and the shutdown fix (D104); Round 4: GREEN-011 to 014 (D106), ORANGE-001 to 005 (D105), GUI-006's asks (D106); Round 5: BLUE-005 (RED-009, RED-010; D107, D108), GREEN-015, GREEN-016 (as M11), ORANGE-006 to 008, M11 grinders (D109), GUI-009 and GREEN-021/022's asks (D110); Round 6: BLUE-006 (RED-011, D111), GREEN-017 to 019, ORANGE-009, ORANGE-011 (prototype), GUI-010, GUI GREEN-026/027 and GUI/Orange's progress cap (D112, D113); Round 7: the consensus audit P0 to P5 (D115 to D117), BLUE-007 (RED-012 to RED-014, D114, reworked by D116), GREEN-020, ORANGE-010 as node policy (D118), ORANGE-012, ORANGE-013, GREEN-031/GUI-013 (D119); Round 8: BLUE-008 (RED-015 to RED-019, D120), B1 and C-1 to C-3 (D121), GREEN-032, ORANGE-014 to 016 (D122) |

## What is in scope

Everything outside `gui/`: the consensus packages, `wire` and `p2p`
(anything a peer sends), `registry`, the local RPC (`pkg/rpc`, including
the new watch stream and follow/reload endpoints), node files and the
store, the wallet, `pkg/cli` and `pkg/daemon`, and the release scripts.
