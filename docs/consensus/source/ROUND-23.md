# Source Round 23: consensus review (Black, Part D)

## Summary

No change in this round adds or alters a validity, weight, selection or
byte-form rule, and no team proposed a consensus change, so **nothing is
passed to the owner for approval**. **Nothing is blocked.**

The round lets a node build on a parent other than its tip, so an owner
can make a rival claim of a mint the selected ledger has already claimed
(D161). It also adds a quicker pre-check for those builds (D162), an RPC
handshake, and more peer-book policy. Black looked hardest at three
places:

- **Building on a chosen parent (D161).** Any held entry was always a
  valid parent: a rival branch is what weight decides between. The node
  now lets its owner build one. The entry it makes goes through
  `HandleEntry` and `AddEntry` with every check, like an entry from a
  peer, and weight selects as before.
- **`ledger.Pool.CheckOn` and `CheckPendingOn` (D161, D162).** These are
  local pre-checks that choose which transactions go into the node's own
  entry. They decide nothing about what a node accepts. If either were
  wrong, the worst case is that the node grinds an entry that its own
  `AddEntry`, and every peer's, refuses. All nodes would refuse it the same
  way, so it can't split them.
- **The peer book (D162).** It decides which peers a node knows. A full
  book is a lack of information, not a hard fork.

The consensus packages that changed are these:
- `ledger` gained `branch.go`, which has read-only helpers: `ClaimEntry`, `ParentOf`, `CheckOn`, `CheckPendingOn`.
- `engine` gained `BuildEntryOn`, `ClaimParent` and `PoolID`.

Neither changes `AddEntry`, `AddLayer`, `prune`, weight or selection. `chain`, `entry`, `layer`, `weight`, `state`, `tx` and `wire` are untouched.

| Field | Value |
| --- | --- |
| Round | Source Round 23 |
| Consensus suggestions | None |
| Base reviewed | `main` @ `215d428` (PR #140: D161, with Round 22's PRs, #133, #134, #136, #138 and #139, merged) |
| Previous base | `main` @ `b997eb0` |
| Blue branches | PR #144 @ `eae4ecf` (BLUE-024, D162: RED-066 to RED-069; carries ORANGE-033) |
| Red | PR #141 @ `a609fc1` (RED-066 to RED-069; each says no consensus rule is touched) |
| Proposals | GREEN-048 (PR #142 @ `5669945`); ORANGE-033 (PR #143 @ `97f4d45`) |
| Result | 9 clear, 0 with conditions, 0 blocked |

## Rival claims and building on a chosen parent (D161)

- **The rule is unchanged.** A mint is claimed once in a ledger, and two
  ledgers that claim it differently are rival branches. The heavier one is
  selected (`TestDoubleSpendResolvedByWeight` has long covered this).
  D161 adds no way to make a branch valid that was not valid before. It
  only lets the node build such a branch: `engine.BuildOn{Parent, Txs}`.
- **The built entry is checked in full.**
  - `Node.BuildOn` passes the entry to `HandleEntry(..., Local)`.
  - `Local` only decides forwarding and the seen list. The entry runs
    through `AddEntry`, which does the same checks as for a peer's entry.
  - Peers check it the same way when it is forwarded.
  - `TestRivalClaimOnBranch` shows every node keeping the rival entry and
    selecting it once it is heavier.
- **Nothing changes what is pending.** The pending pool still refuses a
  claim of a mint the selected ledger has claimed. A rival claim is
  passed directly as `Txs`, with the parent the owner chose.
- **Pruning.** A lighter rival branch is pruned like any other when `k`
  is small. The `poo claim` hint says to keep `k` high enough meanwhile.
  This is the node's own information. A pruned branch can be given again,
  and is taken when it is heavier (as in Round 22's ties test).
- **`poo policy limits -set` and `POST /v1/policy/limits`.** These write
  D159's and D160's bounded settings, checked whole by `Validate`. Round
  22 found that none of them can refuse a heavier ledger, at its floor or
  its ceiling. This is a new way to set them, not new settings.

## `CheckOn` and `CheckPendingOn` (D161, D162, RED-068, RED-069)

Both take the state after the parent (the selected chain reverted to it,
then the branch applied) and keep the transactions that fit, in canonical
order. They refuse a second claim of a mint, an input spent twice, and an
input made in the same entry. They change nothing in the pool.

- **`CheckPendingOn` skips `tx.CheckStateless` and the witness check**
  for transactions the pending pool already took. Blue's argument holds:
  - A witness satisfies an address.
  - An input's owner is the address of the output it spends.
  - That output, if it exists at all after the parent, is the same output
    with the same address on every branch.
  - If it does not exist there, `tx.Owners` fails and the transaction is
    left out.

  So the kept set is the same as `CheckOn`'s, and
  `TestCheckPendingOnMatchesCheckOn` checks this.
- **Even if it were not, nothing in consensus would move.** The result
  only picks what goes into this node's own entry, and `AddEntry`, here
  and on every peer, checks that entry in full. A wrong pick would waste
  this node's grind on an entry every node refuses alike.
- **ORANGE-033** replaces a scan of the kept transactions with two sets:
  the mints claimed and the inputs spent. It refuses the same conflicts.
- **RED-069.** A transaction on another pool's mint is now refused with
  `entry.ErrWrongPool` before the grind. That is `AddEntry`'s own
  refusal, given earlier.

## The peer book and the RPC handshake (D162, RED-066, RED-067)

- **RED-066.** `Dial` sends the full token only after `GET /v1/hello`
  proves that the server knows it. This is a local channel between `poo`
  and the node, and it has no part in what a node accepts.
- **RED-067.**
  - The owner's own add may take the place of a learned record the node
    has reached, but never of a peer in use.
  - A host name counts in the network range it leads to (`Record.Via`).
  - A refused configured peer is logged.

  This is node policy. It decides whom a node knows and dials. At worst a
  node knows fewer peers, which is less information, not a different
  view of which ledger is valid.

## Clear

| Item | Why it is clear |
| --- | --- |
| D161: build on a chosen parent; rival claims; `ClaimEntry`, `ParentOf`, `PoolID` | The entry is checked by `AddEntry` like any other, and weight selects. Read-only helpers |
| D161: `poo policy limits -set`, `POST /v1/policy/limits` | Writes the bounded settings Round 22 cleared, checked whole |
| BLUE-024 (RED-066): the node proves itself before the token is sent | Local RPC only |
| BLUE-024 (RED-067): the owner's add finds a place; names count where they lead | Peer-book policy (information) |
| BLUE-024 (RED-068): `CheckPendingOn` | A local pre-check whose result equals `CheckOn`'s. `AddEntry` checks in full |
| BLUE-024 (RED-069): `ErrWrongPool` before the grind | `AddEntry`'s own refusal, given earlier |
| ORANGE-033: conflicts found by set lookup | Same refusals, faster |
| GREEN-048: next-step hints, `-bytes` with `-parent`, `-set` name hints | CLI text and flags |
| Red (PR #141): RED-066 to RED-069 | Findings only. Each is local, and each is fixed in PR #144 |

## Items for the owner

- **No consensus change is proposed this round.** Nothing needs your
  approval.
- **No hard-fork case is open.** Your rival-claim feature only lets a node
  build a branch that was always valid. Every node checks that branch the
  same way, and the heavier ledger is still selected.

How this was checked:
- Read the code changes from `b997eb0` to `215d428`, the diffs of PRs
  #141, #142, #143 and #144 against `main`, decisions D161 and D162,
  `ledger/branch.go` and the `Origin` handling in `engine.go`.
- `go test ./...` passes on `215d428`, `5669945`, `97f4d45` and
  `eae4ecf`.
- On `eae4ecf`, the `CheckOn`, `CheckPendingOn`, rival-claim and build-on
  tests in `pkg/ledger` and `pkg/engine` pass 3 times out of 3 under
  `-race`.
