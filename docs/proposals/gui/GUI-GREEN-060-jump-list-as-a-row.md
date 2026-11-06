# GUI-GREEN-060: The Jump to list as a row

| Field | Value |
| --- | --- |
| Status | proposed |
| Based on | main @ 7099e84 |
| Area | gui/web/static/style.css (the view list's rules), gui/e2e/qol.mjs, CHANGELOG.md |
| Effort | small |
| Decided by | (Grey) |

## Summary

GREEN-030 added a **Jump to** list at the top of every long view, styled
as a wrapping row (`nav.jump`). It has never been drawn as one. The view
list's rules in style.css name the element, `nav { width: 170px; … }`, so
they apply to every `nav`, the Jump to list included. Each long view
therefore draws a second narrow sidebar inside the page, white with a
border, with the buttons stacked in it, before the first panel.

I measured the list in Chromium at 1,280 px wide on main @ 7099e84, and
again with this change:

| View | Buttons | Jump to on main | This PR |
| --- | --- | --- | --- |
| Node | 12 | 170 × 341 px | 998 × 51 px |
| Pools and payments | 9 | 170 × 259 px | 998 × 51 px |
| Work, Peers | 9 | 170 × 221 px | 998 × 23 px |
| Node policies | 8 | 170 × 249 px | 998 × 51 px |
| Wallet | 17 | 170 × 485 px | 998 × 78 px |
| Explore | 6 | 170 × 176 px | 998 × 23 px |

On Wallet, half of an 800 px window was a strip of buttons before Balance.
At phone width, the list was already full width but still took the white
background and the bottom border of the narrow-window tab bar.

## Idea

- The view list's rules name `#nav`, the view list's own id
  (`<nav id="nav">`), instead of every `nav`. That covers its width,
  border, background, links, active and busy marks, and its narrow-window
  forms (`.layout #nav { display: none }`). `nav.jump` keeps its own rules
  and nothing else changes.
- The view list looks as it did: still 170 px wide, and qol.mjs checks
  that.

## Checked, nothing to change

- **Peers, claimed releases (White S3):** on two nodes joined over P2P,
  the timed panel says "Releases the connected peers claim:
  v0.1.0-alpha.1-dev: 1 peer". A peer's release is at most 64 bytes of
  `0-9 A-Z a-z . - +` (`wire.ValidRelease`), and the page draws it as
  text, so a peer cannot put anything else on the page.
- **Install docs (R11):** `go -C gui build -o poon-gui ./cmd/poon-gui`
  builds `gui/poon-gui`, as the README says.

## For Source (not built here)

- **A dev build's release line has no commit.** `cli.Commit` is set only
  by `scripts/release.sh` (`-X`). A node built from a checkout reports
  `"release": {"version": "0.1.0-alpha.1-dev"}` in `poon status`, with no
  commit, although `poon version -v` on the same binary prints the commit
  from the build info (`vcs.revision`). Every alpha tester who builds
  from a checkout reads as the same release, so the Node view's "different
  releases" note can never tell two of them apart. `node.SetRelease` could
  fall back to `vcs.revision` when `Commit` is empty, as
  `buildID` in pkg/node/snapshot.go already does. The GUI would then show
  it with no change, since the release line already prints the commit
  when there is one.

## White's checklist, for this change

| Line | Answer |
| --- | --- |
| 2. CHANGELOG and FEATURES | A sentence on the GUI bullet under "Unreleased (0.1.0)". FEATURES is unchanged: no feature changes |
| 6. Only through pkg/cli | CSS only |
| 9. `/api/` shapes | Unchanged |
| 14. e2e | All 10 scripts pass locally on this commit |

## Cost and risk

Nine selectors in style.css. No rule, consensus, policy or `/api/`
change.

## Prototype

This PR. qol.mjs checks, at 390 px and at 1,280 px, that the Jump to
list spans the view with no background or border, and that the view
list stays 170 px wide. On main the check fails with "the jump list when
narrow".
