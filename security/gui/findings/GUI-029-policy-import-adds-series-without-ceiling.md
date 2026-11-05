# GUI-029: Import node policies adds a series with no witness ceiling, which Write a node policy refuses, and the plan doesn't say so

| Field | Value |
| --- | --- |
| Severity | low |
| Status | fixed as a warning (Source RED-083 / D171 `unbounded`; GUI/Blue Round 28C/29A `plan-unbounded`; Apply stays, per jerry), merged; verified by GUI/Red R29B @ 4e1c601 |
| Found on | main @ adcf216 (PR #193, D170: `-any-ceiling`, `series_ceiling`, declined series in import plans) |
| Component | pkg/nodepolicy/bundle.go (`mergeInto`: a file's series is checked only by `DeclinesSeries`, and `series_ceiling` is 0, off, by default); pkg/cli/crypto.go (`checkCeiling`, used by `series new` and `init -series` only); gui/web/static/views/policies.js (`showPlan`: an added series is one `+ series` row) |
| Surface | node (the series a node follows, from a policy file someone else made) |
| Fixed by | Source: `Plan.Unbounded` / `CeilingWarning` (D171); GUI/Blue: the `plan-unbounded` Read first line in policies.js `showPlan` |

## Summary

D170 takes Red's condition 1 as tool policy: a series whose witness
ceiling is not set (`max_witness` 0), or above 4 MiB (`CeilingLimit`),
is refused by the tools unless the user says so. The GUI shows it:
- New series has "Allow a witness ceiling of 0 (not set) or above 4 MiB
  (-any-ceiling)".
- Write a node policy has "Accept a series file whose witness ceiling
  is not set or above 4 MiB".

Without the box, `poo init -series FILE` refuses such a file: "the series
sets no witness ceiling (max_witness 0), so one witness could be as
large as a frame; set -max-witness …, or pass -any-ceiling".

Import node policies takes the same series with no question asked:
- `mergeInto` checks a file's new series only with `DeclinesSeries`.
  That declines nothing while `series_ceiling` is 0, and 0 is the
  default (D170: "Nodes are not required to set it").
- poo's plan lists the series as `+ series: 61d1cc6c`, with nothing
  about its ceiling. The GUI draws that as one row.
- Apply adds it.

A policy file is the kind of thing people share (D149). So the one path
that adds a series from someone else's file is the one path with no
ceiling check. What the boxes on the other two forms ask the user to
agree to, the import does silently.

## Reproduction

poo and poo-gui at adcf216. `r25/series.json` is a variable series made
before D170, so it has `max_witness: 0`.

1. `poo init -series r25/series.json` refuses it: "the series sets no
   witness ceiling (max_witness 0) …, or pass -any-ceiling".
2. In a fresh home, `poo init -no-series`, then `poo policy import
   imp.json`, where imp.json lists that series. The plan reads
   `+ series: 61d1cc6c`, followed by the issuer witness warning, and
   nothing about the ceiling. In the GUI (Node policies → Import node
   policies), the preview is the same, and Apply is offered.
3. With `series_ceiling: 1000` set by hand in node.json, the same
   preview says "Left out by this node's series ceiling: 61d1cc6c: the
   series sets no witness ceiling, and series_ceiling is 1000". The
   declined path works. It is only off unless the user turns it on, and
   no form in the GUI sets it.

## Impact

A node that follows a series with no ceiling checks witnesses as large
as a frame (16 MiB, `wire.MaxFrame`). D170's own reason for the 4 MiB
line is that "one witness could take seconds to check". Anyone who
makes transactions in that series can keep the node busy checking
them. The user must import and apply a file someone else made, and the
series is node policy, not consensus, so it can be unfollowed. Rated
**low**, but it is the gap the `-any-ceiling` boxes exist to close.

## Recommendation

- **Source (pkg/nodepolicy, pkg/cli), the real fix.** Hold a policy
  import to the same tool policy as `init -series`. This is node policy
  only, with no consensus change, and a declined series is a lack of
  information (D170), so it is not a hard fork. Either of these would
  do:
  - Decline a new series whose ceiling is not set or above
    `CeilingLimit` unless the import is given `-any-ceiling`, even when
    `series_ceiling` is 0. It goes under `declined` as now.
  - Or make `CeilingLimit` the default `series_ceiling` for new
    policies. D170 leaves the default to Orange's and Red's
    recommendation, and this is Red's.

  At the least, the plan could warn, as it does for a missing issuer
  witness: `warning: series 61d1cc6c sets no witness ceiling`.
- **GUI/Blue (policies.js), once poo says it.** Put such a series in a
  "Read first" line above the plan table, as `plan-defences` and
  `plan-blocks` do (GUI-025), and add the `-any-ceiling` box to the
  import form if poo takes one. A Set series ceiling field (Settable
  limits, or the policy form) would also let a user turn the check on
  without editing node.json.
- e2e (views.mjs): import a file listing a series with `max_witness: 0`
  into a default policy. Expect it under "Left out …" (or a Read first
  warning) and not in the added rows. A declined plan has no e2e yet,
  as GUI Grey noted. The test with `series_ceiling` set, above, can be
  that one.

## Notes — GUI/Blue, Round 28C (routed, not fixed in the GUI)

**The owner's decision (jerry, 2026-11-04 22:40, relayed by the
coordinator).** The 4 MiB default ceiling stays off. An import must not
refuse such a series by default, and must not need an opt-in. Of the
recommendations above, only a warning is open: the first option,
declining without `-any-ceiling`, is out, and so is the second, a
default `series_ceiling`.

**Why the GUI can't warn on its own yet.** `poo policy import -json`
prints the plan (`pkg/nodepolicy.Plan`). An added series shows up there
only as a change line (`+ series: 61d1cc6c`), with nothing about its
`max_witness`. To warn, the GUI would have to read the series out of the
file and judge its ceiling itself. That is node policy worked out in
the browser, and the GUI must never diverge from poo (the owner's hard
rule). It also can't read a file given by path, since only poo-gui's
poo can.

**Routed to Source (through GUI/Grey).** The plan should name a new
series whose ceiling is unset or above `CeilingLimit`. That could be a
line in `Defences`, which poo prints under "read first" and the GUI
already draws as the `plan-defences` alert, or a new field. It should
not decline the series. `init -series` and `series new` keep their
`-any-ceiling` box. Once Source merges, the GUI shows it. With
`Defences`, it needs no change. With a new field, it gets a Read first
line like `plan-blocks`, plus an e2e test from Red's repro.

The optional nit (a plan where every series is declined still offers
Apply) is left as it is this round. Hiding Apply on an empty plan
changes the GUI-023 tests in views.mjs, which pin Apply's digest on an
unchanged file ("already the same"). Grey can decide, and poo's JSON
could carry what its text already does (no digest when nothing
changes).

## Notes — Round 29B verification (held up, as a warning)

I checked this with poo and Playwright on 4e1c601, using the same imp.json
(r25's series, `max_witness: 0`).

- **Merge into a default policy** (`series_ceiling` 0). poo prints
  "warning: series 61d1cc6c: the series sets no witness ceiling: one
  witness can take seconds and hundreds of MB to check". The GUI puts it
  in a "Read first: the import adds or follows more of N series with no
  witness ceiling …" alert above the table. I saw that alert on the
  Replace preview, and views.mjs checks it on a merge. Apply stays, which is
  jerry's call.
- **With `series_ceiling` set** (now settable: `policy limits -set
  series_ceiling=1024`), the merge declines the series and says how to
  change that. Nothing is added.
- **Replace** ignores `series_ceiling`. It adds the series and keeps a
  repeated one twice, though the unbounded warning still shows. Filed
  as GUI-030.
- A series already listed that the import starts to use only through
  `issuers.accept` (the `now_used` alert) is not in `unbounded`, because
  `Merge` checks series that are new or whose follow changed. That's a
  narrow case, since the series was already in the node's own list.
  Noted for Source, not filed.
