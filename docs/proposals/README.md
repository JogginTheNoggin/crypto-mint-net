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
| [ORANGE-010](source/ORANGE-010-keep-every-layer-above-a-floor.md) | Source | Keep every layer above a floor, not only the best per slice | open: the owner's call |
| [ORANGE-011](source/ORANGE-011-simd-lanes-blake3.md) | Source | SIMD lanes, sixteen BLAKE3 layer tries at once | kept as a prototype |
| [ORANGE-012](source/ORANGE-012-sha256-lanes-and-lane-grinders.md) | Source | SHA-256 lanes, and lane searches wired as grinders (opt-in build) | proposed |
| [ORANGE-013](source/ORANGE-013-job-stop-and-progress.md) | Source | Round 6 jobs measured; stop without polling, progress each second | proposed |
