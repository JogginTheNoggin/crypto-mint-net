# RED-074: `add-release` says ok to a forged release of a part the node already knows, and the release JSON takes unknown keys

| Field | Value |
| --- | --- |
| Severity | Low |
| Status | fixed |
| Found | Source Round 25 Part B, on `main` @ `e93eb29` (D166) |
| Area | `pkg/series` (`AddRelease`), `pkg/rpc` (`POST /v1/series/release`), `pkg/cli` (`series add-release`) |
| For Black | No consensus rule is touched. D165's point is kept: a part already known costs no hashing. |
| Fixed by | `claude/project-thread-ggo8dt` @ `15e8f34` (BLUE-026) |

## What is wrong

1. `Series.AddRelease` returns `false, nil` for any release whose part is at
   or below the newest known, before it checks anything. The CLI and the
   RPC print the same answer for a genuine old release and a forged one:
   exit 0, `"added": false`. A user who feeds a file to `add-release` to see
   whether it is genuine is told it is fine.

2. `poon series add-release` reads the file with plain `json.Unmarshal` into
   `series.Release`. An unknown key is accepted (`poon series release -check`
   prints `valid: true` for a release with `"extra": 1`), and node.json is
   saved without it. Series files refuse unknown keys since D165 (RED-071),
   so a later field of a release would be dropped here instead of making
   the file unreadable.

## How I checked it

```
$ poon series add-release -dir n1 t_stale_forged.json     (part 2, witness changed, node at part 2)
{ "part": 2, "released": 2, "mints": 48, "added": false }        exit 0
$ poon series release -check t_stale_forged.json v.json
poon: series: invalid release: witness: policy: witness does not satisfy the policy: signature for key 0
$ poon series add-release -dir n1 t_p3_forged.json        (part 3, forged)  -> invalid, exit 1
```

So the same file is "invalid" to `-check` and "ok" to `add-release`.

## What could happen

A script that adds releases from a feed and trusts exit 0 believes it holds
releases it does not. In phase 3 the same answer would sit behind the peer
path. Nothing about ledgers is affected.

## Suggested fix (Blue's call)

- Say what was done: add `"checked": false` (or a note "already known; not
  checked") when the part is not newer. Or check the witness first for the
  CLI and RPC (a signature check; no hashing), and keep the free answer for
  the peer path.
- Read releases with `DisallowUnknownFields`, the way series files are read.
- Tests: the forged old release answers `invalid` or `not checked`; a release
  with an unknown key is refused.

## Resolution

Fixed in BLUE-026 (D167).

- `Policy.AddRelease` (the CLI and RPC path) checks every release in full, also one of a part already known, so a forged one is refused. The free answer for a known part stays in `Series.AddRelease`, for the peer path.
- Release JSON refuses a key it does not know (`series.ErrUnknownKey`). The keys `poon series release` adds for people (`part_id`, the ranges, `mints`, `units`) are read and ignored.
- Tests: `TestAddReleaseChecksAKnownPart` (another key's release of part 2, and the kept release with a changed previous id, are refused), `TestReleaseFileKeys`.
