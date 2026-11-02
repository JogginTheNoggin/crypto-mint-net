# GREEN-043: Import plan notes read like its lines, and help names the new flags

| Field | Value |
| --- | --- |
| Status | proposed |
| Based on | `main` @ `533dbcd` |
| Area | `pkg/cli` (`policy import` text plan, `poo help`), `docs/modules/poo.md` |
| Effort | small |
| Consensus | none: help text and command-line text output only |
| Decided by | Grey (Black may skip it) |

## Summary

Green tried `policy import` with D150's `-settings` and `-full` and D151's
`-if-digest` from the command line, and found four small problems:

- The "read first" and "skipped, node settings" notes still showed list items as `p2p.peers.[a.example:1]`. GREEN-042 had already changed the plan's own lines to show `p2p.peers: a.example:1`.
- The "file digest ... (-if-digest applies only this file)" line was printed on every run. That included `-apply` and imports with nothing to change, where there is nothing left to preview.
- "1 of the file's settings are already the same".
- `poo help` listed `-full` but not `-settings` or `-if-digest`. The `poo.md` row had no `-if-digest`, and the usage error had no `-if-digest`.

All four are fixed on the branch. The `-json` output, which the GUI reads, is unchanged.

## Idea

- `planFields(fields, short)` passes each name through `planLine` (GREEN-042). Both notes use it, so every part of the plan writes a list item the same way. A note that is not a list item, such as a registry file's "(a local file: only -full takes it)", is unchanged.
- The digest line is printed only on a preview with something to apply. That is the one case where `-apply -if-digest D` is the next step. `-json` still carries `digest` on every row, as GUI-023 needs.
- "is" for one setting, "are" for more.
- `poo help`: `poo policy import FILE|- [-apply] [-settings | -full] [-if-digest D]`. The other flags are in `-h`, as for `export`.
- `poo.md`: the import row gains `[-if-digest D]` and one sentence on it.

## Testing

- `TestPlanNotesReadLikeItsLines` checks four things:
  - A `-settings` preview names `p2p.peers: a.example:1` under "read first", has no `.[` anywhere, prints the digest, and says "1 ... is already the same".
  - `-apply` prints no digest line.
  - A second import with nothing to change prints no digest line.
- `TestPolicyExportImport`'s two note expectations now use the new form.
- gofmt, vet, staticcheck, and `go test -race ./...` pass.

## Risks

A script that read the digest from the text output of `-apply` loses it.
`-json` has it on every run.

## For Grey (not changed here)

- `docs/FEATURES.md` says "A node bundle applies whole or not at all". D150 (RED-045) says every plan is made first and a later failure names what was saved, so a failure while saving can leave some policies saved. The text could say "plans every policy before changing any; a later failure names what was saved".
- The GUI import form (`showPlan` in `gui/web/static/views.js`) does not yet show `skipped_settings` or `defences`. Someone importing from the GUI is not told which settings were skipped, or which lines touch a defence. `parity.mjs` already notes that `-settings` and `-if-digest` arrive in the next GUI Part A. The notes should come with them, and "read first" belongs above the table.
- Still open from GREEN-020: errors name `until_from` where the person typed `-until-from`, and no command renews a lease.
