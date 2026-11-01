# GREEN-032: `layer_floor` that stays set, and `-h` that succeeds

| Field | Value |
| --- | --- |
| Status | adopted (Source Round 8, D122) |
| Based on | `claude/m10-watch` @ `0fd59fa` |
| Area | `pkg/nodepolicy`, `pkg/rpc` (grind schedule), `pkg/cli` (`grind schedule set`, `Run`) |
| Effort | small |
| Consensus | none: node policy (D118, already approved) and command-line behaviour only |
| Decided by | Black (Part D), then Grey |

## Summary

Green tried Round 7's grinder changes (`grind.layer_floor`, `schedule
remove -target`) as a person would. `remove -target` does what D119 says.
`layer_floor` had three problems, and every command's `-h` had one:

1. A `grind` section that sets only `layer_floor` was dropped when it was saved, through the node (`PUT /v1/grind/schedule`, which the GUI uses) or offline. The reply still echoed the value, so the person believed it was set.
2. For the same reason, such a section skipped validation, and the reply accepted `layer_floor: 300` when the limit is 256.
3. No command sets it: `grind schedule set` takes `-device` and `-threads` but not `-layer-floor`, so the person had to edit `node.json`.
4. `poo CMD -h` printed the usage, then `poo: flag: help requested`, and exited 1. Scripts and the GUI terminal saw asking for help as a failure.

All four are prototyped, with tests. GREEN-020 items 3 to 7 (schedule
numbers, lease renewal, flag names in errors, a schedule-change reason,
the empty schedule table) are still open with Grey and are not repeated
here.

## Problem and idea

**1 and 2. A section of only `layer_floor` was lost.** The RPC handler and
the offline path of `grind schedule set` both dropped the section when
`device`, `threads` and `jobs` were empty. That check predates
`layer_floor`. With the section dropped, `Validate` saw no grind section,
so 300 passed, and the reply echoed it. The prototype adds
`nodepolicy.Grind.IsZero()`, which counts every field, and uses it in
both places. A future field cannot be missed the same way, as long as it
is added to `IsZero`.

**3. `grind schedule set -layer-floor S`.** This sets the section's
`layer_floor` like the other defaults. The node checks it (0 to 256) and
applies it without a reload. `docs/modules/poo.md` is updated.

**4. `-h` exits 0.** `cli.Run` returns nil for `flag.ErrHelp`, so `Main`
neither prints "flag: help requested" nor exits 1. Bad flags still fail.
The completion probe calls the inner `run` and is unchanged.

## Testing

- `pkg/rpc` `TestGrindEndpoints`: a schedule of only `layer_floor: 300` is refused, and `12` is saved and read back.
- `pkg/cli` `TestHelpFlagExitsZero`: `pools -h`, `grind schedule set -h` and `peer block --help` exit 0 with the usage and no "help requested".
- By hand: `grind schedule set -layer-floor 300` is refused, and `12` shows in `grind schedule` and in `node.json`.
- gofmt, vet, staticcheck, and `go test -race ./...` pass.

## Risks

None for consensus: `layer_floor` only decides which of its own layers the
node submits (D118), and the rest is command-line behaviour. A script that
relied on `-h` exiting 1 would see 0.
