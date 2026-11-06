# RED-083: A policy import adds a series with no witness ceiling with no warning, and the text plan hides a declined series

| Field | Value |
| --- | --- |
| Severity | Low |
| Status | fixed |
| Found | Source Round 27 Part B, on `main` @ `1a1c729` (D170) |
| Area | `pkg/nodepolicy` (`bundle.go`, `Declined`), `pkg/cli` (`policy import`, `doctor`) |
| For Black | No consensus rule is touched; this is about what the tools tell the owner (jerry chose tools and node policy for the ceiling). |
| Fixed by | `claude/project-thread-ggo8dt` @ `60471a7` (BLUE-028) |

## What is wrong

D170 keeps `max_witness` 0 valid and makes it a tool and node-policy matter:
`poon series new` and `poon init -series` refuse it, and `series_ceiling`
lets a node decline such a series on import. Two places leave a gap:

1. **A shared policy file is the documented way series spread, and
   `policy import` says nothing about the ceiling.** The plan shows
   `+ series: 9d7f095d` and `follows every pool (2)` for a series with
   `max_witness` 0, the same as for the reserve. With `series_ceiling` off
   (the default) nothing is declined, and nothing is said.
2. **When `series_ceiling` is set and a series is declined, the text plan
   does not show it.** `Plan.Declined` is filled, but only `-json` prints
   it; the text plan says `nothing to change`. The owner sees an import
   that did nothing and no reason. The follow rule for that series is
   dropped with it, also unsaid.

`poon doctor` also reports `ok series 1 accepted` for a followed series with
no ceiling.

## How I checked it

```
$ poon policy export -dir n1 -out pexp.json          # n1 follows the documents' example, max_witness 0
$ poon init -dir r1                                   # the reserve series
$ poon policy import -dir r1 pexp.json
  + series: 9d7f095d                                 # no mention of the ceiling
  series 9d7f095d: follows every pool (2)
$ # series_ceiling 1048576 set in r1/node.json (it has no `policy limits` name)
$ poon policy import -dir r1 pexp.json
  nothing to change                                  # text plan
$ poon policy import -dir r1 -json pexp.json         # "declined": ["9d7f095d: the series sets no witness ceiling, and series_ceiling is 1048576"]
```

## What could happen

A node follows a series whose witnesses it cannot bound. I built the worst
ceiling-less witness the types allow: 4 levels of k-of-policies, 16 parts
each, Ed25519 leaves (65,536 signatures). It is 9.2 MB, and `Check` took
4.5 s and allocated 202 MB (about 22 times its size; 12 MB for a 581 KB one,
270 ms). The cost per byte is what it was under the 64 KiB caps, but a
single item is now one long check, and a node that did not choose the
series on purpose is exposed to it. (The reserve series, at 128 KiB, would cost about 60 ms and 3 MB for the
same shape; scaled from the 581 KB case, not run.)

## Suggested fix (Blue's call)

- Print `declined` in the text plan, and do not say `nothing to change` when
  something was declined. Name the follow rules dropped with it.
- Always warn in the plan (and in `poon series info`, `poon series followed`
  and `poon doctor`) for a followed or added series with no ceiling or one
  above `CeilingLimit`: "no witness ceiling: one witness can take seconds
  and hundreds of MB to check".
- Make `series_ceiling` settable and shown by `poon policy limits`, and
  consider writing it (4 MiB) in new nodes' node.json, so a node opts in to
  a looser series instead of opting out.
- Test: the plan of a no-ceiling series prints the warning; a declined
  series is printed in the text plan.

## Not tested

The GUI's import form (it calls the same plan).

## Resolution

Fixed in BLUE-028 (D171). An import plan lists in `unbounded` each series
it adds with no witness ceiling or one above 4 MiB, and the text plan
prints it as a warning. A declined series is printed too, with the number
of follow rules left out, and the plan says `nothing else to change`.
`poon series info`, `poon series followed`, `GET /v1/series` and
`poon doctor` (`ceiling`) warn for followed series. `series_ceiling` is now
settable with `poon policy limits -set`. Its default stays 0: a 4 MiB
default would decline the documents' example series, so it is left to
Grey and jerry. Tests: `TestImportWarnsOfUnboundedSeries`,
`TestDeclinedSeriesCountsItsFollows`, `TestSeriesCeilingIsBounded`,
`TestPlanTextNamesDeclinedAndUnbounded`.
