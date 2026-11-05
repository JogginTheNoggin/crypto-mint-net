# RED-094: A kept unknown value nested 9,990 deep makes `Save` turn a 19 KiB node.json into 190 MiB, and `GET /v1/status` loads node.json on every request

| Field | Value |
| --- | --- |
| Severity | Medium |
| Status | fixed |
| Found | Source Round 32 Part B, on `main` @ `6260590` (D184, W-1) |
| Area | `pkg/nodepolicy/nodepolicy.go` (`Save`), `pkg/nodepolicy/unknown.go`, `pkg/rpc/rpc.go` (`status`) |
| For Black | Not a rule. |
| Fixed by | `claude/project-thread-ggo8dt` @ HASH (BLUE-033) |

## What is wrong

`Save` runs `json.Indent` over the whole file once the policy holds unknown
settings. `Load` accepts a value nested up to the decoder's limit (10,000),
and a kept unknown value is written back as it is. Indent puts two spaces per
level on every line, so a value of depth *d* with one bracket per line grows
as *d*². There is no limit on the size of node.json or of a kept value
(before D184 an unknown value was skipped and never written).

Measured (`pkg/nodepolicy`, throwaway): a node.json of 19 KiB holding
`"future": [[[…]]]` nested 9,990 deep.

```
Load  8 ms          (19 KiB)
Save  6.6 s         → 190 MiB
Load  2.6 s, Save 2.1 s   (stays 190 MiB; every later load and save pays this)
```

Because the output is stable, the file stays that big for good. The cost
lands everywhere node.json is read:

- `GET /v1/status` calls `nodepolicy.Load` on every request (to set
  `policy_changed`), with no lock, so each poll by a front end reads and
  decodes 190 MiB (the load alone took 1.7 to 2.6 s here; memory was not
  measured), and polls that overlap would add up.
- Every start, `poo reload`, and every settings command that saves
  (`policyMu` is held during the 2 s `Save`).
- The disk: 190 MiB for a setting file, plus the temporary file of the same
  size while it is written.

Other shapes were cheap (a 16 MiB string, a 2 million-element array cost
about 0.15 s each way and the file does not grow beyond a few times its
size), so depth is what is dangerous.

## How I checked it

Throwaway test, rounds of `Load` then `Save` on the same path (above). Not
run: the daemon's behaviour under overlapping status polls (read from
`status`).

## Suggested fix (Blue's call)

- Do not indent the kept values: write the known fields indented and each
  kept value compacted (`json.Compact`), or indent with a depth limit.
- Cap what is kept: the number of unknown settings, the size of each value
  and the depth (say 64 levels, 64 KiB), and say at start what was left out.
  An unknown value over the cap should be dropped with a warning, not kept.
- Cap the size `Load` reads (node.json has no legitimate reason to be
  megabytes), and give `status` a cached load (the file's modification time)
  instead of a decode on each request.

## Not tested

A front end polling status against such a file for a long time.

## Resolution

Fixed in BLUE-033 (D185). `Load` keeps at most 256 unknown settings,
each at most 64 KiB compacted and 64 levels deep. One over a limit is
named in `UnknownDropped` with the limit. `Save` writes kept values
compacted, on one line, so Red's 9,990-deep file keeps its size.
`GET /v1/status` loads node.json again only when the file is replaced,
or its size or modification time changes. A cap on node.json's size is
not taken: kept variable series releases can make a real file large.
Tests: `TestBlue033KeptLimits`, `TestBlue033KeptCompacted`,
`TestBlue033StatusCachesPolicy`.
