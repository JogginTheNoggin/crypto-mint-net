# RED-056: A settings import can turn on the series the node lists but does not use

| Field | Value |
| --- | --- |
| Severity | Low |
| Status | fixed |
| Found | Source Round 20 Part B, on `main` @ `522e891` (D155) |
| Area | `pkg/nodepolicy` (`Issuers.Any`, `Policy.Accepted`, `settingsPart`), `pkg/cli` (`printPlan`) |
| For Black | No consensus rule is touched; this is which listed series a node uses. |
| Fixed by | `claude/project-thread-ggo8dt` @ 63e37ed (BLUE-021) |

## What is wrong

D155 says a settings import "never takes what to follow", and I checked
that (below). One setting beside the follow list decides which listed
series the node uses: `issuers.any`. With it `false` a series whose issuer
is not accepted is "listed but unused until accepted"
(`Policy.Accepted`). `issuers.any` is a setting, not part of
`Followed`, so a settings file carries it, and `-replace` or
`-prefer-file` sets it. A node's default is `true`, so a file made on a
default node says `true`.

## How I checked it

Node b: `issuers.any: false`, one listed series followed and unused
("warning: series 9d7f095d: issuer not accepted; it is listed but
unused"). Settings exported from a default node (`any: true`):

```
$ poon settings import s.json -replace
  read first: ... issuers.any
  ~ issuers.any: false -> true
```

The warning about the unused series is gone from this plan, and
`Accepted()` of the result has it. The plan names the setting under
"read first", but does not say that the series will now be used and,
because it is followed, mined into and relayed.

## What could happen

An owner who deliberately accepted only some issuers imports a shared
setup and, with `-replace`, silently starts using every series they had
listed but not accepted.

## Suggested fix (Blue's call)

When the result accepts a series that `local` refused, add a plan line
naming them ("now used: series X"), and treat the flip as a conflict
that the default import keeps (it already does without `-replace`).

## What I checked and found fine

For every setting in the node policy (one at a time, from a
reflection-filled policy), a `SettingsOnly` merge, with and without
`-replace`, `-prefer-file` and `-full`, left `series`, `follow` and
`issuers.accept` exactly as they were, and the plan listed every other
change. A bare node.json, a policy bundle with follows and a settings
bundle with follows (refused by `ReadBundle`) behave the same.

## Resolution

Fixed (D156). `Merge` compares the series local refuses
(`Policy.Accepted`) with those the result accepts. Each one the import
turns on is listed in `Plan.NowUsed` and under "read first" ("now used:
series X"). The CLI prints "now used: series X, listed but not accepted
before: the node will follow, mine into and relay it". As Red found, the
default import keeps local's value. `-replace` and `-prefer-file` are
the user's choice to take the file's, and now they say what that turns
on. Test: `TestSettingsImportNamesSeriesNowUsed` (pkg/nodepolicy).
