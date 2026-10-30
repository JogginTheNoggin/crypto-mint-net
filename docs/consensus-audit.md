# Consensus audit

## Summary

This is a read-only audit, asked for by the owner on 2026-10-30, of every
rule in the code that decides whether a ledger, entry, layer, transaction,
pool, claim or weight is valid or selected. It covers the Source
integration line `claude/m10-watch` at `9972137` and the pending Blue
branch `claude/project-thread-ggo8dt` at `cf6a921` (BLUE-007, D114). Each
rule is checked against the design documents and `docs/decisions.md`, and
put in one of three classes:

- **(a) From the documents.** The rule is written in them.
- **(b) A necessary concretization.** The documents leave a detail open
  (an exact byte form, a number size, a bound every type must declare) and
  the code fills it in without adding a new idea.
- **(c) New.** Nothing in the documents asks for it.

Local node policy is kept apart. A **grey zone** item is node policy that
can still change which ledger a node ends up selecting, or makes a node
refuse data that is valid.

**Result.** The validity, weight and selection rules match the documents.
No security fix or Orange change added a new validity or selection rule:
D105, D107, D111 and D114 change only the order and cost of checks, and
`entry.CheckShape` and the ORANGE-004 preverify reject nothing that the
full check would accept. Red's suggestion that deep forks need extra work
is **not** in the code. Two hard size limits have no basis in the
documents and act like consensus (C1, C2). Four grey-zone items can stop a
node from reaching a heavier valid ledger or make it refuse valid data
(G1 to G4). G1, the revert budget, matters most: with BLUE-007 merged it
works as a depth limit on switching, which the main document forbids. G4
looks like a security gap and is worth a Red check.

Nothing was changed or removed. The owner decides each item.

## Items for the owner

### (c) New rules

#### C1. A cap on the size of an entry

- **What it does.** An entry holding more than 1,048,576 transactions
  cannot be decoded, so it is refused. Separately, every message between
  nodes is at most 16 MiB, so an entry whose byte form is larger cannot be
  received from any peer. The store's record limit is 64 MiB.
- **Where.** `pkg/entry/entry.go:33` (`MaxTxs`), `pkg/wire/wire.go:44`
  (`MaxFrame`, D30, D37), `pkg/store/store.go:68` (`MaxRecordSize`).
- **Why it was added.** To stop one message from making a node allocate
  without limit. The code calls `MaxTxs` a decoding bound and says it is
  not a consensus limit.
- **Basis in the documents.** None. Section 8 of the main document says
  "The design sets no limit on how many transactions an entry holds", and
  section 24 lists a series limit as an open question.
- **Why it is a rule in practice.** Every node running this code refuses
  such an entry, and another implementation that accepted it would select
  a different ledger. No honest builder makes an entry this large today.
- **If removed.** One peer could make a node allocate gigabytes from one
  message. Removing it safely needs a decoder and wire format that stream
  an entry in parts. The other choice is to keep a limit and write it into
  the documents, or make it a series setting.

#### C2. A cap on the inputs and outputs of a transaction

- **What it does.** A transaction with more than 65,536 inputs or 65,536
  outputs (or more than 131,072 witnesses) cannot be decoded, so it is
  refused, and so is any entry holding it.
- **Where.** `pkg/tx/tx.go:37-38` (`MaxInputs`, `MaxOutputs`), in
  `DecodeFrom`.
- **Why it was added.** The same decoding protection as C1.
- **Basis in the documents.** None. Section 7 sets no number of inputs or
  outputs.
- **If removed.** As C1: unbounded allocation from one message. The
  16 MiB message limit would still cap a transaction at about 400,000
  outputs.

### Grey zone

#### G1. The revert budget works as a depth limit on switching

- **What it does.** Checking an entry whose parent is below the selected
  tip makes the node undo its ledger back to that parent. The node
  "charges" the sender for it: it counts that work against a per-address
  allowance, the revert budget, of 200,000 units a minute, where a unit is
  one entry undone or applied plus one per transaction in it. "Charge"
  means only this count against an allowance; no money or units move.
  Over the allowance the entry is dropped and the address gets one flag
  per minute. With D111 (on `claude/m10-watch`) this applies to pushed
  entries. BLUE-007 (D114) applies it to entries fetched in a chase too.
- **Where.** `pkg/p2p/chase.go:546` (`spendRevert`), `pkg/p2p/forward.go`
  (pushed entries), `pkg/tracking/tracking.go:225` (the default),
  `pkg/engine/engine.go` (`EntryCost`). D111, D114.
- **Why it was added.** RED-011 and RED-012: an entry with no work on an
  old parent made the node undo its whole selected chain under its lock,
  over and over, for free.
- **The effect on selection.** The first entry of a heavier branch costs
  the whole undo back to the fork. If that one entry costs more than the
  whole minute's allowance, every peer is refused every time, so the node
  can never take in the branch and never switches to it. With the
  defaults that is a fork more than about 200,000 empty entries, or about
  2,000 entries of 100 transactions, below the tip. Section 10.2 of the
  main document says "There is no depth after which a node refuses to
  switch". This was found by reading the code and has not been
  reproduced.
- **If removed.** The RED-011 attack comes back: about 60 ms of locked
  undo per entry at 40,000 entries, repeatable by anyone. A middle way
  would be to stop charging chased entries once the chase has checked the
  branch's work (step 4 of the chasing document), or to let one spend
  larger than the allowance through once per window.

#### G2. A chase stops after 4,096 entries in a row without layers

- **What it does.** In a series without entry hash credit, a fetch ends
  after more than 4,096 entries in a row that carry no layers. A valid,
  heavier ledger whose work all sits on later entries (allowed, since a
  layer on a later entry counts for every entry before it) cannot be
  fetched past such a run.
- **Where.** `pkg/p2p/chase.go` (`fetch`, `errBareRun`),
  `pkg/p2p/host.go:239` (`MaxBareRun`). D114, BLUE-007 only.
- **Why it was added.** RED-012: a peer could feed a node that holds
  nothing an endless branch of empty entries for free.
- **Basis in the documents.** Section 17.4 makes keeping entries with no
  layers node policy, but no document limits a run of them in a chase.
- **If removed.** That flood comes back for a node catching up from
  nothing. The chase already checks provisional layers before fetching,
  so a limit tied to that check might be enough.

#### G3. The chase depth limit and the main document disagree

- **What it does.** A chase pages back at most 16 hash chain pages
  (about 4,096 entries with the default page) to find the common entry.
  A deeper split is given up and the sender backed off, unless
  `FastCatchUp` is on (off by default) and the sender is a priority peer.
- **Where.** `pkg/p2p/host.go:237` (`DepthLimit`), D54, D62.
- **Basis in the documents.** The chasing document asks for exactly this
  depth limit as node policy. The main document says a node has no depth
  limit on switching. The code follows the chasing document. The two
  documents need one answer.
- **If removed.** A peer could send endless made-up hash pages.

#### G4. A peer can put a valid entry on the known-bad list

- **What it does.** When a chase sees provably false data, it puts entry
  hashes on a known-bad list for 24 hours. After that the node drops
  pushed entries with that hash or built on it, and pushed layers on it.
  Three cases list a hash that can belong to a perfectly valid entry: an
  entry sent out of chain order (`does not link`), an entry that is not
  the hash the peer listed (both hashes are listed), and a layer below
  the minimum strength sent with a valid entry (the entry's hash is
  listed, even when it is the node's own tip).
- **Where.** `pkg/p2p/chase.go:507`, `:511`, `:583`;
  `pkg/p2p/forward.go:103`, `:131`, `:153`; `pkg/chase/chase.go:93`.
  D52, D53.
- **Basis in the documents.** The chasing document asks for the list. It
  does not say which hash goes on it when a reply is out of order or a
  layer is weak.
- **The effect.** One peer could make a node ignore new layers on, and
  new entries after, a valid entry of the honest ledger for a day. Chases
  and requested data still work, so the node is slowed, not stopped.
  Found by reading the code and not reproduced. Worth a Red check in
  Round 7 Part B.
- **If removed.** False branches would be fetched again. Listing only the
  hash that is shown false (the bad layer itself, or the sender's listed
  hash) would keep the protection.

#### G5. The main document puts forwarding in consensus

The main document's table in section 17.8 places "forwarding every
transaction, entry and layer a node has checked" in the consensus tier.
The chasing, ledger data and forwarding documents make requested data
never forwarded, and let budgets, the seen list and the known-bad list
drop data unchecked. The code follows those later documents (D15, D27,
D36, D55, D111). This is a disagreement between documents, not a new
rule.

#### G6. Other node policy that changes what a node holds

All of these are asked for in the documents as node policy, and none
changes how a node selects from what it holds:

- Pruning to the top K = 8 branches, 64 tie branches, and keeping the
  newest entry while it is a leaf (D12, D74, D79). Section 17.7 accepts
  that this can make nodes differ until they see a dropped ledger again.
- The seen list: a pruned entry pushed again is dropped as seen, and only
  requested data gets past it (D15, D27).
- The pending pool: the first of two conflicting pending transactions
  wins, a spend of a pending output is refused, at most 10,000 pending per
  pool (D12, D13, D72), as the forwarding document says.
- Accepting any issuer until a project issuer exists (D24).
- A minimum strength for keeping layers, if one is added as node policy
  (Source ORANGE-010). Section 17.4 allows it. A node that drops weak
  layers computes a lower weight than its peers and can select a
  different ledger, so it belongs in this list.

#### Checked and clean

- **`ErrDeepParent` (D111).** Only the preview before the lock refuses a
  deep parent. `AddEntry` still takes the entry and checks it in full.
- **ORANGE-004 preverify and `entry.CheckShape` (D105, D107).** They run
  the same checks as `AddEntry` and `mempool.Add`, in the same order, and
  only decide when signatures are checked. An item they stop is one the
  full check refuses too.
- **The witness cache (D105).** It skips a satisfaction check only for a
  witness already satisfied for the same policy, data and tid.
- **A bad witness is an offence (D107).** Peer scoring only.
- **BLUE-007's kept state (D114).** It keeps the state after the last
  entry so the next entry on it starts there, and drops it on any switch
  or prune. Validity and selection are unchanged.
- **No work gate on deep forks.** Red's suggestion is not implemented
  (D111 leaves it open for the owner).

## Every rule, by class

### Transactions

| Rule | Class | Where | Note |
|---|---|---|---|
| One mint per transaction; mint in the series, serial below N | (a) | `tx.CheckForm` | Section 11 |
| At least one output; amounts at least 1; no two outputs to one address | (a) | `tx.CheckForm` | Section 7, 11 |
| A claim has no inputs, outputs add up to S, one witness per output | (a) | `tx.CheckForm`, `CheckStateless` | Section 12 |
| A claim only on an unclaimed mint | (a) | `tx.Owners` | |
| A spend: mint claimed, inputs unspent and of this mint, none repeated | (a) | `tx.Owners`, `CheckForm` | |
| Outputs add up to the inputs | (a) | `tx.Owners` | |
| One witness per input, in order, each satisfying its portion's address | (a) | `tx.CheckAgainst` | Section 6.4 |
| Amounts and supply are 64-bit; a sum that overflows is refused | (b) | `tx.OutputSum` | No valid transaction overflows, since S fits |
| tid = H("PoO-tx" ∥ body), witnesses outside | (b) | `tx.TID`, D3 | The document's formula has no tag; the byte form is the series' to fix |
| Input index is 32 bits | (b) | D2 | |
| At most 65,536 inputs and outputs | **(c)** | C2 above | |

### Witnesses and policies

| Rule | Class | Where | Note |
|---|---|---|---|
| Policy hashes to the address, type and algorithms allowed, then satisfy | (a) | `policy.Registry.Check` | Section 6.4, cheap check first |
| Signed message is "PoO-witness-v1" ∥ tid | (a) | D9 | Section 6.3 |
| Witness size at most the series' ceiling, measured as encoded | (b) | D19 | |
| Threshold: 1 to 16 distinct keys, exactly K signatures, increasing indices | (b) | D7 | A declared type bound and one form per witness |
| Hashlock: preimage at most 1,024 bytes; k-of-policies: 1 to 16 parts, nested at most 4 deep | (b) | D82 | Type bounds |
| Public keys at most 4,096 bytes, signatures 8,192; params and data at most 64 KiB each | (b) | D81, `pkg/policy/policy.go` | Bounds every type and algorithm must declare (section 6.2); they hold even where a series sets no ceiling |
| ECDSA signatures in the lower half of s only | (b) | D81 | Section 6.3 asks for one encoding per signature |
| Algorithm numbers 1 to 9, hash numbers 1 to 5, policy types 1 to 3 | (b) | D7, D80, D81, D82 | Entries in the number lists, which the design lets grow |

### Entries

| Rule | Class | Where | Note |
|---|---|---|---|
| Hash = H("PoO-entry" ∥ parent ∥ tids ∥ nonce) | (a) | `entry.HashOf` | |
| Nonce only when a minimum entry strength is set; else it must be 0 on the wire | (b) | D5 | The document says the nonce is absent |
| Nonce is 8 bytes | (b) | D5 | |
| Every transaction on a mint of the entry's pool and series | (a) | `entry.CheckShape` | |
| Canonical order: by serial, then tid; strictly increasing | (a) | `entry.CheckShape`, D18 | Duplicates fail as out of order |
| Strength at least the minimum entry strength | (a) | `entry.CheckShape` | |
| Parent is the pool_id or a held valid entry of the pool | (a) | `ledger.AddEntry` | |
| Each transaction valid after the parent; no conflicts; no spend of a bundle-mate | (a) | `entry.CheckAgainst` | |
| An entry with no transactions is valid | (b) | D16 | The documents allow any number |
| At most 1,048,576 transactions, and a byte form within 16 MiB | **(c)** | C1 above | |

### Layers, weight and selection

| Rule | Class | Where | Note |
|---|---|---|---|
| Layer hash = H(entry hash ∥ nonce), no tag | (a) | D6 | |
| Layer nonce is 8 bytes | (b) | D6 | Section 24 leaves the format open |
| Below the minimum layer strength: not counted | (a) | `AddLayerHashed` | |
| Counted once by layer hash; only on a held entry | (a) | `AddLayerHashed` | |
| Credit 2^strength, exact big integers | (a) | D10 | |
| Entry hash credit where the series counts it | (a) | `AddEntry` | |
| Weight = sum of own weights; select the heaviest; switch only when strictly heavier | (a) | `pkg/ledger`, D11 | |
| Equal weights: keep the first selected; among new candidates, the first seen | (a) | D11 | Section 10.3 |
| Displaced transactions: unchanged, carried or void, in old order | (a) | D14 | Section 13 |

### Series and pools

| Rule | Class | Where | Note |
|---|---|---|---|
| N at least 1, S at least 1, P from 1 to N | (a) | `series.validate` | |
| Pool of serial s is s / P; pool_id = H("PoO-pool" ∥ series_id ∥ k as 8 bytes) | (a) | `series.PoolID` | |
| series_id = H("PoO-series" ∥ params), params start with format version and hash number; issuer witness outside | (b) | D3, D4 | Section 16.2 |
| At least one policy type and one algorithm; lists sorted, no repeats; at most 1,024 each | (b) | `series.validate`, `DecodeParams` | One byte form per series |
| Strength minimums at most 256 | (b) | `series.validate` | A 32-byte hash has 256 bits |
| A number this node lacks, or format version other than 1, makes the series unreadable | (a) | `series.New` | Section 16.7 |
| The issuer witness signs H("PoO-issuer-v1" ∥ series_id) | (b) | D8 | |

## How this was checked

The design documents were read as they are in the project files
(`Proof of Ownership`, `Proof of Ownership Pools and Portions`, `Linked
Ledger Transactions and Entries`, `Ledger Data`, `Chasing Heavier
Ledgers`, `Forwarding`, `Node Policy` and the rest), without changing
them. Where they disagree the code follows the main document (D18), except
for forwarding (G5) and the chase depth limit (G3). The code read was
`pkg/tx`, `pkg/entry`, `pkg/layer`, `pkg/ledger`, `pkg/state`,
`pkg/series`, `pkg/policy`, `pkg/hash`, `pkg/weight`, `pkg/encoding`,
`pkg/engine`, `pkg/mempool`, and the parts of `pkg/p2p`, `pkg/chase`,
`pkg/tracking` and `pkg/wire` that refuse or drop data. Every decision from
D1 to D114 was read. G1 and G4 come from reading the code; no test
reproduces them yet.
