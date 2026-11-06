# RED-013: Concurrent writes of node.json share one temporary file and can leave it corrupt

| Field | Value |
| --- | --- |
| Severity | low |
| Status | fixed |
| Found on | `claude/m10-watch` @ 9972137 (code 84d3e69, Source Round 6 Part B) |
| Component | pkg/nodepolicy/nodepolicy.go (Save), pkg/rpc/grind.go (grindSetSchedule), pkg/rpc/view.go (follow), pkg/grind/jobs.go (Manager.Schedule) |
| Surface | RPC / node files |
| Fixed by | `claude/project-thread-ggo8dt` @ 80ea33e (BLUE-007) |

## Summary

`PUT /v1/grind/schedule` (D113) is a second RPC that rewrites `node.json`,
next to the follow and unfollow endpoints. Each handler reads the file
(`nodepolicy.Load`), changes one section and writes it back with
`nodepolicy.Save`, with nothing that serialises the three steps:

```go
// pkg/nodepolicy/nodepolicy.go, Save
tmp := path + ".tmp"                                   // one name for every writer
os.WriteFile(tmp, append(b, '\n'), 0o600)              // O_TRUNC, then write
os.Rename(tmp, path)
```

Two writers in flight open the same `node.json.tmp` with truncation and write
into it at once. If the document of the first is longer, bytes of it remain
after the second's, and the rename publishes a file that is not JSON; the
other writer's rename then fails with "no such file". Independently, the
read-modify-write loses an update: two PUTs, or a PUT and a follow, that both
read the old file each write back only their own change.

Whoever does this holds the control token (the owner, a GUI, a script, the
terminal), so it is not an attack by a stranger. It is easy to do by
accident: a GUI schedule editor and `poon grind schedule add` in a terminal,
or two scripts, or a double click sending the same PUT twice.

## Reproduction

A temporary test in `pkg/nodepolicy` (not committed): four goroutines per
round call `Save` on one path, two with the default policy and two with a
16-job grind section; then `Load`; 3,000 rounds:

```
rounds=3000  loads ok=2784  corrupt=216  save errors=7722
first corrupt load: invalid character '"' after top-level value
```

A corrupt `node.json` fails `Load`, so the next `poon node` start refuses to
run, and every RPC that loads it answers with an error until the file is
repaired by hand.

## Impact

Low. The trigger is concurrent use by the owner, the loss is the node's
configuration file (rebuildable, but the node does not start meanwhile), and
there is no remote or privilege angle. It is worth fixing because D113 makes
the file a live API that more than one program (GUI, CLI, scripts) edits.

## Recommendation

- Serialise the policy read-modify-write in the `rpc.Handler` with one
  mutex held from `Load` to `Save` for follow, unfollow and schedule, and
  take the same lock for `Manager.Schedule` (below).
- Make `Save` safe on its own: write to `os.CreateTemp(dir, FileName+".*")`,
  `Sync` it, rename it, and sync the directory, so even an offline `poon`
  command racing the node cannot tear the file.
- `Manager.Schedule` is `StopAll("policy")` then `Start` for each job, with
  no lock across the two; two overlapping calls can leave the old and the new
  policy jobs running together (up to `MaxJobs`). Hold a schedule mutex across
  it.

Tests: concurrent `Save` calls always leave a loadable file (the test above);
two concurrent PUTs of different schedules leave `node.json` equal to one of
them and exactly that one's jobs running.

## Resolution

Fixed by BLUE-007 (decision D114). `nodepolicy.Save` writes to its own
`os.CreateTemp` file next to `node.json`, syncs it, renames it and syncs the
directory, removing the temporary file on any error. Concurrent writers, the
offline `poon` commands included, can no longer tear the file. In the
node, `rpc.Handler.policyMu` is held from `Load` to `Save` for follow,
unfollow and `PUT /v1/grind/schedule`, which also applies the schedule
under it. `grind.Manager.Schedule` takes its own mutex, so two calls never
leave both policies' jobs running. A `poon` command editing `node.json`
offline while the node edits it can still lose one of the two updates (no
lock is shared across processes). Tests: `TestConcurrentSavesNeverTear`
(`pkg/nodepolicy`, 200 rounds of four writers, no leftover files),
`TestScheduleIsSerialised` (`pkg/grind`, which fails on the old code), and
concurrent `PUT /v1/grind/schedule` in `TestGrindEndpoints` (`pkg/rpc`).
