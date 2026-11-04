# Source Round 25: consensus review (Black, Part D)

## Summary

This round is **phase 2 of variable supply series** (D166), the
consensus change the owner approved. From now on, a node follows a
variable series and the parts it knows released. Black checked it
against the six requirements in [ROUND-24.md](ROUND-24.md), and **all six
are met**. Black also reran the cross-version test, now extended to
transactions and entries. **Every fixed series, transaction and entry is
unchanged.**

**Result: 11 clear, 0 with conditions, 0 blocked.** Nothing needs the
owner's approval.
- Nothing outside the documents was added.
- Grey's one choice, how a variable mint's part is read from a
  transaction's bytes, is within the documents (see below).
- No team proposed another consensus change.

Black also gives its view on ORANGE-035's design for applying limits in
place: **acceptable**, with the points below.

| Field | Value |
| --- | --- |
| Round | Source Round 25 |
| Consensus change | Variable supply series, phase 2 (D166). Approved by the owner; built to the documents and to Black's six requirements |
| Other consensus suggestions | None |
| Base reviewed | `main` @ `e93eb29` (PR #165: D166, with Round 24's PRs merged) |
| Previous base | `main` @ `08404df` |
| Blue branches | PR #170 @ `ad10c6b` (BLUE-026, D167: RED-072 to RED-076; carries ORANGE-035) |
| Red | PR #169 @ `bbd966f` (RED-072 to RED-076) |
| Proposals | ORANGE-035 (PR #167 @ `707c97e`, with the in-place design); GREEN-050 (PR #168 @ `60b9538`) |
| Result | 11 clear, 0 with conditions, 0 blocked |

## The six requirements

| # | Requirement (ROUND-24) | How D166 meets it | Met |
| --- | --- | --- | --- |
| 1 | A transaction past the newest known release waits. It is never invalid and never kept as invalid | `Series.CheckMint` returns `ErrWaiting`, which is not `ErrInvalid`. `HandleTx` adds a transaction to the seen list only once it is taken. `TestVariableSeriesWaitsForRelease` refuses the same claim twice with `ErrWaiting` and takes it after the release. A pool of an unreleased part is outside `PoolCount`. An entry for one is therefore "not followed" (`poolFor` is nil, and `resolveBig` stops at `Mints()`), so it is dropped with no penalty, never with `ErrInvalidEntry`. The RPC answers 409 `waiting` | Yes |
| 2 | Check a known release before hashing | `CheckMint` compares the part with an atomic count of released parts and works out no part id. `AddRelease` answers a part it already has with no hashing | Yes |
| 3 | No size rule on releases or parts | Nothing new limits a part number or a release, apart from the series' own cap. Working out a far part costs time, in bounded memory (D165) | Yes |
| 4 | One name per mint | `CheckMint` refuses a part that is not `PartOf(serial)` (`ErrWrongPart`), a serial no part can hold (`ErrNoSerial`), and a fixed mint that names a part. All three follow from the series alone, so every node refuses them alike, whether or not it knows a release | Yes |
| 5 | Fixed transactions keep their bytes and tids | The mint is written `series ∥ part ∥ serial` only for a variable series. A fixed mint has part 0, and 0 is never written. Black's cross-version test (below) agrees, as does Grey's `TestFixedTxUnchanged` | Yes |
| 6 | Keep only the newest release; validity never depends on when a release arrived | `series[].release` in node.json holds one release. `AddRelease` checks a newer one in full, witness first, and only then moves the count, which only grows. With the same series and the same release, every node has the same parts | Yes |

### Grey's choice: reading the part from the series id

A transaction's bytes are read without a format marker. `tx.DecodeFrom`
reads a part after the series id only when the series id is a variable
series this process has built (`series.IsVariableID`).

- **Within the documents.** They specify the mint `(series, part,
  serial)` and that the tid covers the part. They do not fix the byte
  layout. This layout gives exactly one form for each mint and leaves
  fixed bytes alone.
- **Why it cannot split nodes.** The same bytes read differently only at
  a node that has not built that variable series. Such a node does not
  follow the series, so it never decides any of its transactions or
  entries. All the nodes that follow a series have built it, read its
  transactions the same way, and run the same checks.
- **An entry of another pool that carries a variable series' transaction**
  is refused as being of the wrong series by every node, however each
  one reads it, because the series id is read first and is the same.
- **One thing to keep, for phase 3 and later:** a series must be built
  before any transaction of it is decoded. This applies to the store on
  start, the wallet, and releases and series passed between nodes.
  D166 says this holds today. Black will check it again when phase 3
  passes releases between nodes.

## Fixed series, transactions and entries: the cross-version test

Black reran [ROUND-24's series test](ROUND-24.md#appendix-the-cross-version-test)
and a new test for transactions and entries (appendix). Each ran on the
build before phase 2 (`08404df`), on `main` (`e93eb29`) and on PR #170.

- **Series.** The output is identical to Round 24's new-build output: 20,000
  random fixed series, plus random version 1 and 3 bytes, all the same as
  `215d428`. As before, only version 2 bytes differ.
- **Transactions.** All three builds give identical results for 20,000
  random fixed-series transactions. These include real claims with valid
  witnesses, random inputs, outputs and witnesses, and foreign series ids.
  The test compared each one's bytes, tid, `CheckForm`, `CheckStateless`
  and decode round trip (6,398 pass `CheckForm`).
- **Entries.** All three builds give identical results for 5,000 random
  entries. The test compared each one's bytes, entry hash,
  `CheckStructure` and decode round trip (463 pass `CheckStructure`).
- **Random bytes.** All three builds give identical results for 20,000
  random byte strings, read both as a transaction and as an entry. Half
  of them start with the example series id.

## BLUE-026 (D167) and ORANGE-035

- **RED-075: the checked-release cache.**
  - `releases-checked.json` beside node.json lets a node skip working out
    `part_id(k − 1)` again for the release it keeps. The id is used only
    when the cache entry matches that release exactly, and the witness is
    still checked.
  - The cache is read only from the node's own directory, never exported
    or imported. A missing or damaged cache costs time only.
  - This is clear. The cache holds what this node itself worked out, the
    same class of local state as node.json and the node's store.
  - A wrong cache can come only from someone editing the node's own
    directory, and even then the release must carry the issuer's own
    signature over the bogus id. Nodes with the same information still
    reach the same answer.
  - Part ranges follow from the series, not from the release, so the
    worst such a node can do is take a part's mints before other nodes
    know that part is released. They would be waiting, not refusing.
- **RED-073.** A kept release that does not check is now left out, and
  the series is followed at part 1. Before this, the whole series was
  refused. Either way the node knows fewer parts, which is missing
  information.
- **RED-072.** Follow and unfollow now see the kept release, and
  unfollow keeps a variable series whole. This is node policy, about
  which pools a node follows.
- **RED-074.** `add-release` checks a release of a part the node already
  knows, and release JSON refuses unknown keys. This is tooling and file
  form; `AddRelease` still answers a known part with no hashing.
- **RED-076.** The series id is quoted in errors. This is output only.
- **ORANGE-035.**
  - One part chain is shared per series id across the process. The ids
    are the same whoever works them out, so this saves work only.
  - The variable-id set is now an atomic map. It holds the same facts.
  - Every release is still checked in full, or from the node's own cache.

## ORANGE-035's design: limits and releases applied in place

Orange's rule is the right one: **after an in-place change, every part of
the node holds what a reload onto the same node.json would give it, and
anything that cannot promise that falls back to the reload.** Black
accepts the design on that rule. Black's view of each setting:

- **`p2p.*` and `tracking.*`.** These are information, and Round 22
  cleared them at their floors and ceilings. Applying them in place is
  the same.
- **`seen_size`.** Keeping more than a reload would is fine. The seen list
  holds only valid items, a chase fetches with `Requested`, which skips
  it, and a re-sent item it drops is one the node already took. Either
  choice is acceptable.
- **`max_pending`.** Pending transactions are never part of a ledger.
  Dropping above the new limit is the same as a reload refilling from
  peers.
- **`ties`.** Acceptable in place. A lower value only forgets tied
  branches at the next prune, and a forgotten tie is taken again when it
  is sent and is heavier (Round 22's test). Pruning never drops the
  selected ledger. The selected tip must not change because of the
  setting, and the design's comparison test should check it.
- **Releases.** `AddRelease` only grows the count. Indexing the new pools
  in place (`Host.AddPools`) is the same as the host indexing them on
  start.
- **Not in this design, and kept on the reload unless Black sees them
  first:**
  - `k`;
  - `check_seconds`;
  - the chase queue and pacing;
  - anything a reload rebuilds that touches selection or chasing.

Ship it with Orange's comparison test (in place against reload, at the
floor and the ceiling, lowered and raised), and include the selected tip
and the followed pools in the comparison.

## Clear

| Item | Why it is clear |
| --- | --- |
| D166: a mint names its part; the tid covers it; fixed bytes unchanged | The approved change, as specified. Fixed forms identical across builds |
| D166: `CheckMint` (`ErrWrongPart`, `ErrNoSerial`, `ErrWaiting`) | Invalid only where every node agrees from the series alone. Otherwise the transaction waits |
| D166: unreleased pools are outside `PoolCount`; waiting is never kept | Missing information. No penalty and no lasting refusal |
| D166: keeping and adding releases (`AddRelease`, node.json, `add-release`, `POST /v1/series/release`) | One release kept. Each new release is checked in full, and the count only grows |
| D166: part 1 needs the issuer witness (`ErrNoPartOne`) | Unreadable, never invalid. The series is kept and not followed |
| D166: the read retry, `series info`, `peers -counts`, `series parts -o table`, `-check -` | Tools and output |
| BLUE-026 (RED-072, RED-073) | Which pools a node follows; missing information |
| BLUE-026 (RED-074, RED-076) | Tools, file form and output |
| BLUE-026 (RED-075) and ORANGE-035 | Same ids and the same checks, with less work. The cache is local state |
| GREEN-050: the followed table, release notes, the daemon waiting for its own lock | Output and local process order. Nothing decided differently |
| Red (PR #169): RED-072 to RED-076 | Findings, all fixed in PR #170. None touches a rule |

## Items for the owner

- **Nothing needs your approval.** Phase 2 is the change you approved,
  built to your documents and to Black's six requirements.
- **Fixed series can't break.** Their series, transactions and entries
  read and check exactly as before this change. Black compared the
  builds on 65,000 random cases.
- **No hard fork is possible.** A node that hasn't seen a release waits
  for it and never calls anything invalid because of it. Only a wrong
  part number or an impossible serial is invalid, and every node works
  that out from the series alone.
- Phase 3 passes releases between nodes. Black will check that a series
  is always built before its transactions are read, and that a release
  passed along is checked in full.

How this was checked:
- Read the documents again, D166 and D167, the series, tx and engine
  changes, `p2p.handlePushed` and `resolveBig`, and the diffs of PRs
  #167, #168, #169 and #170.
- The cross-version tests (appendix and ROUND-24) gave identical fixed
  results on `08404df`, `e93eb29` and `ad10c6b`.
- `go test ./...` passes on `e93eb29`, #167, #168 and #170. `pkg/series`,
  `pkg/tx`, `pkg/engine` and `pkg/nodepolicy` pass under `-race` on
  `ad10c6b`.

## Appendix: the transaction and entry cross-version test

Copy it into `pkg/entry` of each build, run it with
`BLACK_XV_OUT=out.txt go test ./pkg/entry -run TestBlackCrossVersionTxEntry`,
and compare the output files.

```go
package entry_test

import (
	"encoding/hex"
	"fmt"
	"math/rand"
	"os"
	"testing"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/entry"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/policy"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/series"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/testutil"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/tx"
)

func TestBlackCrossVersionTxEntry(t *testing.T) {
	out, err := os.Create(os.Getenv("BLACK_XV_OUT"))
	if err != nil {
		t.Skip("BLACK_XV_OUT not set")
	}
	defer out.Close()
	s := testutil.ExampleSeries()
	k := testutil.NewKeys(s)
	rng := rand.New(rand.NewSource(25))
	rh := func() (h hash.Hash) { rng.Read(h[:]); return }
	mk := func() *tx.Tx {
		if rng.Intn(3) == 0 {
			return k.Claim(uint64(rng.Intn(10)), testutil.P(fmt.Sprint("o", rng.Intn(4)), 100))
		}
		x := &tx.Tx{Mint: series.MintID{Series: s.ID(), Serial: uint64(rng.Intn(12))}}
		if rng.Intn(5) == 0 {
			x.Mint.Series = rh()
		}
		for i := rng.Intn(3); i > 0; i-- {
			x.Inputs = append(x.Inputs, tx.Input{TID: rh(), Index: uint32(rng.Intn(3))})
		}
		for i := rng.Intn(3); i > 0; i-- {
			x.Outputs = append(x.Outputs, tx.Output{Address: rh(), Amount: uint64(rng.Intn(120))})
		}
		for i := rng.Intn(3); i > 0; i-- {
			d := make([]byte, rng.Intn(70))
			rng.Read(d)
			x.Witnesses = append(x.Witnesses, policy.Witness{Policy: policy.Policy{Type: policy.TypeThreshold, Params: d[:rng.Intn(len(d)+1)]}, Data: d})
		}
		return x
	}
	errs := func(err error) string {
		if err == nil {
			return "ok"
		}
		return "err"
	}
	for i := 0; i < 20000; i++ {
		x := mk()
		b := x.Encode()
		tid := x.TID(s.Hash())
		line := fmt.Sprintf("t%d %s tid=%s form=%s stateless=%s", i, hex.EncodeToString(b), tid, errs(tx.CheckForm(s, x)), errs(tx.CheckStateless(s, x, tid)))
		if d, err := tx.Decode(b); err != nil {
			line += " dec=err"
		} else {
			line += " dec=" + hex.EncodeToString(d.Encode())
		}
		fmt.Fprintln(out, line)
	}
	for i := 0; i < 5000; i++ {
		e := &entry.Entry{Parent: s.PoolID(uint64(rng.Intn(3))), Nonce: uint64(rng.Intn(3))}
		if rng.Intn(4) == 0 {
			e.Parent = rh()
		}
		for j := rng.Intn(4); j > 0; j-- {
			e.Txs = append(e.Txs, mk())
		}
		b := e.Encode()
		pool := uint64(rng.Intn(3))
		h, _, err := entry.CheckStructure(s, pool, e)
		line := fmt.Sprintf("e%d %s hash=%s structure=%s/%s", i, hex.EncodeToString(b), e.Hash(s), errs(err), h)
		if d, err := entry.Decode(b); err != nil {
			line += " dec=err"
		} else {
			line += " dec=" + hex.EncodeToString(d.Encode())
		}
		fmt.Fprintln(out, line)
	}
	for i := 0; i < 20000; i++ {
		b := make([]byte, rng.Intn(200))
		rng.Read(b)
		if len(b) >= 32 && rng.Intn(2) == 0 {
			id := s.ID()
			copy(b, id[:])
		}
		line := fmt.Sprintf("b%d %s", i, hex.EncodeToString(b))
		if d, err := tx.Decode(b); err != nil {
			line += " tx=err"
		} else {
			line += " tx=" + hex.EncodeToString(d.Encode())
		}
		if d, err := entry.Decode(b); err != nil {
			line += " entry=err"
		} else {
			line += " entry=" + hex.EncodeToString(d.Encode())
		}
		fmt.Fprintln(out, line)
	}
}
```

Result: the three output files (45,000 lines each) are byte for byte the
same.
