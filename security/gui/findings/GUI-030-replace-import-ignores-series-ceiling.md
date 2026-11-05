# GUI-030: Import node policies with Replace ticked ignores the node's series_ceiling and adds the series it should decline

| Field | Value |
| --- | --- |
| Severity | low |
| Status | open |
| Found on | main @ 4e1c601 (PR #204; D170 `series_ceiling`, made settable by D171 / RED-083) |
| Component | pkg/nodepolicy/bundle.go (`Merge`: only the merge path, `mergeInto`, calls `DeclinesSeries`; `opt.Replace` takes `file.Series` whole); gui/web/static/views/policies.js (Import node policies' "Replace this node policy with the file's instead of merging" box) |
| Surface | node (the series a node follows, from a policy file someone else made) |

## Summary

`series_ceiling` is now a settable limit: Node policies → Settable
limits, or `poo policy limits -set series_ceiling=N`. When it is set, a
policy import declines a series whose witness ceiling is not set, or is
above that value. D170 says: "when set, a policy import declines a
series it would add whose ceiling is not set or above it".

That only holds for a merge. `Merge` reaches `DeclinesSeries` only
through `mergeInto`. With `-replace`, without settings, the code
comment reads "What to follow is the file's; how the node runs stays
its own":
- `res.Series` becomes `file.Series` as it is, with no check.
- The node keeps its own `series_ceiling` (1024 in the repro), so the
  setting still reads as on, but nothing applies it.

The Import form offers this as a checkbox ("Replace this node policy
with the file's instead of merging"). A user who set `series_ceiling` to
keep such series out gets them in by ticking one box. The plan has no
"Left out by this node's series ceiling" line, and no word that the
ceiling was skipped.

GUI-029's fix does help here. The plan's `unbounded` list still names
the series, and the GUI shows it as "Read first: the import adds … with
no witness ceiling". So the user is warned that the series is
unbounded. What they aren't told is that the setting they chose to
refuse it didn't apply.

The same path also keeps a series the file lists twice as two entries
in node.json. A merge dedupes by series id. The GUI then counts "2
series" in the Read first line, and poo prints each warning twice.

## Reproduction

poo and poo-gui at 4e1c601. imp.json lists `r25/series.json` (a variable
series, `max_witness: 0`) twice, with no follow rules.

1. `poo init -no-series`, then `poo policy limits -set
   series_ceiling=1024`.
2. `poo policy import imp.json` (merge) prints "declined: series
   61d1cc6c: the series sets no witness ceiling, and series_ceiling is
   1024 …", then "nothing else to change".
3. `poo policy import imp.json -replace` prints `+ series: 61d1cc6c`,
   with the issuer and ceiling warnings, each twice. There is no
   declined line.
4. In the GUI (Node policies → Import node policies), with Replace
   ticked: the Read first alert counts "2 series with no witness
   ceiling", there is no "Left out" note, and Apply is offered. After
   Apply, node.json holds 2 series entries for 61d1cc6c, and
   `series_ceiling` is still 1024. Checked with Playwright.

## Impact

It is the same as GUI-029's: the node follows a series whose witnesses
can be as large as a frame, and each one takes seconds to check. This
time it gets past a check the user turned on themselves. It needs a
file someone else made, Replace ticked, and Apply pressed past a Read
first warning, and the series can be unfollowed again. Rated **low**.

## Recommendation

- **Source (pkg/nodepolicy/bundle.go), the real fix.** Apply
  `DeclinesSeries` on the Replace paths as on the merge. For
  `opt.Replace && !settings`, check each `file.Series` that `local`
  doesn't already list against `local.SeriesCeiling`, the ceiling that
  stays. Put declined ones, and their follow rules, under
  `plan.Declined`, as `mergeInto` does. For `opt.Replace` with
  settings, the file's own `series_ceiling` replaces the node's, so
  check against the result's. Dedupe `res.Series` by id on Replace as
  merge does, or refuse a file that lists a series twice. This is node
  policy only, and declining is a lack of information (D170), so there
  is no consensus change and no hard fork.
- **GUI/Blue:** nothing more is needed once poo declines. The
  declined-series note and GUI-029's Read first line already draw
  poo's lists. Until then, the Replace box's label could say
  "(series_ceiling is not applied)".
- e2e (views.mjs): with `series_ceiling=1024` set, import a file that
  lists a series with `max_witness: 0`, with Replace ticked. Expect
  "Left out by this node's series ceiling" and node.json unchanged
  after Apply.

## Notes for Source (not part of this finding)

- `nodepolicy.go` says `poo init` "writes CeilingLimit in a new
  node.json" (on `SeriesCeiling` and on `CeilingLimit`). It doesn't:
  `poo init` and `poo init -no-series` at 4e1c601 write no
  `series_ceiling`, and `policy limits` shows `set 0`. D171's "Not done
  here" says that default was left to Grey and jerry, so the comments
  are the stale part.
