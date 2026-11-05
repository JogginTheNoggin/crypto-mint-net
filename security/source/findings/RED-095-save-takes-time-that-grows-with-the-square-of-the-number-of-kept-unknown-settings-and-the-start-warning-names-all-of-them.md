# RED-095: `Save` costs time that grows with the square of the number of kept unknown settings, and the start warning lists every one of them in a single log line

| Field | Value |
| --- | --- |
| Severity | Low |
| Status | fixed |
| Found | Source Round 32 Part B, on `main` @ `6260590` (D184, W-1) |
| Area | `pkg/nodepolicy/unknown.go` (`withUnknown`, `insertSetting`), `pkg/node/node.go` (`Open`) |
| For Black | Not a rule. |
| Fixed by | `claude/project-thread-ggo8dt` @ 290702d (BLUE-033) |

## What is wrong

`withUnknown` calls `insertSetting` once per unknown setting, and each call
decodes the whole encoded policy into a map, adds one key and encodes it
again. *k* kept settings in a file of size *n* cost O(*k*·*n*), and *n* grows
with *k*. Separately, `node.Open` logs one warning with the dotted names of
all unknown settings joined by ", ".

Measured (`pkg/nodepolicy`, throwaway), top-level keys `"k0000001":1,…`:

| unknown settings | file | Load | Save | warning text |
| --- | --- | --- | --- | --- |
| 250 | 3 KiB | 0.4 ms | 24 ms | 2 KiB |
| 1,000 | 12 KiB | 1 ms | 0.38 s | 9 KiB |
| 2,000 | 25 KiB | 1.7 ms | 1.5 s | 19 KiB |
| 4,000 | 50 KiB | 2.1 ms | 5.7 s | 39 KiB |

Time goes up four times for each doubling. By the same growth a 200 KiB file would take about 90 s to
save (extrapolated, not run), with `policyMu` held (the peer-book and limits commands hold it across
`Save`), so every settings command waits behind it. A real newer release will
not have thousands of settings; a node.json taken from elsewhere can.

## How I checked it

Throwaway test, above. The size of the warning is the length of
`strings.Join(p.UnknownSettings(), ", ")`; the line itself was not logged.

## Suggested fix (Blue's call)

- Insert all kept settings in one pass: decode the encoded policy once, put
  every setting in, encode once.
- Cap the number of kept settings (see RED-094) and name at most the first
  few in the warning, with a count, as D184 does for the snapshot reasons.

## Not tested

The log line through the real logger.

## Resolution

Fixed in BLUE-033 (D185). ORANGE-042 (#249, carried) inserts every kept
setting in one pass. The start warnings name the first ten with a count
(`NameSome`), and doctor names the first twenty. With the 256-setting
cap from RED-094, a save stays quick (`TestBlue033KeptLimits`).
