# Source compatibility and release reviews (White)

## Summary

This folder holds the Source/White team's reviews, one per round, and the
checklist each review follows. White was added by the owner on
2026-11-05. It runs in Part D beside Black and guards compatibility
between versions: an upgraded node must still read the data folder,
`node.json`, exports and wallet an older release left, still talk to
older peers, and be able to go back a release without losing anything it
cannot fetch again. White also owns version numbers, release readiness
and approval, downloads and packaging, upgrade and rollback steps, and
when old features are removed. Like Black, White never edits code: it
sends suggestions to Grey, who decides and merges in the next Part A.
Consensus stays with Black.

This file also records the **baseline**: what is versioned today, how
each reader treats an older or newer file, and the gaps White found
before Round 32 (W-1 to W-10). The gaps are suggestions for Grey, not
blockers, because nothing has been released yet.

## Scope

| White owns | White does not own |
| --- | --- |
| Version numbers (`poon version`, `pkg/cli.Version`), the changelog, release notes, tags proposed to the owner | Consensus rules and hard-fork risk: that is Black. A change that needs a new rule goes to Black |
| Format versions of every file and message a node keeps or exchanges (table below) | The GUI: GUI/White reviews `gui/` |
| Upgrade tests: a new build reads an old data folder, settings file, policy, peer and settings export, wallet; and talks to an older peer | Security findings: that is Red. A compatibility break that is also an exploit goes to Red as a note |
| Downgrade and rollback: going back one release loses nothing that cannot be fetched again, or the release notes say so | Speed: that is Orange |
| Release archives, `scripts/release.sh`, reproducible builds (D78), checksums | Deciding: Grey adopts, the owner tags and publishes |
| Deprecations: a feature, flag, setting or endpoint is removed only after a release that warns about it | |

## Verdicts

| Verdict | Meaning | What Grey does in Part A |
| --- | --- | --- |
| compatible | Old files, old data folders and older peers keep working, both ways across one release | Merges as usual |
| compatible with conditions | Compatible only if the stated conditions hold (a release note, a migration step, a test added) | Merges with the conditions and records them in the decision |
| breaking: needs a plan | Changes a file, message, setting, flag or endpoint so that something an older release wrote or sends is refused or misread, without a format bump, a reader for the old form, or a release note | Does not merge as is. White names what breaks and the smallest plan that keeps it working |
| release blocker | Fine to merge, but a release must not be tagged until it is resolved | Merges; White tracks it in the release checklist |

Before the first release (v0.1.0) nothing is in the field, so White only
marks items *breaking* when they would break the first release's own
promise (byte form v1, wire version 1). From v0.1.0 on, the policy below
applies in full.

## Compatibility policy (proposed for v0.1.0)

1. **Byte form v1 and wire version 1 never change** (D76, the owner's
   rules). Black guards these as consensus; White checks the release
   notes and tests say so.
2. **Every file a node writes carries a format number**, and a new
   release reads the format the previous release wrote. A layout change
   that an older reader would misread bumps the format.
3. **Upgrade is automatic; downgrade is one release.** A node upgraded
   from release N-1 starts on its data folder with no manual step. Going
   back from N to N-1 either works, or N's release notes say what to do
   first, and N never rewrites a file in a form N-1 cannot read without
   saying so.
4. **New fields are added, never repurposed.** A field keeps its meaning
   and units forever; a new meaning takes a new name.
5. **Older peers keep working.** A new message type is ignored by older
   nodes (it already is). A change older peers would treat as malformed
   (a new header flag, a new wire version byte) needs Black's and the
   owner's sign-off and a hello-negotiated switch.
6. **Deprecate, then remove.** A flag, setting, endpoint or command is
   marked deprecated in one release (it still works and warns), and
   removed no earlier than the next.
7. **Defaults that move are release items.** Changing a default port,
   limit or path is listed in the release notes with what an existing
   node does.

## Checklist for each round

For every change in the round (Grey's base, Blue's branches, Green and
Orange proposals), White answers:

- [ ] Does it change the layout of a versioned file or message (table
      below)? Is the format bumped, and is the old form still read?
- [ ] Does it add a field to a file read with `DisallowUnknownFields`? An
      older release will then refuse files the new one writes: is that
      acceptable, and is the error clear?
- [ ] Does it add a field to `node.json`? An older release keeps it on
      save only inside objects, within the keeping limits (D184, D185).
      A new field inside a list entry is lost on a rollback-save (W-11):
      put new settings in objects instead.
- [ ] Does it add a field to a shared or saved file (wallet, payment,
      exports)? If an older reader must not ignore it, the file's
      `format` goes up (W-25). A file that crosses machines needs a
      `kind` and `format`, and keeps unknown keys on write-back (W-23,
      W-24).
- [ ] Does it add a kept field that describes other data (an offset, a
      count, a marker)? An older release keeps it unchanged while it
      changes the data, so the reader checks it before trusting it
      (W-33).
- [ ] Does it add a dependency or change how a binary links? Check the
      release archives with `file` (static or dynamic, which loader) and
      run `scripts/release.sh` twice to compare `SHA256SUMS` (W-37, W-9).
- [ ] Does it add a record kind to the pool log, or change a snapshot?
      Can the previous release still start on the data folder?
- [ ] Does it add, rename or remove a command, flag, setting, metric or
      RPC endpoint? Removal needs a deprecation first.
- [ ] Does it change a default (port, limit, path, on or off)? Is it in
      the changelog's Unreleased section?
- [ ] Does it touch `scripts/release.sh`, `go.mod`'s toolchain or the
      archive layout? Do two builds still give the same bytes?
- [ ] Does the changelog's Unreleased section say what users will notice?

Reviews go in `ROUND-<N>.md` here, one per round, with each item's
verdict and suggestions for Grey.

## Release readiness (before any tag)

White signs this off and hands it to the owner, who tags and publishes
(the owner's call, 2026-11-02).

- [ ] Every check in [docs/releasing.md](../../releasing.md) passes, run
      locally while GitHub Actions has no runner.
- [ ] Two clean builds of the tag with `scripts/release.sh` give the same
      `SHA256SUMS`.
- [ ] `TestByteFormV1Frozen` and the vectors pass.
- [ ] The upgrade tests pass from the previous release's data folder,
      `node.json`, exports and wallet (once a previous release exists).
- [ ] A node of this build and a node of the previous release connect,
      exchange items and agree on the heavier ledger.
- [ ] The changelog lists every break, default change and deprecation.
- [ ] No open *release blocker*.

## Baseline: what is versioned today

Checked on `origin/main` @ 4c0f708 (2026-11-05).

| What | Where | Version mark | Older file in a newer reader | Newer file in an older reader |
| --- | --- | --- | --- | --- |
| Byte form (series, tx, entry, layer) | `pkg/series`, `pkg/vectors` | series `FormatVersion` 1, `FormatVariable`; frozen by `TestByteFormV1Frozen` (D76) | Read | A new form is refused (consensus; Black) |
| Wire messages | `pkg/wire` | header byte `Version` 1, must equal | Read | A different byte is malformed and drops the peer |
| Wire message types | `pkg/wire`, `pkg/p2p` (dispatch) | type numbers, never reused | Read | Unknown types are ignored, by design |
| Wire header flags | `pkg/wire` (Decode) | none | Read | Unknown flags are malformed and drop the peer (W-4) |
| Hello | `pkg/wire` (messages.go), `pkg/p2p` | `Protocol` 1 (highest spoken), `Agent` "poon" | Any protocol 1 or more is accepted | Same; nothing negotiates down yet (W-4) |
| Pool log | `pkg/store` | header `PoON-log\x01` | Read | An unknown record kind stops the node at start (`pkg/node` "unknown record kind") (W-3) |
| Pool snapshot | `pkg/node`, `pkg/ledger` | `PoON-snap\x01`, record version 1, and a build id | Ignored unless the same build wrote it, so every upgrade replays once (D178, RED-087) | Ignored, replayed |
| `node.json` | `pkg/nodepolicy` | `format` 1, must equal | Read | Unknown fields are silently ignored, and lost if the older node saves the file (W-1) |
| Policy export bundle | `pkg/nodepolicy` (bundle.go) | `kind` "poon-node-policies", `format` 1; a bare policy file is accepted | Read | Refused: `DisallowUnknownFields` and format must equal |
| Settings export | `pkg/nodepolicy` (settings.go) | same bundle (`kind`, `format` 1) | Read | Refused, as above |
| Peer export | `pkg/peerport` | `kind` "poon-peers", `format` 1 | Read | Refused: `DisallowUnknownFields` and format must equal |
| Peer book file | `pkg/peers` | `format` | Read | Not checked here yet (W-2) |
| Address registry | `pkg/registry` | `format` 1 | Read | Format 1 only |
| Wallet | `pkg/wallet` (`wallet.json`) | `format` 1, must equal | Read | Refused |
| Local RPC | `pkg/rpc` | path prefix `/v1/`; requests decoded with `DisallowUnknownFields` | Read | A newer client sending a new field to an older node is refused (fine while the GUI ships in the same binary) |
| Tool version | `pkg/cli.Version` "0.1.0-dev", set by `-ldflags` in release builds | `poon version` | | |
| Release archives | `scripts/release.sh`, `.github/workflows/release.yml` | tag `vX.Y.Z`, `SHA256SUMS`, reproducible (D78) | | |

## Gaps found (suggestions for Grey, R32A or later)

None of these is a consensus question. Each is node or tool behaviour.

| Id | Gap | Why it matters | Suggestion | When |
| --- | --- | --- | --- | --- |
| W-1 | `nodepolicy.Load` uses plain `json.Unmarshal`, so an older release ignores `node.json` fields it does not know, and any save it makes (`poon follow`, `poon policy limits -set`, `poon reload` paths that save) writes the file back without them. Found by reading | A rollback silently drops settings, including defences such as limits and blocks | Keep unknown top-level and nested keys on load and write them back on save (a raw-JSON side map), and warn once at start that the file has settings this release does not know. Add a test: a file with an extra field survives Load then Save | Before v0.1.0 |
| W-2 | No single place lists every file in a data folder and its format. Some files (peer book) are not checked against a format on read | An upgrade or rollback cannot be tested without knowing what to test | A small `pkg/node` (or `docs/`) table of data-folder files and formats, and `poon doctor` printing each file's format | R32 or R33 |
| W-3 | A pool log record kind the binary does not know stops the node at start | Fine as fail-closed, but it makes a future record kind a one-way upgrade | Before adding any record kind: either let an older node skip unknown kinds it can refetch, or bump the log header and say so in the release notes. Nothing to change now; White checks it when a kind is added | Rule only |
| W-4 | Wire: header version byte and header flags must match exactly, and the hello's `Protocol` is accepted but never used to choose behaviour | Any future header change disconnects every older peer | Nothing to build now (wire v2 is not wanted, per the owner). Record the rule: a header change is only made behind a hello `Protocol` check, with Black's and the owner's sign-off | Rule only |
| W-5 | No upgrade tests: nothing keeps a data folder, `node.json`, exports and wallet from an earlier build and checks a new build reads them | Compatibility is only as good as its tests | After v0.1.0 is tagged, commit a small fixture data folder and files written by v0.1.0 under `testdata/compat/v0.1.0/` and a test that opens each with the current code. Until then, a fixture from the current format 1 | Fixture now, release copy at v0.1.0 |
| W-6 | Snapshots are thrown away on every new build, so the first start after an upgrade replays every pool | Upgrades of large nodes take longer than users expect | Keep the safe behaviour (RED-087); say in the release notes and in `poon node` output that the first start after an upgrade replays. Optional later: key the build id on `snapshotSemantics` plus format, not the commit, once Black and Red agree it is safe | Release note now |
| W-7 | Default ports (P2P TCP 8646, control 8645) will change before release (owner, 2026-11-05). An empty `listen` in `node.json` means the default, so a default change moves existing nodes, and peer books keep the old port | Existing nodes would move port on upgrade; peers would dial the old one | When the owner picks new ports: have `poon init` write the port into `node.json` from then on, keep empty meaning the default, and list the change in the changelog. Before v0.1.0 nobody is affected | When the owner decides |
| W-8 | No deprecation mechanism: flags and settings are removed outright | Scripts and saved files break on upgrade | A helper that marks a flag or setting deprecated (still works, prints a warning, listed by `poon doctor`), and the rule above | After v0.1.0 |
| W-9 | GitHub Actions has no runner, so the Release workflow cannot build a tag | A tag would produce no draft release | Until fixed, White runs `scripts/release.sh` twice locally and compares `SHA256SUMS`, and the owner uploads by hand. The release checklist says so | Until the runner is fixed |
| W-10 | A running node does not report its release version over the local RPC (`GET /v1/status` has no version), so a client of another release, such as a separately installed `poon-gui`, cannot tell it is talking to a different version. Raised by GUI/White | Mixed versions on one machine fail in confusing ways instead of saying so | Add the release version and commit (the values `node.SetRelease` already records, the same ones `poon version` prints) to the status reply, and have `poon status` show them. GUI/White then decides what the GUI does on a mismatch | R32A |

## How White works with the other teams

- White reads what the round produced, like Black, and writes
  `ROUND-<N>.md` here on its own branch.
- Suggestions go to Grey for the next Part A. A suggestion that would add
  or change a consensus rule goes to Black instead, never to the owner.
- GUI/White covers `gui/`, with its files in `docs/white/gui/`.
  Source/White is the primary White (the owner, 2026-11-05): the two
  talk and cooperate, but GUI/White consults Source/White on anything
  they share (release version, archives, changelog, release approval,
  compatibility of the headless code) and follows its lead, not the other
  way around.
  Agreed so far (2026-11-05): `poon` and `poon-gui` carry one release
  version from one tag (as [docs/releasing.md](../../releasing.md)
  already says), and the changelog stays one file with a GUI group of
  bullets.

## Proposals

White's own proposals use the Green and Orange format and go to Grey (and
to the owner where marked):

| Id | Title | Status |
| --- | --- | --- |
| [WHITE-001](WHITE-001-first-public-release.md) | Plan for the first public release (alpha `v0.1.0-alpha.1`) | proposed |
| [WHITE-002](WHITE-002-version-and-features.md) | Nodes tell each other their version and what they support | proposed |
