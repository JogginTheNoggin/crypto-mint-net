# Source Round 19: consensus review (Black, Part D)

## Summary

No change in this round adds or alters a validity, weight, selection or
byte-form rule, and no team proposed a consensus change, so **nothing is
passed to the owner for approval**. **Nothing is blocked.**

This is the first round reviewed under the owner's definition of a hard
fork (2026-11-03, AGENTS.md): nodes that have the same information
cannot agree which ledger is valid or heavier. A lack of information,
or slowness, is not one. The round has three things that make a node
learn of a ledger later or not at all. None of them changes what a node
accepts once it has the ledger, so all three are clear:
- the paced sweep (ORANGE-029);
- unused pools of a huge whole series (D154);
- excepted pools announced as the whole series (D154).

| Field | Value |
| --- | --- |
| Round | Source Round 19 |
| Consensus suggestions | None |
| Base reviewed | `main` @ `4240d32` (PR #98: D153) |
| Previous base | `main` @ `533dbcd` |
| Blue branches | PR #105 @ `25f72cd` (BLUE-020, D154: RED-049, RED-050, RED-051) |
| Red findings | PR #103 @ `e4057cf` (RED-049 to RED-051) |
| Proposals | GREEN-044 (PR #102 @ `774ffc2`); ORANGE-029 (PR #104 @ `172866d`, the paced weight sweep) |
| Result | 8 clear, 0 with conditions, 0 blocked |

## Round 18's items

| Item | Now |
| --- | --- |
| B7, the weight check that could be turned off | An information gap, not a hard fork (the owner, 2026-11-03). Improved anyway by D153 |
| The owner's hard-fork definition | In AGENTS.md and the consensus checklist (PR #96) |
| The pool index on the wire | Not built; the owner has not decided (D153, D154) |

## D153: the weight check cannot be turned off

`check_seconds` below 0 or over `MaxCheckSeconds` (3600) is read as one
hour, both by `Load` (noted in `Lowered`) and by `p2p.FromPolicy`. The
check only asks for stated weight. A heavier stated weight goes to the
chase queue as before, and nothing is refused. Clear. Black's two
passing B7 controls are kept in `check_never_off_test.go`. The failing
one could not wait an hour, and `TestCheckSecondsNeverOff` checks the
clamp instead. That is a fair substitute.

## Red's question: lazy state must not change what is accepted (D154, RED-049)

**Confirmed.**
- **A fresh pool state is decided by its series and index alone.**
  `poolFor` builds it from `ledger.New(s, k.Index, cfg)` and an empty
  pending pool. `poolState` holds only those and the series, so a state
  made, dropped and made again is the same state. Every item meets the
  same state it would have met before.
- **`handling` keeps a new state only if the item left something in it.**
  That means an entry, or a pending or waiting transaction. A refused
  item leaves nothing, so dropping the state loses nothing a later item
  could need.
- **Reads (`peek`) never keep a state.** That covers status, tips,
  payments, holdings, the ledger callback, and the preverify and cost
  hints. Black checked every `Engine.Ledger` caller in `pkg/node`,
  `pkg/watch`, `pkg/grind` and `pkg/p2p`: each only reads. On a fresh
  state, preverify work is wasted rather than kept, and `HandleEntry`
  still checks every entry in full.

**The p2p index of a huge whole series is an information gap, not a hard
fork.** For a series of `engine.ListAll` (65,536) pools or more followed
whole, the host indexes a pool once it is in use. That is when it has
state, an item is kept in it, or a branch is offered. Until then:
- **What is dropped.** A notice, a stated weight or an empty entry for
  that pool is dropped, because the wire names the pool only by its id
  hash. A pushed transaction, or an entry carrying one, is placed by its
  first mint serial, which is checked against the section's pool id.
- **What is not asked or answered.** The weight check does not ask about
  the pool, and requests for it go unanswered (the node holds nothing
  there).

The node has nothing in such a pool. The moment it is given an item
there, the pool is indexed and the item checked as any other. Nodes with
the same ledgers still agree. A pool index on the wire would close the
gap. That is the protocol feature from Round 18, and it is the owner's
call.

## Red's question: RED-050 and the no-caps rule (D154)

**Clear: no cap on what can be followed.**
- **What is limited.** `MaxSections` (4096, the wire's coverage limit)
  counts the separate sections a follow list announces, not pools,
  entries, sizes or weight.
- **Any set of pools can still be followed.** A whole series is one
  section, and `Except` takes pools off it. The refusal message says so.
- **An issuer with more than 4096 series is not a cap either.** Followed
  by issuer, it would be refused, and its series can be followed one by
  one. Either way it is the node's choice of what to follow, not a rule
  within a pool, and no node holding a ledger disagrees on it.
- **The owner's no-caps rulings stay untouched.** C1 and C2 concern entry
  and transaction sizes, and no such limit is added.

**Excepted pools are announced as part of the whole series** (Blue's
point 3).
- Peers may push to this node, or ask it, about a pool it does not
  follow. It drops those pushes (`ErrNotFollowed`) and answers "not
  held", as for any pool it does not follow.
- Those peers learn the ledger from their other peers. This affects
  which peer a node relies on, which is Red's area (peer steering), not
  consensus.

## ORANGE-029: the paced weight sweep

**Clear: it only makes weights arrive later.**
- **Every pool is still asked.** `sweepWeights` asks each pool a peer
  covers, in order, at most 8 at a time and within half the request
  pace. It waits for room and never skips a pool.
- **A sweep always finishes.** While one runs, a new one to that peer is
  not started. The next check starts a full one again, so the tail of a
  long list is not starved.
- **A heavier stated weight is chased as before.** The sweep changes
  when the node asks, not what it does with the answer. With
  `RequestPace` off, there is no wait at all.
- **The worst case is slowness.** If chases use the whole pace, the
  sweep waits for them. Under the owner's definition that is not a hard
  fork.
- **D154 and ORANGE-029 together.** Black merged PR #104 into PR #105
  and ran the engine, node, node policy, p2p and vector tests. They pass,
  and the merge is automatic (`host.go` only). Together they sweep the
  indexed pools.

## Clear

| Item | Why it is clear |
| --- | --- |
| D153: the weight check cannot be turned off | Asks for weight more, never refuses (see above) |
| D153: GREEN-043, ORANGE-028, BLUE-019 merged | Cleared in Round 18 |
| D154 (RED-049): lazy pool state; the indexed huge series | The same state for every item; the rest is an information gap (see above) |
| D154 (RED-050): `Except`, joined follows, `MaxSections` | Which pools a node follows, written in at most 4096 sections; any set still followable (see above) |
| D154 (RED-051): the digest binds the options and plans | The CLI's import only |
| ORANGE-029: the paced sweep | Later weights, every pool still asked (see above) |
| GREEN-044: help, completion and doctor's fix text | Output only |
| Red PR #103 | The findings. Their fix is BLUE-020 |

## Items for the owner

- **No consensus change is proposed this round.** Nothing needs your
  approval.
- **No hard-fork case is open.** This round's slower or missing
  information never makes nodes holding the same ledgers disagree.
- **The pool index on the wire** would close the gap for unused pools of
  a huge series. It is still waiting for your decision, as a protocol
  feature.

How this was checked:
- Read the code changes from `533dbcd` to `4240d32` (PR #98), the diffs
  of PRs #102 to #105 against `main`, and decisions D153 and D154.
- `go test ./pkg/engine ./pkg/node ./pkg/nodepolicy ./pkg/cli ./pkg/p2p
  ./pkg/vectors` passes on `774ffc2`, `172866d` and `25f72cd`. The same
  set without `cli` passes on #105 and #104 merged together.
