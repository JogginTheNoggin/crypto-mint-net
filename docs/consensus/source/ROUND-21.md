# Source Round 21: consensus review (Black, Part D)

## Summary

No change in this round adds or alters a validity, weight, selection or
byte-form rule, and no team proposed a consensus change, so **nothing is
passed to the owner for approval**. **Nothing is blocked** under the
owner's hard-fork definition (AGENTS.md).

The round is the owner's peer feature (D157) and Red's findings on it
(D158): adding peers by hand, labels, filters, and exporting and
importing the peer book. Two rules settle it:
- The peer book, its labels and its blocks decide only whom a node
  talks to.
- Under the owner's definition, a node that does not hear from a peer
  lacks information. That is not a hard fork.

Every ledger a node is given is still checked and selected as before.

| Field | Value |
| --- | --- |
| Round | Source Round 21 |
| Consensus suggestions | None |
| Base reviewed | `main` @ `8662766` (PR #122: D157, with #111, #112, #114, #116 and #117 merged) |
| Previous base | `main` @ `522e891` |
| Blue branches | PR #127 @ `34356d0` (BLUE-022, D158: RED-058 to RED-062) |
| Red findings | PR #125 @ `2c0deba` (RED-058 to RED-062) |
| Proposals | GREEN-046 (PR #124 @ `d282a4b`); ORANGE-031 (PR #126 @ `5b70f63`) |
| Result | 8 clear, 0 with conditions, 0 blocked |

## Round 20's items

| Item | Now |
| --- | --- |
| BLUE-021 (D156), GREEN-045, ORANGE-030 | Merged as cleared. The code on `main` matches what Black reviewed |
| The pool index on the wire | Not built; the owner has not decided |

## D157: the peer book by hand, filters, export and import

None of this reaches the engine.
- `AddPeer`, `LabelPeer`, `ConnectImported` and `Book.PromoteImported`
  change which addresses the connection manager dials.
- The filters only select what `poo peers` and `poo tracking` print.
- An import:
  - adds addresses as `unlisted`;
  - takes blocks only with `-blocks`, never on an address the node
    policy lists or the node's own;
  - never unblocks or removes anything.

**The worst case is isolation.** A shared file taken with `-blocks` could
block the honest peers a node would otherwise meet. Peers listed in
`p2p.peers` are still never blocked, and any peer that connects and
offers a heavier ledger is still chased and taken. A node cut off this
way lacks information. That is a peer-steering risk for Red, already
fenced by the opt-in flag and the plan shown first, and not a hard fork.

`Learnable` is the existing D96 check (`learnable`) made public for the
import. Clear.

## RED-058: imported records take at most a quarter of the book (D158)

Blue's question 1:
- **The share.** Records known only from an imported file are held to
  `MaxUnlisted`: a quarter of `MaxRecords`, so 1,024 by default.
- **Eviction.** An address learned any other way evicts the oldest
  imported-only record when the book or its range is full.
  `evictImportedLocked` never touches a record the node learned, tried
  or verified.
- **What it is.** Local peer-book policy. It limits which candidates the
  book keeps, not what a node can follow or which ledger it selects.

If anything, it keeps the node's own discovery from being crowded out by
a file. Clear.

## RED-061: the text check takes format characters too (D158)

Blue's question 2. `pkg/plain.Text` now refuses Unicode format
characters (Cf, such as zero-width joiners and direction marks) as well
as control characters. It is the same check, applied to node-policy text
and peer labels.
- **An old node.json can stop loading.** One holding such a character no
  longer loads. That is the operator's own file, and it refuses no
  ledger.
- **No series is touched.** As in Round 20, `series.Params` holds no free
  text, so no valid series is refused.

Clear.

## Clear

| Item | Why it is clear |
| --- | --- |
| D157: peers by hand, labels, filters, export and import, dialling imported addresses | Whom a node talks to, never what it accepts (see above) |
| D158 (RED-058): imported records capped at a quarter of the book, evicted first | Local peer-book policy (see above) |
| D158 (RED-059, RED-060, RED-062): plan totals, host-name range keys, digest-bound import | Peer import and the CLI only |
| D158 (RED-061): `pkg/plain`, format characters refused | Node configuration and labels only. No series parameter is text |
| GREEN-046: the offline label check first; zero times read "-" | CLI only |
| ORANGE-031: peer imports at their limits | Peer book and tracker costs only |
| Red PR #125 | The findings. Their fix is BLUE-022 |
| Merged from Round 20 (#111, #112, #114, #116, #117) | Cleared in Round 20 |

## Items for the owner

- **No consensus change is proposed this round.** Nothing needs your
  approval.
- **No hard-fork case is open.** Your peer feature changes only whom a
  node talks to. Every ledger a node is given is still checked and taken
  as before.

How this was checked:
- Read the code changes from `522e891` to `8662766` (PR #122), the diffs
  of PRs #124 to #127 against `main`, and decisions D157 and D158.
- `go test ./pkg/peers ./pkg/tracking ./pkg/peerport ./pkg/nodepolicy
  ./pkg/cli ./pkg/rpc ./pkg/p2p ./pkg/vectors` passes on `d282a4b`,
  `5b70f63` and `34356d0`, and `./pkg/plain` passes on `34356d0`.
