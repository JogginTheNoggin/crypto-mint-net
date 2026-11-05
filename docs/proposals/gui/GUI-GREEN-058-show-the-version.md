# GUI-GREEN-058: Show the version (White G4)

| Field | Value |
| --- | --- |
| Status | adopted (Round 33 Part A, merged as is) |
| Based on | main @ 5f80582 |
| Area | gui/server/server.go (Server.Version, /api/info), gui/cmd/poo-gui/main.go, gui/web/static/api.js, app.js, index.html, style.css, gui/server/server_test.go, gui/e2e/qol.mjs, CHANGELOG.md, docs/FEATURES.md |
| Effort | small |
| Decided by | (Grey) |

## Summary

GUI White's baseline (docs/white/gui/BASELINE.md) lists **G4: the page
does not show its version.** `/api/info` has no version, and Getting
started tells people to run `version` in the terminal. A person filing a
bug, or comparing their GUI with a node (G2), has no way to see it in
the page.

This proposal adds the version to `/api/info`, from the one place it
lives (`cli.Version`, which poo and poo-gui share, D88), and shows it at
the right of the header: `v0.1.0-dev` today, `v0.1.0` in a release
build.

## White's checklist, for this change

| Line | Answer |
| --- | --- |
| 2. CHANGELOG and FEATURES | A sentence on the GUI bullet under "Unreleased (0.1.0)", and the Local web app row in FEATURES.md |
| 3. No version of the GUI's own | The server takes `Version` from poo-gui's `main`, which sets it to `cli.Version`; nothing in `gui/` holds a version string |
| 6. Only through pkg/cli | `gui/server` imports nothing new; `gui/imports_test.go` passes |
| 8. Browser storage | Unchanged; the version is read on each load, not stored |
| 9. `/api/` shapes | `/api/info` gains one field, and the page and server change in the same commit. An older page would ignore the field, and the new page shows nothing if the field is missing |
| 12. Embedded, nothing fetched | Yes |
| 14. e2e | All 10 scripts pass locally on this commit |

## Idea

- **`Server.Version`**, an exported field like `Heartbeat`. poo-gui sets
  it to `cli.Version`, and `/api/info` returns it as `version`.
- **The page** reads it in `api.loadInfo` and shows `v<version>` at the
  right of the header, muted, with a title saying that poo and poo-gui
  share one version and that `poo version` prints the same. If the
  version is missing or empty, the header shows nothing.

## Next, once Source reports the node's version (G2, not built here)

White's G2 asks Source for item W-10: the release and commit in
`GET /v1/status` and `poo status`. When that lands, the same header slot
can say `v0.2.0 · node v0.1.0` and show a plain note when the two
differ. The note would read the node's version from `poo status
-counts`, which Followed pools already runs every 5 s, so it needs no new
call. It must compare only the two strings poo prints: no version rules
in the GUI (D84).

## For G5 (not built here)

G5 asks that views handle the previous release's JSON. A cheap first
step Green could prototype after v0.1.0: record `-o json` samples of the
commands the panels draw (`status -counts`, `series followed`, `policy
limits`, `follow`) under `gui/web/testdata/<release>/`. Then a qol.mjs
check imports each view's draw function (`totalsTable`, `seriesTable`,
`statusNotes`, `followView`) and feeds it each sample with every field
removed in turn. It must draw without throwing. GUI/Red's R29B "?" rule
in `statusNotes` is exactly this case.

## Cost and risk

Five lines in Go and eight in the page. No rule, consensus, policy or
storage change.

## Prototype

This PR.
- `TestInfoVersion` (gui/server) checks that `/api/info` returns
  `Server.Version`.
- qol.mjs checks that the header reads `v` followed by the output of
  `poo version` from the same binary, so the two can never drift.
- At 380 px wide the header still fits: brand, node policy, version.
