# ORANGE-027: Let `poo ledger import` send a ledger as it reads it, and say how far it got

| Field | Value |
| --- | --- |
| Status | proposed (needs Source: pkg/cli/ledgerport.go) |
| Based on | main @ 3c4ae00 |
| Area | `ledgerImportCmd` in pkg/cli/ledgerport.go (Source). The GUI runs it in poo-gui's own process from the Pools view's "Import a ledger" form |
| Effort | small |
| Decided by | |

## Summary

`poo ledger import` decodes the whole ledger file into memory, then
sends it to the node in batches of 256 entries. For a 100,000-entry
ledger (58.6 MB) that costs:

- **On the command line:** 195 MB of memory in `poo`.
- **In the GUI:** poo-gui runs the command in its own process. It grew
  from 14 MB to 198 MB when given a path, and peaked at 383 MB when the
  file was picked in the browser and sent as input. It stayed at 90 to
  198 MB afterwards.

The import also takes 44 to 51 s and prints nothing until it ends. The
form says "running…" for the whole time, with no sign of how far it
has got.

The proposal is for Source:

- decode the entries one at a time and send each batch as soon as it is
  full, so memory stays at about one batch (at most 8 MiB);
- print a progress line to stderr after each batch, which the GUI's
  live pane already shows.

ORANGE-012 and ORANGE-014 did the same for `poo submit entry` and the
policy file pick.

## Problem

```go
var f ledgerFile
if err := json.NewDecoder(r).Decode(&f); err != nil { … }
…
for _, it := range f.Entries { … batch = append(batch, it.Item) … }
```

`Decode` builds every entry, with its summary and its base64 item
decoded to bytes, before the first batch goes out. The node checks each
batch as it arrives, about 0.45 ms an entry. So the file sits whole in
memory for the whole 45 s, and nothing is printed until the total at
the end.

The export side already streams: `writeLedgerFile` writes page by page,
"so a long ledger never sits whole in memory", and stays at 18 MB for
100,000 entries.

## Measurements

The setup is the same as ORANGE-026: one pool of N entries made by a
throwaway Go test, the real `poo` at 3c4ae00, and a second node holding
nothing that receives the import. Memory is the child's peak RSS for
`poo`, and `ps` sampled every 200 ms for poo-gui.

| Entries (file) | `poo ledger import`: time, peak RSS | GUI by path: time, poo-gui RSS | GUI picked file: time, poo-gui RSS |
| --- | --- | --- | --- |
| 2,000 (1.2 MB) | 0.70 s, 18 MB | 0.84 s, 14 → 21 MB | 1.4 s, peak 23 MB |
| 20,000 (11.7 MB) | 8.4 s, 55 MB | 7.9 s, 14 → 58 MB | 9.0 s, peak 55 MB |
| 100,000 (58.6 MB) | 44.5 s, 195 MB | 48.7 s, 14 → 198 MB | 51.3 s, peak 383 MB, 90 MB after |

The page stayed idle the whole time: no long tasks, and frame gaps of
17 to 33 ms. Importing the same file a second time, poo replied that
all 100,000 entries and layers were already known.

The receiving node held 27, 117 and 519 MB after these imports. That is
the node's own state, which Source's disk-backed state plan covers, not
this command.

## Idea

1. Read the file with `json.Decoder` token by token. Check `kind` and
   `format` from the header fields, which come first in the file
   (`writeLedgerFile` writes them first), then decode one entry at a
   time from the `entries` array. Each batch is sent when it reaches
   256 entries or 8 MiB, as now.
2. After each batch, print a line to stderr such as "sent 25,600
   entries: 25,600 kept, 0 known, 0 refused". The `-o json` reply on
   stdout stays the same total, so the GUI's note ("Kept N entries…")
   is unchanged. The GUI shows stderr in the live pane as it arrives,
   so the 45 s wait shows progress with no GUI change.
3. A file whose header comes after its entries (a hand-edited one)
   could be read in two passes, or refused with a clear message.
   Source's choice.

## Cost and risk

- **Source:** about 40 lines in one function. The checks on each entry
  stay in the node, as now (D188: every import fully checked).
- **Risk:** none to consensus. Only how the client reads and sends the
  file changes. What the node accepts does not.
- **Compatibility (White):** the file format and the reply are
  unchanged. Old files import the same way.

## Prototype

None: the change is in Source's code. The numbers above are the
baseline to measure against.

## Modularity

Nothing to challenge.

## Consensus

None.
