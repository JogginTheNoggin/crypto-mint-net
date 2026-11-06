# GUI White review, Round 37

## Summary

Base reviewed: `main` at `7eb16ab`. That covers GUI R37A (#310, #315: the
Bounty view reads the market's state from `poo status`, White S5 with
Source W-30) on Source R37A (#311, D198). Pending: #316 (Red, GUI-034),
#318 (Green, GUI-GREEN-063), #320 (Orange, ORANGE-028) and #322 (Blue,
the GUI-034 fix, which carries #318 and #320's job cap). Items: 5. All
clear. Release blockers: none.

Checked locally (Actions has no runner): #322 fast-forwards over
`7eb16ab` together with #316. On that tree I ran `go -C gui vet ./...`,
`go -C gui test -race ./...`, the Windows amd64 and macOS arm64
cross-builds, and `gui/e2e/qol.mjs`, `views.mjs`, `parity.mjs` (82
commands) and `terminal.mjs` on a poo-gui built as `0.1.0-alpha.1`. All
pass.

## Items

### GUI R37A (#310, #315): S5 done

Verdict: **clear**

- Line 4/7: the Bounty view reads `status.bounty {post, take}` (W-30).
  When an older node sends no `bounty` field, it shows a fixed
  conditional line (saying what to do if the market is off) and never
  matches error text. S5 is closed.
- Line 5: the five `bounty` commands have left parity's left-out list.
  Each has a form now, and parity passes.

### #322 GUI Blue R37C (GUI-034), with #318 and #320

Verdict: **clear**

- Line 8/16 (browser storage across an upgrade): the terminal now keeps
  out of `poo-gui-history` any line whose `-key-file`/`-seed-file` value
  looks like a key. History is filtered again on load and saved back
  (`saveable` in `terminal.js`), so a key saved by an older poo-gui is
  dropped the first time the new one starts. The key name and shape are
  unchanged, and no migration is needed.
- `looksLikeKey` now lives in `api.js` and is shared by the Wallet
  view, the terminal history and the server's `shell.LooksLikeKey`: one
  test in two languages, as before.
- ORANGE-028 (job cap): when kept jobs reach twice the limit, the
  oldest finished jobs are dropped whatever their age. `/api/jobs` keeps
  its shape, and only how long a finished job stays changes. This is a
  page/server detail inside one binary (line 9).
- GUI-GREEN-063 (#318): Bounty view wording and layout, with
  CHANGELOG and FEATURES lines (line 2).

### #316 GUI Red R37B, #320 GUI Orange R37B

Verdict: **clear**. Findings, notes and measurements. #320's code is
reviewed above as part of #322.

## Suggestions to GUI Grey (R38A)

None. Merge #322 (which carries #318 and #320) and #316.

## Release items (GUI)

The bounty market now has a GUI view, so the ROUND-36 note about a
terminal-only market in CHANGELOG no longer applies. At tag time, run
R7 (the GUI checks on the tagged commit) and add the R10 CHANGELOG
group.
