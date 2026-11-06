# GREEN-013: `poon version -v` for bug reports

| Field | Value |
| --- | --- |
| Status | adopted |
| Based on | `claude/m10-watch` @ `7e68fe0` |
| Area | `pkg/cli` (`version`) |
| Effort | small |
| Decided by | Grey, Source Round 4, and `scripts/release.sh` now sets `cli.Commit` with -X, since its builds have no VCS info |

## Summary

`poon version` prints only `0.1.0-dev`. With this change, `poon version -v`
also prints the Go version, the platform, the commit (with a flag if the
working tree was modified), the commit time and the version of every
dependency, all read from the binary's own build information. `-o table`
works too. Prototyped, with a test.

## Problem

Every development build says `0.1.0-dev`, so a bug report, a Red finding
or a GUI "about" screen can't say which code ran. Teams already put
"branch @ commit" in every finding and proposal by hand. The binary knows
its commit; it just doesn't say it.

## Idea

`runtime/debug.ReadBuildInfo`: `vcs.revision`, `vcs.modified` and
`vcs.time`, plus `Deps` (with replacements). Plain `poon version` is
unchanged.

**Note for Grey.** `scripts/release.sh` builds with `-buildvcs=false`
for reproducibility, so release binaries have no commit and `-v` leaves
that field out. If Grey wants the commit in releases too, the release
script can pass it with `-X`, the same way it already passes `Version`.
That keeps the build reproducible, since the commit is an input. I left
the script alone, since releases are Grey's.

## Cost and risk

`poon version` used to ignore its arguments, and now it parses flags. It
still accepts `-root`, `-policy` and `-dir`, which scripts and
`TestViewOnly` pass to every command, and ignores them. A flag it doesn't
know is now an error instead of being silently ignored.

About 70 lines, standard library only. It reveals nothing a person
holding the binary can't already get from `go version -m`.

## Prototype

On `claude/source-green`: `pkg/cli/version.go` and `version_test.go`,
`version` routed to `versionCmd` in `main.go` with its usage line, and the
`poon.md` row.
