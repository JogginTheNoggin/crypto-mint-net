# WHITE-001: Plan for the first public release

| Field | Value |
| --- | --- |
| Status | adopted in part by the owner (2026-11-05: name, ports rule, registry place, series) |
| Based on | origin/main @ 4c0f708 |
| Area | release process, versioning, `scripts/release.sh`, docs |
| Effort | medium (mostly docs and a few small code items) |
| Decided by | the owner (stage, version, tag, going public, license); Grey (code items) |

## Summary

White suggests the first public release be an **alpha**, tagged
**`v0.1.0-alpha.1`**, published as a GitHub pre-release with archives for
seven platforms, checksums and a signature. Alpha means: the consensus
byte form and wire version 1 are frozen, but node files, commands and
defaults may still change between alphas (with an upgrade path or a
release note), there is no outside security audit, and nothing of value
should be used with it. Before the tag, a short list of items must be
done: a license, the new default ports, a way for a new node to find
peers, the two R32A gaps (W-1, W-10), the version name in the hello
(WHITE-002: the release and features in the hello, and room for new fields in hello and ping), and a release checklist run locally because GitHub
Actions has no runner. Beta follows once the formats have upgrade tests
and outside testers have run nodes for a while; 1.0 follows an outside
audit. Tagging, publishing and making the repository public stay the
owner's decisions.

## Problem

The repository says "Pre-release" and `poo version` prints `0.1.0-dev`.
There is a release workflow and reproducible builds (D78), but no
decision on what the first public build is called, what it promises,
what must be done first, or how later versions relate to it. Without
that, users cannot tell what will keep working, and the teams cannot
tell what counts as a breaking change.

## Idea

### 1. Stages

| Stage | Tag form | Promise to users | Enter when |
| --- | --- | --- | --- |
| alpha | `v0.1.0-alpha.N` | Byte form v1 and wire version 1 frozen (D76). Wallet files always readable by later releases. Node files (`node.json`, exports, data folder) get an upgrade path or a release note saying what to do; a data folder may at worst need a resync from peers, never a lost wallet. Commands and defaults may change, with notes. No outside audit: test use only, nothing of value | The pre-alpha list below is done |
| beta | `v0.N.0-beta.N` | As alpha, plus: every node file format has an upgrade test from the previous release (W-5); a release can be rolled back one step without loss (W-1, W-3); flags and settings are deprecated a release before removal (W-8) | Two or more alphas shipped; outside people ran nodes; no open critical or high Red finding; upgrade tests in place |
| release candidate | `vX.Y.Z-rc.N` | Nothing changes before the final tag but fixes | Owner picks a version to stabilise |
| stable | `v1.0.0` and on | Full SemVer for commands, files and RPC; long-term support for the last stable | An outside security review, license settled, beta run long enough for the owner |

Why alpha and not beta: the project has had only internal review, the
default ports are about to change, and node files have no upgrade tests
yet. Calling it beta would promise more than White can check today.

Why `0.1.0-alpha.1` and not plain `0.1.0`: the pre-release suffix makes
GitHub, Go tooling and package managers treat it as unstable and sort it
before `0.1.0`, so a later plain `0.1.0` (or a beta) reads as a step up.
`0.1.0` itself stays free for the first beta or for when the owner wants
it.

### 2. Version numbers

- **Release version**: SemVer, `MAJOR.MINOR.PATCH[-stage.N]`, one for
  `poo` and `poo-gui` from one tag (agreed with GUI/White). Printed by
  `poo version`, set by `-ldflags` in `scripts/release.sh`.
- **Separate numbers that do not follow the release**: byte form (v1,
  frozen), wire version byte (1, frozen), hello `Protocol` (rises when
  peers gain a new optional ability, see WHITE-002), and each file's
  `format`. A release note lists any that moved.
- While 0.x: MINOR rises for new features or any file change with an
  upgrade path; PATCH for fixes only. A change that needs a manual step
  is called out at the top of the notes.
- `pkg/cli.Version` on main becomes the next planned version with `-dev`
  (`0.1.0-alpha.1-dev` until the tag, then `0.1.0-alpha.2-dev`), so a
  build from main never claims to be a release.

### 3. Before alpha (release blockers)

| # | Item | Owner | Why |
| --- | --- | --- | --- |
| R1 | Choose a license and add `LICENSE` | the owner | Without one the code is not open source: "all rights reserved" (README) |
| R2 | New default ports (P2P 8646, RPC 8645 today) and W-7: `poo init` writes the port into `node.json` | the owner picks; Grey builds | Changing them after public nodes exist moves every node that left `listen` empty |
| R3 | A way for a first node to find peers: at least one registry file the project publishes (for example in the repo or on the release page) and `poo init` naming it, or docs that say how to get one | the owner (where it lives); Grey | Today a new node has no peers unless given `-registry` or `-peer` |
| R4 | W-1: `node.json` keeps fields an older release does not know | Grey (R32A suggested) | Rolling back an alpha must not drop settings |
| R5 | W-10: node release version and commit in `GET /v1/status` and `poo status` | Grey (R32A suggested) | GUI and support need to know what a node runs |
| R6 | WHITE-002: hello carries the release and features; hello, ping and pong end with an extensions list (owner's choice, 2026-11-05) | Grey | The hello and ping layouts freeze at the tag, so this is the last chance to add them without breaking released nodes |
| R7 | Release checklist run locally (no Actions runner): all checks of `docs/releasing.md` and two `scripts/release.sh` builds with equal `SHA256SUMS`. GUI set: `go -C gui vet ./...`, `go -C gui test -race ./...`, windows/amd64 and darwin/arm64 builds, and every gui-e2e script listed in `.github/workflows/ci.yml`, run against the `poo-gui` that `release.sh` built so the `-ldflags` version is covered | White runs the Source part; GUI/White runs the GUI set and records it in its review | `release.yml` cannot run today |
| R8 | Sign `SHA256SUMS` (for example `ssh-keygen -Y sign` with a key the owner holds; its public key in the README) | the owner holds the key; White writes the steps | Reproducible builds prove what the code builds to; a signature proves the owner published it |
| R9 | README "Status" says alpha, what that means, and how to report a bug; `SECURITY.md` has a private way to report | Grey | First public readers land there |
| R10 | `CHANGELOG.md`: "Unreleased (0.1.0)" becomes `0.1.0-alpha.1` with the date, a GUI group of bullets, a "Known issues" list (naming GUI/White G1, settings reset on restart, if not fixed by then) | Grey and GUI/Grey at Part A; White checks | Users need to know what is missing |
| R11 | `poo-gui` install path: documented as from the archives or `go -C gui build`, not `go install` (the `gui/go.mod` replace) | GUI/Grey docs | `go install .../poo-gui@tag` fails today (GUI/White G3) |
| R12 | Release workflow marks a pre-release tag as a GitHub pre-release (`--prerelease` when the tag has a `-`) | Grey | So the page does not show an alpha as "Latest" |

Nice before alpha, not blocking: the GUI keeps its settings across
restarts (GUI/White G1); `poo doctor` prints each data file's format
(W-2); a release note that the first start after an upgrade replays every
pool (W-6).

### 4. Release steps (alpha)

1. Grey freezes main at the end of a Part A that White marked ready
   (no open *release blocker* in White's and Black's reviews).
2. White runs R7 on that commit, writes the release notes draft, and
   hands the checklist to the owner; GUI/White confirms the GUI part.
3. The owner makes the repository public (if not yet), tags
   `v0.1.0-alpha.1`, and either lets the workflow build the draft or,
   without a runner, uploads White's archives, `SHA256SUMS` and the
   signature by hand.
4. The owner publishes the pre-release. White checks the published
   checksums against a fresh local build.

### 5. After alpha

- Each round, White's review says whether a change needs a note in the
  next alpha. An alpha ships when the owner asks, usually every few
  rounds.
- From alpha.1 on, White keeps a copy of a data folder and every file
  alpha.1 writes as test fixtures (W-5), and every later release must open
  them.
- Older alphas are not supported once a newer one is out; the notes say
  how to upgrade.

## Cost and risk

Mostly docs and process. Code items are small (R2, R4, R5, R6, R12).
The main risk is going public before R1 and R2: an unlicensed public
repository, or ports that move after people run nodes. Calling it alpha
lowers the risk of users trusting it with value.

## Open questions for the owner

1. Is `v0.1.0-alpha.1` the name you want, or plain `v0.1.0` labelled alpha?
2. Which new default ports (R2)? (The license, R1, is the owner's call
   whenever the owner is ready; it is listed only because a public
   release needs one.)
3. Where should the first public registry live (R3)?
4. Should the alpha network use the reserve default series, or a separate
   test series so nothing "real" starts during alpha? Choosing a series is
   not a consensus change, but it decides what early users hold.

## The owner's answers (2026-11-05)

| Question | Answer | What follows |
| --- | --- | --- |
| 1. Name | `v0.1.0-alpha.1`, as recommended | `pkg/cli.Version` on main becomes `0.1.0-alpha.1-dev`; R12 marks the tag a GitHub pre-release |
| 2. Ports | Anything under 10000 that does not clash with existing applications (the owner thinks mobile software uses 8645 and 8646); the owner then picked **7336 and 7337** | **7336 for the local RPC and 7337 for peers** (W-7), keeping today's pattern (RPC one below peers). Both are unassigned in the IANA registry (checked 2026-11-05 against its CSV); no registry can rule out every unofficial use. With W-7, `poo init` also writes the ports into `node.json` from now on |
| 3. First registry | On a website, for now | R3: the registry file lives on the owner's website. `poo init` names its URL by default once the owner gives it, and the docs say how to change it. White checks that the file is served over `https` and that a node still starts when it cannot be reached |
| 4. Alpha series | The default series (reserve) | No test series. The alpha notes say plainly that the network is live on the reserve default series and that the software is alpha (see Stages) |
| License | Not yet | R1 stays open. A public alpha without a license can still be read and run, but it is not open source until a license is added. The owner decides when |

## Prototype

None. This branch holds only the proposal.
