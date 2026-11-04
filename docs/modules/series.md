# pkg/series

## Summary

A series: the published rules every mint in it follows. `Params` is the
human-editable form (also JSON); `New` validates it into a `*Series`, whose
id is `H("PoO-series" ∥ encode(params))` (decision D4). The series answers
every series-tier question the other packages ask: which hash, how many
mints, which pool a serial is in, pool ids, minimum strengths, whether entry
hashes count as credit, which policies are allowed, and the issuer.

A series is fixed, which is the series of the first documents, or
variable (PoO section 16.8, D163). A variable series adds a part size M
and an optional largest number of parts. Its issuer releases parts of M
more mints, in order. Each part's id names the part before it, and the
issuer's witness on a release sits outside the id. A fixed series is
written in format version 1, unchanged. A variable series is written in
version 2 ([../protocol/encoding-variable.md](../protocol/encoding-variable.md)).
The phases are in [../variable-supply.md](../variable-supply.md).

## API

- `Params`, `Encode`, `DecodeParams`; `New(params)` and
  `NewWithRegistry(params, reg)`; errors `ErrInvalid`, `ErrUnreadable`.
- `ID`, `Hash`, `PoolCount`, `PoolOf(serial)`, `PoolRange(k)`, `PoolID(k)`,
  `Mint(serial)`, `EntriesHaveNonce`, `MinEntryStrength`, `EntryHashCredit`,
  `AllowsPolicyType`, `AllowsKeyAlg`, `MaxWitnessSize`, `CheckWitness`,
  `Address`, `IssuerMessageTID`, `CheckIssuer`.
- `MintID{Series, Part, Serial}`: `Part` is set for a variable series only
  (D166).
- Variable supply (parts.go):
  - `SupplyType` (`Fixed`, `Variable`), `IsVariable`, `PartSize` and
    `MaxParts`.
  - `MintsThrough(k)`, `PartRange(k)`, `PartOf(serial)` and `PartID(k)`; `PartIDContext`, `UnsignedContext` and `CheckReleaseContext` stop when their context is done (GUI-028, D170).
  - `Reserve()` and `ReserveID`: the built-in default reserve series and its pinned id; `IsReserve(id)` matches the full id only (D170).
  - `ReleaseMessage(part_id)` and `Unsigned(k)`.
  - `Release{Series, Part, Previous, Witness}` with `ID`, `Encode`,
    `DecodeRelease` and JSON, and `CheckRelease`.
  - Errors `ErrFixed`, `ErrNoPart` and `ErrRelease`.
- Released parts (released.go, D166):
  - `Released()`, `AddRelease(r)` (a known part is answered with no
    hashing; a newer one is checked, then the count moves up; it never goes
    down), `NewestRelease()`.
  - `CheckMint(m)`: `ErrWrongPart` when the part is not `PartOf(serial)` or
    a fixed mint names one, `ErrNoSerial` past the series, `ErrWaiting`
    for a part not released here yet (checked before any hashing).
  - `IsVariableID(id)`: whether this process built a variable series with
    that id, which the transaction decoder uses to read the part.
  - `Mints()` and `PoolCount()` count released parts only.
- JSON (json.go): `Params.MarshalJSON` writes a variable series' hash as
  `variable_hash`, with no `hash`, so an older node refuses it;
  `UnmarshalJSON` reads both forms and refuses an unknown key
  (`ErrUnknownKey`, RED-071, D165).

## Guarantees

A `*Series` is immutable and valid: mints, supply and pool size are above
zero, the hash is known, and every listed type and algorithm is known to the
registry. A series naming an unknown hash or type is unreadable, not
invalid: another node may support it.

A fixed series has exactly the byte form, series_id and JSON it had before
variable supply. `TestFixedSeriesUnchanged` checks this against a frozen
copy of the version 1 encoder. A variable series names an issuer, and its N
and M are whole multiples of P, so every pool sits inside one part. The id
of a part follows from the series and the part number alone, so there is
one part k. `CheckRelease` checks the issuer's witness first, so only the
issuer can make a node compute a long chain of part ids. That chain costs
time, not memory: `PartID` keeps at most 65,536 ids and holds its lock for
16,384 hashes at a time (RED-070, D165). A part is never refused for being
far out.

A series' released count only grows, and two nodes holding the same series
and newest release have the same parts. A mint past them waits
(`ErrWaiting`): never invalid.

## Extending

A new series setting must not change the byte form of an existing series.
Write it in a new format version, which only the series that use it opt
into (as format version 2 does for variable supply), give it its own
vectors, and record a decision. Version 1 is frozen (D76).
