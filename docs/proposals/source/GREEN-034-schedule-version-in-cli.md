# GREEN-034: The schedule's version on the command line

| Field | Value |
| --- | --- |
| Status | adopted (D130) |
| Based on | `claude/m10-watch` @ `1ae4d46` |
| Area | `pkg/cli` (`grind schedule`), `docs/modules/poon.md` |
| Effort | small |
| Consensus | none: the grinding schedule in node.json and the command line only |
| Decided by | Grey (Black may skip it) |

## Summary

D125 gave the grind schedule a `version`, so a change can say which
version it was made against and be refused if the section has changed
since. The RPC has it: `GET /v1/grind/schedule` returns it, a `PUT` honours
`If-Match`, and an edit can carry `version`. The command line had two
gaps:

1. `poon grind schedule` always printed `"version": ""`, even with a running node, whose own reply carries the real version. The listing builds a new reply from the section and leaves the version out.
2. No `poon grind schedule` change could pass a version, so a script could not say "only if nobody changed it since I looked". This matters most for `remove -all` and `set`, which name no job.

The prototype prints the real version (`cur.Version()`, the same function
the node uses, online or offline). It also adds `-if-version V` to `add`,
`remove` and `set`, which goes into `GrindEdit.Version`. A stale version
fails with "the grind schedule changed meanwhile: list it again", the
same message as a stale number.

## Problem

Found while using the new schedule as a person would:

```
$ poon grind schedule | tail -2
  "version": ""
$ curl .../v1/grind/schedule
{"grind":{"jobs":[{"target":"tip","hours":"22-06"}]},"version":"c86180e716b7f277"}
```

The listing reads the section through the node, then writes
`rpc.ScheduleReply{Grind: cur}` and drops the version. Even if it printed
the version, nothing on the command line could send it back.

## Idea

- The listing writes `Version: cur.Version()`. Online, this is the same hash the node returned, because the node uses the same function on the same section.
- `add`, `remove` and `set` take `-if-version V` and set `edit.Version`. Online, the node checks it under its lock (D125). Offline, `GrindEdit.Apply` checks it against node.json.
- The `poon.md` row for `grind schedule` names both.

`remove -target T` from D119 still works. Since D125, a numbered removal
names the whole job, so `-target` is now only a check of what the person
meant. Grey may want to say so in its help, or keep it as it is.

## Testing

- `TestGrindScheduleOffline` now checks that the listing has a version, that an add with that version is applied, and that a removal against the old version is refused.
- By hand, with a running node: the listed version matches the RPC's, and `remove 1 -if-version OLD` exits 1 with the message above.
- gofmt, vet, staticcheck, and `go test -race ./...` pass.

## Risks

None. A change with no `-if-version` behaves as before.
