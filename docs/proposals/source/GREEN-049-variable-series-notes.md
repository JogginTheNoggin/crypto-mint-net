# GREEN-049: Say a variable series is kept, not wrong

| Field | Value |
| --- | --- |
| Status | proposed (prototyped) |
| Based on | `main` @ `08404df` |
| Area | `pkg/cli` (`initCmd`, `doctor`, `seriesParts`), no docs change |
| Effort | small |
| Consensus | none: command-line notes and one error message only. `Accepted`, the byte forms, part ids and `CheckRelease` are untouched, and a node still refuses to follow a variable series (D163) |
| Decided by | Grey (Black may skip it) |

## Summary

Green tried the D163 issuer tools and the D164 fixes as a user. The tools
work. Signing, releasing, checking a release against the wrong series and
refusing parts past the cap all behave as `docs/variable-supply.md` says.
`poo peers -counts` and the empty `-parent` refusal also work. Three things
mislead a node runner.

1. **`poo init -series` gave no hint for a variable series.** It wrote
   node.json with "(1 series)". Then the node logged "series not accepted"
   and "follow rule names a series not known or not accepted", and followed
   0 pools.
2. **`poo doctor` said to fix a series that is correct.** For a variable
   series its fix was "correct or remove it in the policy". The series is
   valid. This node simply does not follow variable series yet, and phase 2
   will. Removing it would mean adding it again later.
3. **`poo series parts -from K` past the last part gave the wrong
   error.** On a series with `-max-parts 3`, `-from 5` said "-to must be
   from -from to -from + 999", because the default `-to` had been capped
   to 3.

All three are fixed on the branch.

## Idea

- `variableNote`, in `pkg/cli/node.go`, holds one sentence: "this node keeps a variable supply series in its policy but does not follow it yet; leave it there, and a node that follows variable series will (docs/variable-supply.md)". Phase 2 deletes it.
- `poo init` prints `note: series ID is a variable supply series: <variableNote>` after the "wrote" line, once for each variable series. A fixed series prints nothing extra.
- `poo doctor` counts the refused series that wrap `nodepolicy.ErrVariableNotFollowed`:
  - When every refused series is variable, the fix is `variableNote`.
  - When some are and some are not, the fix is "correct or remove the other series that are not accepted; " followed by `variableNote`.
  - When none is variable, the fix is unchanged.
- `poo series parts` refuses a `-from` past the largest number of parts with "-from 5 is past the last part: series ID has at most 3 parts".

## How it was tested

- `TestVariableSeriesNotes` (`pkg/cli/seriesparts_test.go`) covers the parts error, the init note, the doctor fix, and that a fixed series' init has no note. It fails on `main` and passes on the branch.
- These all pass: gofmt, go vet, staticcheck, `go test -race ./...`, and the nine GUI e2e scripts.

## Sent to Red

The part-id cache (`partChain.ids` in `pkg/series/parts.go`) keeps every id
in one slice. `poo series parts FILE -from 20000000 -to 20000000` used 2.9
GB and took 23 s. `poo series release FILE -part 4000000000000000000` ran
until it was killed. Only the issuer can cause this, because the witness is
checked first. In phase 2, though, a node checking an issuer-signed release
of a huge part would run out of memory, not just spend time. Red has the
details, and the fix is theirs to file.

## Ideas for later

- `poo peers -counts` still prints the empty `peers` and `book` lists, which
  `-o table` shows as `peers -` and `book -`. Leaving them out when
  `-counts` is set (`omitempty`, or a reply of its own) would make the
  output only what was asked for. The GUI reads this reply, so this is
  left to Grey.
- `poo series parts` and `poo series release -check` print JSON only. An
  `-o table` would list the parts one row each, like the other list
  commands.
