# GUI White review, Round 36

## Summary

Base reviewed: `main` at `d25f777`, which holds #296 (White S4, the dead
Pay note dropped) and #298 (watch-only claim to a wallet policy,
`keygen -out`), on top of Source R36A (bounty market phase 1, D196).
Pending: #302 (Red), #303 (Orange), #305 (Green, GUI-GREEN-062 Bounty
view) and #307 (Blue, GUI-033 plus GUI-032 key redaction). Items: 6. All
are clear. #305 gets one suggestion (S5), and none blocks a release.

Checked locally (Actions has no runner) with all four pending PRs
merged onto `d25f777`, which merges cleanly: `go -C gui vet ./...`,
`go -C gui test ./...`, the Windows amd64 and macOS arm64 cross-builds,
and `gui/e2e/qol.mjs`, `views.mjs` and `parity.mjs` (82 commands) on a
poon-gui built as `0.1.0-alpha.1`. All pass.

## Items

### #296 GUI R36A: White S4

Verdict: **clear**. `unsignedNote` is gone, so nothing reads the old
"signatures missing" text line any more.

### #298 GUI R36A: claim to a wallet policy, `keygen -out` (GREEN-060)

Verdict: **clear**

- Line 4/5: the forms send poon's own flags (`claim -to-policy -out`,
  `keygen -out`), and parity lists the five `bounty` commands as
  terminal-only in phase 1, each with its reason.
- Secrets: `keygen -out` writes the key to a file through poon, so the
  page never shows it, and `secretOutput` still covers the printing
  form.

### #307 GUI Blue R36C (GUI-033, GUI-032 residual)

Verdict: **clear**

- `shell.LooksLikeKey` hides a key pasted as the value of `-key-file` or
  `-seed-file` in the Jobs list. The page uses the same test. This is a
  redaction in `gui/shell`: it changes what the Jobs list shows, not
  what the core is given, so no command or file format changes.
- Line 9: `/api/jobs` keeps its shape; only the redacted text differs.

### #305 GUI-GREEN-062, a Bounty view (proposal with prototype)

Verdict: **clear with notes** (S5), if Grey adopts it

- Line 5: it takes the five bounty commands out of parity's left-out
  list and gives each a form, sending poon's own flags. Parity passes.
- Line 4: the view reads `bounty list` JSON. Source White's R36 review
  (#306) marks feature `bounty` version 1 and leaves room for a version
  2 (W-29). Fields a later phase adds at the end show up through the
  generic renderer, so the GUI needs nothing for them.
- Line 4, text coupling: `offNote` decides that the market is off by
  matching `the bounty market is off` in poon's error text. This is the
  same pattern S4 removed from Pay. See S5.

### #302 GUI Red R36B, #303 GUI Orange R36B

Verdict: **clear**. Findings, notes and measurements only.

## Suggestions to GUI Grey (R37A)

- **S5 (if #305 is adopted):** do not key the "market is off" note on
  error text. Either show poon's error as printed, under a fixed line
  ("This node does not run the bounty market; see node policy
  `bounty`"), or, better, read a field. Source White has been asked
  whether `poon status` (or `bounty list`) can report the market's state
  in JSON. When that lands, use the field and keep the text match only
  as a fallback, with a test.

## Release items (GUI)

The bounty market is new in this release. If the Bounty view is not
adopted before the tag, CHANGELOG should say the market is
terminal-only in the GUI for 0.1.0-alpha.1. Otherwise there are no
changes from ROUND-35.
