# RED-030: `poo follow`, `poo unfollow` and `poo wallet follow` still write node.json without the data directory lock

| Field | Value |
| --- | --- |
| Severity | low |
| Status | open |
| Found on | `claude/m10-watch` @ 1ae02ca (Source Round 11 Part B) |
| Component | pkg/cli/view.go (`followCmd`), pkg/cli/wallet.go (`walletFollow`), pkg/cli/node.go (init) |
| Surface | local operator (CLI) |
| Fixed by | |

## Summary

GUI-017 (D134) takes `nodepolicy.Lock` for an offline `poo grind schedule`
edit, so node.json is never written behind a running node whose RPC did not
answer (the node's next write would undo it). Other commands change node.json
with no lock:

- `poo follow` / `poo unfollow`: when `dialNode` or the follow call fails with
  anything but an `rpc.Error` ("the node did not answer: change the file
  instead"), it loads, edits and saves node.json;
- `poo wallet follow`: loads and saves node.json first and only then
  tries the node (no lock either way);
- `poo init -dir` with `-force`.

## Reproduction

A temporary test in `pkg/cli` (not committed): `init -example`, then the test
holds `nodepolicy.Lock(dir)` as a running node does (no `rpc.addr`), and runs
`poo unfollow -series <id> -no-reload -dir <dir>`:

```
follow under another's lock: err=<nil> changed=true
```

(The same command for `grind schedule set` returns `ErrLocked`, per
`TestOfflineScheduleEditTakesTheLock`.)

## Impact

Low. Only a local operator is affected: a follow rule change written
behind a node is lost at the node's next write (or the node's in-memory
policy and the file differ until a reload). No network effect, no
consensus effect.

## Recommendation

Make one helper in `pkg/cli` that does the same as `grindSchedule`'s offline
path (take the lock without waiting, refuse with the same two messages when
a node or another command holds it) and use it in each command that saves
node.json when no node answered; `wallet follow` takes the lock before it
loads. A non-Unix platform keeps the advisory note.

Test: as above, for each command.

## Resolution

Open.
