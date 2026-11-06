# BLUE-031: Snapshots name the release that wrote them; every start checks the restore; leftover temporary snapshots removed

| Field | Value |
| --- | --- |
| Fixes | RED-087 (medium), RED-088 (medium), RED-089 (low) |
| Branch | `claude/project-thread-ggo8dt` (PR against `main`) |
| Base | `main` @ `cf8c859` (Source Round 30 Part C), with GREEN-055 (#224) merged in |
| Code | `957d707` |
| Decisions | D181 |
| With | GREEN-055 (PR #224), carried: it changes the same `-reverify` code in `loadPool` and the doctor's `snapshots` check. |
| Review | No consensus change. For Black: a snapshot is a cache of what the log builds. These changes only make a node use one less often (another release, a restore that does not match), never decide anything from one. |

## Summary

- **RED-087.**
  - The build id is `poon snap<N> <version> <commit>`, then the module
    version and VCS revision.
  - `cli` calls `node.SetRelease(Version, Commit)` at init.
  - A dirty tree adds an executable hash.
  - `snapshotSemantics` is bumped by hand.
- **RED-088.**
  - Every start compares the restored fingerprint with the header's, and
    refuses the snapshot on a difference.
  - `-reverify` lists the pools it compared nothing for, and doctor
    warns.
- **RED-089.**
  - `.snap.tmp` is removed when its log is opened, counted, and named by
    doctor.

## Consensus

None.

## Tests

- `pkg/node/blue031_test.go`: `TestBuildIDNamesTheRelease`, `TestSnapshotRestoreIsChecked`, `TestReverifyBehindTheLogIsReported`, `TestSnapshotTmpRemoved`
- `pkg/cli/blue031_test.go`: `TestSnapshotsCheckNotComparedAndTmp`
