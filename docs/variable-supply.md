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
| 2 | 25 | Mints and transactions: a mint names its part, the tid covers it, claims and spends on released parts, a node keeps the newest release it knows for each series, and a transaction past it waits instead of failing. Pools past part 1. A node follows variable series. | Built (D166) |
| 3 | 26 | Passing releases on: nodes hand releases to their peers and through the RPC, the GUI shows parts and releases, and Red and Black test the whole path. | Built (D168); the GUI and the Red and Black passes follow in Round 26 |

Fixed series stay exactly as they are in every phase. Each phase adds
regression tests for this: `TestFixedSeriesUnchanged` and
`TestFixedSeriesIDPinned` in `pkg/series`, the frozen v1 vectors
(`TestByteFormV1Frozen`), and the existing ledger and node tests, which
run on fixed series only.

## Phase 1, as built

- **Params.** `series.Params` gains `SupplyType` (`fixed`, which is left out,
  or `variable`), `PartSize` (M) and `MaxParts` (0 means no limit). For a
  fixed series all three stay zero, and its JSON gains no key.
- **JSON form** (RED-071, D165). A variable series' JSON names its hash
  under `variable_hash` and leaves `hash` out. A node built before D163
  ignores keys it does not know, so with `hash` it would read the JSON as a
  fixed series with the same mints and another id. Without it, that node
  finds hash 0, which names no hash, and says the series is unreadable:
  `poo series info` fails, and a node lists it as not accepted. A fixed
  series' JSON is unchanged. This node also reads D163's first form (with
  `hash`), and refuses any key it does not know (`series.ErrUnknownKey`),
  so a later extension is unreadable here instead of misread. Compare the
  `series_id` that `poo series info` prints with the one you were given: a
  series with another id is not the series.
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
  - `PartID`, which computes the chain. It keeps a mark every so many
    parts (1,024 at first, doubled whenever there would be more than
    65,536 marks), the newest id and the last answer, so its memory stays
    under 2 MiB however far out a part is, and part k up to the newest
    costs at most one stride of hashes. It works far out in chunks of
    16,384 hashes under its lock, so other calls go on meanwhile. Part k
    costs k hashes once: time, never memory, and never a refusal, since
    refusing a valid release for being far out would make nodes with the
    same release disagree (RED-070, ORANGE-034, D165);
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

## Phase 2, as built (D166)

- **Naming a mint.** `series.MintID` has a `Part`, set for a variable series
  only. A variable transaction's body is `series ∥ U64 part ∥ U64 serial ∥ …`,
  so the tid covers the part. A fixed transaction is unchanged, byte for
  byte. The decoder reads the part when the series id is one of the
  variable series this process has built (`series.IsVariableID`): every
  entry belongs to a pool of a followed series, so the series is known
  before its transactions are read. The form is in
  [protocol/encoding-variable.md](protocol/encoding-variable.md#transactions).
- **One name.** The part must be `PartOf(serial)`. A fixed mint may not name
  a part (`series.ErrWrongPart`).
- **Released parts.** A series counts its released parts. Part 1 is
  released by the issuer witness: a variable series in node.json without
  one is kept and not followed (`nodepolicy.ErrNoPartOne`). `Mints()` and
  `PoolCount()` count released parts only.
- **Waiting.** A mint on a part past the newest release returns
  `series.ErrWaiting`, decided before any hashing. It is never invalid and
  never cached, so the same transaction is taken once the release arrives.
  Pools of unreleased parts are not followed yet, so p2p ignores their
  entries with no penalty. The RPC answers 409 `waiting`.
- **Releases.** A node keeps only the newest release of each series
  (`series[].release` in node.json), which proves every part before it.
  `Series.AddRelease` answers a known part at once, checks a newer one
  (witness first) and moves the count up. The count never goes down.
  Every command that reads a listed series builds it with its kept release
  (`SeriesInfo.Build`, D167), so `follow`, `unfollow` and policy import
  see the same parts as the node. A kept release that does not check is
  left out, never the series. `poo series add-release` checks every
  release, also one of a known part. A node keeps its own record of the
  releases it checked (`releases-checked.json`), so the next process
  checks only the witness, not the chain.
- **Tools.** `poo series add-release RELEASE` (and `POST
  /v1/series/release`) checks a release, saves it and reloads the node
  (since D168, takes it in place).
  `poo series followed` (and `GET /v1/series`) shows `variable{released,
  next_part, mints, units, release}`. `poo series sign` prints a signed
  series file that `poo init -series` and `poo series info` read.
- **Tests.** `TestFixedTxUnchanged` (2,000 random fixed transactions against
  a frozen v1 body), `TestVariableMints`, `TestVariableSeriesWaitsForRelease`,
  `TestVariableFollowed`, `TestVariableSeriesOnANode`, and a part 2 claim in
  `pkg/vectors/testdata/variable.json`.

## Phase 3, as built (D168)

- **Passing releases on.** A new wire v1 message, `release` (type 14),
  carries a release's byte form in its series' section. A node that does
  not know the type ignores it, so no protocol version changes. A node
  sends a peer the newest release of each variable series it follows
  when they connect, and passes a release it has just taken to its
  connected peers that follow the series.
- **Taking one.** A release of a series the node does not follow, or of a
  part it already knows, is dropped with no hashing (D165's point 4), and
  counted as a repeat. A later one is checked in full, witness first, so
  only the issuer can make a node work out a long chain. It is then kept
  in node.json under the same lock as every other edit of it, and added
  to the series the node runs (`node.AddRelease`). A release that does not
  check is an offence: its validity depends only on the series, so no
  honest node passes one on.
- **Bursts are coalesced** (RED-078, D169). A release from a peer has its
  witness checked at once (one signature). The first of a series is taken
  at once; any that arrive in the next 2 seconds wait, and only the
  highest of them is taken when the window ends, because it proves every
  part before it. A peer that replays releases oldest first therefore costs
  a node one save, relay, reload and credit per window, not one per
  release. A copy of the part the node holds is neither taken nor counted
  as a repeat, since honest links cross.
- **Off the read loop, and stoppable** (RED-082, D171). The full check of
  a release runs in one worker per series, never in the peer's read loop,
  so that peer's later messages (claims, blocks) are still read while a
  far part is worked out. The walk uses the host's context, so stopping
  the node cancels it, and a cancelled check is not counted against the
  peer. A part above 2^20 is logged when its check starts. There is still
  no cap on parts (D165).
- **In place, as a reload would.** The p2p host indexes the new part's
  pools as it does on start (`Host.AddPools`). Where that cannot give what
  a start gives, the node reloads onto the saved node.json instead: a
  series followed by a list of pools (its coverage can change), or one
  followed whole that the release makes large enough to be indexed by the
  pools in use (D154). `poo series add-release` takes the same path and
  replies `in_place`, or `reloading` when it cannot.
- **A series is built before its transactions are read** (Black, Round 25).
  A release names a series the node already built at start; a release of
  any other series is dropped. Nothing about a release changes how a
  transaction is decoded: the variable form follows from the series id,
  registered when the series was built.
- **Tests.** `TestReleasesPassBetweenPeers` (on connect, onward, a list
  follow reloads, a bad release is refused) and
  `TestReleaseInPlaceMatchesReopen` (released parts, followed and listed
  pools, the host's index and coverage, against a node opened on the same
  node.json) in `pkg/p2p`; `TestVariableSeriesOnANode` in `pkg/cli`.

## Phase 1's open questions, answered

These were the implementation questions phase 2 had to settle.


These are implementation questions. The documents settle what is valid, but
not how this code carries it. D166 answers each one.

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
  valid. None is built: D165 keeps the memory bounded, so only time is
  spent.
