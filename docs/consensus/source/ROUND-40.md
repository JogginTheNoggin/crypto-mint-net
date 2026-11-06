# Source Round 40: consensus review (Black, Part D)

## Summary

This round renames the project, at jerry's request:
- **D204** renames the tool from poo to poon.
- **D205** renames every other use of the old name, including the local
  RPC hello label.
- **D206** renames the protocol's own labels from `PoO-` to `PoON-`. This
  is a consensus label change, and jerry approved it explicitly ("i said
  everything", 2026-11-06, in reply to the question about the PoO
  labels).

Every id, address and key changes as a result. The default series is
now `45772e75…f4c5`. Blue's D207 fixes RED-122 and RED-123, and carries
Green's GREEN-065 and Orange's ORANGE-050.

**Result: 10 clear, 0 with conditions, 0 blocked.**

- **D206 is a label change and nothing else.** Black proved it on the
  code, as follows:
  1. Take `main` at `b94d861`.
  2. Put every `PoON-` label back to `PoO-`.
  3. Set back the two pinned ids that tests compare against.

  With only that changed, the new build gives the old build's bytes
  exactly:
  - Black's Round 24 and Round 25 cross-version outputs (series, pools,
    transactions, entries, ids) are byte for byte those of the old
    baseline.
  - `TestByteFormV1Frozen` passes with the old frozen digest
    (`d89968f2…754f`) and the old `vectors.json` and `variable.json`.
  - The policy package's known answers and cross-version file pass with
    the old files, including key derivation, addresses and signatures.
  - The reserve series rebuilds to the old id `fe52c1ac…f139`, and the
    pinned example series to the old `d0b927f7…095a`.

  So no layout, field order, rule, limit or verdict changed. Only the
  label bytes inside the hashed messages did.
- **The labels map one to one.** The same 20 label strings exist before
  and after, each with the same number of uses and with only the prefix
  changed. No two labels merged, none was added or dropped, and none is
  left behind as `PoO-` in the code. Domain separation between hashes is
  the same as before.
- **No hard-fork risk.**
  - Two nodes of this build, given the same information, agree on the
    same ledger, exactly as before.
  - A build from before D206 computes different series ids. It therefore
    follows different series and never judges the same ledger as a build
    after D206. That makes it a separate network, not a disagreement
    about one ledger.
  - No build before D206 was released, so no running node is left behind.
- **Byte form v1 freezes at the tag with these labels.** This is the last
  point where a label could change without new type or algorithm numbers
  (Round 27, Round 32). After the tag, a label change is a change to
  what is valid, and it would need new numbers.
- **The rest is names.** None of the following is consensus:
  - the tool and binary names, environment variables, file kinds and
    metrics;
  - the hello's agent text;
  - the local RPC hello label;
  - `POON_VIEW_ONLY` failing closed;
  - held-offer display.

| Field | Value |
| --- | --- |
| Round | Source Round 40 |
| Consensus | D206: a label change approved by jerry. No rule added, removed or changed. D204, D205 and D207: none |
| Other consensus suggestions | None |
| Base reviewed | `main` @ `b94d861` (PR #353: D204; #358: D205; #363: D206; GUI #354 between) |
| Previous base | `main` @ `21c34c8` (the rename compared against `296a208`, `main` just before #353) |
| Blue branches | PR #365 @ `2343bc5` (BLUE-041, D207: RED-122, RED-123; carries GREEN-065 #356 and ORANGE-050 #360) |
| Red | PR #355 @ `59a28d3` (RED-122, RED-123) |
| Proposals | GREEN-065 (PR #356 @ `34ef477`); ORANGE-050 (PR #360 @ `0ee3114`); both carried in #365 |
| Result | 10 clear, 0 with conditions, 0 blocked |

## D206: the protocol's labels (a consensus label change)

### What changed

Every label that began `PoO-` now begins `PoON-`:
- **Consensus hash labels.** `address`, `tx`, `entry`, `pool`, `series`,
  `witness-v1`, `issuer-v1`, `part`, `part-v1`, `request-v1`,
  `verified-v1` and `verified-set-v1`.
- **Key labels.** `key-v1`, `wallet-<name>-v1`, `wallet-seed-v1` and
  `test-key:`.
- **Files.** The snapshot magic `PoON-snap` and the store's `PoON-log`.

Every fixed value was made again from the code: `vectors.json` and its
digest, `variable.json`, `known.json`, `reserve.json`, the pinned example
series id and `series.ReserveID`.

### Black's proof that only labels changed

Black ran three checks.

**1. Normalised source diff.** Black took `pkg/` at `296a208` (`main`
before the rename) and at `b94d861`, removed test files, and replaced
`PoON` with `PoO` and `poon` with `poo` in both. Then it compared them.
- Every consensus package is identical: `hash`, `encoding`, `tx`,
  `entry`, `layer`, `ledger`, `engine`, `node`, `wire`, `tracking`,
  `chase` and `store`.
- In `series`, only the `ReserveID` constant differs.
- In `policy`, only regenerated test data differs.
- The other differences are in `bounty`, `cli`, `grind`, `nodepolicy`,
  `p2p/bounty.go` and `rpc`. They are the reviewed GREEN-064 and D204
  changes to messages and display, and none of them is in a consensus
  path.

**2. Old labels on the new code.** In a worktree of `b94d861`, Black set
every `PoON-` label in code and tests back to `PoO-`, and set
`ReserveID` back to `fe52c1ac…f139`. Then:
- Black's Round 24 and Round 25 cross-version tests wrote output
  identical, byte for byte, to the old baseline (`s26` and `tx26`).
- With the old `reserve.json`, `TestReserveSeriesPinned` passes, so the
  reserve series' parameters still hash to the old id.
- With the old `vectors.json`, `variable.json` and frozen digest
  `d89968f2…754f`, `pkg/vectors` passes, including
  `TestByteFormV1Frozen`.
- With the old `known.json` and `crossversion.json`, `pkg/policy`
  passes, including the known answers for key derivation, policy
  encoding, addresses and signatures.
- With the old pinned example id `d0b927f7…095a`, `pkg/series` passes.
  The `store` and `wallet` packages pass too.

**3. The new build against itself.** On `b94d861` and on #365, with the
new labels, the cross-version outputs are identical to each other, and
`TestByteFormV1Frozen` passes with the new digest.

Together these show that D206 changes the label bytes and nothing else.
Every value that depends on a label changed only because its label did.

### Why it opens no hard-fork risk

- **The same rules.** Under the owner's definition, a hard fork needs
  nodes with the same information to disagree on which ledger is valid.
  Every node of this build applies the same rules to the same bytes, as
  before. The cross-version verdicts that Grey made again from `439b369`
  with the same label change match too.
- **Old builds are a separate network.** A series id hashes its
  parameters under `PoON-series`. A build from before D206 gets a
  different id for every series, and a different address for every key.
  The two builds never follow the same series, so they never weigh the
  same ledger. That is no information shared, not a disagreement over
  shared information.
- **Nothing released.** No node outside development runs an old build.
  D207 and the docs say data from earlier builds needs a fresh start,
  and a node pointed at it refuses to start, so nothing old is read
  wrongly.
- **Separation kept.** Each hash still starts with its own distinct
  label. The labels are the same 20 as before with one prefix, so no new
  overlap is possible between, for example, a tid and an entry id.

### The freeze

`docs/protocol/encoding.md` still says byte form v1 is frozen (D76), and
it now shows `PoON-` in the hashed messages (for example `tid =
H("PoON-tx" ∥ body)`). D206 states that byte form v1 is frozen from the
tag with these labels. Black agrees, and repeats Round 32's point: after
the alpha tag, changing any label, layout or limit changes what is valid,
and it needs new type or algorithm numbers. The cross-version vectors at
`b94d861` are the ones every later change must be checked against.

## D204, D205, D207: names and local fixes

- **The tool's name (D204, D205).** The changes cover:
  - the binary;
  - environment variables (`POON_*` through `brand.EnvPrefix`);
  - the data folder and file kinds;
  - metrics;
  - the hello's agent (`poon`).

  The hello's agent is free text that nothing decides on (Round 32). File
  kinds belong to the tool. None of these is hashed into an entry, layer,
  transaction or id.
- **The RPC hello label (D205).** `poon-rpc-hello` proves a local client
  and its own node to each other. It is never on the peer wire and never
  in a ledger.
- **The memo id frozen at 16 bytes (W-36).** A wallet record (Round 39).
- **`POON_VIEW_ONLY` fails closed (RED-122).** A local safety switch.
- **Held offers past 16 layers read filled (RED-123).** Display only. The
  payer's answer is unchanged.
- **GREEN-065 and ORANGE-050.** Usage text, shell completion, and one
  benchmark fixed to run again.

## Clear

| Item | Why it is clear |
| --- | --- |
| D206: every `PoO-` label renamed `PoON-` (hash, key, snapshot, log), approved by jerry | A label change only, proven by putting the old labels back on the new code: every fixed value is the old one. No rule changed |
| D206: fixed values regenerated (vectors and digest, known answers, reserve id `45772e75…f4c5`, pinned example id) | Made from the code. Each differs only through its label |
| D206: byte form v1 frozen from the tag with the `PoON-` labels | Agrees with Round 32. Later label changes need new numbers |
| D204, D205: tool, binary, environment, data folder, file kinds, metrics, hello agent, local RPC label renamed | Names and local proofs. Nothing hashed into the ledger |
| D204 (W-36, GUI-GREEN-065): memo id frozen at 16 bytes; `asked`; held filled; `zero` per limit | Wallet record and display |
| D207 (RED-122): `POON_VIEW_ONLY` fails closed; data from earlier builds is a fresh start | Local safety and docs. Old data is refused, never misread |
| D207 (RED-123): held offers past `MaxAcks` read filled | Display only |
| GREEN-065 (carried in #365): usage spacing, `_poon` completion | CLI |
| ORANGE-050 (carried in #365): no regression from the rename; benchmark fixed | Measurements and a test-only fix |
| Red (PR #355): RED-122, RED-123 | Findings, fixed in #365. Neither touches a rule |

## Items for the owner

- **Nothing needs your approval this round.** You already approved the
  label change.
- **The protocol rename changed names only.** Black put the old labels
  back on the new code and got every old id, address, signature and test
  value back exactly. No rule changed, and nodes running this build agree
  on ledgers exactly as before.
- **Older test builds are now a different network.** They compute
  different ids, so they never follow the same series as this build. No
  one is running a released copy, so no one is cut off.
- **From the first tag these labels are fixed.** Changing them later
  would need new type or algorithm numbers, like any change to what is
  valid.

How this was checked:
- Read D204 to D207, the diffs of PRs #353, #358, #363, #355, #356, #360
  and #365, `pkg/brand`, `series/reserve.go`, `docs/protocol/encoding.md`,
  and jerry's message approving the change.
- Listed every `PoO-` label string at `296a208` and every `PoON-` string
  at `b94d861`: the same 20 labels with the same counts, and none left as
  `PoO-`.
- The normalised source diff and the old-labels-on-new-code run described
  above.
- The Round 24 and Round 25 cross-version tests and `TestByteFormV1Frozen`:
  identical on `b94d861` and `2343bc5`.
- `go test ./...` passes on `b94d861`, #355, #356, #360 and #365. The
  series, policy, wallet, store, cli, bounty and rpc packages pass under
  `-race` on `2343bc5`.
