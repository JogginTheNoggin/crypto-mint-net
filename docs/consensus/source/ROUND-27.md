# Source Round 27: consensus review (Black, Part D)

## Summary

This round makes the change the owner approved on 2026-11-04. It removes
the network-wide witness size caps, so each witness is bounded only by
three things:
- its type's declared bounds;
- its algorithms' exact key and signature sizes;
- its series' witness ceiling.

It also adds SLH-DSA-SHA2-256s as key algorithm 11, makes the reserve
series the `poon init` default (D170), and moves the full check of a far
release off the peer's read loop (D171).

**Result: 10 clear, 0 with conditions, 0 blocked.**

- D170's consensus change is exactly the plan the owner approved. Nothing
  else in it is a rule.
  - The ceiling refusals in the tools and `series_ceiling` are tooling and
    node policy, as the owner chose.
  - `max_witness` 0 is still a valid series, and no design document
    changes.
- **The change only loosens.** Black's own differential test, 1,260
  witnesses judged by `439b369` and by this build under seven ceilings,
  found:
  - no witness that the old build took and this build refuses;
  - no difference at all under any ceiling from 1 byte to 64 KiB.
- **The pinned cross-version vector is sound.** Black reran its old half
  on `439b369` and got the pinned verdicts exactly.
- **Fixed series, transactions and entries are unchanged.** The Round 24
  and Round 25 cross-version tests are byte for byte the same on `main`
  and on PR #191.
- **The reserve series id matches the adopted document.** Black worked it
  out by hand from the document's values, with Python's SHA-512/256 and
  not the project's code, and got the pinned id.
- **Nothing goes to the owner this round.**

| Field | Value |
| --- | --- |
| Round | Source Round 27 |
| Consensus | D170: the owner's approved change (per-series size limits; algorithm 11; before v0.1.0, as Black and Red required). D171: none |
| Other consensus suggestions | None |
| Base reviewed | `main` @ `1a1c729` (PR #187: D170, with Round 26's PRs merged) |
| Previous base | `main` @ `439b369` |
| Blue branches | PR #191 @ `26dfc8b` (BLUE-028, D171: RED-082, RED-083) |
| Red | PR #190 @ `4033237` (RED-082, RED-083) |
| Proposals | ORANGE-037 (PR #188 @ `a651b5d`); GREEN-052 (PR #189 @ `e5f705e`) |
| Result | 10 clear, 0 with conditions, 0 blocked |

## Per-series size limits (D170)

### It is the approved change and nothing more

- **Removed.** `MaxKeySize`, `MaxSigSize`, `MaxParamsSize` and
  `MaxDataSize` are gone.
  - Params, witness data and k-of-policies parts are now read bounded
    only by the bytes received. `Reader.Raw` refuses a length the input
    cannot hold, so memory stays in proportion to what was sent, as D117
    already relies on for entries.
- **Unchanged.** The type bounds (16 keys, 16 parts nested 4 deep, a
  1,024-byte preimage) and the order of `CheckCached` are as they were.
  The order is: ceiling, then address, then allowed, then satisfy.
- **Earlier, not different.** The threshold data is now decoded against
  its params. It must have exactly K signatures, with key indices
  increasing and in range, and each signature at its key's exact size.
  - Before this change, `Satisfy` refused all of these after decoding.
    Now they are refused before a signature's bytes are copied, with the
    same verdict.
  - A known algorithm's public key is read at most at its exact size.
    That size was already checked.
- **The same path for issuer and release witnesses.** They go through
  `Registry.Check` under the series' own rules, ceiling included. So the
  witness results below cover them too.
- **Decoding needs no series anywhere.** `DecodeWitness` and
  `DecodePolicy` take no series, and every caller uses them:
  - `tx.DecodeFrom`;
  - a part's issuer witness;
  - a node policy's issuer witness;
  - the wallet;
  - the fuzz targets.

  `decodeThresholdDataFor` needs only the params and the registry. A node
  that has not built a series still frames its transactions and never
  judges them (PoON 16.7).

### Black's checks of "only loosens"

Black wrote a differential test of its own, kept out of the repo. It
covers more than the nine pinned vectors.

- **The witnesses.** 180 witnesses, each with six mutations:
  - a flipped byte;
  - cut short;
  - an extra byte;
  - two changed length fields;
  - a cut in the middle.
- **The witness kinds.**
  - Thresholds from 1-of-1 to 16-of-16 for each of algorithms 1 to 10;
  - 60 mixed-algorithm thresholds;
  - 20 hashlocks, some with preimages over 1,024 bytes;
  - 40 k-of-policies over thresholds.
- **The judging.** Each was judged by `policy.Default().Check` on
  `439b369` and on `1a1c729`, under ceilings 0, 300, 4,096, 65,535,
  65,536, 65,552 and 131,072.

| Result | Count |
| --- | --- |
| Verdicts compared | 8,820 (1,260 witnesses × 7 ceilings) |
| Old build valid, new build invalid | **0** |
| Any difference under a ceiling from 1 to 65,536 | **0** |
| Old invalid, new valid | 4: the ML-DSA-87 16-of-16 and SLH-DSA-128s 9-of-9 witnesses, under no ceiling and under 128 KiB only |

Every mutation that the old build refused at decode (a length over a cap)
is refused by this build at the ceiling, or at the exact-size check.

- **The pinned vector** (`pkg/policy/testdata/crossversion.json`). Black
  ran `testdata/crossversion-old` on a fresh worktree of `439b369`
  against a copy of the file. The nine `old` verdicts came out exactly as
  pinned.
- **The SLH-DSA-256s vector.** It is invalid to `439b369`, which has no
  algorithm 11. In real use an old build cannot build a series that lists
  11 at all, so it finds the series unreadable, never in disagreement.

### Why it cannot split nodes

- **Same series, same answer.** Every limit now lives either in the series
  id (`max_witness`, `key_algs`, `policy_types`) or in the algorithm's own
  number. Two nodes that read the same series apply the same numbers.
- **Old and new builds.** An old and a new build reading a looser series
  would disagree. That is why the owner, Black and Red required the
  change before any release. It holds today: no v0.1.0 has been tagged,
  and the repo is private. After a release, the same kind of change would
  need new algorithm or type numbers.

### The tool refusals and `series_ceiling` add no rule

- **The tools.** `poon series new` and `poon init -series` refuse a missing
  ceiling, or one over 4 MiB, unless `-any-ceiling` is given. That is what
  the tool writes, not what a node accepts. `poon init -example` is exempt.
- **The node setting.** `series_ceiling` (`Policy.DeclinesSeries`) acts
  only in a policy import.
  - It declines a series that would be *added*, together with its follow
    rules.
  - It never touches a series already listed, a ledger, or a witness.
  - Declining to follow a series is a lack of information.
- **Unchanged by this round.** `max_witness` 0 stays valid, and PoON 16.1
  is untouched.

### Spendability, algorithm 11 and in-place limits

- **`CheckSpendable`** (worst case, per series) is still wallet-only. It
  is called from `Registry.CheckPolicy`, never from `Check` or
  `CheckCached`.
- **SLH-DSA-SHA2-256s (algorithm 11).**
  - It is FIPS 205 with exact sizes (64-byte keys, 29,792-byte
    signatures), and the known answers are pinned.
  - Only a series that lists 11 can use it.
  - The fixed cross-version test (algorithms 0, 1, 2 and 9) is unchanged.
- **`in_place`** is a label on limits that already exist.

## The default reserve series (D170)

- **What it is.** Node policy: what `poon init` follows when given nothing
  else. Any node may follow any series.
- **What Black checked.** That the pinned params match "Reserve Currency
  Default Series" (`/mnt/project-files/reserve-series/`). The document
  gives:
  - 32,768 mints of 10^14 units, in pools of 32;
  - SHA-512/256 (number 2);
  - an entry floor of 20 and a layer floor of 24, with no entry credit;
  - threshold and hashlock;
  - Ed25519, ML-DSA-65, ML-DSA-87, SLH-DSA-SHA2-128s and SLH-DSA-SHA2-256s
    (1, 8, 9, 10, 11);
  - a ceiling of 131,072;
  - no issuer.
- **How it was checked.**
  - Black read `encoded_params` field by field, by hand, against the
    document.
  - Black then worked out `SHA-512/256("PoON-series" ∥ encoded_params)`
    with Python's `hashlib`, not the project's code.

  The result is `45772e75a4670fd7c37dd66898550c2e657fb2c35b547335be2ce7a64466f4c5`,
  the same as `series.ReserveID`.
- **Naming the reserve.** `"builtin": "reserve"` is shown only when the
  full id matches. That is display.

## Releases off the read loop (D171, BLUE-028)

- **RED-082.** A peer's release is still checked on arrival
  (`CheckReleaseSigned`) and then in full by the same `CheckRelease`. The
  only change is where the full check runs: in one worker per series,
  under the host's context.
- **A cancelled check is not a verdict.**
  - `takeRelease` returns nothing when the host's context has ended. So
    there is no offence, and nothing is kept.
  - `CheckReleaseContext` returns the context's error, never
    `ErrRelease`, when it stops.
  - The checked-release cache holds only passes (ORANGE-036).

  So the part waits, which is a lack of information, and the release is
  checked again when it comes again.
- **RED-083.** This adds bounds on `series_ceiling` (0, or 1 KiB to
  1 GiB), warnings for series with no ceiling or a huge one, and plan
  text. Nothing a node accepts changes.
- **Not adopted.** Orange's 4 MiB `series_ceiling` default was left to
  Grey and jerry. As node policy it would be clear either way, because it
  only declines a series being added.

## Clear

| Item | Why it is clear |
| --- | --- |
| D170: no network-wide witness size limit; signatures read at exact size | The owner's approved change, made before any release. Only loosens (Black's differential test and the pinned vector). Same numbers for every node that reads a series |
| D170: cross-version vector | Sound. Its old half reproduces on `439b369` |
| D170: SLH-DSA-SHA2-256s (algorithm 11) | Red's revised key list, chosen by the owner. Only for series that list it. Exact sizes, known answers pinned |
| D170: tool ceiling refusals, `series_ceiling` | Tooling and node policy, as the owner chose. `max_witness` 0 stays valid |
| D170: worst-case `CheckSpendable`, `poon crypto -max-witness` | Wallet and tooling |
| D170: the default reserve series, `builtin` naming | Node policy. The id matches the document, worked out independently |
| D170: `in_place`, cancellable far part ids (GUI-028) | Labels. Stopping says nothing about validity |
| BLUE-028 (RED-082, RED-083) | Where and when a release is checked, and warnings. Never whether it is valid |
| ORANGE-037, GREEN-052 | Measurements; `keygen` shows addresses per series and `crypto -o table` |
| Red (PR #190): RED-082, RED-083 | Findings, both fixed in PR #191. Neither touches a rule |

## Items for the owner

- **Nothing needs your approval this round.** The per-series limits are
  the change you approved, built to the conditions Black and Red set:
  - before v0.1.0;
  - with a cross-version test;
  - with exact per-algorithm sizes;
  - with a ceiling in the default series.
- **The change only loosens.** No witness that was valid before is
  refused now. Series with a ceiling of 64 KiB or less decide exactly as
  before.
- **The reserve series is exactly the one in your document.** Its id
  `45772e75…f4c5` was checked by hand.
- **Before v0.1.0, keep it that way.** Once a build is released, any
  further loosening of witness limits needs new algorithm or type
  numbers.

How this was checked:
- Read D170 and D171, the diffs from `439b369` to `1a1c729` in
  `pkg/policy`, `pkg/series`, `pkg/nodepolicy` and `pkg/p2p`, and the
  diffs of PRs #188 to #191.
- Black's differential witness test (8,820 verdicts) and a rerun of the
  pinned old verdicts on `439b369`.
- The reserve id, worked out independently.
- The Round 24 and Round 25 cross-version tests: identical on `1a1c729`
  and `26dfc8b`.
- `go test ./...` passes on `1a1c729`, #188, #189 and #191. The policy,
  series, node policy and p2p packages pass under `-race` on `26dfc8b`.
