# RED-081: A work request with `tries` and no `for` may run for hours on a series with a high minimum layer strength

| Field | Value |
| --- | --- |
| Severity | Low |
| Status | fixed |
| Found | Source Round 26 Part B, on `main` @ `439b369` (D168) |
| Area | `pkg/rpc` (`WorkTriesFor`, `work`, `plan`), `pkg/cli` (`poo work`) |
| For Black | No consensus rule is touched. |
| Fixed by | `claude/project-thread-ggo8dt` @ `a117710` (BLUE-027) |

## What is wrong

`MaxWorkTries` is documented as the bound so "a request cannot hold the node
for long" (2^24 tries, about two seconds). D168 makes the bound
`2^(min_layer+4)`, up to 2^40, so that a series with a high floor can be
worked at all. For a request that gives `tries` and no `for`, nothing else
bounds the time: `plan` limits `for` to `MaxGrindFor` (4 minutes), but `tries`
alone has no time limit. At the 9.3 million tries a second I measured for
SHA-512/256 on three threads, a floor of 32 allows 2^36 tries (about two
hours) and the maximum 2^40 about 33 hours, in one HTTP request that holds
the CPU, until the client disconnects.

The series' floor is chosen by whoever creates the series, and a node
follows any series its owner lists. The request needs the owner token, so
this is a footgun rather than an attack: the GUI or a script that passes a
large `tries`, or `poo work` with a default sized from a floor (`DefaultWorkTries`)
on a series with a floor of 36, blocks for hours with no output.

## How I checked it

By reading the code and the arithmetic above; I did not run a two-hour
request. `WorkTriesFor`, `work` and `plan` are the only checks on the path.

## Suggested fix (Blue's call)

When `tries` is above the old 2^24 and no `for` is given, apply
`MaxGrindFor` (or the grind-job path) as the time bound as well, so a
request stops after four minutes whatever the tries; say in the reply that it
stopped on time. Document in `poo work -h` the time a default takes at the
measured rate.
Test: a work request on a series with floor 32 and `tries` 2^36 returns
within `MaxGrindFor`.

## Resolution

Fixed in BLUE-027 (D169).

- A work request with more than `MaxWorkTries` tries and no `for` is bounded by `MaxGrindFor` (4 minutes), as a `for` request is.
- When the bound stops the request, the reply says `stopped_on_time`, or the error says where it stopped and points to a grind job for longer work.
- `poo work -h` says how far 4 minutes goes.
- Test: `TestLargeWorkStopsOnTime` runs a series with floor 40 and 2^34 tries, with the bound set to 300 ms for the test.
