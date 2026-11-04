# GUI-GREEN-053: Say what a change did

| Field | Value |
| --- | --- |
| Status | adopted (Round 28 Part A) |
| Based on | main @ b9fed52 |
| Area | gui/web/static/views.js (appliedNote), views/policies.js (Set limits), views/node.js (Add a release), views/pools.js (Work), docs/gui-variable-supply-plan.md, gui/e2e/views.mjs, gui/e2e/variable.mjs |
| Effort | small |
| Decided by | (Grey) |

## Summary

Since D168, a running node takes some changes **in place** and reloads
for the rest. `poo policy limits -set` and `poo series add-release` both
say which happened: their JSON reply carries `saved`, `in_place` and
`reloading`, and add-release adds a `note` when nothing changed. The
GUI showed that reply only as a raw table, so the one answer a person
wants ("did it take?") was a `true` buried in a row.

This proposal adds one helper, `appliedNote(r, area)`. It puts one plain
line at the top of the result, read only from those reply fields. Set
limits and Add a release use it. It also fixes two small loose ends
found in the same pass.

## Problem

1. **The outcome of a change is hard to read.** After Save on Set
   limits, the result is a table of every limit followed by
   `saved true`, `in_place true`. A person cannot tell at a glance
   whether the node applied the change, is restarting its services, or
   will only use it at the next start (the node was not running).
2. **Set limits says something that is no longer always true.** Its
   help text said "a running node takes them at once". Since D168, only
   the peer book, tracker and seen list limits are applied in place.
   `p2p.verified_per_range`, `p2p.registry_seconds`, `max_pending` and
   `ties` still reload the node, which drops and re-dials its peers.
3. **Add a release has the same gap.** A release is applied in place,
   applied by a reload (for example, a series followed by a list of
   pools), or is already known. In the last case poo's `note` explains
   why, but it sat in a table cell.
4. **Tries on Work states no default.** Since D168, `poo work`'s default
   follows the series' layer floor: 65,536, or 2^(floor+2) when that is
   larger. The field was blank, so a person could not tell that leaving
   it empty is the right choice for a strong series.
5. **The plan names the wrong SLH-DSA set.** The Round 27 section of
   docs/gui-variable-supply-plan.md says SLH-DSA-256s. Source added
   SLH-DSA-SHA2-128s (D168), because the larger sets do not fit
   `MaxSigSize`.

## Idea

- **`appliedNote(r, area)` in views.js.** For a reply that is a JSON
  object, it picks one line:
  - `in_place`: "Saved to node.json; the running node applied it at
    once, with no reload."
  - `reloading`: "Saved to node.json; the running node is reloading to
    apply it."
  - `saved` only: "Saved to node.json; no running node took it, so it
    applies when the node next starts."
  - otherwise: "Nothing changed", followed by poo's `note` when there is
    one.

  The line goes above the raw result, which stays as poo printed it.
  The helper reads only those four fields, so the GUI holds no rule
  (D84): the list of limits that apply in place stays in Source
  (`nodepolicy.InPlace`). A later command with the same reply shape can
  opt in with one call.
- **Set limits** calls it, and its help text now says that the node
  applies the limits at once where poo can, or else reloads, and that
  the line over the result says which.
- **Add a release** calls it.
- **Tries** gets the placeholder "default 65536, or more for a series
  with a high layer floor; with For: as many as fit". parity.mjs
  (GUI-GREEN-043) already compares the stated 65536 with `poo work -h`.
- **The plan** now names SLH-DSA-SHA2-128s and says why the larger sets
  are not there.

## For Source (not built here)

- **Mark which limits apply in place.** `poo policy limits` could give
  each limit an `in_place` field, from `nodepolicy.InPlace`. The
  Settable limits table could then show which limits change without a
  reload before Save, without copying Source's list into the GUI. This
  matters to anyone setting `p2p.verified_per_range` on a busy node,
  because a reload drops its connections.

## Cost and risk

About 20 lines in views.js and one call in each of two forms. The reply
fields are the RPC's own (`rpc.LimitsReply`, `rpc.ReleaseReply`). A reply
without them, such as a list or text, gets no line. The policy import
plan keeps its own note, since its reply is a list. No consensus,
validity or policy change.

## Prototype

This PR.
- views.mjs: Set limits saves two in-place limits on a running node and
  checks the line "the running node applied it at once, with no reload".
- variable.mjs: Add a release checks that the line says the node took
  the release, in place or by a reload.
- parity.mjs checks Tries' stated default against poo.
