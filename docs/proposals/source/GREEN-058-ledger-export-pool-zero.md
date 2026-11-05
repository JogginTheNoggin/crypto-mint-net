# GREEN-058: Ledger export refuses a start past the tip, and import says what it needs

| Field | Value |
| --- | --- |
| Status | proposed (prototyped) |
| Based on | `main` @ `941049d`, merged with `main` after #256 |
| Area | `pkg/cli` (`writeLedgerFile`, `ledgerImportCmd`), docs (poo) |
| Effort | small |
| Consensus | none: what the CLI accepts and what its errors say. The node checks every imported entry in full, as before (D188) |
| Decided by | Grey (Black may skip it) |

## Summary

Green ran Round 33 as a node runner:

- **D187.** `poo init` wrote 7336 and 7337. No `8645` or `8646` is left
  in the docs, only in GUI e2e test data.
- **D186.** Two local nodes connected. `poo peers` showed the peer's
  `release` and `claimed_features`, and `-counts` counted releases.
- **D188.** Green built entries on one node and exported them, imported
  them into a second node, imported them again, and tried the error
  cases.

Green first found that `-pool 0` was refused. Grey fixed that in #256
first, so this PR keeps only the two other loose ends:

1. **A `-from` past the tip wrote an empty file that claimed a range.**
   `-from 5` on a ledger 1 high exited 0 with a header reading `"from": 5,
   "to": 1` and no entries. With `-out` the file looked valid.
2. **An import the node cannot take named nothing.** It said only
   "engine: series not known" or "engine: pool not followed", although
   the file names both. If a later batch failed, the counts from earlier
   batches were lost, so a runner could not tell where the import
   stopped.

## Idea

- **The range.** When the pool has selected entries and `-from` is past
  its height, the export fails: "-from 5 is past the tip: pool 0's
  selected ledger is 1 high". Nothing is written. Two cases stay as D188
  and #256 have them:
  - A pool with no selected entry still exports as an empty file.
  - A `-to` past the tip still stops at the tip.
- **Import errors** (`importErr`):
  - An unknown series: the error adds "the file is for series S, which
    this node does not know; add the series to the node policy, follow
    pool K, then import again".
  - A pool not followed: the error adds "poo follow -series S -pool K,
    then import again".
  - A batch that fails after others went through: the error adds "the
    first N of M entries were sent: X kept, Y already held".
- **Docs.** The export and import rows in poo.md.

## How it was tested

- **`TestLedgerExportPastTipAndImportHints`** (pkg/cli) covers:
  - `-from 5` on a ledger 1 high is refused;
  - an import into a node without the series names the series;
  - a file changed to pool 7 names the `poo follow` command.

  It fails on `main`.
- **`TestLedgerExportImport`** (D188, with #256's pool 0 case) still
  passes.
- **By hand:** the same cases against two running nodes.
- **Checks run locally:**
  - gofmt, go vet and staticcheck;
  - `go test -race ./...`;
  - the nine GUI e2e scripts.

## Ideas for later

- `poo peers -o table` shows `claimed_features` as `[4]`. Listing the
  names would make the column useful.
- "wrote 1 entries" could read "wrote 1 entry".
