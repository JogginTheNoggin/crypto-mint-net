# Source consensus reviews (Black)

## Summary

This folder holds the Source/Black team's consensus reviews, one per
round, and the checklist every review follows. Black runs in Part D, after
Blue, and reads everything the round produced: Grey's new base, Blue's
fix branches, and the Green and Orange proposals. It answers two
questions for each item:

1. **Does it add or change a consensus rule?** A consensus rule decides
   whether a ledger, entry, layer, transaction, pool, claim or weight is
   valid or selected, or fixes a byte form. Only the design documents
   (the project's knowledge files) and the owner's explicit approvals may
   do that. Anything else is *blocked: consensus*. Every suggested
   consensus change from any team comes to Black first: Black either
   pre-approves it and passes it to the owner for final approval, or
   filters it out (see [The consensus gate](#the-consensus-gate)).
2. **Can it make a hard fork possible?** Under any setting, input or
   peer behaviour, could nodes that hold the same ledgers disagree on
   which is valid or heavier, so that a node refuses for good a heavier
   valid ledger it has been given? If so it is *blocked: hard fork*. A
   lack of information, voluntary or involuntary, and slowness are not
   hard forks (the owner, 2026-11-03; AGENTS.md, Black reviews). Black
   may still note them as node-policy improvements.

Black never edits code or other teams' files. It flags, filters and
explains, and suggests a node-policy shape that keeps the rule. Grey may
adopt only items Black has cleared. The baseline is
[docs/consensus-audit.md](../../consensus-audit.md) and the owner's
decisions recorded in it.

## The owner's rules Black enforces

| Rule | Source |
| --- | --- |
| No new consensus rules outside the design documents without the owner's explicit approval | Owner, 2026-10-30; AGENTS.md Rules |
| Prefer node policy, but no node policy may ever make a heavier valid ledger unacceptable | Owner, 2026-10-31; consensus audit |
| Hard forks must be impossible under any condition, whatever the settings | Owner, 2026-10-31 |
| A hard fork is nodes with the same information that cannot agree on the valid or heavier ledger; a lack of information or slowness is not one | Owner, 2026-11-03 |
| No depth limits, not even as node policy (the main document wins over the chasing document) | Owner, 2026-10-31 (G3) |
| No size limit that every peer would hit (no entry or transaction size cap) | Owner, 2026-10-31 (C1, C2) |
| No extra proof of work for deep forks (Red's deep-fork PoW idea) | Owner, 2026-10-30 |
| Byte form v1 is frozen; a new form is a new version beside it | D76 |

The test for "unacceptable", from the audit: **if a valid ledger heavier
than a node's selected ledger is served by any peer the node can reach,
the node takes it in and selects it in the end, whatever its settings.**
A setting may decide when, how fast, from whom and in what order, and may
ask for proof of work before spending anything. It may never refuse such
a ledger for good.

## The consensus gate

The owner's rule (2026-10-31): all suggestions for consensus changes go
to Black first, and Black decides whether the owner should see them.
Black pre-approves; the owner gives final approval.

For each suggestion, Black answers in its review:

- [ ] What rule does it add or change, in the words of the design
      documents? Which document section, if any, supports it?
- [ ] Can node policy that only delays, paces, orders, chooses peers or
      asks for work first get the same result? If yes, filter it out and
      describe that shape.
- [ ] Does it keep hard forks impossible under every setting (checklist
      section 3)? If not, filter it out.
- [ ] Does it bring back something the owner already refused (standing
      rulings below)? If yes, filter it out.
- [ ] Is the gain worth a rule every node must follow forever?

**Pre-approved** suggestions go under "Items for the owner" in the
round's review, written plainly: the problem, the rule, why node policy
is not enough, the hard-fork check, and what happens if the owner says
no. **Filtered** suggestions are listed with the reason, and the sending
team is told. Pre-approval is not approval: nothing is built until the
owner says yes, and the owner's decision is recorded in
`docs/consensus-audit.md`.

## Standing rulings (baseline)

Items already decided. A change that brings one of them back is blocked
without further argument.

| Item | Ruling |
| --- | --- |
| C1, entry size cap (`entry.MaxTxs`, frame-sized entries) | Refused. Replace with streaming in parts (audit P1) |
| C2, transaction input and output cap | Refused. Same as C1 |
| G1, revert budget acting as a depth limit | Refused. Charge only unproven work, never refuse for good (P2) |
| G2, `MaxBareRun` | Refused. Remove; recheck work during a fetch (P3) |
| G3, chase `DepthLimit` | Refused. No depth limit; find the split by halving (P4) |
| G4, valid entries on the known-bad list | Pending the owner (P5) |
| ORANGE-010, minimum layer strength as node policy | Allowed only if the node keeps each entry's counted credit |
| Deep-fork proof of work | Not to be built |

## Review checklist

Run every section for every item in the round. Write "n/a" with a reason
rather than skipping a line.

### 1. Find what changed

- [ ] List the round's commits: `git log <previous base>..<new base>`,
      each Blue branch against its base, and each proposal's prototype
      branch.
- [ ] List every changed file in the consensus packages: `pkg/tx`,
      `pkg/entry`, `pkg/layer`, `pkg/ledger`, `pkg/state`, `pkg/series`,
      `pkg/policy`, `pkg/hash`, `pkg/weight`, `pkg/encoding`,
      `pkg/engine`. Any change there is read line by line.
- [ ] List every changed file on the paths that refuse, drop, delay or
      choose data: `pkg/p2p` (chase, forward, serve), `pkg/chase`,
      `pkg/tracking`, `pkg/peers`, `pkg/mempool`, `pkg/wire`,
      `pkg/store`, `pkg/node`, `pkg/nodepolicy`.
- [ ] Read every new or changed decision in `docs/decisions.md`, and
      every proposal's Idea section.

### 2. Consensus rules (validity, weight, selection, byte form)

- [ ] Does anything newly refuse a transaction, entry, layer, series or
      witness that the previous base accepted? Find the rule in the
      design documents or an owner approval, or block it.
- [ ] Does anything newly accept something the previous base refused?
      (A loosened rule is a change too, and is a hard fork for older
      nodes.)
- [ ] Is weight still the sum of counted credit, 2^strength per layer,
      with entry hash credit only where the series counts it, in exact
      big integers?
- [ ] Is selection still "heaviest wins, switch only when strictly
      heavier, ties keep the first selected"? No new tie-break, no
      preference by age, depth, peer or arrival time.
- [ ] Does selection read only what the ledger holds, never the clock,
      the network, peer scores or node policy?
- [ ] `go test ./pkg/vectors` passes and `TestByteFormV1Frozen` is
      untouched. No hash, tag, field order or number size changed.
- [ ] New policy types, algorithms or hash numbers only extend the
      registries and never reuse a number.
- [ ] Any new constant inside a decoder or validator is a declared type
      bound (class (b) in the audit), not a limit on how much a valid
      ledger may hold.
- [ ] An optimization (preverify, caches, kept state, ordering) rejects
      exactly what the full check rejects and nothing more. Look for an
      input the fast path refuses and the slow path accepts.

### 3. Hard-fork risk (node policy and network behaviour)

For each new or changed limit, budget, list, timer, queue or setting:

- [ ] Set it to its tightest allowed value. Can a valid heavier ledger
      still be taken in and selected in the end? Name the case if not.
- [ ] Is it a depth limit in disguise? (A cost that grows with fork depth
      against a fixed allowance is one.)
- [ ] Is it a size limit every peer would hit? (A cap on one item's size
      that no peer can get around is one.)
- [ ] Does it refuse for good, or only delay? Over the limit, does the
      item go back to a queue, to a later window or to another peer?
- [ ] Can one peer use it to make the node ignore valid data (a
      known-bad list, a seen list, a ban, a back-off) for every peer?
- [ ] Can all of a node's peers be pushed past it at once, so no peer
      can deliver the heavier ledger?
- [ ] Does pruning, layer dropping or storage policy change the weight
      the node computes for a ledger it still holds? (Dropping layer
      bytes is fine; dropping counted credit is not.)
- [ ] Does a restart, a crash during a switch or a full disk leave the
      node on a lighter ledger with no way back?
- [ ] Does the `pkg/sim` guard scenario for the rule (audit P0) still
      pass with the default policy and with the tightest values? Once it
      exists, a change that weakens it is blocked.

### 4. Documents and records

- [ ] Every consensus rule the code follows is still class (a) or (b)
      in the audit tables; a new one is added there as (c) and sent to
      the owner.
- [ ] Where the design documents disagree, the code follows the reading
      recorded in `docs/decisions.md`, and no new disagreement was
      introduced silently.
- [ ] Each node-policy setting has a documented default and says it
      delays, paces, orders or chooses, never refuses for good.

### 5. Verdict

Give each item one verdict, as defined in
[AGENTS.md](../../../AGENTS.md#black-reviews-part-d): **clear**, **clear
with conditions**, **blocked: consensus** or **blocked: hard fork**. For
every blocked item, name the rule it breaks, the concrete case (inputs,
settings, peers) that shows it, and a node-policy shape that would keep
the rule.

## Review format

One file per round: `ROUND-<N>.md`, for example `ROUND-7.md`. It starts
with a Summary, then this table:

| Field | Value |
| --- | --- |
| Round | Source Round N |
| Consensus suggestions | ids, each pre-approved or filtered |
| Base reviewed | branch @ commit (from `security/source/BASE.md`) |
| Previous base | branch @ commit |
| Blue branches | branch @ commit, one per line |
| Proposals | ids reviewed |
| Result | n clear, n with conditions, n blocked |

Then one section per item with its verdict, what the checklist found,
and for blocked items the case and the suggested node-policy shape. End
with **Items for the owner**: everything that needs the owner's approval,
in plain words.

## Reviews

| Round | File | Result |
| --- | --- | --- |
| Source Round 7 | [ROUND-7.md](ROUND-7.md) | 13 clear, 4 with conditions, 1 blocked: hard fork (B1, reproduced); no consensus suggestions |
| Source Round 8 | [ROUND-8.md](ROUND-8.md) | 13 clear, 2 with conditions (C-5 on BLUE-009, RED-022), 2 blocked: hard fork (B2 measured by Orange, B3 by reading); B1 fixed; no consensus suggestions |
| Source Round 9 | [ROUND-9.md](ROUND-9.md) | 12 clear, 1 with conditions (C-6 on BLUE-010: kept progress must outlast the stall backoff), 0 blocked; B2, B3, C-5 fixed; no consensus suggestions |
| Source Round 10 | [ROUND-10.md](ROUND-10.md) | 11 clear, 1 with conditions (fix the flaky C-6 guard test), 1 blocked: hard fork (B4, a trickling sender holds the disk reserve; by reading); C-6 met; no consensus suggestions |
| Source Round 11 | [ROUND-11.md](ROUND-11.md) | 10 clear, 0 with conditions, 1 blocked: hard fork (B5, reproduced: an honest fetch marked slow never takes the reserve back from a trickler); B4 closed only for a moving fetch; the C-6 guard test fixed (D133); no consensus suggestions |
| Source Round 12 | [ROUND-12.md](ROUND-12.md) | 6 clear, 0 with conditions, 1 blocked: hard fork (B6, reproduced: two colluding senders hold the parts reserve with BLUE-013's paid cuts); ORANGE-022's D115 case on main upheld, superseded by BLUE-013; B5 fixed; no consensus suggestions |
| Source Round 13 | [ROUND-13.md](ROUND-13.md) | 8 clear, 0 with conditions, 0 blocked; B6 fixed (D140); ORANGE-022 closed; the two-thirds holder is a bandwidth contest; no consensus suggestions |
| Source Round 14 | [ROUND-14.md](ROUND-14.md) | 7 clear, 0 with conditions, 0 blocked; the full-token entry body cap lifted (removes a size bound); ORANGE-024's stale-waiter note is a delay paid in bytes; no consensus suggestions |
| Source Round 15 | [ROUND-15.md](ROUND-15.md) | 7 clear, 0 with conditions, 0 blocked; wallet, CLI and local RPC only; the entry body has no size bound; no consensus suggestions |
| Source Round 16 | [ROUND-16.md](ROUND-16.md) | 10 clear, 0 with conditions, 0 blocked; wallet, CLI, local RPC and durable writes only; the entry body is still a hint with no size bound (64 MiB start, D147); no consensus suggestions |
| Source Round 17 | [ROUND-17.md](ROUND-17.md) | 9 clear, 0 with conditions, 0 blocked; node policy export and import; Red's question answered: k, the reply limits and max_pending cannot refuse a heavier ledger (tested at both extremes); no consensus suggestions |
| Source Round 18 | [ROUND-18.md](ROUND-18.md) | 9 clear, 0 with conditions, 0 blocked; B7 (check_seconds -1 with peers that send no notices) reproduced, an information gap and not a hard fork under the owner's definition (2026-11-03), optional node-policy fix; lazy pool state cannot change what is accepted; the wire pool index is a protocol change for the owner; no consensus suggestions |
| Source Round 19 | [ROUND-19.md](ROUND-19.md) | 8 clear, 0 with conditions, 0 blocked; first round under the owner's hard-fork definition; lazy pool state meets the same state; MaxSections caps sections, not what can be followed; the paced sweep and unindexed huge-series pools are information gaps; no consensus suggestions |
| Source Round 20 | [ROUND-20.md](ROUND-20.md) | 8 clear, 0 with conditions, 0 blocked; settings export and import change no acceptance; RED-057 chase-only records tested end to end (an unused pool of a huge series is still chased to a heavier ledger); the text check touches no series parameter; no consensus suggestions |
| Source Round 21 | [ROUND-21.md](ROUND-21.md) | 8 clear, 0 with conditions, 0 blocked; the peer feature (by hand, filters, export and import) and imported blocks change only whom a node talks to, an information question; RED-058 quarter share is local peer-book policy; the text check touches no series parameter; no consensus suggestions |
| Source Round 22 | [ROUND-22.md](ROUND-22.md) | 9 clear, 0 with conditions, 0 blocked; no settable limit can refuse a heavier ledger: book, tracker and discovery limits are information, `seen_size` and `max_pending` are memory, and a tie forgotten at the `ties` floor is taken when given again and heavier (tested); the peer book's give-way order is node policy; GREEN-047's survey: size caps and weight counting stay fixed; no consensus suggestions |
| Source Round 23 | [ROUND-23.md](ROUND-23.md) | 9 clear, 0 with conditions, 0 blocked; rival claims build a branch that was always valid, checked in full by `AddEntry` and selected by weight; `CheckOn` and `CheckPendingOn` are local pre-checks that pick what goes into the node's own entry; the RPC handshake and the peer book are local; no consensus suggestions |
| Source Round 24 | [ROUND-24.md](ROUND-24.md) | 10 clear, 0 with conditions, 0 blocked; variable supply phase 1 (D163), the owner's approved consensus change, built as specified; Grey's choices agreed (format version 2, U64 part number, cap counts part 1, release message, witness first); fixed series identical across builds (cross-version test); no cap on parts (RED-070); phase 2 requirements listed; no other consensus suggestions |
| Source Round 25 | [ROUND-25.md](ROUND-25.md) | 11 clear, 0 with conditions, 0 blocked; variable supply phase 2 (D166) meets Black's six requirements; fixed series, transactions and entries identical across builds (cross-version tests); reading the part by series id cannot split followers; the checked-release cache is local state; ORANGE-035's in-place limits design accepted on its "same as a reload" rule; no other consensus suggestions |
| Source Round 26 | [ROUND-26.md](ROUND-26.md) | 12 clear, 0 with conditions, 0 blocked; variable supply phase 3 (release gossip, wire type 14 ignored by old nodes) is information; in-place limits on Round 25's terms; SLH-DSA-SHA2-128s (owner's request) only for series that list it; `CheckSpendable` is wallet-only; fixed forms identical across builds; for the owner: larger SLH-DSA sets need `MaxSigSize` raised (no hard fork if done with new algorithm numbers; Black's conditions listed) |
| Source Round 27 | [ROUND-27.md](ROUND-27.md) | 10 clear, 0 with conditions, 0 blocked; per-series witness size limits (D170), the owner's approved change, only loosen (Black's differential test of 8,820 verdicts: nothing valid lost, no difference under any ceiling up to 64 KiB) and the pinned cross-version vector reproduces on `439b369`; SLH-DSA-SHA2-256s only for series that list it; ceiling refusals and `series_ceiling` are tooling and node policy; the reserve series id matches the adopted document (worked out independently); far releases checked off the read loop (D171) change no verdict; fixed forms identical across builds |
| Source Round 28 | [ROUND-28.md](ROUND-28.md) | 9 clear, 1 clear with a condition, 0 blocked; entry bodies on disk (D172) change cost, not decisions: no verdict reads a body outside memory, and selection is identical with a 0-byte cache and with every body read failing (Black's differential, 691 reorganisations); a deep switch never needs a body from disk; unreadable bodies are missing information (D173); displaced sorting without witnesses matches `CheckAgainst`; condition: narrow D173's panic guard so a panic while a ledger changes stops the node instead of running on; fixed forms identical across builds |
| Source Round 29 | [ROUND-29.md](ROUND-29.md) | 12 clear, 0 with conditions, 0 blocked; cold items and undo (D175) leave selection identical at windows of 1 to 16 and any cache, and the recomputed undo matches `Apply`'s record (Black's differential); a `ReadFault` stops the node and a restart from the store selects the same (Black's fault and restart model, 9,125 faults); a deep switch is slower, never refused; Round 28's panic condition met (D174, D177); spends cost 3 view units, which paces only; wording note: a fault can follow a counted layer or inserted entry, and the safety is the stop and rebuild; fixed forms identical across builds |
| Source Round 30 | [ROUND-30.md](ROUND-30.md) | 10 clear, 0 with conditions, 0 blocked; a start from the node's own snapshot (D178) selects what a full replay does (Black restored the random-history twin after every step: 38,578 restores, 691 reorganisations, identical); a snapshot from another release, another pool, a changed log, a damaged file or a restore that does not round trip is never used, and a replay follows (D181); trusting the node's own snapshot is the owner's decision; `Renumber` keeps tie order local and consistent; ORANGE-040 proposal 1 is clear either way; fixed forms identical across builds |
| Source Round 31 | [ROUND-31.md](ROUND-31.md) | 10 clear, 0 with conditions, 0 blocked; the one-pass snapshot gather (D182, `SnapshotPrint`) writes the same records and fingerprint (Black restored the twin from it after every step: 60,231 gathers per window, 691 reorganisations, identical); snapshot cadence decides only when the node writes its own file; opt-in port mapping (D182, D183) is node policy, off by default and machine-local, changes only what a node advertises, and the self branch still refuses the connection; ORANGE-041 proposal 1 clear on stated terms; GUI ORANGE-023 count is output only; fixed forms identical across builds |
| Source Round 32 | [ROUND-32.md](ROUND-32.md) | 10 clear, 0 with conditions, 0 blocked; the variable series set as a `sync.Map` (GUI ORANGE-024) holds the same ids, added and read at the same points (passes under `-race`); kept node.json settings (W-1, RED-093 to RED-096) are node policy, and the node runs with what the decoder applies; WHITE-002 step 1 (`poo/VERSION` in the hello) is not a consensus change, since nothing decides on `Agent`; step 2 clear on stated terms (no verdict, baseline kept, no refusing or ranking by version); for the owner: a public alpha tag fixes the consensus byte form, as WHITE-001 says; fixed forms identical across builds |
| Source Round 33 | [ROUND-33.md](ROUND-33.md) | 9 clear, 1 clear with a condition, 0 blocked; release, features and extensions in hello, ping and pong (D186) amend only the wire half of D76, and nothing decides on them (byte form v1 frozen and identical); ledger import (D188) runs every item through the peer path as `Requested`; ORANGE-043 parallel preverify calls the same `CheckWitnesses` and only fills the cache; import bounds refuse no entry by size; condition: a malformed ping or partial pong still earns a lasting `InvalidMessage` offence against term 4, so fix it or correct the wording, and add a test at the frozen maximum |
| Source Round 34 | [ROUND-34.md](ROUND-34.md) | 10 clear, 0 with conditions, 0 blocked; Round 33 condition met (`ErrBadExtras` ends only the connection, tested at the frozen maximum); wider hello limits (64 KiB extensions, 255 features) are wire room frozen at the tag, and nothing decides on them; the 3 MiB hello frame fits every valid hello (about 2.0 MiB); inbox byte caps never touch replies, an empty queue always takes one message, and no entry is refused by size; on a discovery frame limit: only safe at or above the largest valid ping, frozen with wire v1, and Black recommends leaving it out; fixed forms identical across builds |
| Source Round 35 | [ROUND-35.md](ROUND-35.md) | 10 clear, 0 with conditions, 0 blocked; cold keys (D193) make the same transactions on another machine; one signature per key per transaction (ORANGE-045) is valid because every witness signs `Message(tid)`, and the pool accepts it (Black's ledger test with a control); claims through `poo sign` are the claim `Wallet.Claim` builds; `tx.DecodeVariable` is tool-side only and the tid covers the same bytes; the 64 KiB ping and pong extension total is `ErrBadExtras` and frozen; fixed forms identical across builds |
