# GUI White review, Round 40

## Summary

Base reviewed: `main` at `b94d861` (GUI R40A #352 and #354, `a79be4b`,
with Source R40A #353, #358 and the PoON label follow-up #363 under
them). Pending: #357 (Red), #359 (Green), #361 (Orange) and #362 (Blue,
which carries the other three). Items: 7. Suggest change: 0. Release
blockers: none new. The GUI has none left, only the tag-time steps R7
and R10.

This round renamed poo to `poon` everywhere (D204, D205), and the
protocol's labels from PoO to PoON (D206). jerry decided on "everything,
no migration", since nothing is released. From the GUI side the rename
is complete and consistent: the binary, archives, cookie, header,
storage keys and config folder all say poon, and nothing in `gui/`
still says poo.

Checked locally (Actions has no runner), on #362 merged onto `b94d861`
(a clean merge): `go -C gui vet ./...`, `go -C gui test -race ./...`,
the Windows amd64 and macOS arm64 cross-builds, and `gui/e2e/qol.mjs`,
`views.mjs`, `parity.mjs` (82 commands) and `terminal.mjs` on a poon-gui
built as `0.1.0-alpha.1`. All pass.

## Items

### #352 and #354 GUI R40A (poon-gui rename)

Verdict: **clear**

- Line 3: the GUI still sets no version of its own and prints
  `cli.Version`.
- Line 11: `scripts/release.sh` builds `poon` and `poon-gui` archives
  (`poon-gui-VERSION-OS-ARCH`), and README and docs/gui.md name
  `go -C gui build -o poon-gui ./cmd/poon-gui`. The R11 install text
  follows the new name.
- Line 9: the cookie (`poon_gui`) and header (`X-Poon-GUI`) changed in
  the same commit as the page, which is the rule for an embedded page.
- Line 16: the saved port is now under `poon-gui/last-port`. A dev build
  user's old `poo-gui/last-port` is not read, so poon-gui starts on a new
  port once, then keeps it. That is acceptable unreleased (no migration,
  D204).

### Source R40A and #363 (PoO to PoON labels), as they reach the GUI

Verdict: **clear**, no GUI change needed

- Line 4: no JSON field a view reads was renamed. Ids, keys and hashes
  change value (the default series id is now `45772e75…`) but not shape.
  The GUI shows them as printed and computes none itself.
- The only GUI edits were e2e samples (`qol.mjs`, `views.mjs`), updated
  in #363. Nothing in `gui/web/static`, README or docs/gui.md names an
  old id.
- Line 17: old poo wallet, ledger and settings files are refused by the
  core with its own message. The GUI only shows what was printed.
- Consulted Source White, who leads on this (D206).

### #357 GUI Red R40B (GUI-036, GUI-037)

Verdict: **clear**. Findings only.

### #359 GUI-GREEN-066

Verdict: **clear**. The header fix keeps page and server in step
(line 9), and the notes match Source's choice not to read old files.

### #361 GUI Orange R40B

Verdict: **clear**. A quick check, with no code or API change. It
conflicts with main on its own but merges cleanly as part of #362.

### #362 GUI Blue R40C (GUI-036 fix, store.js)

Verdict: **clear with notes**

- Line 8: every browser key now comes from `store.js` with the
  `poon-gui-` prefix, and `gui/web/store_test.go` fails if any source
  still names `poo-gui-`. That puts G6's rule in one place, which helps
  any later key change.
- GUI-037 (old `poo-gui-*` keys left behind): no migration is the right
  call. The old keys also sit under a different origin, because the saved
  port path changed, so poon-gui could not reach them anyway. Dev build
  users clear the site data if they care.
- GUI-036: hiding a whole seed typed in words is page-side only, with
  no storage or API change. History still drops secret lines on load.
- Note: when the first release is cut, a future key change must read
  the old key and move it (checklist line 8). The `store.js` comment
  says so.

## Suggestions to GUI Grey (R41A)

None. Merge #362 as usual.

## Open release items (GUI)

| Item | State |
| --- | --- |
| R7 / G8: GUI check set run on the tagged commit | At tag time; passes on #362 over `b94d861` today |
| R10 / G7: GUI group under `0.1.0-alpha.1` in CHANGELOG | At tag time |
| R11 / G3: poon-gui install path | Done (#269), now with the poon-gui name |

## Baseline status

No change. G5 and G6 stay after the first release; `store.js` makes G6
easier.
