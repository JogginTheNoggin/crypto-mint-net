# GREEN-058: Ledger export takes pool 0, and import says what it needs

| Field | Value |
| --- | --- |
| Status | proposed (prototyped) |
| Based on | `main` @ `941049d` |
| Area | `pkg/cli` (`ledgerExportCmd`, `writeLedgerFile`, `ledgerImportCmd`), docs (poo) |
| Effort | small |
| Consensus | none: which command lines the CLI accepts and what its errors say. The node checks every imported entry in full, as before (D188) |
| Decided by | Grey (Black may skip it) |

## Summary

Green ran Round 33 as a node runner:

- **D187.** `poo init` wrote 7336 and 7337. No `8645` or `8646` is left
  in the docs, only in GUI e2e test data.
- **D186.** Two local nodes connected. `poo peers` showed the peer's
  `release` and `claimed_features`, and `-counts` counted releases.
- **D188.** Green built entries on one node and exported pools 0 and 1,
  imported them into a second node, imported them again, and tried the
  error cases.

Three loose ends showed up, all in `poo ledger export` and
`poo ledger import`:

1. **`poo ledger export -pool 0` said "needs -pool".** The check was
   `*pool == 0`, so pool 0 could never be exported. Every series has a
   pool 0, and in the worked example low serials land there. The GUI
   form due this round runs the same code.
2. **A `-from` past the tip wrote an empty file that claimed a range.**
   `-from 5` on a ledger 1 high exited 0 with a header reading `"from": 5,
   "to": 1` and no entries. A pool with no selected entry did the same.
   With `-out` the result was "wrote 0 entries" and a file that looks
   valid.
3. **An import the node cannot take named nothing.** It said only
   "engine: series not known" or "engine: pool not followed", although
   the file names both. If a later batch failed, the counts from earlier
   batches were lost too, so a runner could not tell where the import
   stopped. "wrote 1 entries" also read oddly.

## Idea

- **Export.** `-pool` is required to be given (`fs.Visit`), not to be
  non-zero.
- **The range.** When `-from` is past the selected height, the export
  fails with either "-from 5 is past the tip: pool 0's selected ledger
  is 1 high" or "pool 1 has no selected entries yet, so there is nothing
  to export". Nothing is written, since `-out` writes only a whole
  export. A `-to` past the tip still stops at the tip, as D188 says.
- **Import errors** (`importErr`):
  - An unknown series: the error adds "the file is for series S, which
    this node does not know; add the series to the node policy, follow
    pool K, then import again".
  - A pool not followed: the error adds "poo follow -series S -pool K,
    then import again".
  - A batch that fails after others went through: the error adds "the
    first N of M entries were sent: X kept, Y already held".
- **Wording.** "wrote 1 entry to FILE".
- **Docs.** The export and import rows in poo.md.

## How it was tested

- **`TestLedgerExportPoolZeroAndHints`** (pkg/cli) covers:
  - pool 0 exports one entry;
  - `-from 5` is refused;
  - an empty pool 1 is refused;
  - an import into a node without the series names the series;
  - a file changed to pool 7 names the `poo follow` command.

  It fails on `main` at the first step.
- **`TestLedgerExportImport`** (D188) still passes.
- **By hand:** the same cases against two running nodes.
- **Checks run locally:**
  - gofmt, go vet and staticcheck;
  - `go test -race ./...`;
  - the nine GUI e2e scripts.

## Ideas for later

- `poo peers -o table` shows `claimed_features` as `[4]`. Listing the
  names (`layers-from,many-layers,parts,release`) would make the column
  useful. That is a small change to the table renderer.
- `poo ledger import` could take `-dry-run` to report what the node
  would keep without keeping it. It is only a thought, since import
  already checks everything and is safe to repeat.
