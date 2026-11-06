# Releasing

## Summary

How a release is made and how anyone can check one. A release is a tag
`vX.Y.Z` on the main branch. Pushing the tag runs the tests, builds `poon`
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
git tag -a v0.1.0 -m "poon 0.1.0"
git push origin v0.1.0
```

The Release workflow (`.github/workflows/release.yml`) does the rest and
leaves a draft. Publishing it is a separate, manual step.

## The archives

| Platform | Archive |
| --- | --- |
| Linux amd64, arm64 | `poon-VERSION-linux-ARCH.tar.gz` |
| macOS amd64, arm64 | `poon-VERSION-darwin-ARCH.tar.gz` |
| Windows amd64, arm64 | `poon-VERSION-windows-ARCH.zip` |
| FreeBSD amd64 | `poon-VERSION-freebsd-amd64.tar.gz` |

Each platform also gets a `poon-gui-VERSION-OS-ARCH` archive with the GUI
([gui.md](gui.md)). `poon-gui` contains the same commands as `poon`
(`poon-gui poon ARGS` runs `poon ARGS`), so it can be installed instead of
`poon` or beside it. The `poon` archives never contain GUI code.

Each holds the `poon` (or `poon-gui`) binary, the README and, once there is one, the
license. Binaries are static (`CGO_ENABLED=0`) and print their version with
`poon version`.

## Signing

The checksums file is signed so anyone can tell the project's maintainers
published it (W-21, D192). Each maintainer has their own SSH key; no key
is shared, kept in the repository or given to CI. The public keys are in
[allowed_signers](allowed_signers), all under one name, `poon-release`, so
any one maintainer can sign a release and a check never depends on one
person.

A maintainer makes a key once and adds its public half to
`docs/allowed_signers` by a reviewed change:

```sh
ssh-keygen -t ed25519 -f ~/.ssh/poon-release -C "poon release: NAME"
# docs/allowed_signers gets a line:
# poon-release namespaces="file" ssh-ed25519 AAAA... NAME
```

At each release, after `scripts/release.sh`, one maintainer signs:

```sh
ssh-keygen -Y sign -f ~/.ssh/poon-release -n file dist/SHA256SUMS
```

and uploads `SHA256SUMS.sig` beside `SHA256SUMS`. A maintainer who steps
down is not deleted: their line gets `valid-before="YYYYMMDD"`, so
releases they signed before then still check and later ones do not.
Keyless signing (Sigstore) can come later, once CI has a runner.

## Checking a release

```sh
ssh-keygen -Y verify -f docs/allowed_signers -I poon-release -n file \
  -s SHA256SUMS.sig < SHA256SUMS
sha256sum -c SHA256SUMS
```

The first line checks a maintainer signed the checksums; the second that
the archives match them. To check that the archives are what the code
builds to:

```sh
git checkout v0.1.0
./scripts/release.sh 0.1.0
diff dist/SHA256SUMS SHA256SUMS   # the file from the release page
```

The script needs a Linux machine with GNU tar and `zip`, since it fixes
archive times and owners with GNU options. The `go` command fetches the
toolchain `go.mod` names, so the compiler is the same one CI used.
