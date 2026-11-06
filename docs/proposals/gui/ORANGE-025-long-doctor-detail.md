# ORANGE-025: Cut a long Health check line, and let `poon doctor` sum up

| Field | Value |
| --- | --- |
| Status | adopted (R33A, the GUI prototype as is); the second half routed to Source |
| Based on | main @ 5f80582 |
| Area | gui/web/static/views/node.js (`doctorReport`, new `shortDetail`), gui/e2e/qol.mjs; for Source, the `ceiling` check in pkg/cli/doctor.go |
| Effort | small |
| Decided by | |

## Summary

The Health check is the first panel of the Node view. It draws each
`poon doctor` check on one line. The `ceiling` check names every
followed series that sets no witness ceiling, joined into one detail
string. At 4,096 such series, that detail is 455,000 characters. The
Health check panel becomes 52,346 px tall, and every other panel of the
Node view is pushed 52,000 px down the page.

The prototype on this branch shows the first 1,000 characters of a
detail, followed by "…" and a "Show all N characters" toggle that shows
the whole text while it is open. With it the panel is 695 px tall. The
text is poon's own and nothing is computed: the cut is only on screen,
like the 500-row cut on tables (ORANGE-019) and the "Read first" line
(ORANGE-016).

For Source: `poon doctor` could sum up this check the way its snapshots
check already does, with the number of series and the first few ids
("4096 followed series set no witness ceiling: 1800ac16, …, and 4091
more"). That would keep `poon doctor` readable in a terminal too.

## Problem

`doctor.go` builds the ceiling check from `strings.Join(loose, "; ")`,
with one entry for each series that has no ceiling, or one above 4 MiB:

```
series 1800ac16: the series sets no witness ceiling: one witness can take seconds and hundreds of MB to check; …
```

Each entry is about 110 characters. `doctorReport` put the whole
detail in a single `span`, which wraps into about 2,300 lines at a
normal window width.

A node that follows many series without a ceiling is uncommon: the
reserve series sets one, and a new series file needs `-any-ceiling`
without one (D171). But nothing stops a node policy from following
thousands of series, and the doctor names each one. When it happens,
the Node view is unusable: the panels a person opens the view for are
52,000 px down.

## Measurements

The setup was the probe home with 4,096 signed variable series and no
witness ceiling (ORANGE-019), the real `poon` and `poon-gui`, and
Chromium through Playwright at 1280 by 900. The probe opened the Node
view, waited for the Health check, and measured that panel. Builds
alternated.

| Health check at 4,096 series | main @ 5f80582 | this branch |
| --- | --- | --- |
| `poon doctor -o json` | 456 KB, 1.7 to 2.0 s | the same |
| Characters in the panel | 455,841 | 2,214 |
| Longest line | 454,776 characters | 1,149 |
| Panel height | 52,346 px | 695 px |
| Node view busy, 3 runs | 0.74 to 0.79 s | 0.61 to 0.72 s |
| Node view settled, 3 runs | 1.74 to 1.89 s | 1.64 to 1.78 s |

The settle time hardly changes, because it waits on `poon doctor`
itself; ORANGE-024 is about that. The page work is smaller, and the
view is usable again.

On the reserve series every detail is under 1,000 characters, so the
panel looks exactly as before.

## Idea

1. **GUI (built here):** `shortDetail(text)` shows a detail of up to
   1,000 characters as before. A longer one shows its first 1,000
   characters and "…", with a `details` "Show all N characters". While
   that is open, the line shows the whole text, and closing it shows
   the cut again. Copy output and "Output as printed" still carry poon's
   whole answer.
2. **Source (proposed):** the `ceiling` check prints a count and the
   first few series ids instead of every one, as the `snapshots` check
   already does. The warning for each series stays in `poon series
   followed`, which the Followed series panel already shows in its
   warning column, 500 rows at a time.

## Cost and risk

- **GUI:** about 15 lines in node.js and one qol.mjs check.
- **Risk:** someone reading the Health check sees a cut line where they
  saw a very long one. The note says how long it is, and one press shows
  all of it.
- **Compatibility (White):** nothing stored and no API path changes.
  The GUI still reads `checks[].detail` as a string, so output from an
  older `poon` draws the same way.

## Prototype

Built on this branch. `gui/e2e/qol.mjs` checks that:

- a 30,000-character detail shows 1,001 characters (1,000 and "…"),
  with "Show all 30000 characters";
- opening it shows all 30,000 characters, and closing it shows 1,001
  again;
- a short detail has no toggle.

All ten e2e scripts pass.

## Modularity

Nothing to challenge.

## Consensus

None.
