# GREEN-059: A dev build names its commit, and the dry run says whether the pool is followed

| Field | Value |
| --- | --- |
| Status | proposed (prototyped) |
| Based on | `main` @ `f1ac406` |
| Area | `pkg/cli` (`releaseCommit`, `vcsCommit`, `LedgerDryRun`, the `poo help` line), `pkg/nodepolicy` (`Policy.FollowsPool`), docs (rpc, poo) |
| Effort | small |
| Consensus | none: what status, doctor and a dry run report. The hello still carries the version only (D186), and the node checks every imported entry in full (D188) |
| Decided by | Grey (Black may skip it; White may note the dev-build snapshot replay below) |

## Summary

Green ran Round 34 as a node runner:

- **D190 ports.** `poo init` wrote 7732 and 7733.
- **`poo-ledger`.** Export writes the new kind, and the import streams.
- **The dry run.** `poo ledger import -dry-run` read files for a
  followed pool, for a pool not followed, and on a node without the
  series.
- **The release line.** GUI Green noted that a dev build's release has
  no commit, and Green looked into it.

Three loose ends:

1. **A dev build's `release.commit` was empty.** `poo status`, the GUI and
   doctor's `release` check (GREEN-057) showed only
   `0.1.0-alpha.1-dev`. That version is the same for every build until
   the tag, so two different dev builds looked identical, and doctor
   could not tell a stale node from a fresh one. The commit came only
   from `-X Commit`, which only `scripts/release.sh` sets. `poo version
   -v` already read the VCS revision from the build info.
2. **The dry run said the series was known, but not whether the pool was
   followed.** A file for pool 7 gave `series_known: true` and exit 0,
   and the real import then failed with "pool not followed". The dry
   run already reads node.json, so it can tell.
3. **`poo help` left out `-dry-run`.** The usage line said `poo ledger
   import FILE|-`, though poo.md and the command's own usage error
   name it.

## Idea

- **`releaseCommit()`.** The release commit is `Commit` when `-X` sets
  it, so release builds are unchanged. Otherwise it is the build's
  `vcs.revision`, with `+modified` added when `vcs.modified` is true.
  `node.SetRelease` gets that value, so `poo status`, `GET /v1/status`,
  the GUI and doctor all show it. The hello still sends `Version` only,
  so peers learn nothing new.
- **`Policy.FollowsPool(accepted, id, k)`** (nodepolicy, beside the
  follow code) applies the follow rules the way the node does at start.
  A pool counts as followed when any of these holds:
  - a rule names the series' issuer;
  - a whole-series rule covers it, unless that rule excepts the pool;
  - a rule lists the pool, or a mint in it.

  It is modular, so other offline commands can use it.
- **The dry run** adds `pool_followed`. When a real import would be
  refused, it adds a `note`: either "add the series to the node policy
  and follow pool K first", or "poo follow -series S -pool K first". The
  exit status still reflects only problems in the file.
- **`poo help`** shows `poo ledger import [-dry-run] FILE|-`.
- **Docs.** rpc.md (`release.commit`) and the dry-run row in poo.md.

## For White

A dev build's snapshot `buildID` now includes its commit, through
`release.commit`. The first start of a dev build after this change
replays its pools once, as any new build already does. Release builds
set `Commit` by `-X` and are unchanged.

## How it was tested

- **`TestVCSCommit`** (pkg/cli) covers a clean revision, a modified one
  and a missing one, and checks that `-X Commit` wins.
- **`TestDryRunSaysPoolFollowed`** (pkg/cli):
  - Setup: an example node, then `poo unfollow -series S -pool 0`.
  - Expected: pool 0 shows as followed with no note before the unfollow,
    and as not followed with a `poo follow` note after it. Pool 1 stays
    followed.
  - It fails on `main`, where `pool_followed` does not exist.
- **By hand:**
  - `poo status` showed `release.commit f1ac4065…+modified`.
  - Doctor printed "the node runs 0.1.0-alpha.1-dev (f1ac4065…+modified),
    as this poo does".
  - The dry run of a pool 7 file printed the note.
- **Checks run locally:**
  - gofmt, go vet and staticcheck;
  - `go test -race ./...`;
  - the nine GUI e2e scripts.

## Ideas for later

- `poo build` takes no `-o` flag, unlike most commands. It always prints
  JSON, so `-o table` fails with a full flag listing.
- `poo version` could print the commit next to a dev version, since
  `-v` already gives the full details.
