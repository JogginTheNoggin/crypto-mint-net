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
| [GREEN-001](source/GREEN-001-cli-help-and-typos.md) | Source | Short errors, typo suggestions and `poo help COMMAND` | adopted |
| [GREEN-002](source/GREEN-002-prometheus-metrics.md) | Source | Prometheus metrics at `GET /v1/metrics` | adopted |
| [GREEN-003](source/GREEN-003-table-output.md) | Source | Readable table output for view commands | adopted |
| [GREEN-004](source/GREEN-004-poo-doctor.md) | Source | `poo doctor`, one command to say what is wrong | adopted |
| [GREEN-005](source/GREEN-005-shell-completion.md) | Source | Shell completion (`poo completion bash\|zsh\|fish`) | adopted |
| [GREEN-006](source/GREEN-006-structured-logs.md) | Source | Structured, levelled node logs with `log/slog` | adopted |
| [GREEN-007](source/GREEN-007-read-only-scope.md) | Source | A read-only control scope in the core | adopted |
| [GREEN-008](source/GREEN-008-fold-older-read-commands.md) | Source | Fold the older read commands into the views | adopted in part |
| [GREEN-009](source/GREEN-009-pool-by-serial.md) | Source | `-serial` for `build` and `work` | adopted |
| [GREEN-010](source/GREEN-010-docs-drift-test.md) | Source | A test that the docs name real commands and flags | adopted |
| [GREEN-011](source/GREEN-011-refuse-bad-reload.md) | Source | A reload onto a broken policy keeps the node running | adopted |
| [GREEN-012](source/GREEN-012-o-on-every-read.md) | Source | `-o table` on the read commands that lacked it | adopted |
| [GREEN-013](source/GREEN-013-version-build-info.md) | Source | `poo version -v` for bug reports | adopted |
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
| [GREEN-017](source/GREEN-017-grind-usability.md) | Source | Four rough edges in `poo grind` | adopted |
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
| [GREEN-039](source/GREEN-039-submit-from-the-command-line.md) | Source | Make `poo submit` usable with `poo` alone | adopted (D145) |
| [ORANGE-024](source/ORANGE-024-round-14-room-contention-measured.md) | Source | Contention at the disk reserve measured after D142; no change proposed | measured |
| [GREEN-040](source/GREEN-040-submit-takes-the-reply.md) | Source | `poo submit` takes the reply as printed; a missing wallet says what to do | adopted (D147) |
| [ORANGE-025](source/ORANGE-025-entry-body-read-once.md) | Source | Wallet lock and entry submit measured; read an entry body once | adopted, with a 64 MiB start that grows (D147) |
| [GREEN-041](source/GREEN-041-help-lists-only-flags-that-work.md) | Source | Help lists only the flags a command takes | adopted (D149) |
| [ORANGE-026](source/ORANGE-026-entry-body-ends-at-its-size.md) | Source | Entry submit re-measured after D147; let the buffer end at the stated size | adopted (D149) |
