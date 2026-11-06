# GREEN-065: The renamed tool from a fresh install

| Field | Value |
| --- | --- |
| Status | proposed (prototyped) |
| Based on | `main` @ `46b638d` |
| Area | `pkg/cli` (`poon help` columns, completion function names), docs (running-a-node, README) |
| Effort | small |
| Consensus | none: help text, shell scripts and doc spacing |
| Decided by | Grey; Black has nothing to check (no code path a node runs changes) |

## Summary

BASE.md asked Green to walk the renamed tool from a fresh install as a
new user: help, docs and errors. Green did this with an empty `HOME`,
no `POON_*` variables and no data folder. The steps were:

- build `./cmd/poon`
- run `poon` with no arguments, `poon status` with no node, `version`,
  `init -example`, `node`, `status`, `doctor`, `demo`, `keygen`,
  `wallet balance` with no wallet, `claim` with no seed, the
  market-off error, and a bad `-set`
- read the README and running-a-node.md

The rename reads well. Every error starts `poon:`, names `poon` commands
and `POON_SEED`, and the data folder is `~/.poon`. Source, GUI, scripts
and CI no longer say "poo" outside the round logs and decisions, which
keep the old name on purpose (D204). The labels `pkg/brand` keeps
(`PoO-wallet-`, `poo-rpc-hello`) are also unchanged on purpose. "PoO" in
comments and docs is the consensus mechanism, not the tool.

Three things were left over:

1. **`poon help` had 18 descriptions one column right.** Those are the
   lines where the command and its description share a line, such as
   `poon status`, `poon doctor [-json]`, `poon pools [FILTER]` and
   `poon help [COMMAND]`. Their spacing was set for the three-letter
   name, so they started at column 31 while every other description
   starts at 30.
2. **The shell completion still said `_poo`.** The zsh script asks to be
   saved "as _poo in a directory on $fpath", and running-a-node.md
   writes it to `_poo`. Its function, and the bash one, were still
   named `_poo`. Completion works, because `#compdef poon` and `complete
   -F` name the tool, but a zsh user who lists `$fpath` finds a `_poo`
   file for a tool that no longer exists.
3. **Comments in two code blocks no longer lined up.**
   running-a-node.md's first payment block and its wallet block, and
   the README's quick start, had a trailing `#` comment one column off
   where `poo` became `poon` (or `POO_` became `POON_`).

## The change

- The 18 usage lines in `pkg/cli/main.go` lose one space each, so every
  description starts at column 30.
- The completion functions are `_poon` (bash and zsh), and the zsh
  script says to save it as `_poon`. running-a-node.md writes
  `"${fpath[1]}/_poon"`.
- running-a-node.md and the README: the comments in those code blocks
  line up again.

## How it was tested

- `TestUsageColumnsAndCompletionName` (pkg/cli/green065_test.go) checks
  that every one-line entry in the usage starts its description in the
  column the continuation lines use, and that no completion script says
  `poo`. On main it reports 18 columns and the completion names.
- The walk above, by hand, from an empty `HOME`.
- gofmt, go vet, staticcheck and `go test -race ./...` pass.

## For White

The zsh completion file name changes from `_poo` to `_poon`. Nothing is
released, so no one has the old file.

## Not done

- `poon grind help` and `poon wallet help` have a few continuation lines
  at another column (28 or 34 rather than 30 or 35). That was so before
  the rename, so it is left out here.
- Some help lines are longer than 80 columns (`poon build`, `poon bounty
  post`). They were before the rename too.
