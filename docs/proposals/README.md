# Proposals

## Summary

Ideas from the Green teams (quality-of-life improvements, nice-to-haves
and explorations) and the Orange teams (performance and optimization,
with measurements). Each is a file `<layer>/GREEN-<NNN>-<short-slug>.md`
or `<layer>/ORANGE-<NNN>-<short-slug>.md` in the format given in
[AGENTS.md](../../AGENTS.md#green-proposals)
([Orange](../../AGENTS.md#orange-proposals)). Grey
decides whether to adopt each one and records the outcome in the file.

| Id | Layer | Title | Status |
| --- | --- | --- | --- |
| [GREEN-001](source/GREEN-001-cli-help-and-typos.md) | Source | Short errors, typo suggestions and `poon help COMMAND` | adopted |
| [GREEN-002](source/GREEN-002-prometheus-metrics.md) | Source | Prometheus metrics at `GET /v1/metrics` | adopted |
| [GREEN-003](source/GREEN-003-table-output.md) | Source | Readable table output for view commands | adopted |
| [GREEN-004](source/GREEN-004-poon-doctor.md) | Source | `poon doctor`, one command to say what is wrong | adopted |
| [GREEN-005](source/GREEN-005-shell-completion.md) | Source | Shell completion (`poon completion bash\|zsh\|fish`) | adopted |
| [GREEN-006](source/GREEN-006-structured-logs.md) | Source | Structured, levelled node logs with `log/slog` | adopted |
| [GREEN-007](source/GREEN-007-read-only-scope.md) | Source | A read-only control scope in the core | adopted |
| [GREEN-008](source/GREEN-008-fold-older-read-commands.md) | Source | Fold the older read commands into the views | adopted in part |
| [GREEN-009](source/GREEN-009-pool-by-serial.md) | Source | `-serial` for `build` and `work` | adopted |
| [GREEN-010](source/GREEN-010-docs-drift-test.md) | Source | A test that the docs name real commands and flags | adopted |
| [GREEN-011](source/GREEN-011-refuse-bad-reload.md) | Source | A reload onto a broken policy keeps the node running | adopted |
| [GREEN-012](source/GREEN-012-o-on-every-read.md) | Source | `-o table` on the read commands that lacked it | adopted |
| [GREEN-013](source/GREEN-013-version-build-info.md) | Source | `poon version -v` for bug reports | adopted |
| [GREEN-014](source/GREEN-014-address-paging.md) | Source | Address history pages that skip nothing | adopted |
| [ORANGE-001](source/ORANGE-001-prune-off-chain-set.md) | Source | Rank inactive branches without scanning every entry | adopted |
| [ORANGE-002](source/ORANGE-002-remember-verified-witnesses.md) | Source | Remember witnesses already verified | adopted |
| [ORANGE-003](source/ORANGE-003-reuse-hash-digests.md) | Source | Reuse hash digests | adopted |
| [ORANGE-004](source/ORANGE-004-verify-before-the-lock.md) | Source | Verify signatures before the node's lock | adopted with a change |
| [ORANGE-005](source/ORANGE-005-store-group-commit.md) | Source | Group commit for the store | adopted |
| [GREEN-015](source/GREEN-015-refuse-unused-flags.md) | Source | Refuse flags that do nothing for the subcommand given | adopted |
| [GREEN-016](source/GREEN-016-grinder-commands.md) | Source | The grinder's commands (for M11) | adopted with changes |
| [ORANGE-006](source/ORANGE-006-grinder-per-hash.md) | Source | Grinding baseline, and an allocation-free grinder per hash | adopted |
| [ORANGE-007](source/ORANGE-007-grind-on-all-cores.md) | Source | Grind on several cores, with the same result | adopted |
| [ORANGE-008](source/ORANGE-008-fewer-weight-allocations.md) | Source | Fewer big-number allocations, and hash a layer once | adopted |
| [GREEN-017](source/GREEN-017-grind-usability.md) | Source | Four rough edges in `poon grind` | adopted |
| [GREEN-018](source/GREEN-018-readable-grind-tables.md) | Source | Readable tables for nested objects and rates | adopted |
| [GREEN-019](source/GREEN-019-grind-until-tx-safe.md) | Source | Grind until my payment is safe (`tx:TID` target) | adopted |
| [ORANGE-009](source/ORANGE-009-m11-measured-and-blake3-cv.md) | Source | M11 measured, and a BLAKE3 grinder that keeps chaining values | adopted |
| [ORANGE-010](source/ORANGE-010-keep-every-layer-above-a-floor.md) | Source | Keep every layer above a floor, not only the best per slice | adopted as node policy (D118) |
| [ORANGE-011](source/ORANGE-011-simd-lanes-blake3.md) | Source | SIMD lanes, sixteen BLAKE3 layer tries at once | kept as a prototype |
| [GREEN-020](source/GREEN-020-grind-schedule-and-leases.md) | Source | Schedules, leases and tx targets, used as a person would | items 1 and 2 adopted (D119) |
| [ORANGE-012](source/ORANGE-012-sha256-lanes-and-lane-grinders.md) | Source | SHA-256 lanes, and lane searches wired as grinders (opt-in build) | adopted (D119) |
| [ORANGE-013](source/ORANGE-013-job-stop-and-progress.md) | Source | Round 6 jobs measured; stop without polling, progress each second | adopted (D119) |
| [GREEN-032](source/GREEN-032-layer-floor-and-help.md) | Source | `layer_floor` that stays set, and `-h` that succeeds | adopted (D122) |
| [ORANGE-014](source/ORANGE-014-large-items-witnesses.md) | Source | Large entries measured; check many witnesses once, on every core | adopted (D122) |
| [ORANGE-015](source/ORANGE-015-chase-measured-no-branch-walk.md) | Source | The new chase measured; no branch walk on every event | adopted (D122) |
| [ORANGE-016](source/ORANGE-016-layer-floor-traffic.md) | Source | layer_floor traffic measured; bound it with layer_rate | adopted (D122) |
| [ORANGE-017](source/ORANGE-017-encode-large-items-once.md) | Source | Large entry memory measured; encode a large item once | adopted (D126) |
| [ORANGE-018](source/ORANGE-018-chase-request-pace.md) | Source | The B1 chase measured; pace requests under the sender's budget | adopted (D124) |
| [GREEN-033](source/GREEN-033-parts-in-data-dir.md) | Source | Put large-entry parts in the data directory, and document `layer_rate` | adopted (D126) |
| [ORANGE-019](source/ORANGE-019-ask-in-larger-hash-pages.md) | Source | Ask for hashes in pages of 4,096, as peers already serve | adopted (D130) |
| [GREEN-034](source/GREEN-034-schedule-version-in-cli.md) | Source | The schedule's version on the command line | adopted (D130) |
| [ORANGE-020](source/ORANGE-020-catch-up-signatures-on-every-core.md) | Source | Catching up measured; check each reply's signatures on every core | adopted (D135) |
| [GREEN-035](source/GREEN-035-schedule-room-and-restarts.md) | Source | Leave room for user jobs, and change only what a schedule edit changes | adopted (D135) |
| [ORANGE-021](source/ORANGE-021-restart-signatures-on-every-core.md) | Source | Restart measured; check the store's signatures on every core | adopted (D138) |
| [GREEN-036](source/GREEN-036-doctor-fix-dir-and-live-floor.md) | Source | Doctor fixes that change the checked policy, and a live layer floor | adopted (D138) |
| [ORANGE-022](source/ORANGE-022-slow-fetches-finish-first-keeps.md) | Source | Turn-taking measured; the fetch that would finish first keeps the room | superseded by D139 and D140 (finding upheld; D141) |
| [GREEN-037](source/GREEN-037-say-when-a-reload-is-due.md) | Source | Say when a node.json change waits for a reload | adopted (D141) |
| [ORANGE-023](source/ORANGE-023-probe-throws-the-whole-reply-away.md) | Source | Probing measured; a probe throws the whole reply away | adopted (D142) |
| [GREEN-038](source/GREEN-038-headless-gui-parity.md) | Source | Keep the command line level with the GUI | adopted (D143) |
| [GREEN-039](source/GREEN-039-submit-from-the-command-line.md) | Source | Make `poon submit` usable with `poon` alone | adopted (D145) |
| [ORANGE-024](source/ORANGE-024-round-14-room-contention-measured.md) | Source | Contention at the disk reserve measured after D142; no change proposed | measured |
| [GREEN-040](source/GREEN-040-submit-takes-the-reply.md) | Source | `poon submit` takes the reply as printed; a missing wallet says what to do | adopted (D147) |
| [ORANGE-025](source/ORANGE-025-entry-body-read-once.md) | Source | Wallet lock and entry submit measured; read an entry body once | adopted, with a 64 MiB start that grows (D147) |
| [GREEN-041](source/GREEN-041-help-lists-only-flags-that-work.md) | Source | Help lists only the flags a command takes | adopted (D149) |
| [GREEN-042](source/GREEN-042-grind-series-help-and-import-plan-values.md) | Source | Grind -series help, and import plans that show values | adopted (D151) |
| [GREEN-043](source/GREEN-043-import-plan-notes-read-like-its-lines.md) | Source | Import plan notes read like its lines, and help names the new flags | adopted (D153) |
| [GREEN-044](source/GREEN-044-help-names-grind-schedule-and-doctor-fix-for-check-seconds.md) | Source | Help names grind schedule, and doctor's fix for check_seconds | adopted (D155) |
| [GREEN-045](source/GREEN-045-subcommand-help-and-map-path-notes.md) | Source | "poon CMD -h" for every command with subcommands, and -map-path notes | adopted (D157) |
| [GREEN-046](source/GREEN-046-offline-peer-add-label-and-zero-times.md) | Source | Offline peer add checks its label first, and zero times read "-" | adopted (D159) |
| [GREEN-047](source/GREEN-047-policy-limits-and-other-limits.md) | Source | poon policy limits, and which other limits could be settable | adopted (D161) |
| [GREEN-048](source/GREEN-048-build-next-steps-and-limit-names.md) | Source | Build and claim next steps that work, and -set names it suggests | proposed |
| [GREEN-049](source/GREEN-049-variable-series-notes.md) | Source | Say a variable series is kept, not wrong: init and doctor notes, parts -from past the cap | proposed |
| [GREEN-050](source/GREEN-050-releases-and-the-reload-gap.md) | Source | Release tooling notes, a readable followed table, and a reload that waits for its lock | proposed |
| [GREEN-051](source/GREEN-051-seed-commands-take-alg.md) | Source | Commands that take a seed take -alg, so SLH-DSA keys can claim, pay, sign and release | proposed |
| [GREEN-052](source/GREEN-052-keygen-series-addresses.md) | Source | keygen gives the key's address in the node's own series (the reserve hashes with SHA-512/256); poon crypto -o table | proposed |
| [GREEN-053](source/GREEN-053-bodies-released-on-write.md) | Source | Drop an entry's body into the cache when it is written; doctor's columns in line | proposed |
| [GREEN-054](source/GREEN-054-flags-after-the-word.md) | Source | Flags may follow the word in poon tx, address, tracking and policy use | proposed |
| [GREEN-055](source/GREEN-055-reverify-reports-damaged-snapshots.md) | Source | -reverify reports a damaged snapshot, and doctor reports -reverify | proposed |
| [GREEN-056](source/GREEN-056-port-map-says-why-where-when.md) | Source | Port mapping status says why, where and when | proposed |
| [GREEN-057](source/GREEN-057-unknown-settings-and-release-in-doctor.md) | Source | Doctor names unknown settings (warning for those a save drops) and a release mismatch | proposed |
| [GREEN-058](source/GREEN-058-ledger-export-pool-zero.md) | Source | Ledger export refuses a start past the tip; import errors name what the node needs | proposed |
| [GREEN-059](source/GREEN-059-dev-commit-and-dry-run-follow.md) | Source | A dev build names its commit in release; the import dry run says whether the pool is followed | proposed |
| [GREEN-060](source/GREEN-060-cold-claim-and-key-file.md) | Source | The cold-key steps work with the key never on the node: keygen -out, wallet claim -to-policy, poon sign signs claims | proposed |
| [GREEN-061](source/GREEN-061-bounty-offer-finds-its-pool.md) | Source | A bounty offer goes to the pool that holds its entry; a pool that does not hold it is refused | proposed |
| [GREEN-062](source/GREEN-062-bounty-on-with-limits.md) | Source | Turning the bounty market on with poon policy limits -set: on/off values, notes when posting cannot work, errors that name the command | proposed |
| [GREEN-063](source/GREEN-063-bounty-done-and-watch-tid.md) | Source | A bounty from post to paid and through a restart: poon bounty list -done N, poon wallet watch -tid | proposed |
| [GREEN-064](source/GREEN-064-max-strength-flow.md) | Source | A fast and a slow grinder against offers of several strengths: -max-strength over the setting refused, dropped offers explained, list filtered, minute_strength in grind bench | proposed |
| [ORANGE-026](source/ORANGE-026-entry-body-ends-at-its-size.md) | Source | Entry submit re-measured after D147; let the buffer end at the stated size | adopted (D149) |
| [ORANGE-027](source/ORANGE-027-round-17-modularity-measured.md) | Source | Round 17 measured; the modular seams cost nothing worth an exception | measured; no exception needed (D151) |
| [ORANGE-028](source/ORANGE-028-round-18-import-and-unlock-measured.md) | Source | Round 18 measured; imports are linear, Unlock's check is free | measured; no change needed (D153) |
| [ORANGE-029](source/ORANGE-029-weight-checks-within-the-budget.md) | Source | Round 19 measured; ask weights within the peer's request budget | adopted (D155) |
| [ORANGE-030](source/ORANGE-030-status-reads-make-no-state.md) | Source | Round 20 measured; read an unused pool without making its state | adopted (D157) |
| [ORANGE-031](source/ORANGE-031-peer-import-at-its-limits.md) | Source | Round 21 measured; peer imports at their limits | adopted (D159) |
| [ORANGE-032](source/ORANGE-032-limits-at-their-ceilings.md) | Source | Round 22 measured; the settable limits at their ceilings | adopted (D161) |
| [ORANGE-033](source/ORANGE-033-rival-builds-and-live-limits.md) | Source | Round 23 measured; rival-claim builds and live limit changes | proposed |
| [ORANGE-034](source/ORANGE-034-parts-as-ranges-and-part-ids.md) | Source | Round 24 measured; parts as ranges, part ids, peer counts | proposed |
| [ORANGE-035](source/ORANGE-035-releases-and-in-place-limits.md) | Source | Round 25 measured; releases, the waiting path, and a design for in-place limits | proposed |
| [ORANGE-036](source/ORANGE-036-release-gossip-and-work-sizing.md) | Source | Round 26 measured; release gossip, in-place limits, SLH-DSA and work sizing | proposed |
| [ORANGE-037](source/ORANGE-037-reserve-default-and-slh-dsa-256s.md) | Source | Round 27 measured; the reserve default, SLH-DSA-256s and witnesses with no network-wide cap | proposed |
| [ORANGE-038](source/ORANGE-038-bodies-held-through-their-items.md) | Source | Round 28 measured; entry bodies were still held through their items | proposed |
| [ORANGE-039](source/ORANGE-039-cold-entries-measured.md) | Source | Round 29 measured; cold entries, the recent window and the pending pool after a deep switch | proposed |
| [ORANGE-040](source/ORANGE-040-snapshots-measured.md) | Source | Round 30 measured; pool snapshots start 10 to 30 times faster, but gathering one holds the node for seconds | proposed |
| [ORANGE-041](source/ORANGE-041-snapshot-pause-and-port-mapping.md) | Source | Round 31 measured; the snapshot pause after D182, the per-start check, and port mapping | proposed |
| [ORANGE-042](source/ORANGE-042-unknown-settings-and-status-size.md) | Source | Round 32 measured; saving unknown settings was quadratic, and the status reply after an upgrade | proposed |
| [ORANGE-043](source/ORANGE-043-ledger-export-import-and-hello.md) | Source | Round 33 measured; ledger export and import, and the hello's cost | proposed |
| [ORANGE-044](source/ORANGE-044-hello-ping-limits-and-peer-memory.md) | Source | Round 34 measured; the hello and ping at the new limits, and memory per connection | proposed |
| [ORANGE-045](source/ORANGE-045-poon-sign-and-header-keys.md) | Source | Round 35 measured; `poon sign` signed each input apart, and skipped header keys | proposed |
| [ORANGE-046](source/ORANGE-046-bounty-market-measured.md) | Source | Round 36 measured; the bounty market's messages are cheap, but its file only grows | proposed |
| [ORANGE-047](source/ORANGE-047-bounty-file-remeasured.md) | Source | Round 37 measured; the bounty book no longer grows with its history, and keepjson's remaining cost | proposed |
| [ORANGE-048](source/ORANGE-048-start-and-wallet-remeasured.md) | Source | Round 38 measured; the start and the wallet no longer grow slower with history | proposed |
| [ORANGE-049](source/ORANGE-049-list-by-strength.md) | Source | Round 39 measured; a bounty list filtered by strength costs at most 30 µs | proposed |
