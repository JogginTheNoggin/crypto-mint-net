# GUI White review, Round 32

## Summary

Base reviewed: `main` at `6260590` (GUI R32A `5f80582`, #242, plus Source
R32A #245 merged after it). Items: 4 (main, #243, #244, #248). Suggest
change: 1 (S2). Release blockers: none new. G1 is done, the node now
reports its release (W-10), and #244 shows the GUI's own. One
compatibility slip came from the Source side: Source R32A capped
`snapshots_unused` at 8 entries and added `snapshots_unused_count`, but
the Node view still counts the list, so after an upgrade it says "8
snapshots not used" when hundreds were (S2). Every PR merges cleanly on
`6260590`.

Checked locally (Actions has no runner): `go -C gui vet ./...` and
`go -C gui test ./...` on main with #244 and #248 merged, Windows amd64
and macOS arm64 cross-builds, and `gui/e2e/qol.mjs` and `views.mjs` on a
poo-gui built with `-X ...cli.Version=0.1.0-alpha.1`. All pass, and the
header shows `v0.1.0-alpha.1`, so a pre-release version string works end
to end.

## Items

### main @ 5f80582 (GUI R32A, #242)

Verdict: **clear with notes**

- Line 16 (settings survive an upgrade): G1 is done. poo-gui saves only
  the port number, `0600` in a `0700` folder, and rebinds on 127.0.0.1
  only. `-listen` overrides it and saves nothing. Red's #243 checked the
  file handling.
- Line 2: CHANGELOG and FEATURES are updated.
- Note N1: running two poo-gui at once. The second gets a new port and
  writes it to `last-port`, so next time poo-gui comes back on the
  second one's port and the first one's saved settings are not found.
  This is rare, and a sentence in docs/gui.md covers it ("run one
  poo-gui, or give each its own `-listen`").
- Note N2, for GUI Red to weigh, not a White item: the origin is now
  predictable. Any other local web page served later on that port (while
  poo-gui is not running) shares the origin's browser storage: the node
  policy path, wallet file path, theme and history (which already has no
  secret lines). Random ports allowed this too, only less often.

### Source R32A, as it reaches the GUI (#245)

Verdict: **suggest change** (S2), and S1 is now possible

- Line 4 (fields a view reads): `replay.snapshots_unused` now holds at
  most 8 reasons, and `replay.snapshots_unused_count` holds the full
  number (GUI ORANGE-023, D184). `views/node.js` still prints
  `snapshots_unused.length`.
- Line 7 (node of another version): `status.release` {version, commit}
  now exists.

### #243 GUI Red R32B notes

Verdict: **clear**. Findings only, no code.

### #244 GUI-GREEN-058, version in the header

Verdict: **clear with notes**

- Lines 3 and 9: the version comes from `cli.Version` through
  `/api/info`, and the GUI keeps no version of its own. The page and
  server change together, and an older server that sends no `version`
  leaves the badge hidden. The badge is set with `textContent`.
  CHANGELOG and FEATURES are updated, with a test and an e2e check.
- Note N3: G4 also asked for the commit. Adding `cli.Commit` beside the
  version in `/api/info`, shown as a tooltip, would help bug reports
  from dev builds, which all say `0.1.0-dev`. Optional.

### #248 GUI Orange R32B (ORANGE-024, ORANGE-025)

Verdict: **clear**. ORANGE-025's `shortDetail` only shortens what is
drawn: poo's JSON and the full text are unchanged (line 4), and "Show
all" sets the text with a text node. No storage, API or packaging
change.

## Suggestions to GUI Grey (R33A)

- **S1 (G2, GUI half):** the Node view's status shows the node's release
  from `status.release` ("Node runs 0.1.0-alpha.1"). When that differs
  from the GUI's version (from `/api/info`, after #244), it adds a plain
  note: "This GUI is 0.1.0-alpha.2 and the node is 0.1.0-alpha.1; some
  forms may offer what the node does not have yet." When an older node
  sends no `release`, it says "the node did not report its version" and
  never guesses. Test with a status sample that has no `release`.
- **S2 (line 4):** in `statusNotes`, count with
  `rp.snapshots_unused_count ?? rp.snapshots_unused.length`, and say
  "showing 8" when the list is shorter than the count. The fallback keeps
  older nodes right.
- N1 and N3 when convenient.

## Open release blockers

None from the GUI. The GUI set in WHITE-001 R7 (G8) must be run again on
the tagged commit.

## Release plan check (WHITE-001, WHITE-002)

The GUI side of Source White's plan matches this team's:

- one version from one tag;
- archives named `poo-gui-0.1.0-alpha.1-OS-ARCH`;
- poo-gui installed from the archives or with `go -C gui build` (R11);
- the exact GUI check set in R7;
- a GUI group in the CHANGELOG, and G1 in Known issues (R10). G1 is now
  fixed, so it can come out of Known issues;
- each peer's `version` and claimed features in `poo peers -o json`,
  left out rather than guessed.
