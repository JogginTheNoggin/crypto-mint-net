# GREEN-045: "poo CMD -h" for every command with subcommands, and -map-path notes

| Field | Value |
| --- | --- |
| Status | adopted (D157) |
| Based on | `main` @ `522e891` |
| Area | `pkg/cli` (`run`, policy and settings import text output), `pkg/nodepolicy` (`PathMap.Maps`), `docs/modules/poo.md` |
| Effort | small |
| Consensus | none: help, and notes in the import's text output |
| Decided by | Grey (Black may skip it) |

## Summary

Green tried D155's `poo settings` and found that `poo settings -h` and
`poo settings help` fail with exit 1. The same is true of every command
made of subcommands except `grind`, `sim` and `wallet`. Each failed in
its own way:

- `policy -h` said "needs one NAME".
- `series -h` said "unknown series subcommand".
- `peer`, `submit`, `completion` and `settings` printed an error.

GREEN-032 made `-h` exit 0 for flags. Now `poo CMD help`, `-h` or
`--help` prints the same lines as `poo help CMD` and exits 0. A test
covers every such command, so a new one is covered too.

Green also found two cases where `-map-path` quietly does nothing:

- Without `-full`, no registry file is taken, so a map changes nothing.
- A map whose OLD matches no registry file in the file is most likely mistyped. For example, `/old/hom` for `/old/home`.

The text plan now adds a note for each case. The plan, the digest and
`-json` are unchanged.

## Idea

- `run`: when the only word after a command is `help`, `-h` or `--help`, and the command has subcommands (`subcommands`), it runs `helpCmd`. `ownHelp` lists `grind`, `sim` and `wallet`, which already answer from their own usage.
- `nodepolicy.PathMap.Maps(loc)` reports whether a map rewrites a path. It is never true for a registry URL, which `mapPaths` never rewrites.
- `pathMapNotes` returns "-map-path rewrites registry files, which only -full takes" without `-full`. It also returns "-map-path OLD=NEW matches no registry file in the file" for each unused map. Both imports print the notes, because they share `importBundleCmd`.
- The note for the reserved `settings` section said "this version imports node policies only". `poo settings import` exists now and still does not read that section, so the note now says "the file also holds a "settings" section, which this version does not read".

## Testing

- `TestSubcommandHelpWords`: for every command `subcommands` lists, `help`, `-h` and `--help` each exit 0. Each prints `poo help CMD`'s text, except for `grind`, `sim` and `wallet`, which print their own usage. It fails on `main`.
- `TestMapPathNotes`:
  - Without `-full` the note says only `-full` takes registry files.
  - A mistyped map is named.
  - A used map gets no note.
  - A URL is never mapped.
- gofmt, vet, staticcheck, and `go test -race ./...` pass.

## Risks

A script that passed `help` or `-h` as a policy NAME, peer ADDR or
similar now gets the usage instead of an error. None of those is a
valid name or address for those commands.

## For Grey and Red (not changed here)

- **`poo settings export` without `-full` keeps local registry file paths.** The export of `TestSettingsExportImport` keeps `/old/home/reg.txt`. Import treats such a path as machine-local and takes it only with `-full`. So the default export carries a path that its own default import drops. A path such as `/home/NAME/...` can name the person, and D155 says nothing personal goes in. Green has not changed this. Red may want to look, and Grey can decide whether `ExportSettings` (and `policy export`) should leave registry files out unless `-full`, as import does.
- **"p2p.registries[X]" text.** The "skipped, node settings" and "may not fit this machine" notes write a registry as `p2p.registries[X]`. The plan's own lines write it as `p2p.registries: X`, as GREEN-043 made them. Both strings are made in `nodepolicy`, which the GUI shows from `-json`, so Green left them alone.
- Still open from GREEN-020: errors name `until_from` where the person typed `-until-from`, and no command renews a lease.
