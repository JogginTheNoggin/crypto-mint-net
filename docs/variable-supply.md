# Variable supply series: the plan

This page says how the node builds variable supply series, phase by phase.
The source documents are *Mint Series: Fixed and Variable Supply* and
*Proof of Ownership* section 16.8 (with the notes in sections 5, 16.3 and
16.4). jerry approved this consensus change for Round 24 onward. It is
built only as those documents specify. Anything else needs his permission.

## What the documents specify

- **Supply type.** A series is fixed, which is today's series, or variable.
- **What a variable series adds.** It adds a part size M and an optional
  largest number of parts. Both are inside the series_id. It must also name
  an issuer.
- **Pool size.** N and M must both be whole multiples of the pool size P.
- **Part 1.** Part 1 is the N mints of the definition. The issuer witness on
  the series releases it.
- **Later parts.** Part k, from 2 on, holds serials N + (k − 2)·M to
  N + (k − 1)·M − 1. Its id is
  `part_id(k) = H("PoO-part" ∥ series_id ∥ k ∥ part_id(k − 1))`, where
  part 1 names the series_id as its previous part.
- **Releases.** The issuer releases part k with the same issuer witness, over
  the part id. The release sits beside the part, outside its id. A release
  proves every part before it. Nothing withdraws a part.
- **Naming a variable mint.** A mint of a variable series is named
  (series, part, serial). The part must be the one whose range holds the
  serial. Claims, transactions and pools work as for a fixed series. The
  tid covers the part.
- **Waiting.** A transaction past the releases a node knows waits. It is
  never invalid. Missing information is not a hard fork.
- **Fixed series.** A fixed series keeps its exact byte form and
  series_id.

## Phases

| Phase | Round | What | State |
| --- | --- | --- | --- |
| 1 | 24 | The definition, parts and releases: params, byte form v2, validation, part ranges and ids, release records and checks, issuer tools (`poo series new -variable`, `sign`, `release`, `parts`), vectors. A node refuses to follow a variable series: it is unreadable at this node, never invalid. | Built (D163) |
| 2 | 25 | Mints and transactions: a mint names its part, the tid covers it, claims and spends on released parts, a node keeps the newest release it knows for each series, and a transaction past it waits instead of failing. Pools past part 1. A node follows variable series. | Next |
| 3 | 26 | Passing releases on: nodes hand releases to their peers and through the RPC, the GUI shows parts and releases, and Red and Black test the whole path. | Later |

Fixed series stay exactly as they are in every phase. Each phase adds
regression tests for this: `TestFixedSeriesUnchanged` and
`TestFixedSeriesIDPinned` in `pkg/series`, the frozen v1 vectors
(`TestByteFormV1Frozen`), and the existing ledger and node tests, which
run on fixed series only.

## Phase 1, as built

- **Params.** `series.Params` gains `SupplyType` (`fixed`, which is left out,
  or `variable`), `PartSize` (M) and `MaxParts` (0 means no limit). For a
  fixed series all three stay zero, and its JSON gains no key.
- **Byte form.** A fixed series is written in format version 1, unchanged.
  A variable series is written in format version 2: version 1's fields,
  then `U8 supply type (1)`, `U64 part size` and `U64 max parts`. This
  follows the stability rule in
  [protocol/encoding.md](protocol/encoding.md#stability): an extension is
  a new format version that a series opts into. A node built before
  phase 1 reads version 2 as unreadable, never as invalid. Version 2
  carrying a fixed supply type is refused, so each series has one byte
  form. The forms are in
  [protocol/encoding-variable.md](protocol/encoding-variable.md).
- **Validation.** A variable series must:
  - name an issuer;
  - have M ≥ 1;
  - have N and M be whole multiples of P;
  - have a largest number of parts whose serials fit in 64 bits.

  A fixed series may not set M or a largest number of parts.
- **Parts.** These are in `pkg/series/parts.go`:
  - `PartRange`, `PartOf` and `MintsThrough`;
  - `PartID`, which computes the chain and remembers it;
  - `ReleaseMessage`, which is `H("PoO-part-v1" ∥ part_id)`. A release
    witness is checked against this message, the same way the issuer
    witness on the series is checked against `H("PoO-issuer-v1" ∥
    series_id)`. The two tags differ, so a release is never valid as a
    transaction witness or as the series' issuer witness, and neither is
    valid as a release.
- **Releases.** `Release{Series, Part, Previous, Witness}` has a byte form
  (series ∥ part ∥ previous ∥ witness) and a JSON form with the witness in
  hex. `Series.CheckRelease` checks these in order:
  1. part ≥ 2, and within the cap;
  2. the issuer's witness over the release's own part id. This comes first,
     so only the issuer can make a node compute a long chain of ids;
  3. that `Previous` is `part_id(k − 1)`.
- **Not followed yet.** `nodepolicy.Policy.Accepted` refuses a variable
  series with `ErrVariableNotFollowed`, which wraps `series.ErrUnreadable`.
  This is the "unknown means unreadable" rule (PoO 16.7). Until phase 2 the
  node never decides anything about a variable mint.
- **Tools.**
  - `poo series new -variable -part-size M [-max-parts K] -issuer ADDR`.
  - `poo series sign FILE` prints the `issuer_witness` for node.json.
  - `poo series release FILE -part K` signs a release as the issuer, and
    refuses a key that is not the issuer's.
  - `poo series release FILE -check RELEASE` checks one.
  - `poo series parts FILE` lists the parts' serials, pools and ids.
- **Vectors.** `pkg/vectors/testdata/variable.json` holds the document's
  example series (N = 50,000, M = 10,000, S = 1, P = 1,000), its part ids
  and a release of part 2. It sits beside the frozen v1 file.

## Open for phase 2

These are implementation questions. The documents settle what is valid, but
not how this code carries it.

- **Where the part goes in a transaction's bytes.** A transaction is
  decoded today without knowing its series. A part in the body changes the
  transaction byte form, which needs a format version of its own under the
  stability rule. One choice is to decode with the series known, since
  every entry belongs to one pool of one series. The other is a marker
  that a fixed transaction never has. Grey picks one in Round 25, and
  Black reviews it. Either way, fixed transactions keep their bytes and
  tids.
- **Keeping releases.** A node stores the newest valid release of each
  variable series in its node.json, next to the series and its issuer
  witness. It needs only the newest, because a release proves every part
  before it.
- **Very high part numbers.** Checking a release of part k takes k hashes
  the first time. Only the issuer can make a node do this, because the
  witness is checked first. A node policy may limit how far a node works
  out part ids, as a limit on what it follows. It may never limit what is
  valid.
