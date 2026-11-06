# GUI-031: A ledger import that fails partway shows only an error, though the node already kept entries and switched to them

| Field | Value |
| --- | --- |
| Severity | low |
| Status | fixed: Source printed partial totals and stops between batches (main @ 58273f7), and the GUI draws them; verified by GUI/Red R35B @ 58273f7 |
| Fixed by | GUI/Blue, Round 33C (pools.js `ledgerImport`: a failed or stopped import shows a `.ledger-import-partial` alert that earlier parts may already be kept; views.mjs checks it). Drawing poo's partial totals waits on Source |
| Found on | main @ 3c4ae00 (PR #258, Pools and payments → "Import a ledger"; D188) |
| Component | pkg/cli/ledgerport.go (`ledgerImportCmd`: `send` returns the RPC error and drops `total`); gui/web/static/views/pools.js (`ledgerImport`'s `onResult` draws the summary only when `r.code === 0`) |
| Surface | node (what the selected ledger is after an import) |

## Summary

`poo ledger import` sends a ledger file to the node in batches: at most
256 items (`node.MaxImportItems`), or 8 MiB (`watch.ExportBytes`). Each
batch is checked in full, kept, and can switch the node to a heavier
branch, before the next batch is sent. The totals are added up in
`total`, and printed only once every batch has been sent.

If one batch fails, `send` returns its error at once, and `total` is
dropped. That covers a node stopped or restarted mid-import, the
5-minute RPC timeout, or a series unfollowed in between. poo prints only
the error and exits 1. Nothing says that earlier batches were kept, or
that the node switched to the imported branch.

The GUI draws the "Kept N entries … the node switched to the imported
branch" line only when poo exits 0, so it says nothing either. The form
shows `exit 1` and the dial error. A user who reads that as "nothing was
imported" is wrong: the node's selected ledger has changed.

A related point: Stop doesn't end an import between batches. The loop
doesn't check the job's context, and the RPC client has none. A large
file runs to its end, or to its first error, and the GUI's
`stop-pending` note shows meanwhile (GUI-028's mitigation). That is
bounded by the file, so it isn't filed separately. The same fix covers
it.

## Reproduction

poo and poo-gui at 3c4ae00, two `poo init -example` nodes (`-p2p off`).

1. On src: claim serials 5 and 6, build, and work each into pool 1, as
   `TestLedgerExportImport` does. `poo ledger export -pool 1 -out
   l.json` writes 2 entries.
2. imp.json is l.json with 12 more items of 8 MiB each (zeros) added
   after the 2 real ones. Each of those goes in its own batch, and the
   node would refuse each one as undecodable. The extra items only
   make the import last long enough to stop the node in the middle.
3. On a fresh dst node: `poo ledger import imp.json`. Once dst's pool 1
   is at height 2, stop the dst node.
   - poo prints only `poo: Post "http://127.0.0.1:…/v1/ledger/import":
     dial tcp …: connection refused`, and exits 1. There is no count.
   - After a restart, dst's pool 1 is at height 2: both entries are
     kept, and the selected ledger is the imported one.
4. Same in the GUI (Playwright, Pools and payments → Import a ledger,
   Ledger file imp.json, confirm). The run ends `exit 1` with the dial
   error. No `.ledger-imported` line is drawn. The node is at height 2.

## Impact

Nothing is trusted that shouldn't be. Every kept entry passed the
node's full checks, and a switch happens only as it would for a peer
(D188). So this isn't a consensus issue, and it isn't a hard fork. The
harm is a wrong belief: the user thinks the import didn't happen when
it partly did, and the node's selected branch has moved. They may
import again (harmless, the kept items count as known), or they may
act on the old branch, for example by building on a tip that is no
longer selected. Rated **low**.

## Recommendation

- **Source (pkg/cli/ledgerport.go), the real fix.** When a batch fails,
  print what the earlier batches did before the error. The `total` so
  far, plus how far it got (items sent, out of how many), would do, and
  the exit stays non-zero. In the JSON reply (the default), one way is an `ImportResult`
  with a `stopped` field (the error, and the index it stopped at)
  printed to stdout, with the error still on stderr. Also check the
  command's context between batches, so Stop ends an import at the next
  batch, with the same partial totals. This is a tool change only. The
  node's checks and what it keeps don't change.
- **GUI/Blue (pools.js), once poo prints it.** Draw the summary line
  from poo's reply whatever the exit code, as long as it parses with a
  numeric `kept`. On a partial import, make it a `next-step` alert:
  "The import stopped partway (poo's reason below). Before it stopped,
  the node kept N entries and M layers[, and switched to the imported
  branch]." Until then, a failed import could add one plain line under
  the error: "An import sends the file in parts; parts sent before the
  error may already be kept. Ledger (Pools and payments) shows where the
  pool stands now." That needs no poo reply. It only says what D188's
  batching already means.
- e2e (views.mjs): the GUI part can be tested without a node stop, by
  calling the form's `onResult` (or a pure helper it uses) with code 1
  and a reply that has `kept: 2, switched: true` and a `stopped` field.
  Expect the alert. The Source part fits next to
  `TestLedgerExportImport`: a stub client that fails on the second
  batch, with `kept` printed from the first.

## Notes — Round 34B verification (the interim line held up)

I checked this on 7099e84 with poo and Playwright, using the same repro:
the 2-entry file with 12 items of 8 MiB after it, and the node stopped
once pool 1 reached height 2.

- The run ends `exit 1` with the dial error. Above it, the
  `.ledger-import-partial` alert reads "The import did not finish. Parts
  of the file sent before it ended may already be kept, and the node may
  have switched to the imported branch: Selected ledger (Explore) and
  Followed pools show where the pool stands. …". Both panel names exist
  under those titles. The node held both entries after a restart, which
  is what the alert warns of.
- **Nit, not a finding.** The alert shows on any non-zero exit,
  including errors before anything was sent: a missing file, a file
  that isn't a ledger file, or no node running. There it says parts "may
  already be kept" when none can be. Its "may" keeps it true, and once
  poo prints partial totals, the GUI can show the alert only when the
  reply has them. Until then it could skip the alert when poo's error
  starts with "not a ledger file", or is an open error. That's optional.
- Source's part (partial totals, Stop between batches) is still open.
  `ledgerImportCmd` at 7099e84 still drops `total` on a batch error.

*Round 34C (GUI/Blue):* Source #266, on main in #272, prints the
partial totals (`sent`, `stopped`) and checks Stop between batches.
With nothing sent, it prints only its error. GUI/Grey's R34A follow-up
draws the partial summary from that reply. It shows no alert when there
is no reply, which settles the nit without matching poo's error text.
GUI/Blue left the import form to that change, to avoid a clash.

## Notes — Round 35B verification (fixed)

I checked this on 58273f7 with the same file. The ledger kind is now
`poo-ledger`, so I renamed it in the file.

- **Node stopped mid-import (CLI).** poo prints the reply to stdout:
  `kept: 2`, `layers_kept: 2`, `switched: true`, `sent: 3`, the refused
  item, and `stopped` with the dial error. It also prints the error,
  with "the first 3 entries were sent: 2 kept, 0 already held", to
  stderr, and exits 1.
- **The same in the GUI (Playwright).** The alert reads "The import
  stopped after 2 entries were sent (…connection refused…): kept 2
  entries and 2 layers; …; the node switched to the imported branch.
  Importing the same file again is safe …".
- **Stop between batches.** With Stop pressed once pool 1 was at height
  2, the job ended 0.8 s later, with "stopped: context canceled" and the
  same totals in the alert. Before, Stop did nothing until the file was
  done.
- My R34B nit (an alert with nothing sent) is gone: the GUI draws the
  alert only from poo's totals.

