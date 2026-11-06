# Source Round 37: compatibility review (White, Part D)

## Summary

White's review of Grey's base `main` @ `7eb16ab` (#311, D198), Green's
GREEN-062 (#314), Orange's ORANGE-047 (#313, docs only) and Blue's
BLUE-038 (#317, D199, which carries #314), at `pr317` head `39e7cb2`.
**Nothing is breaking and nothing blocks the round.**

D198 did White's Round 36 asks as written:
- W-27: offers are named by their bytes (`Offer.Raw`).
- W-28: `bounties.json` keeps later fields at three levels.
- W-29: the rule is in wire.md.
- W-30: `bounty` is in `poo status`.

White answers D198's four questions below. One small ask:

- **W-31 (before the tag).** The done file's reader checks its `kind`
  but not its `format`. It also finds each offer's id by the line
  starting with `{"id":"`. That works, but a later release that changes
  the line layout would make this release miss offers that are already
  finished. It would then keep them in the book and could owe them
  again. Refuse a format above 1, as `bounties.json` does, and write the
  line rule down.

One rule for wire.md, so later bounty phases don't trip on the new
1 KiB offer bound:

- **W-32 (rule).** Offer bytes are at most 1 KiB under bounty version 1.
  A signature or key (phase 2) never goes inside the offer bytes. It goes
  in a separate field or message under bounty version 2, so the offer id
  stays the same for every release.

Orange's Proposal 3 (wallet without the JSON methods) is compatible if
the bytes stay identical. GREEN-062's `notes` field is compatible.
Nothing needs the owner.

Tests: `go build ./...`, and `go test` of `pkg/bounty`, `pkg/keepjson`,
`pkg/wallet`, `pkg/nodepolicy` and `pkg/wire`, the bounty and BLUE-03x
tests in `pkg/p2p`, and the bounty, GREEN-06x and limits tests in
`pkg/cli`, all pass at `39e7cb2`.

## Verdicts

| Item | Verdict | Notes |
| --- | --- | --- |
| D198 W-27: `Offer.Raw`, id = SHA-256 of the bytes received, lists pass on the bytes | compatible | Done. On load, the bytes are the offer and the fields beside them are for people. `checkMine` refuses an entry whose id is not the hash of its bytes |
| D198 W-28: `bounties.json` keeps later fields (top, offer, claim); a later format refused with a clear message | compatible | Done |
| D198 W-29: the rule in wire.md | compatible | Text checked. It says what White asked. See W-32 for the one thing it leaves out |
| D198 W-30: `bounty: {post, take}` always in `poo status` | compatible | An added field. GUI R37A (#315) reads it |
| D198 ORANGE-046 P1: `bounties-done.jsonl`, header `poo-bounties-done` format 1, appended and synced before removal | compatible with conditions | An older release with no done file only loses history. **Condition:** W-31 |
| D198 ORANGE-046 P2: leases saved on a 200 ms timer; post, cancel, accepted claim and payment still written before they answer | compatible | Meets White's condition. Losing a lease to a crash only means the grinder takes it again |
| D198 ORANGE-046 P3, P4 | compatible | P4 has the byte-for-byte test White asked for (`TestOrange046ScanMatchesTheMap`) |
| D198: claimed layers at or above the series floor go to the engine | n/a (Black) | No file or message change |
| D198 GUI-GREEN-062: the bounty settings as settable limits, `outcome_name`, `state` and `closes` in output | compatible | Added output fields. The on/off limits take 1 or 0. See the 2^53 question below |
| D198: a node.json amount over 2^53 - 1 is refused | compatible (answer below) | Acceptable before the tag |
| GREEN-062 (#314): `notes` in `/v1/policy/limits`; `ErrNoBounty` text reworded; `ErrNoTake` | compatible | `notes` is optional and older nodes leave it out. Rewording the error is exactly why W-30 exists: the GUI reads the status field first and uses the text only as a fallback |
| BLUE-038 RED-115: `MaxOfferBytes` 1,024 | compatible with conditions | An offer is 96 bytes today. **Condition:** W-32, written in wire.md beside type 70 and 72 |
| BLUE-038 RED-116: held offers per address and /64; the oldest unleased offer goes when full | compatible | Memory only |
| BLUE-038 RED-117: done file read in bounded pieces; an offer in both files is left out and logged | compatible | This depends on the line layout, which W-31 freezes |
| ORANGE-047 (#313) | n/a (measurements) | Proposal 3 answered below. Proposals 1 and 2 are Grey's |

## White's answers to D198's questions

1. **`Offer.Raw` in `bounties.json`.** Right. The id is the hash of
   those bytes, so they have to be kept. The fields beside them are for
   people and are read again from the bytes on load. A file written by
   an R36 dev build (no `raw`) re-encodes from the fields, which gives
   the same id for an offer with no later fields. No release ever wrote
   such a file.
2. **The done file's kind and format.** The header is right. The reader
   needs the format check and a frozen line start (W-31).
3. **The W-29 text in wire.md.** It says what White asked. Add the
   offer-size bound and the signature rule (W-32).
4. **Refusing a node.json amount over 2^53 - 1.** Acceptable before the
   tag. No node.json in the wild holds a bounty amount, and 2^53 - 1
   units (about 9 x 10^15) is far above the default series' supply of
   10^14. One thing to know: raising the cap later is easy going forward
   but not going back. A release that raised it would write node.json
   values that an earlier release refuses at start. If a series with a
   larger supply is expected, the way out is amounts as decimal strings,
   and that change is cheaper before the tag. White doesn't think it's
   needed for the alpha.

## W-31: the done file checks its format and freezes its line start

`doneIDs` refuses a wrong `kind` but reads any `format` as format 1. It
finds each finished offer by the line starting with `{"id":"<64 hex>"`.
If a later release wrote a format 2 done file with another line layout,
this release would find no ids. An offer still in `bounties.json` after
a crash would then go back into the book with its claims, and could be
paid twice.

**Ask (before the tag):**

- Refuse a done file whose `format` is above 1, with the message W-28
  uses ("from a later release (format N); run that release, or move the
  file aside"). Refusing is safer than guessing, because a guess can owe
  money twice.
- Write the line rule into `docs/modules/bounty.md` and beside
  `DoneFileFormat`: within format 1, every line after the header begins
  with `{"id":"<hex>"`, and later fields come after the id. A layout
  change is format 2.
- Test: a done file with `"format": 2` is refused, and a format 1 line
  with an unknown key after the id is read.

## W-32 (rule): the offer bound and signatures

RED-115 bounds an offer's bytes at 1 KiB, in type 70 and in each type 72
entry. Under W-29, a trailing field older nodes can ignore stays at
bounty version 1, so the total of every such field must stay under
1 KiB. That rules out post-quantum signatures: ML-DSA-65 is 3,309 bytes
and SLH-DSA-128s is 7,856. A signed offer that carried its signature
inside the offer bytes would therefore be refused by every version 1
node. It would also have a different id from the unsigned form, which
breaks W-27.

**Rule for wire.md (beside types 70 and 72):**

- Offer bytes are at most 1,024 under bounty version 1, and this bound
  is frozen with the feature version.
- A signature, key or proof about an offer (phase 2) is never part of
  the offer bytes. It travels beside them, in a new message type or a
  version 2 list entry (`Bytes` offer ∥ `Bytes` proof), sent only to
  peers listing `bounty` 2 or more. The offer id stays the hash of the
  offer bytes alone, so every release names the offer alike.

No code this round.

## ORANGE-047 Proposal 3: the wallet without the JSON methods

**Compatible with conditions.** Reading and writing the plain fields
directly is fine as long as the file on disk is exactly what it is
today:

- The same bytes with no kept keys. With kept keys, the kept values are
  indented exactly as `MarshalIndent` indents them today, in the same
  place and order (after the known fields, sorted).
- Keys match as they do now: case folded, the last repeated key wins,
  the same limits and errors.
- `Open` still applies every check it applies today (format, key
  algorithm, gap), and `wallet passwd`'s unchanged-file check
  (GUI-022) still compares the bytes read.
- Test: write wallets through both paths and compare the bytes. Cover
  no kept keys, kept scalar, object and array values, and a kept key in
  another case.

## What Grey should take into Round 38 Part A

1. Merge #317 (with #314), #313 (docs), and Black's Part D PR.
2. **W-31:** the done file refuses a later format, and its line start is
   written down. Before the tag.
3. **W-32:** the offer bound and the signature rule in wire.md.
4. ORANGE-047 Proposal 3 under the conditions above, if Grey takes it.

## Release readiness after this round

From White's side, only W-31 is left before the freeze, and it is
small. After it: the final review round, the freeze, then the tag steps
in WHITE-001 (changelog, two matching local builds, SHA256SUMS and an
optional signature). Still with the owner: license, at least one
registry node and its URL, optional maintainer keys in
`docs/allowed_signers`, and making the repo public.
