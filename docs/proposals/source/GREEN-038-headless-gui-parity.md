# GREEN-038: Keep the command line level with the GUI

| Field | Value |
| --- | --- |
| Status | proposed (items 1 and 2 prototyped) |
| Based on | `claude/m10-watch` @ `97456c1` |
| Area | `pkg/cli` (`follow`, a new parity test), `docs/modules/poo.md` |
| Effort | small |
| Consensus | none: the command line only |
| Decided by | Grey (Black may skip it) |

## Summary

jerry's hard rule is that the GUI is only a view over the headless node,
and that its terminal works exactly like `poo`. D139 (GUI-018) gave
`POST /v1/follow` a `replace` that the GUI uses, but `poo follow` could not
send it. So the GUI could set a series' followed pools in one change,
while the command line needed an `unfollow` and then a `follow` (two
reloads, with a window following nothing). Green then swept every
`rpc.Client` method for a `poo` command that calls it.

1. `poo follow -replace` (prototyped). With `-series`, it follows exactly the given pools and mints in place of the series' rule, in one change, through the node or under the lock offline. `unfollow` refuses the flag.
2. A parity test (prototyped). `TestEveryClientCallHasACommand` fails when an `rpc.Client` method is called by no `poo` command and is not listed, with a reason, in `noCommandYet`. It also fails when a listed method has gained a command. So the next GUI-only call shows up in CI rather than in a Green round.
3. What the sweep found (proposals): `SubmitEntry`, `SubmitLayer` and `GrindJob` have no command.

## Problem and idea

**1.** `poo follow -series S -pool 1 -replace` gives `follow: [{series S,
pools [1]}]`, whatever was followed before. `-mint 2 -replace` then gives
`mints [2]` and no pools. The offline path calls the same
`nodepolicy.ReplaceFollow` that the node and `poo wallet follow` use. The
`poo.md` row is updated.

**2.** The test reads `pkg/rpc/client.go` for `func (c *Client) Name(`
and looks for `.Name(` in `pkg/cli`'s non-test files. The list today:

| Method | Why there is no command |
| --- | --- |
| `GrindJob` | `poo grind jobs` lists every job. One job by ID is item 3 |
| `GrindReplaceSchedule`, `GrindSetSchedule` | `poo grind schedule` sends edits (D125), which cover every replacement |
| `SubmitEntry`, `SubmitLayer` | Item 3 |

Limits: the test sees methods, not request fields (it would not have
caught item 1, because `wallet follow` already called `Follow`), and it
sees only GUI calls that go through `rpc.Client`. GUI/Grey could add a
similar check on the GUI side.

**3. Proposals.**

- `poo submit entry|layer -series S -pool K HEX` (or `-file F`) sends an encoded item to the node, as `POST /v1/entry` and `/v1/layer` do. This would let someone grind a layer on another machine and hand it in. It would also make a headless test of these endpoints possible. The node validates every submitted item exactly as one from a peer, so no rule changes.
- `poo grind jobs ID` prints one job (`GET /v1/grind/{id}`). It is cheaper than the whole list for a script that follows one job, and matches what the GUI can show.

## Testing

- `TestFollowReplace`: replacing with a pool, then with a mint, leaves exactly what was given, and `unfollow -replace` is refused.
- `TestEveryClientCallHasACommand` passes today, and fails as it should when an entry is removed from the list.
- By hand, with a running node: `follow -mint 2 -replace` leaves only pool 0 followed, after the reload.
- gofmt, vet, staticcheck, and `go test -race ./...` pass.

## Risks

None for the node. The parity test is a little more work for whoever
adds an `rpc.Client` method: they must add a command or say why not, and
that is the point of it.
