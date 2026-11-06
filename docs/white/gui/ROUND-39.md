# GUI White review, Round 39

## Summary

Base reviewed: `main` at `21c34c8`. That covers GUI R39A: #338 (one date
formatter for the Bounty view) and #341 (`-max-strength` on Ask and Work,
and the market's notes), on top of Source R39A (#339, D202). Pending:
#349 (Blue, GUI-035), which carries #344 (Red), #345 (Green,
GUI-GREEN-065) and #348 (Orange). Items: 3. All clear. There are no
suggestions and no release blockers.

Checked locally (Actions has no runner): #349 fast-forwards over
`21c34c8`. On it I ran `go -C gui vet ./...`, `go -C gui test -race
./...`, the Windows amd64 and macOS arm64 cross-builds, and
`gui/e2e/qol.mjs`, `views.mjs`, `parity.mjs` (82 commands) and
`terminal.mjs` on a poo-gui built as `0.1.0-alpha.1`. All pass.

## Items

### #338, #341 GUI R39A

Verdict: **clear**

- Line 5: `-max-strength` is the core's own flag (D202), and parity
  passes. An empty field sends nothing, so the node's
  `bounty.max_strength` applies.
- Line 4: the market's `notes` are drawn only when `bounty list` gives
  an array, each item as printed. Without the field nothing is drawn.
- The date formatter only changes how the time cells are built. The
  `datetime` attribute still holds the value the core printed.
- The off-market hint now says `on` rather than `1`, which matches the
  switch limits from R38.

### #349 GUI Blue R39C (GUI-035), with #344, #345 and #348

Verdict: **clear**

- `shell.Redact` now hides a key typed as several words after
  `-key-file`/`-seed-file` in the Jobs list, and the page's
  `secretLine` reads the value the same way, capped at 512 characters.
  This is redaction only: the core gets the same arguments,
  `/api/jobs` keeps its shape, and the `poo-gui-history` key and shape
  are unchanged.
- GUI-GREEN-065 (#345): wording for max strength, with CHANGELOG and
  FEATURES lines.
- #344 (Red) and #348 (Orange) are docs and measurements.

## Suggestions to GUI Grey (R40A)

None. Merge #349, which carries #344, #345 and #348.

## Release items (GUI)

Unchanged. At tag time, run R7 (the GUI checks on the tagged commit) and
add the R10 CHANGELOG group.
