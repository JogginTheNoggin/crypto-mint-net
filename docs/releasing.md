# Releasing

## Summary

How a release is made and how anyone can check one. A release is a tag
`vX.Y.Z` on the main branch. Pushing the tag runs the tests, builds `poo`
for seven platforms with `scripts/release.sh`, and opens a draft GitHub
release with the archives and `SHA256SUMS`. The owner reads the draft and
publishes it. Builds are reproducible (decision D78): rebuilding a tag with
the same toolchain gives the same bytes, so a published archive can be
checked without trusting whoever built it.

## Before tagging

1. CI is green on the main branch.
2. Run the security checks of [security.md](security.md):
   `go vet ./...`, `staticcheck ./...`, `govulncheck ./...`, and
   `FUZZTIME=1m ./scripts/fuzz.sh`. Fix or record what they find.
3. If a newer Go patch release exists, update the `toolchain` line in
   `go.mod` (D77) and let CI run.
4. Move the "Unreleased" section of [../CHANGELOG.md](../CHANGELOG.md)
   under the new version and date.
5. `TestByteFormV1Frozen` must pass: a release never changes byte form v1
   (D76).

## Tagging

```sh
git tag -a v0.1.0 -m "poo 0.1.0"
git push origin v0.1.0
```

The Release workflow (`.github/workflows/release.yml`) does the rest and
leaves a draft. Publishing it is a separate, manual step.

## The archives

| Platform | Archive |
| --- | --- |
| Linux amd64, arm64 | `poo-VERSION-linux-ARCH.tar.gz` |
| macOS amd64, arm64 | `poo-VERSION-darwin-ARCH.tar.gz` |
| Windows amd64, arm64 | `poo-VERSION-windows-ARCH.zip` |
| FreeBSD amd64 | `poo-VERSION-freebsd-amd64.tar.gz` |

Each holds the `poo` binary, the README and, once there is one, the
license. Binaries are static (`CGO_ENABLED=0`) and print their version with
`poo version`.

## Checking a release

```sh
git checkout v0.1.0
./scripts/release.sh 0.1.0
diff dist/SHA256SUMS SHA256SUMS   # the file from the release page
```

The script needs a Linux machine with GNU tar and `zip`, since it fixes
archive times and owners with GNU options. The `go` command fetches the
toolchain `go.mod` names, so the compiler is the same one CI used.
