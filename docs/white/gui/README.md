# GUI compatibility and release reviews (White)

## Summary

This folder holds the GUI/White team's reviews, one per round, and the
checklist every review follows. White was added by the owner on
2026-11-05 and runs in Part D, side by side with Black (the GUI had no
Part D before; it now has one for White). White reads what the round
produced, Grey's new base and any Blue, Green and Orange work waiting to
merge, and asks one question of each item: **can a person get it, run it,
and move from the last version to this one without losing anything or
being surprised?** That covers version control, backwards compatibility,
downloadability and packaging, releases and release approval, and smooth
upgrades.

White only suggests. It never edits code or another team's files, and
GUI Grey decides what to adopt in the next Part A. It never publishes a
release, tags a version or changes the repository's visibility: those are
the owner's calls. Source White covers the headless node and is the
primary White team (the owner, 2026-11-05): on anything that crosses both
layers, such as the release version, tagging, and how `poon-gui` and
`poon node` of different versions work together, GUI White consults Source
White and follows its lead. Source White's checklist is
[../source/README.md](../source/README.md).

The baseline (what the GUI versions today and the gaps found on
2026-11-05) is [BASELINE.md](BASELINE.md).

## Rules White applies

| Rule | Source |
| --- | --- |
| The GUI is only a visual layer over the headless code and never diverges from it | Owner, 2026-10-27; AGENTS.md |
| `poon` and `poon-gui` ship from one tag, at one version (`cli.Version`); the `poon` archive never contains GUI code | docs/gui.md (D88), docs/releasing.md |
| A release never changes byte form v1 | D76 |
| Builds are reproducible | D78 |
| The repository stays private; tagging v0.1.0 and going public are the owner's calls | Owner, 2026-10-26 and 2026-11-02 |
| White suggestions never add or change a consensus rule; anything that might goes to Black first | AGENTS.md, Black reviews |

## Checklist (each round)

For every item the round produced, answer each line, or write "n/a".

**Version control**

1. The change is on the integration line or a named branch, and
   `security/gui/BASE.md` names the commit reviewed.
2. A user-visible change has a line under "Unreleased" in
   `CHANGELOG.md` (GUI group) and, for a feature, in `docs/FEATURES.md`
   (GUI section).
3. Nothing in `gui/` sets its own version: the GUI prints and reports
   `cli.Version`, so poon-gui and `poon` can never drift apart.

**Compatibility with the headless node**

4. The GUI reads poon's JSON output (the default). Any field a view reads
   that was renamed, removed or changed type in this round is listed, and
   the view still handles output from the previous release (a missing
   field shows as empty, not as an error).
5. A flag the GUI sends still exists in `poon CMD -h` (the parity check
   `gui/e2e/parity.mjs` passes), and a flag removed from a form is still
   reachable from the terminal.
6. RPC calls go through `pkg/cli`/`pkg/rpc` only (`gui/imports_test.go`
   passes); the GUI adds no wire or RPC shape of its own.
7. If the GUI may talk to a running `poon node` of another version (a
   separate `poon` archive), the round says what happens: works, a clear
   message, or a known gap.

**Backwards compatibility in the browser**

8. Browser storage keys (`poon-gui-*`) are unchanged, or the change reads
   the old key and moves it, as `terminal.js` does for history. A saved
   value the new code does not understand is ignored, never a crash.
9. Server API paths under `/api/` keep their request and reply shapes, or
   the page and server change together in the same commit (they are
   embedded in one binary, so this is the rule, not a version header).
10. Cached pages cannot run old scripts against a new server
    (`Cache-Control: no-store` stays on).

**Downloadability and packaging**

11. `scripts/release.sh` still builds `poon-gui` for every platform in
    docs/releasing.md, and `go -C gui build` cross-builds for Windows and
    macOS (CI's gui job).
12. New files the GUI needs at run time are embedded (`gui/web/static`
    via `embed`); nothing is fetched from the network, no npm, no CDN.
13. A new dependency in `gui/go.mod` is named, with why, and keeps the
    build reproducible.

**Releases and approval**

14. The GUI e2e checks listed in `.github/workflows/ci.yml` (gui-e2e)
    pass locally on the reviewed commit (GitHub Actions has no runner yet,
    so local runs are the record).
15. Anything that should block the next release, or needs the owner's
    approval before one, is listed as such.

**Smooth upgrades**

16. A person who upgrades poon-gui keeps their settings (`-root`,
    `-policy`, wallet file, theme, terminal history) or is told plainly
    what they must set again.
17. Files the GUI writes through poon (node.json, policies, exports) are
    written by poon, so their compatibility is Source White's; White notes
    any GUI form that writes a field an older `poon` would reject.

## Verdicts

| Verdict | Meaning | What Grey does in Part A |
| --- | --- | --- |
| clear | No compatibility, packaging or upgrade concern | Adopt or merge as usual |
| clear with notes | Fine to merge; the notes are follow-ups (a CHANGELOG line, a doc) | Merge, and pick up the notes when convenient |
| suggest change | Merging as is would break an upgrade, a download or compatibility; White names the case and a fix | Fix before merging, or record in the decision why not |
| release blocker | Fine on main, but must be fixed before the next tagged release | Keep it on the list in `BASELINE.md` until done |

White's verdicts never override Black's: an item Black blocks stays
blocked whatever White says.

## Review format

One file per round, `ROUND-<N>.md` in this folder, written on
`claude/gui-white`:

```markdown
# GUI White review, Round N

## Summary
Base reviewed: COMMIT. Items: N. Suggest change: N. Release blockers: N.

## Items
### ITEM (PR, branch or proposal)
Verdict: clear | clear with notes | suggest change | release blocker
Checklist lines that apply, each with what was checked.
Suggestion (if any): the change, and how to test it.

## Open release blockers
The running list, carried from BASELINE.md.
```

When the review is done, White tells the coordinator that GUI Round N
Part D (White) is finished, and sends its suggestions to GUI Grey.
