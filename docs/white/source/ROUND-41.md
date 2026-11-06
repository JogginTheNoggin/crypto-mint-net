# Source Round 41: compatibility and release review (White, Part D)

## Summary

White's review of Grey's base `main` @ `6cf8002` (#369, D208: the GPU
grinder through OpenCL and `purego`), Green's GREEN-066 (#372), Orange's
ORANGE-051 (#373, docs only) and Blue's BLUE-042 (#376, D209, which
carries #372), at `pr376` head `f971408`. **Nothing is breaking for
compatibility.** No byte form, wire layout, file kind or setting shape
changes.

**One release finding, before the tag (W-37):** the Linux release
binaries are no longer static. To call `dlopen` without cgo, `purego`
links its own loader shim into every Linux build. The `poon` and
`poon-gui` Linux archives now need the glibc dynamic loader
(`/lib64/ld-linux-x86-64.so.2`, `/lib/ld-linux-aarch64.so.1`) and
`libc.so.6`, `libdl.so.2` and `libpthread.so.0`. On a system without
glibc (Alpine and other musl distributions, `FROM scratch` or
distroless-static containers, BusyBox systems), the binary does not
start at all, and the shell says "not found". Before #369 the same
binary ran everywhere. `docs/releasing.md` still says "Binaries are
static (`CGO_ENABLED=0`)".

The rest is good news for the release:

- **Reproducible builds still hold (W-9 check).** White ran
  `scripts/release.sh 0.1.0-alpha.1` twice at `f971408`. Both runs
  produced 14 archives with identical `SHA256SUMS`, so `purego` does not
  break reproducibility.
- **Every platform builds:** Linux, macOS and Windows on amd64 and arm64
  (with the grinder), and FreeBSD amd64 (without it).
- **The released Linux binary runs on a glibc machine with no OpenCL
  library.** It prints `0.1.0-alpha.1`, and `poon grind devices` lists
  the CPU and why no GPU grinds.
- The kernel is embedded (`go:embed`), so archives need no extra file.
  On Windows, `OpenCL.dll` is loaded only from System32 (RED-127).

Tests: the release build above, plus the package tests named in D208 and
D209. Nothing needs the owner unless Grey picks option C below.

## Verdicts

| Item | Verdict | Notes |
| --- | --- | --- |
| D208: GPU grinder via OpenCL, `purego` v0.11.2, layers for SHA-256 and SHA-512/256 | compatible with conditions | Node code only. Results are the same as on the CPU. **Condition:** W-37 (Linux binaries now dynamic) |
| D208: new dependency `github.com/ebitengine/purego` | compatible | Pinned in go.sum. Apache-2.0. See W-38 for licence notices in archives |
| D208: per-OS loading: Linux and macOS through `dlopen`, Windows through `LoadLibraryEx` from System32, FreeBSD none | compatible | FreeBSD gets a working CPU-only build and says why there is no GPU. macOS builds are always dynamic, so nothing changes there |
| D208: `POON_OPENCL`, `POON_OPENCL_ALL` | compatible | New variables under the `POON_` prefix (`brand.EnvPrefix`). Off and on values read case-free (GREEN-066, RED-125) |
| D208: `grind.layer_floor` now sees the strongest layer of each ~50 ms chunk | compatible | Same setting and shape, finer timing. Before the tag |
| D208: `poon doctor`, `grind devices` (`from_node`, `opencl`), `grind bench -device`, `ErrNoGrinder` | compatible | Added fields and clearer errors |
| GREEN-066 (#372) | compatible | Added `opencl` field and note text |
| ORANGE-051 (#373) | n/a (measurements) | P2 is Grey's |
| BLUE-042 RED-124: driver failure moves to every CPU core, GPU retried after a minute | compatible | Same result |
| BLUE-042 RED-126: a reload keeps the node id | compatible | In memory only. A restart still draws a new id |
| BLUE-042 RED-127: `OpenCL.dll` from System32 only | compatible | Right for a downloaded binary |

## W-37 (before the tag): Linux archives need glibc now

What White saw on the `linux-amd64` archive built at `f971408`:

```
ELF 64-bit LSB executable, x86-64, dynamically linked,
interpreter /lib64/ld-linux-x86-64.so.2
NEEDED libdl.so.2, libpthread.so.0, libc.so.6
```

The `linux-arm64` archive is the same with `/lib/ld-linux-aarch64.so.1`.
Most desktop and server distributions have glibc, so most GPU users are
fine. But a node is exactly the program people run in a small container
or on Alpine, and for them the download stops working with no useful
message.

**Pick one before the tag (White recommends A):**

- **A. Two Linux archives per architecture.** Keep
  `poon-VERSION-linux-ARCH` with the GPU grinder (glibc). Add
  `poon-VERSION-linux-ARCH-static`, built with a `nogpu` build tag that
  leaves out `opencl*.go` and `purego`, so it stays fully static and runs
  anywhere. Do the same for `poon-gui`. Changes:
  - `scripts/release.sh` loops once more for the two Linux platforms;
  - the `opencl` files' build lines gain `&& !nogpu`;
  - a `nogpu` stub reports "built without GPU support";
  - releasing.md and the website's download list name both archives.
  Both stay reproducible. White can check that at tag time.
- **B. One dynamic Linux archive, documented.** Change releasing.md and
  the install docs to "Linux binaries need glibc; on Alpine or a minimal
  container, build from source with `-tags nogpu`". It still needs the
  `nogpu` tag, but no extra archive.
- **C. Static Linux without the GPU.** Build the Linux archives with
  `nogpu` and have GPU users build from source. That undoes jerry's
  "in every build" for Linux, so it would be his call.

Whichever is chosen, fix the sentence "Binaries are static
(`CGO_ENABLED=0`)" in `docs/releasing.md`. Add a release check too: run
`file` on each Linux binary, so the archive's kind (static or dynamic) is
a known fact and never a surprise.

## W-38 (at tag time): third-party licence notices in the archives

The archives carry `README.md` and, once there is one, the project's
`LICENSE`. They do not carry the licences of the modules compiled in.
Those are BSD and MIT (Go's `x/` packages, btcec, decred, circl,
blake3), and now Apache-2.0 (`purego`), and most of those licences ask
for their notice to travel with the binary. **Ask (with the release
steps, once the owner picks the licence):** generate a
`THIRD_PARTY_LICENSES` file from `go.mod` (for example with `go-licenses
report`, or by hand from each module's LICENSE) and copy it into every
archive in `scripts/release.sh`. White adds this to the release
checklist. It doesn't block the round.

## What Grey should take into Round 42 Part A

1. Merge #376 (with #372), #373 (docs), and Black's Part D PR.
2. **W-37:** pick A (recommended), B or C. Fix the "static" sentence in
   releasing.md and add a `file` check to the release steps.
3. **W-38:** a third-party licence file in the archives, at tag time.

## Release readiness after this round

W-37 is the one item left before the freeze from White's side. W-38
joins the tag steps. Still with the owner: license, at least one registry
node and its URL, optional maintainer keys in `docs/allowed_signers`,
and making the repo public.
