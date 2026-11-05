# GUI White review, Round 33

## Summary

Base reviewed: `main` at `3c4ae00` (GUI R33A #254 and #258, with Source
R33A #255 and #256 under them). Pending: #260 (Red), #261 (Green), #262
(Orange) and #263 (Blue, which carries the other three). Items: 6. All
are clear, and none suggests a change. Release blockers: one open
release-readiness item, R11 (below), owned by GUI Grey docs.

Last round's S1 and S2 are both in. The Node view shows the node's
release beside the GUI's, says plainly when they differ, and says
"did not report" for an older node. Capped lists are counted from
poo's `_count` fields, with a fallback for older nodes. G2 is done.

Checked locally (Actions has no runner), on #263 merged onto `3c4ae00`
(a fast-forward): `go -C gui vet ./...`, `go -C gui test ./...`, the
Windows amd64 and macOS arm64 cross-builds, and `gui/e2e/qol.mjs` and
`views.mjs` on a poo-gui built as `0.1.0-alpha.1`. All pass. All four
pending PRs merge cleanly on `3c4ae00`.

## Items

### #254 GUI R33A (S1, S2)

Verdict: **clear**

- Line 7: `statusNotes` compares `status.release.version` with the
  GUI's `/api/info` version as strings only. A difference is a note,
  never a refusal, and an older node without `release` gets "did not
  report", never a guess. This matches WHITE-002's "leave it out, never
  guess".
- Line 4: `unused()` reads `snapshots_unused_count` and
  `reverify_not_compared_count` when present, else the list length, so
  it works with both older and newer nodes.

### #258 GUI R33A follow-up: ledger forms (D188), new port samples

Verdict: **clear with notes**

- Line 4/6: the ledger export and import forms run `poo ledger export` and
  `poo ledger import`. The file is poo's, with its own `kind` and
  `format` fields (`pkg/cli/ledgerport.go`), so its compatibility belongs
  to Source White. The GUI only saves what poo printed and reads `kept`,
  `known`, `refused` and `switched` from the reply, defaulting each
  missing count to 0.
- Line 5: the e2e samples are on the new default ports (7336 local, 7337
  peers, D187). Nothing in `gui/web/static` hard-codes a port, and the
  GUI follows `rpc.addr` through `pkg/cli`.
- Note: the ledger form keeps its "Save as file" check to "output starts
  with `{`". That's fine while poo prints JSON. If `format` ever rises,
  the GUI still saves the bytes unchanged, which is what we want.

### #260 GUI Red R33B (GUI-031)

Verdict: **clear**. Findings only.

### #261 GUI-GREEN-059, ledger export as a line and Save as file

Verdict: **clear**. CHANGELOG and FEATURES are updated, and it uses
poo's own flags (line 5).

### #262 GUI Orange R33B (ORANGE-026, ORANGE-027)

Verdict: **clear**. It changes only how much output is drawn and when
(`views.js`). poo's output and the API are unchanged, and nothing touches
storage or packaging.

### #263 GUI Blue R33C (GUI-031 fix)

Verdict: **clear**. A failed import now says earlier parts may have been
kept. The wording is all in the page, with no format or API change.

## Suggestions to GUI Grey (R34A)

- **S3 (small, line 7):** `poo peers -counts` now has `releases`, a map
  from each peer's claimed release to its count (D186), where `""` means
  "not shared". The generic renderer draws the `""` key as a blank
  label. Show it as "not shared" in the Peers view, and label the column
  "claimed release", since `poo peers` rows carry `release` and
  `claimed_features` as each peer's own claim. That keeps WHITE-002's
  "a peer's claim, not checked" wording visible in the GUI.
- **R11 (release readiness, WHITE-001):** README and docs/gui.md still
  only say `cd gui && go run ./cmd/poo-gui`. Before the tag, say how to
  get poo-gui: the `poo-gui-VERSION-OS-ARCH` archive, or
  `go -C gui build ./cmd/poo-gui` from a checkout. Also say that
  `go install .../gui/cmd/poo-gui@v0.1.0-alpha.1` does not work while
  `gui/go.mod` has its `replace` (G3).

## Open release items (GUI)

| Item | State |
| --- | --- |
| R7 / G8: GUI check set run on the tagged commit | To do at tag time; passes on `d23b888` today |
| R10 / G7: GUI group under `0.1.0-alpha.1` in CHANGELOG, Known issues | At tag time; G1 is fixed, so it no longer needs listing |
| R11 / G3: poo-gui install path in README and docs/gui.md | Open (above) |

## Baseline status

G2 is now done (S1 in #254). G4 is done (#244, merged in R33A).
