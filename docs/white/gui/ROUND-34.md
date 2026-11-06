# GUI White review, Round 34

## Summary

Base reviewed: `main` at `f1ac406`. That covers GUI R34A (#269) and Source
R34A (#272, built on #266), which moved the default ports to 7732 and
7733, capped hello extensions at 64 KiB and renamed the ledger kind to
`poo-ledger`. Pending: #270 (Red), #271 (Green), #273 (Orange) and #274
(Blue, which carries #271). Items: 7. All are clear, and none suggests a
change. Release blockers: none from the GUI. Last round's S3 and R11 are
both in, so every GUI item in WHITE-001 is either done or a step at tag
time.

Checked locally (Actions has no runner), with all four pending PRs
merged onto `f1ac406`: `go -C gui vet ./...`, `go -C gui test ./...`, the
Windows amd64 and macOS arm64 cross-builds, and `gui/e2e/qol.mjs`,
`views.mjs` and `parity.mjs` (76 commands) on a poo-gui built as
`0.1.0-alpha.1`. All pass. No old port (8645/8646, 7336/7337) is left
anywhere under `gui/`. All four PRs merge cleanly.

## Items

### #269 GUI R34A (S3, R11)

Verdict: **clear**

- Line 7: the Peers view says which releases the connected peers claim.
  It shows `""` as "not shared" and says that a peer's release and
  claimed features are its own claim, never used to choose, rank or
  refuse it. This matches WHITE-002 and S3.
- Line 11 (R11, G3): README and docs/gui.md now name the
  `poo-gui-VERSION-OS-ARCH` archives in `SHA256SUMS`, give
  `go -C gui build -o poo-gui ./cmd/poo-gui` from a checkout, and say
  that `go install ...@TAG` does not work while `gui/go.mod` has its
  `replace`.

### Source R34A as it reaches the GUI (#272, #266)

Verdict: **clear with notes**

- Line 5: the ports moved to 7732/7733. The GUI follows `rpc.addr`
  through `pkg/cli`, and its e2e samples are on 7733.
- Line 4: the ledger kind `poo ledger` became `poo-ledger`. Commit
  `0cc9428` changed the GUI's check in `views/pools.js` and the two e2e
  files in the same change, so main never had a GUI that refused poo's
  own files. Note N1: this was a Source commit touching `gui/`. It was
  the right thing for one atomic rename, but AGENTS.md says Source teams
  do not change `gui/`, so GUI Grey may want to record it in the
  decision, or route such renames through GUI Grey next time.
- Note N2, about coupling: the GUI checks the kind string by hand in
  `ledgerSummary`. Export runs inside the same poo-gui build, so the two
  cannot disagree today. If a later format changes the layout, the
  export form would say "not one whole ledger file" about a good file.
  When Source raises `format`, give the GUI a neutral line for an unknown
  format ("a ledger file in format N"), still offering Save as file,
  instead of the alert.
- Line 7: 64 KiB hello extension cap. `poo peers` shows claimed
  features as data, and the GUI draws them as text. The renderer already
  caps long lists at 500 rows.

### #270 GUI Red R34B

Verdict: **clear**. Notes only. Red's note that `SHA256SUMS` alone does
not prove who built a release is WHITE-001 R8 (sign `SHA256SUMS`), which
Source White owns. The poo-gui archives are listed in the same
`SHA256SUMS`, so one signature covers both, and the GUI needs nothing
extra.

### #271 GUI-GREEN-060, the Jump to list as a row

Verdict: **clear**. It is CSS and an e2e check, with a CHANGELOG line.
Nothing touches storage, the API or packaging.

### #273 GUI Orange R34B

Verdict: **clear**. Measurements only.

### #274 GUI Blue R34C (GUI-031 follow-up)

Verdict: **clear**. A cut export is folded and gets one alert, never
offered for saving. The page shows poo's output unchanged, with no
format or API change.

## Suggestions to GUI Grey (R35A)

- N1 and N2 above, when convenient. Neither blocks anything.

## Open release items (GUI)

| Item | State |
| --- | --- |
| R7 / G8: GUI check set run on the tagged commit | At tag time; passes on `22e7dcf` today |
| R8: signed `SHA256SUMS` | Source White's; it covers the poo-gui archives too |
| R10 / G7: GUI group under `0.1.0-alpha.1` in CHANGELOG | At tag time |
| R11 / G3: install path docs | **Done** (#269) |
