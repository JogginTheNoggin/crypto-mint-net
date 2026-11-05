# RED-087: Every release build names itself `poo (devel)`, so the "never trust a pool an older build computed" check cannot tell one release from another

| Field | Value |
| --- | --- |
| Severity | Medium (latent: it first matters at the second release, and the fix is a few lines to make before v0.1.0 is tagged) |
| Status | open |
| Found | Source Round 30 Part B, on `main` @ `cf8c859` (D178) |
| Area | `pkg/node/snapshot.go` (`buildID`, `loadSnapshot`), `scripts/release.sh`, `pkg/cli` (`Version`, `Commit`) |
| For Black | No consensus rule is touched. This is the guard D178 gives for "an upgrade replays once", and for the day a release changes how a pool is computed it is the only thing standing between a restored pool and a replayed one. |
| Fixed by | |

## What is wrong

D178: a node uses a snapshot only if "the build that wrote it" matches, so
"an upgrade replays once, so it never trusts a pool an older build
computed". `buildID()` builds that name from `debug.ReadBuildInfo`: the main
module's version, plus `vcs.revision` and `vcs.modified` when the binary has
them.

`scripts/release.sh` builds with `-trimpath -buildvcs=false` (for
reproducible archives) and puts the version in by `-X ...cli.Version`. With
`-buildvcs=false` there is no `vcs.revision`, and a module built from its own
directory has the version `(devel)`. So for every release archive, on every
platform, `buildID()` is the same string.

I built a one-line program that prints `node.buildID()` and `cli.Version`
(throwaway, removed), with the release script's flags:

```
-X cli.Version=0.1.0  ->  cli.Version="0.1.0"  buildID="poo (devel)"
-X cli.Version=0.2.0  ->  cli.Version="0.2.0"  buildID="poo (devel)"
plain `go build`      ->  buildID="poo v0.0.0-20261105054118-cf8c859ca57e+dirty cf8c859ca57e...+modified"
```

So snapshots written by v0.1.0 are used by v0.2.0, and by every later release,
whatever those releases changed. Two builds of one dirty tree at one commit
also share an id (`+modified`), so a developer's edit of `Snapshot`, `Restore`
or the selection code does not retire the snapshots written before it.

## What could happen

- A release changes how a pool is computed or what a snapshot holds (a
  tie rule, the unspent set after a new transaction kind, a new field in the
  skeleton, a fix to `Renumber`). A node upgraded in place starts from the
  v0.1.0 snapshot and runs on a pool the new code would not build from the
  log: the same entries, a different selected ledger or state. By jerry's
  definition that is nodes with the same information disagreeing, for as long
  as that node keeps its snapshot (every clean stop rewrites it from the
  already-wrong pool, so it never heals by itself).
- It is exactly the case D178 names as covered, and the tests cover it with
  `SetBuildID`, which hides that the real id never changes.
- `snapshotVersion` (1) and the magic's last byte would catch a change to the
  file format, but only if someone remembers to bump them; semantics that
  leave the format alone are what this check was for.

## Suggested fix (Blue's call)

- Make the id carry the release: pass `cli.Version` and `cli.Commit` (they are
  set by `-X`) into the node (`node.Options.Build`, or a package variable the
  `cli` package sets in `init`; `node` cannot import `cli`), and use them
  before the build info: `poo 0.2.0 <commit>`. For a dev build keep the module
  version and revision, and for `+modified` add a hash of the executable
  (`os.Executable`, hashed once) so two dirty builds differ.
- Add a constant `snapshotSemantics` that is bumped by hand in the same
  commit as any change to what a pool is, as a second guard, and list it in
  the id.
- Test: build two binaries with different `-X` versions (the script's flags)
  and check that a snapshot from one is reported unused by the other.

## Not tested

A real upgrade between two builds; `go install ...@version` (it records a
module version, so it would differ); builds by other packagers who keep VCS
info.
