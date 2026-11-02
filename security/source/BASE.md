# Source base

## Summary

The commit Source/Red attacks and Source/Blue branches from. Source/Grey
updates this file after every merge of a Blue branch. See
[AGENTS.md](../../AGENTS.md#teams-read-first).

| Field | Value |
| --- | --- |
| Branch | `claude/m10-watch` |
| Commit | the merge of Source Round 18 Part A on `main` (Red, Green and Orange: branch from `main` once it is merged) |
| Round | Source Round 18: Part A (Grey) done. Next, Part B: Red, Green and Orange branch from `main`, then Blue (C) and Black (D). Red, please try: the seal check at `Unlock` and at `Hold` (D151), in every wallet command, a rotation made at each step included; `-if-digest` on a file and on standard input; and BLUE-018's `-settings` and `defences`, the import body cap, and the all-or-nothing node bundle (D150). Black: D150 and D151 claim no consensus change |
| Why not `main` | It is `main`: Grey merges each Part A there (owner's rule, 2026-11-02) |
| Checks passing at this commit | gofmt, go vet, staticcheck, govulncheck, go test -race ./..., GOEXPERIMENT=simd tests, scripts/fuzz.sh, TestHeavierLedgerHuge, TestHeavierLedgerLongBareRun (100,000 bare entries), windows, darwin and freebsd builds |
| Merged so far | BLUE-001 (D95), BLUE-002 (D96–D98), BLUE-003 (RED-006, D99), BLUE-004 (RED-007, RED-008; D102, D103); RED-001 to RED-008 (RED-003 won't fix); GREEN-001 to 007, 009, 010, and GREEN-008 in part; Grey: view-only mode and hidden secrets (D101), structured logs with Red's guards and the shutdown fix (D104); Round 4: GREEN-011 to 014 (D106), ORANGE-001 to 005 (D105), GUI-006's asks (D106); Round 5: BLUE-005 (RED-009, RED-010; D107, D108), GREEN-015, GREEN-016 (as M11), ORANGE-006 to 008, M11 grinders (D109), GUI-009 and GREEN-021/022's asks (D110); Round 6: BLUE-006 (RED-011, D111), GREEN-017 to 019, ORANGE-009, ORANGE-011 (prototype), GUI-010, GUI GREEN-026/027 and GUI/Orange's progress cap (D112, D113); Round 7: the consensus audit P0 to P5 (D115 to D117), BLUE-007 (RED-012 to RED-014, D114, reworked by D116), GREEN-020, ORANGE-010 as node policy (D118), ORANGE-012, ORANGE-013, GREEN-031/GUI-013 (D119); Round 8: BLUE-008 (RED-015 to RED-019, D120), B1 and C-1 to C-3 (D121), GREEN-032, ORANGE-014 to 016 (D122); Round 9: BLUE-009 (RED-020 to RED-022, D123), B2, B3 and C-5 with ORANGE-018 (D124), GUI-014 (D125), ORANGE-017 and GREEN-033 (D126), GUI-015 (D127); Round 10: BLUE-010 (RED-023 to RED-025, D128) with C-6 (D129), ORANGE-019, GREEN-034 and the grind limits (D130); Round 11: BLUE-011 (RED-026, RED-027, D131), Black's B4 (D132), the learned pace (D133), GUI-017 (D134), ORANGE-020 and GREEN-035 (D135); Round 12: BLUE-012 (RED-028 to RED-030, D136), Black's B5 (D137), ORANGE-021 and GREEN-036 (D138); Round 13: BLUE-013 (RED-031 to RED-034, GUI-018, D139), Black's B6 (D140), GREEN-037 (D141); Round 14: BLUE-014 (RED-035 to RED-037, ORANGE-023, D142), GREEN-038 and its three commands (D143); Round 15: BLUE-015 (RED-038, D144), GREEN-039, ORANGE-024 (measured), GUI-019 and GUI-020 (D145); Round 16: BLUE-016 (RED-039, D146), GREEN-040, ORANGE-025 with a growing hint, GUI ORANGE-012, the GUI-017 Windows residual (D147); Round 17: BLUE-017 (RED-040 to RED-042, D148), GREEN-041, ORANGE-026, node policy export and import, GUI-022 (D149); Round 18: BLUE-018 (RED-043 to RED-045, D150), GREEN-042, ORANGE-027 (measured), GUI-022 for every command, -if-digest, Black's extreme-settings test (D151) |

## What is in scope

Everything outside `gui/`: the consensus packages, `wire` and `p2p`
(anything a peer sends), `registry`, the local RPC (`pkg/rpc`, including
the new watch stream and follow/reload endpoints), node files and the
store, the wallet, `pkg/cli` and `pkg/daemon`, and the release scripts.
