# Source Round 26: consensus review (Black, Part D)

## Summary

This round has four pieces:

- **Phase 3 of variable supply.** Releases now pass between nodes, as
  wire message type 14.
- **Limits applied in place.** This builds ORANGE-035's design, which
  Black accepted in Round 25.
- **SLH-DSA-SHA2-128s as key algorithm 10.** The owner asked for it
  this round.
- **Work tries sized from the series floor.**

Blue's PR adds a spendability check for policies, and its release queue
holds a burst of releases briefly.

**Result: 12 clear, 0 with conditions, 0 blocked.**

- Nothing here adds a validity rule outside the documents.
- The new algorithm is the one the owner asked for, and only a series that
  lists it can use it.
- No other consensus change is proposed.

**One question goes to the owner:** whether to allow the larger SLH-DSA
sets, which needs a decode limit raised. Black's view is below.

The coordinator asked whether **`CheckSpendable`** is a consensus rule.
**It is not.** It is a wallet check, called only where an address is made
or paid. No witness check calls it, and it cannot refuse a spend that
would be valid (see below).

| Field | Value |
| --- | --- |
| Round | Source Round 26 |
| Consensus | No new rule. Phase 3 is part of the approved variable supply change; SLH-DSA-SHA2-128s is the owner's request (2026-11-04) and applies only to series that list it |
| Other consensus suggestions | None |
| Base reviewed | `main` @ `439b369` (PR #175: D168, with Round 25's PRs merged) |
| Previous base | `main` @ `e93eb29` |
| Blue branches | PR #179 @ `fb8a8f3` (BLUE-027, D169: RED-077 to RED-081; carries ORANGE-036) |
| Red | PR #177 @ `8b0e0e3` (RED-078 to RED-081) |
| Proposals | ORANGE-036 (PR #178 @ `c58de47`); GREEN-051 (PR #176 @ `ab1b01b`) |
| Result | 12 clear, 0 with conditions, 0 blocked |

## Fixed series, transactions and entries: unchanged again

Black reran both cross-version tests, from [ROUND-24](ROUND-24.md) and
[ROUND-25](ROUND-25.md), on `main` (`439b369`) and on PR #179 (`fb8a8f3`).
Both runs were byte for byte the same as the earlier builds. That covers
20,000 fixed series, 20,000 transactions, 5,000 entries and 20,000 random
byte strings.

## Phase 3: releases between nodes (D168, BLUE-027)

- **Old nodes are unaffected.** A node from before this change ignores
  message type 14, as it ignores every type it does not know
  (`dispatch`: "Unknown types are ignored"). So nothing changes for it,
  and the protocol stays version 1. The variable supply document says
  nodes pass releases on to their peers.
- **A release is information.**
  - A node that has not received a release only waits for that part
    (D166).
  - Receiving a release sooner or later changes when a node knows a part,
    never what it accepts.
  - Every node holding the same series and the same release has the same
    parts.
- **What a node does with a release.**
  - It drops one for a series it does not follow, or for a part it
    already knows, without hashing.
  - For a later part, it checks the witness on arrival
    (`CheckReleaseSigned`, BLUE-027). It then works out the chain in
    full (`CheckRelease`) before the release is taken.
  - It treats a release that fails as an offence. That is right, because
    a release's validity depends only on the series and its own bytes,
    so no honest node passes on a bad one.
- **RED-078: the release queue.**
  - The first release of a series is taken at once.
  - For 2 s after that, only the highest release is held, and it is then
    taken in full.
  - A held part is a part not yet known, which is the same as waiting.
  - Nothing is refused that would otherwise be taken, and the release
    finally taken is checked exactly as before.
- **The cache of checked releases (ORANGE-036, `verified`).** It is keyed
  by series id, registry and the release's exact bytes. It holds only
  releases that checked, so the same bytes give the same answer and a
  forged release is always checked in full.
- **A series is built before its transactions are read.** This is
  Black's Round 25 point. A release only names a series the node built
  at start, and it does not change how a transaction is decoded.
- **New parts applied in place.**
  - `Host.AddPools` indexes the new pools the way a node does at start.
  - Where it can't promise that, the node reloads onto the saved
    node.json instead.
  - `TestReleaseInPlaceMatchesReopen` compares the two paths.

## Limits in place (D168, on Black's Round 25 terms)

- **Which limits apply in place.** These are the book and tracker limits
  that Round 22 cleared at their floors and ceilings, plus `seen_size`.
  `seen_size` is given a new, empty seen list, exactly as a reload gives
  one.
- **Which stay on the reload.** `verified_per_range`, `registry_seconds`,
  `max_pending` and `ties` stay on the reload until each has its own
  tested in-place path. `k`, `check_seconds` and chase pacing are not
  settable limits and are untouched, as Black asked.
- **The tests cover what Black asked for.** `TestLimitsSetInPlace` checks
  that the selected tips and the followed pools do not change, at the
  floors and ceilings, lowered and raised. RED-080's fix adds the grind
  schedule and manual peers to that comparison.
- **Tracker trimming.** Records of the same age are now trimmed in
  address order, so the same records trim the same way every time. That
  is about peers, not ledgers.

## SLH-DSA-SHA2-128s, key algorithm 10

- **The owner asked for it.** It applies only to a series whose
  `key_algs` lists algorithm 10.
  - That value is inside the series_id, so no existing series changes.
  - A node from before this change cannot build such a series and calls
    it unreadable, never invalid.
  - The fixed cross-version test (key algorithms 0, 1, 2 and 9) is
    unchanged.
- **Verification is deterministic.**
  - It is FIPS 205 in its pure form, with an empty context.
  - The key and signature lengths are checked exactly (32 and 7,856
    bytes).
  - The known answers in `pkg/policy/testdata/known.json` pin it, so any
    other implementation must give the same results.
  - Signing is randomised, which is fine: a witness sits outside the tid
    and outside a part id, so any valid signature gives the same
    transaction or part.
  - SLH-DSA is stateless, so it meets the documents' requirement that a
    signer's algorithm allows many signatures.
- **Sizes.** A 7,856-byte signature fits `MaxSigSize` (8,192). Up to 8
  of them fit in one witness, within `MaxDataSize` (64 KiB).

### For the owner: the larger SLH-DSA sets need `MaxSigSize` raised

SLH-DSA-SHA2-192s signatures are 16,224 bytes and 256s signatures are
29,792 bytes, both over `MaxSigSize` (8,192). Every node applies that
limit when it reads a witness, so raising it is a change to what nodes
decode. **Black's view: it can be done without a hard fork, but it needs
your permission, and it should be done in a specific way.**

- **Why no node would disagree.**
  - Every algorithm today has a fixed signature size below 8,192 bytes,
    and the length is checked when a signature is verified.
  - So a witness with a longer signature is refused today when it is
    read, and after the change it would be refused when it is checked.
    No series a node can read today could hold such a witness as valid.
  - A series that lists a new, larger algorithm is unreadable at older
    nodes, so they never decide anything about it.
  - Nodes that read the same series therefore reach the same answer.
- **Black's conditions, if you want it:**
  1. Raise the limit in the same release that registers the new
     algorithm numbers. Keep the exact per-algorithm size check.
  2. Run a cross-version test on witnesses: every witness for every
     existing series must decide the same before and after.
  3. Raise `MaxDataSize` too, or a 256s policy is limited to 2
     signatures. At 64 KiB a witness holds only 2 of them, and a 3-of-n
     policy could never be spent. `CheckSpendable` would warn about it,
     but the address would still be unusable.
- **Black's recommendation:** if you want a larger margin than 128s,
  192s (2^192 security) needs `MaxSigSize` at 16,384 and leaves room for
  4 signatures per witness. Consider 256s only if you also raise
  `MaxDataSize`.

## `CheckSpendable` is not consensus (BLUE-027: RED-077, RED-079)

- **Where it is called.** `policy.CheckSpendable` is called only from
  `Registry.CheckPolicy`. That in turn is called only by
  `wallet.CheckPolicy` (a payer checking an address it was given) and
  the CLI wallet. It is not on `Registry.Check` or any path that checks a
  witness in a transaction or a release.
- **It cannot refuse a valid spend.** It adds up the least witness data a
  policy can be satisfied with: the K smallest signatures, the K
  cheapest parts, or an empty preimage. Any witness that decodes and fits
  `max_witness` is at least that large, so it already passes the check.
  An unknown type counts as no data, so it is never refused.
- **The decode limits are unchanged.** This is a warning before money is
  sent to an address that could never be spent, which is wallet policy.

## Work tries (D168, RED-081)

`WorkTriesFor` and `DefaultWorkTries` size how long one `poo work` call
grinds. RED-081 bounds a large request in time. Both affect only this
node's own grinding. A layer is valid by its strength, whoever grinds it
and however long.

## Clear

| Item | Why it is clear |
| --- | --- |
| D168: wire type 14 releases; sending on connect; passing on; offence for a bad release | Information. Old nodes ignore the type. Validity depends only on the series and the release |
| D168: new parts in place (`AddPools`), or a reload | Ends where a start would, and is tested |
| D168: in-place `-set` for the book and tracker limits and `seen_size` | Round 25's rule. The tests compare selected tips and followed pools |
| D168: SLH-DSA-SHA2-128s (algorithm 10) | The owner's request. Only for series that list it. Deterministic, with known answers pinned |
| D168: work tries from the floor; exit code 3 for waiting; `mints_released` | Tooling |
| D168: the daemon waits for its own lock on reload (GREEN-050) | Local process order |
| BLUE-027 (RED-077, RED-079): `CheckSpendable` | Wallet only. It never refuses a spend that would be valid |
| BLUE-027 (RED-078) with ORANGE-036: witness on arrival, holding the highest release, the cache of checked releases | Changes when a node learns of a part, never what it accepts |
| BLUE-027 (RED-080, RED-081): grind and peers compared in place; work bounded in time | Local |
| ORANGE-036: measurements | Measurements and the changes above |
| GREEN-051: seed commands take `-alg` | Key tools |
| Red (PR #177): RED-078 to RED-081 | Findings, all fixed in PR #179. None touches a rule |

## Items for the owner

- **Nothing new needs your approval this round.**
  - Releases now spread between nodes. That is the last phase of the
    variable supply change you approved.
  - SLH-DSA-SHA2-128s is the algorithm you asked for, and only series
    that list it are affected.
- **Fixed series are still unchanged.** Black's cross-version tests give
  identical results on this round's builds.
- **One open question for you: the larger SLH-DSA sets (192s, 256s).**
  They need the signature size limit raised. Black sees no hard-fork risk
  if it is done with the new algorithm numbers, a witness cross-version
  test, and a larger data limit for 256s. It is your call whether to do
  it. Black suggests 192s if you want more margin than 128s.

How this was checked:
- Read D168 and D169, `p2p/release.go`, `node/release.go`, the old and new
  `dispatch`, `policy/slhdsa.go`, `policy/spendable.go` and its callers,
  and the diffs of PRs #176, #177, #178 and #179.
- Cross-version tests: identical on `439b369` and `fb8a8f3`.
- `go test ./...` passes on `439b369`, #176, #178 and #179. The release,
  algorithm, spendability and heavier-ledger tests pass under `-race` on
  `fb8a8f3`.
