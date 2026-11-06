# RED-030: `poon follow`, `poon unfollow` and `poon wallet follow` still write node.json without the data directory lock

| Field | Value |
| --- | --- |
| Severity | low |
| Status | fixed |
| Found on | `claude/m10-watch` @ 1ae02ca (Source Round 11 Part B) |
| Component | pkg/cli/view.go (`followCmd`), pkg/cli/wallet.go (`walletFollow`), pkg/cli/node.go (init) |
| Surface | local operator (CLI) |
| Fixed by | `claude/project-thread-ggo8dt` @ 75a71e7 (BLUE-012) |

## Summary

GUI-017 (D134) takes `nodepolicy.Lock` for an offline `poon grind schedule`
edit, so node.json is never written behind a running node whose RPC did not
answer (the node's next write would undo it). Other commands change node.json
with no lock:

- `poon follow` / `poon unfollow`: when `dialNode` or the follow call fails with
  anything but an `rpc.Error` ("the node did not answer: change the file
  instead"), it loads, edits and saves node.json;
- `poon wallet follow`: loads and saves node.json first and only then
  tries the node (no lock either way);
- `poon init -dir` with `-force`.

## Reproduction

A temporary test in `pkg/cli` (not committed): `init -example`, then the test
holds `nodepolicy.Lock(dir)` as a running node does (no `rpc.addr`), and runs
`poon unfollow -series <id> -no-reload -dir <dir>`:

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

Fixed by BLUE-012 (decision D136), as recommended. `lockPolicy`
(`pkg/cli/policylock.go`) takes `nodepolicy.Lock` without waiting and
gives D134's two messages. The offline schedule edit now uses it too.

What each command does:
- `poon follow` and `poon unfollow`, with no node answering, write
  `node.json` only under the lock.
- `poon init -dir` writes under the lock, which covers `-force`.
- `poon wallet follow`, when the node answers, changes the follow list
  through the node: it removes the series' rule without a reload, then
  adds the wallet's pools and applies them. Otherwise it writes under the
  lock.

Test: `TestPolicyWritesTakeTheLock` (`pkg/cli`) covers follow, unfollow
and `init -force` under another process's lock. The online path of
`wallet follow` is covered by the existing light-mode test in
`TestWalletEndToEnd`.
