# GUI White baseline: what is versioned today, and the gaps

## Summary

Taken on 2026-11-05 from `main` at `4c0f708` (GUI R31A), before White's
first Part D. The GUI is in good shape for a first release: it has no
version of its own, it ships from the same tag as `poo`, its page and
server live in one binary, and it fetches nothing from the network. The
real gaps are about what happens across versions and across restarts:
browser settings are lost on every restart because the port is random, a
GUI cannot tell when the running node is a different version, and
`poo-gui` cannot be installed with `go install`. None of them blocks
v0.1.0 by itself; G1 and G2 are the ones worth fixing first.

## What is versioned today

| Thing | How it is versioned | Where |
| --- | --- | --- |
| poo-gui release | Same `cli.Version` as poo, set with `-ldflags` at release; printed on start (`poo-gui 0.1.0: opened ...`) | `pkg/cli/main.go`, `gui/cmd/poo-gui/main.go` |
| Release archives | `poo-gui-VERSION-OS-ARCH`, built beside `poo` from one tag by `scripts/release.sh`, reproducible, `SHA256SUMS`, draft release published by hand | `docs/releasing.md`, `.github/workflows/release.yml` |
| GUI module | Own module `gui/`, pinned to the core with `replace ... => ../` | `gui/go.mod` |
| Page and server | Embedded together (`embed`), so they always match; `Cache-Control: no-store` | `gui/web/web.go`, `gui/server/server.go` |
| Server API `/api/*` | No version; safe because page and server ship in one binary | `gui/server/server.go` |
| Data the GUI shows | poo's JSON output (the default `-o`), read by the views | `gui/web/static/views/*.js` |
| Flags the GUI sends | Checked against `poo CMD -h` by `gui/e2e/parity.mjs` | GUI-GREEN-042 |
| Browser storage | Keys `poo-gui-where`, `poo-gui-theme`, `poo-gui-history`, `poo-gui-wallet`, `poo-gui-wallet-series`; no schema version; history drops lines an older version saved that may hold a secret | `where.js`, `terminal.js`, `views/wallet.js` |
| Grind schedule edits | Content version with `If-Match` (GUI-014, D125) | `pkg/rpc/grind.go` |
| Imports (policy, settings, peers) | Preview, then Apply with `-if-digest` | docs/gui.md |
| Changelog | One `CHANGELOG.md`, GUI bullet under "Unreleased (0.1.0)" | `CHANGELOG.md` |

## Status (updated each round)

| # | Status |
| --- | --- |
| G1 | **Done** in GUI R32A (#242): poo-gui comes back on the port it used last (`last-port` in poo-gui's config folder); `-listen` still overrides. Notes in ROUND-32.md |
| G2 | Source half done (W-10, D184: `release` in `GET /v1/status` and `poo status`). GUI half open: S1 in ROUND-32.md |
| G3 | Decided with Source White (WHITE-001 R11): document archives or `go -C gui build` for 0.1; require-at-tag later |
| G4 | In GUI-GREEN-058 (#244), for R33A |
| G5, G6 | Open, after the first release |
| G7 | In WHITE-001 R10 |
| G8 | In WHITE-001 R7; GUI White ran the GUI set locally for R32 (ROUND-32.md) |

## Gaps (as found on 2026-11-05)

| # | Gap | Why it matters | Suggestion | Owner | Release blocker? |
| --- | --- | --- | --- | --- | --- |
| G1 | Saved browser settings do not survive a restart. `-listen` defaults to `127.0.0.1:0`, a new port every launch, and browser storage is kept per origin (scheme, host **and port**). So `-root`/`-policy`, the wallet file, the theme and terminal history start empty each time, and old entries pile up under dead ports. | Every restart, and so every upgrade, loses the person's setup; a view could run against the default node policy when they meant another one. | Keep the settings server-side: poo-gui stores them in a small file in its config folder and the page reads them through `/api/info` (or a new `/api/prefs`), with browser storage kept only as a fallback. Alternatively remember the last port and try it first. Grey picks; check with Red that the file holds no secret (history already drops secret lines). | GUI Grey | No, but first in line |
| G2 | The GUI cannot tell which version the running node is. `poo node` reports no release version over RPC (`/v1/hello` and status carry none), and poo-gui ships in its own archive, so a poo-gui 0.2 can talk to a poo 0.1 node. | A form may send a flag or read a field the older node lacks, and the person gets an odd error with no hint why. | **Source item W-10 (Source White, for Source R32A):** release version and commit in `GET /v1/status` and `poo status`. Then GUI shows "GUI x, node y" in Node/Settings and a plain note when they differ. | Source Grey, then GUI Grey | No |
| G3 | `poo-gui` cannot be installed with `go install ...@version`: Go refuses a module with a `replace` directive. | Downloadability for people who install from source rather than archives. | Before (or at) the first public release: tag the core module and change the `replace` into a `require` of the same version, as docs/gui.md already plans, or document that `go install` works only for `poo` and poo-gui comes from archives or `go -C gui build`. | GUI Grey, with Source White on tagging | Decide before going public |
| G4 | The page does not show its version. `/api/info` has no version; the Start page tells people to run `version` in the terminal. | Support and bug reports need it; also needed to show G2's mismatch. | Add `version` (and commit) to `/api/info` and show it in the header or Settings. Small. | GUI Grey | No |
| G5 | No check that views cope with the previous release's JSON. e2e runs against the same tree, so it can never see a field that an older or newer node does not send. | Matters once G2 lets the versions differ, and after the first release. | After v0.1.0: keep a recorded `-o json` sample per view command from the last release under `gui/web/testdata` and check each view renders it (missing field = empty, not an error). | GUI Grey (Green could prototype) | No |
| G6 | Browser storage has no schema version. Today each value is tolerant (unknown JSON falls back to defaults), but there is no rule for changing a key's shape. | Future changes could silently drop or misread saved settings. | Adopt the rule in checklist line 8 (read the old key, move it, never crash). No code change needed now. G1 may make this moot. | GUI Grey | No |
| G7 | No GUI-side release notes beyond one CHANGELOG bullet, and FEATURES.md is not checked against CHANGELOG at release. | People upgrading want to know what moved in the GUI. | At release, Grey moves GUI items under the version in CHANGELOG (docs/releasing.md step 4 already covers the file); White checks it in the release round. | GUI Grey | Check at release |
| G8 | Release checks rely on CI, and GitHub Actions has no runner. | docs/releasing.md step 1 ("CI is green") cannot be met as written. | Record local runs of `go -C gui test -race ./...`, the cross-builds and the ten gui-e2e scripts in the release round's review until a runner exists. Shared with Source White. | Both Greys | Yes, until a runner or a recorded local run exists |

## Shared with Source White

Source White is the primary White team; GUI White follows its lead on
these.

- One version for `poo` and `poo-gui`, from one tag (already the rule).
- G2 needs a Source change first.
- G3 needs a core-module tag, which is a release decision.
- G8 is the same for both teams.
