# GUI White review, Round 35

## Summary

Base reviewed: `main` at `58273f7` (GUI R35A #285, the cold-key forms on
Source D193). Pending: #294 (Blue's GUI-032 fix, carrying Green #290's
steps panel and Pay warning), checked against Source #291, which is not
merged yet. Items: 3. Verdicts: two clear, and one clear with a
follow-up (S4, after #291). Release blockers: none.

Checked locally (Actions has no runner) with #294 and #291 both merged
onto `58273f7`, which merges cleanly: `go -C gui vet ./...`,
`go -C gui test ./...`, the Windows amd64 and macOS arm64 cross-builds,
and `gui/e2e/qol.mjs`, `views.mjs` and `parity.mjs` (77 commands) on a
poo-gui built as `0.1.0-alpha.1`. All pass.

## Items

### #285 GUI R35A: cold-key forms (D193)

Verdict: **clear**

- Line 4 and secrets: every new form runs poo's own flags (`keygen
  -raw`, `sign -key-file - -alg -raw -hash -out -show`, `wallet create
  -watch-only`, `wallet pay -change`). A key typed for `-key-file -` is
  read through poo's masked reader (`stdio.secret`), so the GUI masks
  that line however the core words the prompt. Main says `seed: `, and #291
  says `key: `. The GUI does not match prompt text, so the rename needs
  nothing from it.
- `signNote` reads only poo's JSON reply (`address`, `missing`, `legs`,
  `added`, `wrote`, `shown`) and draws nothing when a field is missing.
- Line 5: parity passes (77 commands).

### #294 GUI Blue R35C (GUI-032) with Green #290 (GUI-GREEN-061)

Verdict: **clear with notes** (S4)

- GUI-032: a key pasted into Key file is refused before anything runs,
  and the field is emptied, so the key never goes on a command line or
  into history. This is page-only, with no API or storage change.
- The steps panel and Pay's warning match #291: watch-only pay without
  `-out` is refused before anything is recorded.
- Line 4, text coupling: `unsignedNote` reads poo's human text (a line
  starting `signatures missing`, exit 0) rather than JSON. With #291
  merged, the core no longer does this. A payment that still needs
  signatures and has no `-out` is refused with an error (non-zero),
  and nothing is recorded or held. So once #291 lands, the note can
  never show, and its wording ("The wallet has recorded this payment and
  holds its units: Payments sent lists it as unsigned") describes the
  old behaviour.

### Source #291 as it reaches the GUI (pending)

Verdict: **clear**. The `key:` prompt is masked by the same reader as
before, key values are never echoed, and watch-only pay refuses without
`-out`. The Pay warning already says this. Source White's R36 notes
(PR #281) are a `kind`/`format` for the payment file and a wallet file
that keeps unknown fields. The GUI never parses either file: it passes
paths, and it reads only the JSON replies. Neither change needs a GUI
change.

## Suggestions to GUI Grey (R36A)

- **S4 (after #291 merges):** remove `unsignedNote` and its call.
  poo-gui always runs its own built-in poo, so no older core can reach
  the "recorded with signatures missing" state through the GUI. If
  you'd rather keep a note, key it on the error the core now returns (exit
  code non-zero), not on the text line, and say "nothing was recorded".
  Either way, take any e2e check that still expects the old line out of
  the test files.

## Open release items (GUI)

Unchanged from ROUND-34: R7 run on the tagged commit and the R10
CHANGELOG group at tag time. R8 (signed `SHA256SUMS`) is Source White's,
and the poo-gui archives are in the same file.
