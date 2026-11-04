# BLUE-025: Part ids cost time, never memory, and no part is refused; a variable series' JSON is unreadable to an older node

| Field | Value |
| --- | --- |
| Fixes | RED-070 (medium), RED-071 (low) |
| Branch | `claude/project-thread-ggo8dt` (PR against `main`) |
| Base | `main` @ `08404df` (Source Round 24 Part C), with ORANGE-034 (#157) merged in |
| Code | `c05286b` |
| Decisions | D165 |
| With | ORANGE-034 (PR #157): its marks every 1,024 parts are the start of RED-070's fix, so the PR carries its commit |
| Review | No consensus change. For Black: no part number is refused at any size; the JSON form of a fixed series is unchanged. |

## Summary

- **RED-070.**
  - `PartID` keeps a mark every so many parts (1,024 at first). When
    there would be more than 65,536 marks it keeps every other one and
    doubles the stride, so memory stays under 2 MiB however far out a part
    is. The ids are the same as before.
  - It also keeps the newest id and the last answer, so part k up to the
    newest costs at most one stride of hashes.
  - It works far out in chunks of 16,384 hashes and lets go of the lock
    between them, so other calls are answered meanwhile.
  - Nothing refuses a release for being far out. A refusal would make
    nodes with the same release disagree, which is a hard fork. Part k
    costs k hashes once, and only the issuer can ask for it, because the
    witness is checked first.
- **RED-071.**
  - A variable series' JSON names its hash under `variable_hash` and
    leaves `hash` out. A node built before D163 ignores keys it does not
    know, finds hash 0, which names no hash, and calls the series
    unreadable. Checked against `215d428`: `poo series info` fails with
    "unknown hash function number: 0" and `poo doctor` lists it as not
    accepted.
  - This node reads both forms (D163's first form had `hash`) and refuses
    a key it does not know (`series.ErrUnknownKey`), so a later extension
    is unreadable here instead of misread.
  - A fixed series' JSON is byte for byte what it was.

## Consensus

None.

- Part ids, release checks and the series_id are unchanged; only how the
  ids are kept in memory changed.
- JSON is a file form, not the byte form the series_id hashes. A fixed
  series' JSON is unchanged.

## Tests

- `pkg/series`:
  - `TestPartChainBoundedAndExact` (small strides; every id against a
    plain chain over 5,000 parts, also from many goroutines)
  - `TestPartIDFarOutInBoundedMemory` (8 million parts within 16 MiB of
    heap; a known part is answered while a long call runs)
  - `TestVariableJSONIsUnreadableToOlderReaders`
- Fixed series: `TestFixedSeriesUnchanged`, `TestFixedSeriesIDPinned`,
  `TestByteFormV1Frozen` and the ledger and node tests all pass.
