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
