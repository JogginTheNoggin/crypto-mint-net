# BLUE-018: An import takes what to follow unless asked; linear merge; all-or-nothing bundles

| Field | Value |
| --- | --- |
| Fixes | RED-043 (medium), RED-044 (low), RED-045 (low) |
| Branch | `claude/project-thread-ggo8dt` (PR against `main`) |
| Base | `main` @ `b17e892` (Source Round 17 Part C) |
| Code | `cfd1cbd` |
| Decisions | D150 |
| Review | No consensus change and no limit. |

## Summary

- **RED-043.**
  - A default `poo policy import` (and `POST /v1/policy/import`) takes
    only what to follow: series, follows and accepted issuers.
  - Every other setting in the file is a node setting. The plan lists
    those as "skipped, node settings (-settings takes them)".
  - `-settings` takes them. The plan then lists first ("read first") each
    line that turns off a defence or makes the node reach out.
  - A registry that is a local file still needs `-full`.
  - The node's registry fetch refuses non-public addresses unless
    `allow_local_peers`.
- **RED-044.** The follow entries of a file are merged in linear time.
  4000 series now take 0.4 s, down from 10.4 s.
- **RED-045.**
  - A bundle of several node policies is planned whole before any is
    saved.
  - The import endpoint takes any bundle the CLI reads.
  - Windows device names and a trailing `.` are refused for new node
    policies.

## What changed

- `pkg/nodepolicy/bundle.go`:
  - `MergeOptions.Settings`.
  - `Followed`, `Defences`.
  - `Plan.SkippedSettings` and `Plan.Defences`.
  - `Merge` skips node settings unless `Settings` or `Full`, and skips
    file registries unless `Full`.
  - `mergeInto` keeps the built series and calls `addFollows`.
- `pkg/nodepolicy/follow.go`: `addFollows` and `checkFollowIn`.
- `pkg/nodepolicy/manager.go`: `checkNewName`, used by `Manager.Save` for
  a new node policy and by `ReadBundle`.
- `pkg/cli/policyport.go`:
  - `-settings`.
  - Plans every node policy before applying any.
  - The error names what was saved.
  - Prints the defence and skipped-settings lines.
- `pkg/rpc/rpc.go`: `MaxImportBody` for a full-token
  `POST /v1/policy/import`.
- `pkg/p2p/discovery.go` and `addr.go`: `registryClient`, `viaProxy` and
  `publicDialer`.
- Docs:
  - `docs/modules/nodepolicy.md`, `poo.md`, `rpc.md` and `registry.md`
  - `docs/running-a-node.md`
  - `docs/decisions.md` (D150)

## Tests

- `pkg/nodepolicy`:
  - `TestDefaultImportTakesOnlyWhatToFollow`
  - `TestSettingsImportNamesDefences`
  - `TestManySeriesMergeIsLinear`
  - `TestBundleRefusesWindowsNames`
  - `TestAddFollowsMatchesAddFollow`
- `pkg/cli`:
  - `TestNodeBundleImportsAllOrNothing` (fails on b17e892)
  - `TestRunningNodeTakesALargeImport`
  - `TestPolicyImportUsageNamesSettings`
  - `TestPolicyExportImport`, updated: the peer now needs `-settings`
- `pkg/p2p`: `TestRegistryRefusesLocalAddresses`
- `TestMergeAddsAndKeepsLocal`, `TestMergeReplace` and
  `TestMergeRefusesWhatWouldNotValidate` now pass `Settings`.

## For Black

There is no consensus change. Node settings such as `k`, `entry_limit`,
`hash_limit`, `layer_limit` and `max_pending` no longer come in from a
file by default. Red asked you to confirm that none of them can make a
heavier ledger unacceptable.

## For the GUI

The import form should offer `settings`, and show `plan.defences` first
and `plan.skipped_settings` next to `plan.skipped`.
