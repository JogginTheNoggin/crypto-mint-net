# GUI White review, Round 38

## Summary

Base reviewed: `main` at `1e71cf0`. That covers GUI R38A: #324 (Bounty
tables drawn 500 rows at a time) and #327 (switch limits as on/off, the
limits notes, and the bounty state read from `status -counts`), on top of
Source R38A (#325, D200). Pending: #336 (Blue, `secretLine` in linear
time), which carries #330 (Red), #331 (Green, GUI-GREEN-064) and #333
(Orange). Items: 3. All clear. Release blockers: none.

Checked locally (Actions has no runner): #336 is a fast-forward over
`1e71cf0`. On it I ran `go -C gui vet ./...`, `go -C gui test -race
./...`, the Windows amd64 and macOS arm64 cross-builds, and
`gui/e2e/qol.mjs`, `views.mjs`, `parity.mjs` (82 commands) and
`terminal.mjs` on a poo-gui built as `0.1.0-alpha.1`. All pass.

## Items

### #324 GUI R38A: Bounty tables in pages of 500

Verdict: **clear**. This changes only how much is drawn at once. poo's
JSON is unchanged, and the page says how many rows remain.

### #327 GUI R38A: switch limits, limits notes, bounty state

Verdict: **clear**

- Line 4/7, older nodes: a limit shows as on/off only when `policy
  limits` marks it `switch: true` (D198). A node without the field gets
  the usual number box. The select sends `NAME=on` or `NAME=off`, which
  poo parses (`on/true/yes`, `off/false/no`), and "unchanged" sends
  nothing.
- `notes` from `policy limits -set` is drawn only when it is an array of
  strings, each shown as printed it. Without it nothing is drawn.
- The Bounty view now reads `bounty` from `status -counts`, keeping the
  fixed line for a status without the field. S5 stays closed.
- Line 2: CHANGELOG carries the R38A lines.

### #336 GUI Blue R38C, with #330, #331 and #333

Verdict: **clear**

- `secretLine` now checks a key-file value in linear time. What it
  hides is unchanged, and so are the `poo-gui-history` key and shape,
  so saved history needs nothing on upgrade.
- GUI-GREEN-064 (#331): wording and layout for switches and notes, with
  CHANGELOG and FEATURES lines.
- #330 (Red, no new findings) and #333 (Orange, measurements, ORANGE-028
  marked adopted) are docs only.

## Suggestions to GUI Grey (R39A)

None. Merge #336, which carries #330, #331 and #333.

## Release items (GUI)

Unchanged. At tag time, run R7 (the GUI checks on the tagged commit) and
add the R10 CHANGELOG group.
