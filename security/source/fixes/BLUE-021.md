# BLUE-021: Plain exports leave out personal data; file text is checked and escaped; lone conflicts save; -map-path joins; series now used are named; a stated weight indexes no pool

| Field | Value |
| --- | --- |
| Fixes | RED-052 (medium), RED-053 (medium), RED-054 (low), RED-055 (low), RED-056 (low), RED-057 (medium) |
| Branch | `claude/project-thread-ggo8dt` (PR against `main`) |
| Base | `main` @ `522e891` (Source Round 20 Part C) |
| Code | `63e37ed` |
| Decisions | D156 |
| Review | No consensus change and no limit on what can be followed. For Black: RED-057 (index records for chases) and the `Validate` text check (RED-053). |

## Summary

- **RED-052.**
  - A plain export leaves out local registry files, registry URLs with a
    user, password, query or fragment, and grind jobs on one payment or
    entry (`Personal`).
  - It says what the file holds (peers, tracked peers, registries) and
    what it left out.
  - `-full` keeps everything.
- **RED-053.**
  - `Validate` refuses control characters, bidirectional overrides and
    bytes that are not UTF-8 in any text.
  - The plan printing escapes every value from a file (`termSafe`).
  - The fit check never opens a network or device path.
- **RED-054.** A conflict that took the file's value counts as a change,
  so it is saved.
- **RED-055.**
  - `-map-path` joins with one separator and matches Windows paths in
    any case.
  - A mapped registry file is taken without `-full`.
- **RED-056.** A series an issuer setting turns on is named ("now used")
  under "read first".
- **RED-057.**
  - A branch offered in a pool of a big whole series no longer indexes
    the pool.
  - The record is kept for the chase only and dropped once the queue
    holds no chase for it.
  - The pool is indexed when something is kept in it.

## Measured

Red's case for RED-057 is a series of 2^40 pools followed whole, with
entries that build on nothing in a different pool each and a stated
weight of 2^62. `TestStatedWeightIndexesNoPool` runs it:

| | before (522e891) | after |
| --- | --- | --- |
| Pools indexed by 20 such entries | each indexed its pool for good (Red: 1200 a minute per address) | 0 |
| Chase records once the sender leaves | none kept apart: the pools stayed in the index | 0 |

## Consensus

None.

- No change touches which ledger a node selects.
- The p2p change only decides when a pool joins the index. A chase runs
  as before, and a chase that keeps something indexes the pool.
- The text check refuses only text no setting can validly hold.
- No series and no follow is capped.

## Tests

- `pkg/nodepolicy` (blue021_test.go):
  - `TestExportLeavesOutPersonal`
  - `TestControlCharactersRefused`
  - `TestFitDoesNotOpenNetworkPaths`
  - `TestPreferFileConflictAloneIsAChange`
  - `TestMapPathJoins`
  - `TestMapPathWithoutFull`
  - `TestSettingsImportNamesSeriesNowUsed`
- `pkg/cli` (blue021_test.go, and settingsport_test.go updated):
  - `TestImportRefusesTerminalEscapes`
  - `TestTermSafe`
  - `TestPreferFileAppliesALoneConflict`
  - `TestSettingsExportImport`
- `pkg/p2p` (blue021_test.go):
  - `TestStatedWeightIndexesNoPool`
