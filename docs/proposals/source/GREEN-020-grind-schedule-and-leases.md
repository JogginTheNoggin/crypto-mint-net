# GREEN-020: Schedules, leases and tx targets, used as a person would

| Field | Value |
| --- | --- |
| Status | items 1 and 2 adopted (Source Round 7, D119); the rest open |
| Based on | `claude/m10-watch` @ `9972137` |
| Area | `pkg/grind`, `pkg/cli` (`grind`, `table.go`) |
| Effort | small |
| Decided by | Grey |

## Summary

Green ran a node and used `poo grind schedule`, `-lease` and `tx:TID`
targets as a person would. All three do what GREEN-017, GREEN-019 and
GREEN-026 asked for. A `tx:TID` job ground the payment's entry until from(e)
reached the goal, then ended `done`. An unknown TID waits with a useful
hint, and a lapsed lease stops its job with a clear reason. Seven rough
edges remain. Two of them are prototyped, with tests.

1. A job started outside its hours is reported `running` (prototyped).
2. `-o table` printed `tx:` targets and start times in full (prototyped).
3. `grind schedule` does not number its jobs, but `schedule remove N` takes those numbers.
4. `grind start -lease` cannot be renewed from the command line.
5. Errors name JSON fields (`until_from`) where the person typed flags (`-until-from`).
6. A job replaced by `schedule set` ends with the reason `stopped`.
7. An empty schedule prints `grind  -` in a table.

## Problem and idea

**1. Waiting jobs start as `running` (prototyped).** `poo grind schedule
add pending -hours 22-06` at 10:24 replied with the job in state
`running`, and so did the `start` watch event. A moment later `grind jobs`
showed it `waiting`, "outside its hours 22-06". A person reading the reply
believes the job is grinding now. The prototype has `Manager.Start` set
the job's state, reason and hint before the reply and the `start` event.
It uses a new `outsideHours` helper, which the runner's loop also uses, so
the two can never disagree. Test: `TestStartOutsideHours`.

**2. Table cells for `tx:` targets and times (prototyped).** A `tx:TID`
target was printed as all 67 characters, so `grind jobs -o table` became
very wide. `started` was printed to the nanosecond. In table mode, a hash
after a short prefix (`tx:`) is now shortened like any hash
(`tx:45634243..525a`), and an RFC 3339 time with a fraction is printed to
the second. `-wide` and the JSON are unchanged. Test:
`TestTablePrefixedHashAndTime`.

**3. Number the schedule's jobs.** `poo grind schedule remove 2` removes
"the jobs numbered from 1 as listed", but neither the JSON nor the table
shows a number, so the person has to count. The table could add a `#`
column. The JSON is `node.json`'s own `grind` section, so it should stay
as it is, and the numbers would appear only in the table.

**4. A lease the command line cannot renew.** `grind start -lease 30s`
starts a job that stops 30 seconds later unless something calls
`POST /v1/grind/ID/renew`. No `poo` command does this, so from a shell
the flag only means "stop after 30 seconds", which `-for` already says.
There are two ways to fix this. One is a `poo grind renew ID` command,
for scripts that follow a job themselves. The other is to keep `-lease`
on the RPC and `grind run` only, and drop it from `grind start`. The
help line also starts with a stray `start:`.

**5. Errors that name the JSON field.** `grind start tx:TID` with no goal
says "a tx:TID target needs until_from", and `-until-from 9e9` says
"until_from: weight: invalid". The person typed `-until-from`. The core
check is shared with the RPC, so the command line could rewrite the field
names to flag names (`until_from` to `-until-from`), the way `flagsFor`
already names flags.

**6. The reason when the schedule changes.** `grind schedule set
-threads 2` restarted the scheduled jobs with the new defaults, which is
right. But the old job ended with the reason `stopped`, as if the person
had stopped it. "the schedule changed" would say why.

**7. An empty schedule in a table.** `grind schedule -o table` with no
jobs prints `grind  -`. "(no scheduled jobs)" would read better.

## Testing

Items 1 and 2 have the tests named above. The full suite passes with the
race detector, along with gofmt, vet and staticcheck.

## Risks

Item 1 changes the `start` watch event's state from always `running` to
`waiting` for a job outside its hours. The GUI reads that event, so
GUI/Grey should hear of it. No consensus code is touched.
