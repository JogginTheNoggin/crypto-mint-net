# Source Round 24: consensus review (Black, Part D)

## Summary

This round brings the first consensus change since Black began: **variable
supply series**, which the owner approved. The specification is *Mint
Series: Fixed and Variable Supply* and Proof of Ownership section 16.8.
Phase 1 (D163) builds what a variable series, its parts and its releases
are. A node still refuses to follow a variable series, as unreadable,
until phase 2 builds its transactions.

**Result: 10 clear, 0 with conditions, 0 blocked.** Nothing needs the
owner's approval.
- The change is the one the owner approved, built as the documents say.
- Each choice Grey made fills in a detail the documents leave open, and
  each is a reading the documents support (see below).
- No team proposed any other consensus change.

The coordinator asked two questions.

- **Can fixed series break? No.**
  - A fixed series keeps format version 1, byte for byte, and keeps its
    series_id and JSON.
  - Black checked this independently by comparing the build before this
    change (`215d428`) with `main` and with PR #159. They gave the same
    answers for 20,000 random fixed series: whether each is valid, its
    byte form, its series_id, its JSON, and its round trips through bytes
    and JSON (474 of them valid).
  - They also gave the same answers for 20,000 random byte strings of
    format versions 1 and 3.
  - The only difference is in version 2 bytes, which the old build calls
    unreadable. The new build reads them as a variable series, or as
    invalid when they are malformed.
- **Is a hard fork possible? No.**
  - The documents leave nothing in a part that its issuer can choose.
  - Every check in phase 1 is a pure function of the series and the
    release.
  - Nodes holding the same series and the same releases therefore reach
    the same answers.
  - A node that has not seen a release only lacks information.
  - In phase 1 no node follows a variable series at all, so no ledger is
    decided differently by anyone.

| Field | Value |
| --- | --- |
| Round | Source Round 24 |
| Consensus change | Variable supply series, phase 1 (D163). Approved by the owner; built as the documents specify |
| Other consensus suggestions | None |
| Base reviewed | `main` @ `08404df` (PR #154: D163, D164) |
| Previous base | `main` @ `215d428` |
| Blue branches | PR #159 @ `aaca943` (BLUE-025, D165: RED-070, RED-071; carries ORANGE-034) |
| Red | PR #156 @ `721e7a8` (RED-070, RED-071) |
| Proposals | ORANGE-034 (PR #157 @ `9cda494`); GREEN-049 (PR #158 @ `0543c57`) |
| Result | 10 clear, 0 with conditions, 0 blocked |

## Grey's D163 choices

| Choice | What the documents say | Black's view |
| --- | --- | --- |
| A variable series is format version 2: version 1's fields, then `U8` supply type, `U64` part size and `U64` largest number of parts | "Fixed series keep their exact byte form. The supply type field is left out of a fixed series." The encoding's stability rule makes any extension a new format version | **Agreed.** It is the only form that leaves every fixed series' bytes as they are. Version 2 with a supply type other than variable is refused, so each series has exactly one byte form. A node from before this change reads version 2 as unreadable, never as invalid (checked above) |
| The part number in `part_id` is written as `U64` | `part_id(k) = H("PoO-part" ∥ series_id ∥ k ∥ part_id(k − 1))`, with no width given | **Agreed.** It matches how a pool index is written in `pool_id`. It is pinned by `pkg/vectors/testdata/variable.json`, so every implementation must match it |
| `MaxParts` counts part 1 | "Max parts: optional cap on how many parts can ever exist." Part 1 is a part ("the definition is part 1") | **Agreed.** This is the plain reading. A cap of 1 is a variable series that never grows |
| The release signs `H("PoO-part-v1" ∥ part_id)` under the usual witness message | "A release signs 'PoO-part-v1' and a witness signs 'PoO-witness-v1', so neither is ever valid as the other." The release is "signed with the same issuer witness" | **Agreed.** It is built exactly like the issuer witness on a series (`H("PoO-issuer-v1" ∥ series_id)` as a pseudo-tid), so the same `Registry.Check` runs. The tags keep a release, an issuer witness and a transaction witness from ever being valid as one another |
| `CheckRelease` checks the issuer's witness, over the part id worked out from the release's own fields, before it works out the chain to check `Previous` | The node "recomputes part_id(k) from the series and k, checks k ≥ 2, checks k is within the cap … and checks the issuer witness" | **Agreed.** A release is valid only when every check passes, so the order cannot change the answer, only what a bad release costs. Checking the witness first means only the issuer can make a node work out a long chain |
| A part whose serials would pass 64 bits is no part, and a cap whose serials would not fit is refused | Serials are 64-bit numbers. The documents set no other bound | **Agreed.** Such a part could never hold a mint. Every node refuses it the same way, from the series alone. It is not a size rule a node chooses (see RED-070) |

## Why no hard fork is possible in phase 1

- **One version of each part.** The part id covers only the tag, the
  series_id, `k` and the previous id. The witness sits outside it.
  `Release.ID` is worked out from the release's own fields, and `Previous`
  must equal `part_id(k − 1)`.
- **Validity is a pure function.**
  - `CheckRelease` reads the series, the release and the policy registry.
    It does not read time, arrival order, the cache or node policy.
  - The part-id cache (`partChain`) and the witness cache
    (`policy.Verified`) only save work.
  - `TestPartChainBoundedAndExact` and `TestPartIDsAnyOrder` check that
    the cache gives the naive chain's ids in any order, and from 8
    callers at once.
- **Nothing withdraws a part.** No message closes a series or unreleases
  a part.
- **Not followed yet.** `Policy.Accepted` refuses a variable series with
  `ErrVariableNotFollowed`, which wraps `series.ErrUnreadable`.
  - No transaction, entry or pool of a variable series is checked, kept
    or selected anywhere.
  - Every node treats a variable series the same way: it is not followed.

## RED-070, ORANGE-034 and D165: part ids without a cap

The part-id chain now keeps a mark every stride. When the marks pass
65,536, it drops every other one and doubles the stride. Memory is
bounded, about 2 MiB, and the ids are the same.

The fix sets **no cap on how far out a part may be**, and Black agrees
that it must not. A release of part 10⁹ is valid at every node that has
the series. A node that refused it for being far out would disagree with
a node holding the same information, which is a hard fork under the
owner's definition.

A far release costs `k` hashes once, which is time, not memory. Only the
issuer can force this cost, because the witness is checked first. Red's
note and Orange's note both reach the same view: a node policy may make a
far part slow, worked out in the background, but never invalid. Black
will review any such setting.

## RED-071 and D165: a variable series' JSON

An older `poon` ignored the new keys and read a variable series' JSON as a
fixed series with another id. That was never a fork: the id differed, so
nothing on the network was confused. It did break the "unknown means
unreadable" rule for files.

`MarshalJSON` now writes a variable series' hash as `variable_hash`. An
older reader therefore finds no hash and refuses the series.
- A fixed series' JSON is unchanged. The cross-version check above
  compared it byte for byte.
- `UnmarshalJSON` refuses keys it does not know (`ErrUnknownKey`), so a
  later extension is unreadable rather than misread.
- Black checked that no key has ever been removed from `Params`. Every
  series JSON that any `poon` has written is read the same way.
- `Params` is never embedded in another JSON struct, so the new methods
  cannot capture a wider object.

## What phase 2 must hold (for Grey's Round 25 design)

These are not conditions on this round, which builds no transaction of a
variable series. They are what Black will check in phase 2. Each one is
from the documents, Red's note or D165.

1. **A transaction past the newest known release waits.**
   - It is never invalid, and never kept as invalid: not in the seen list,
     not as a mark against a peer that stays, and not as a refusal that
     lasts after the release arrives.
   - It is checked again once a release reaches its part.
   - A ledger that holds it is undecided at that node, never refused.
2. **Check a known release before hashing** (Red's point 4, D165). A part
   that no known release reaches is "not released yet", answered with no
   part-id work.
3. **No size rule on releases or parts.** A node policy may limit what a
   node follows or how fast it works, never what is valid.
4. **One name per mint.** The part named must be `PartOf(serial)`.
   `PartOf` follows from the series alone, so a wrong pairing is invalid at
   every node, whether or not a release is known.
5. **Fixed transactions keep their bytes and tids.** Grey's choice of
   where the part goes in a transaction's bytes must leave every fixed
   transaction, entry and tid as it is. Black will run the same
   cross-version comparison on transactions and entries.
6. **Keeping the newest release.** A node keeps only the newest valid
   release. Validity depends on whether a release exists, never on when it
   arrived. A release reaching a node late is a delay, not a fork.

## Clear

| Item | Why it is clear |
| --- | --- |
| D163: params, format version 2, validation | The approved change, as specified. Fixed series are unchanged, checked across builds |
| D163: `PartRange`, `PartOf`, `MintsThrough`, `PartID` | As specified. Pinned by vectors (`TestGoldenVariable`), with the documents' table in `TestPartRanges` |
| D163: `Release`, `CheckRelease`, the release message | As specified. Every check is a pure function. The witness tags keep each kind of signature distinct |
| D163: a variable series is not followed (`ErrVariableNotFollowed`) | Unreadable, never invalid. No ledger of it is decided anywhere |
| D163: issuer tools (`series new -variable`, `sign`, `release`, `parts`) | Local tools. A release is checked by the same `CheckRelease` |
| D164: peer import digest, `peers -counts`, an empty `-parent` | Local tools and the local RPC |
| BLUE-025 (RED-070) with ORANGE-034: part ids in bounded memory, no cap | Same ids, and no release is refused for its size |
| BLUE-025 (RED-071): `variable_hash`, unknown keys refused | File form only. The byte form and series_id are unchanged |
| GREEN-049: notes for a variable series kept but not followed | CLI text and one error message |
| Red (PR #156): RED-070, RED-071 and the notes on size caps and check order | Findings, both fixed in PR #159. Black adopts the notes as phase 2 requirements 2 and 3 |

## Items for the owner

- **Nothing needs your approval.** The variable supply change is the one
  you approved, built as your documents say. Black agrees with each
  detail Grey had to fill in: the byte form, the 8-byte part number, the
  cap counting part 1, the release's signed message, and the order of
  checks.
- **Fixed series cannot break.** Their bytes, ids and files are the same,
  checked against the build before this change.
- **No hard fork is possible.** Nothing in a part can be chosen. Every
  check gives the same answer at every node with the same series and
  releases. No size limit refuses a release.
- Phase 2 (Round 25) is where a node first follows a variable series and
  its transactions. Black has listed above what it will check.

How this was checked:
- Read the documents, D163, D164, D165, `docs/variable-supply.md`,
  `pkg/series/parts.go`, `json.go` and the `series.go` changes, and the
  diffs of PRs #156, #157, #158 and #159.
- Black's cross-version test (appendix) gave identical results on
  `215d428`, `08404df` and #159 for every fixed series and for every
  version 1 and version 3 byte string.
- `go test ./...` passes on `08404df`, #157, #158 and #159. `pkg/series`,
  `pkg/vectors` and `pkg/nodepolicy` pass under `-race` on #159.

## Appendix: the cross-version test

Copy it into `pkg/series` of each build, run it with
`BLACK_XV_OUT=out.txt go test ./pkg/series -run TestBlackCrossVersionFixed`,
and compare the output files. Lines starting `p` are fixed series. Lines
starting `b` are random bytes.

```go
package series_test

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"os"
	"testing"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/policy"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/series"
)

func TestBlackCrossVersionFixed(t *testing.T) {
	out, err := os.Create(os.Getenv("BLACK_XV_OUT"))
	if err != nil {
		t.Skip("BLACK_XV_OUT not set")
	}
	defer out.Close()
	rng := rand.New(rand.NewSource(24))
	pick := func(xs ...uint64) uint64 { return xs[rng.Intn(len(xs))] }
	for i := 0; i < 20000; i++ {
		p := series.Params{
			HashID:           hash.ID(pick(0, uint64(hash.SHA256), uint64(hash.SHA256), 2, 3, 99)),
			Mints:            pick(0, 1, 7, 8, 50000, 1<<40, ^uint64(0)),
			Supply:           pick(0, 1, 100, ^uint64(0)),
			PoolSize:         pick(0, 1, 4, 1000, 1<<63),
			MinLayerStrength: uint16(pick(0, 1, 8, 256, 257)),
			MinEntryStrength: uint16(pick(0, 4, 256, 300)),
			EntryHashCredit:  rng.Intn(2) == 0,
			MaxWitness:       uint32(pick(0, 1, 4096)),
		}
		switch rng.Intn(4) {
		case 0:
			p.PolicyTypes = []policy.TypeID{policy.TypeThreshold}
		case 1:
			p.PolicyTypes = []policy.TypeID{policy.TypeThreshold, policy.TypeThreshold}
		case 2:
			p.PolicyTypes = []policy.TypeID{policy.TypeID(pick(0, 1, 2, 7))}
		}
		switch rng.Intn(4) {
		case 0:
			p.KeyAlgs = []policy.AlgID{policy.AlgEd25519}
		case 1:
			p.KeyAlgs = []policy.AlgID{policy.AlgID(pick(0, 1, 2, 9)), policy.AlgID(pick(0, 1, 2, 9))}
		case 2:
			p.KeyAlgs = []policy.AlgID{policy.AlgID(pick(0, 1, 2, 9))}
		}
		if rng.Intn(2) == 0 {
			var h hash.Hash
			rng.Read(h[:])
			p.Issuer = &h
		}
		enc := p.Encode()
		j, _ := json.Marshal(p)
		line := fmt.Sprintf("p%d enc=%s json=%s", i, hex.EncodeToString(enc), j)
		if s, err := series.New(p); err != nil {
			line += fmt.Sprintf(" new=err(unreadable=%v)", errors.Is(err, series.ErrUnreadable))
		} else {
			line += " new=ok id=" + s.ID().String()
		}
		var q series.Params
		if err := json.Unmarshal(j, &q); err != nil {
			line += " json=err"
		} else if s, err := series.New(q); err == nil {
			line += " jsonid=" + s.ID().String()
		}
		if d, err := series.DecodeParams(enc); err != nil {
			line += fmt.Sprintf(" dec=err(unreadable=%v)", errors.Is(err, series.ErrUnreadable))
		} else {
			line += " dec=" + hex.EncodeToString(d.Encode())
		}
		fmt.Fprintln(out, line)
	}
	for i := 0; i < 20000; i++ {
		b := make([]byte, rng.Intn(140))
		rng.Read(b)
		if len(b) > 0 {
			b[0] = byte(pick(1, 1, 1, 2, 3))
		}
		line := fmt.Sprintf("b%d %s", i, hex.EncodeToString(b))
		if d, err := series.DecodeParams(b); err != nil {
			line += fmt.Sprintf(" err(unreadable=%v)", errors.Is(err, series.ErrUnreadable))
		} else {
			line += " ok=" + hex.EncodeToString(d.Encode())
		}
		fmt.Fprintln(out, line)
	}
}
```

Result: the `p` lines (474 valid fixed series) are identical across the
three builds, and so are the `b` lines of versions 1 and 3. Only the 4,001
`b` lines of version 2 differ. `215d428` calls them unreadable, and the new
builds read them as a variable series, refusing the malformed ones.
