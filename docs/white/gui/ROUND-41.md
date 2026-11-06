# GUI White review, Round 41

## Summary

Base reviewed: `main` at `56c5e0a` (GUI R41A #368 Ledger view and #371
Devices as a table of the CPU and any GPU, with Source R41A's GPU grinder,
D208, under them). Pending: #374 (Red), #375 (Green) and #377 (Orange).
Items: 6. Suggest change: 0. Release items: one new, S6, which is the GUI
half of Source White's W-37 (#378).

The GPU grinder reaches poon-gui too. `pkg/grind` now loads OpenCL with
`purego`, and poon-gui compiles `pkg/grind` in, so the Linux poon-gui is
no longer a static binary either. A `CGO_ENABLED=0` linux/amd64 build of
poon-gui at `56c5e0a` gives "dynamically linked, interpreter
/lib64/ld-linux-x86-64.so.2". It needs glibc, the same as `poon`. Most
desktops have glibc, so this matters less for the GUI than for the node,
but whatever Grey picks for W-37 should cover both programs.

Checked locally (Actions has no runner), on #375, #374 and #377 merged
onto `56c5e0a` (all merge cleanly): `go -C gui vet ./...`,
`go -C gui test -race ./...`, the Windows amd64 and macOS arm64
cross-builds, and all eleven gui-e2e scripts in `ci.yml` (terminal,
views, qol, watch, viewonly, bounds, work, streams, parity with 82
commands, variable, ledger) on a poon-gui built as `0.1.0-alpha.1`. All
pass.

## Items

### #368 GUI R41A, Ledger view

Verdict: **clear**

- Line 4: every screen runs `poon pools`, `ledger`, `pending`, `tx` or
  `address` once and reads the JSON (`parseJSON`). A missing field shows
  empty (`str()`), never an error. Nothing reads the human text.
- Line 5: the flags it sends (`-pool`, `-below`, `-limit`, `-series`)
  are all in the parity check, which passes.
- Line 9: places live in the address bar (`#/ledger?pool=…`), so a link
  is a hash route, not a new `/api/` path. No server change.
- Line 2: CHANGELOG and FEATURES are updated, and `ledger.mjs` is in
  `ci.yml`.

### #371 GUI R41A, Devices table (D208)

Verdict: **clear with notes**

- Line 4: `devicesTable` draws only fields from `poon grind devices`
  (`id`, `name`, `hashes`, `auto_for`, `note`, `from_node`, `local`). A
  node that sends none of the new fields gets empty cells. The "No GPU"
  line reads `kind`, a field, not the text.
- Line 13: `github.com/ebitengine/purego v0.11.2` is new in `gui/go.mod`
  (indirect, through `pkg/grind`), pinned in `go.sum`. It needs no cgo,
  and the Windows and macOS cross-builds still work. Its licence is
  Apache-2.0, so it joins W-38's notice file.
- Line 11: see S6. The Linux poon-gui is now dynamic.
- Line 8: the new `show-commands` setting is named in `store.js`
  (`poon-gui-show-commands`), and anything other than `'1'` reads as off,
  so an odd saved value is ignored.

### #374 GUI Red R41B

Verdict: **clear**. Findings only: GUI-036 verified, GUI-037 closed.

### #375 GUI-GREEN-067, ledger navigation and where commands are still needed

Verdict: **clear**

- Line 4: the mint screen reads `poon unspent -serial` and `poon pools`
  as JSON. It finds the pool from `first_serial` and `last_serial` as
  printed, so the GUI computes no ledger rule. "Add a layer to it" reads
  `hash` and `next` from `poon build`'s JSON, not its words, which keeps
  the White rule from R36.
- Line 5: `-serial` on `unspent`, `work` and `build` is in the parity
  check.
- Line 2: CHANGELOG, FEATURES and docs/gui.md are updated.

### #377 GUI Orange R41B (ORANGE-029)

Verdict: **clear**. Measurements and a proposal only, with no code.

### Source R41A and #378 (Source White, W-37 and W-38), as they reach the GUI

Verdict: **release item** (S6). It follows Source White, which leads on
release form.

## Suggestions to GUI Grey (R42A)

- **S6 (GUI half of W-37, before the tag, line 11):** whichever option
  Source Grey picks for `poon`, do the same for `poon-gui`:
  - **A** (Source White's pick): also build
    `poon-gui-VERSION-linux-ARCH-static` with `-tags nogpu` in
    `scripts/release.sh`. The tag reaches `pkg/grind` through the GUI
    build, so nothing in `gui/` changes.
  - **B**: say in the install text that the Linux poon-gui needs glibc,
    and give `go -C gui build -tags nogpu ./cmd/poon-gui` for Alpine.
  - **C**: no GUI change beyond the docs.

  In every case, fix docs/gui.md line 24, "It needs no CGO … so it builds
  for every platform the headless tool builds for". That is still true
  for building, but add that the released Linux binary needs glibc
  (unless it is the static one). Also make sure the README's
  `poon-gui-VERSION-OS-ARCH` line names the static archive if A is
  chosen. Test it with `file` on each Linux `poon-gui` in the release
  check, as Source White asks for `poon`.
- **W-38 (at tag time):** the third-party licence file goes into the
  `poon-gui` archives too. The GUI module pulls the same modules, so one
  file from the root `go.mod` plus `gui/go.mod` covers both.

## Open release items (GUI)

| Item | State |
| --- | --- |
| S6 / W-37: Linux poon-gui is dynamic (glibc) | Before the tag; follows Source's pick |
| W-38: third-party licences in poon-gui archives | At tag time, with Source |
| R7 / G8: GUI check set run on the tagged commit | At tag time; all eleven scripts pass on `56c5e0a` with the B items today |
| R10 / G7: GUI group under `0.1.0-alpha.1` in CHANGELOG | At tag time |

## Baseline status

No change to G1 to G8. S6 is new.
