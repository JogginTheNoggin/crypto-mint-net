# Source Round 36: compatibility review (White, Part D)

## Summary

White's review of Grey's base `main` @ `d25f777` (#297: D195 for W-23
and W-24, D196 for the bounty market), Green's GREEN-061 (#299),
Orange's ORANGE-046 (#301, docs only) and Blue's BLUE-037 (#304, D197,
which carries #299), at `pr304` head `ecc4cac`. **Nothing is breaking
and nothing blocks the round.**

D195 does what White asked. The payment file has `kind` and `format`,
and the payment and wallet files keep fields they do not know
(`pkg/keepjson`).

The bounty market is built the right way for compatibility:
- It is off by default.
- It is listed in the hello as a versioned feature (`bounty`, version 1)
  and spoken only to peers that list it.
- It uses new message types that older nodes already ignore (70 to 76).
- Its decoders ignore trailing bytes, so later phases can add fields.

White checked what D196 asked: the feature name, the message numbers and
the `bounties.json` kind and format. All three fit WHITE-001 and the
wire rules.

Two small asks, both before the tag:
- **W-27:** the offer id is worked out from the re-encoded offer, so a
  field a later phase adds would give older and newer nodes different
  ids for the same offer. Work it out from the bytes as received.
- **W-28:** `bounties.json`, the one record of what a node owes
  grinders, drops unknown fields on save. Keep them, as the wallet now
  does (W-24).

One rule goes into wire.md (W-29): how later bounty phases change the
messages. White's answers to Orange's proposals are below: Proposals 1
and 4 need White, and 2 and 3 change no format. Nothing needs the owner.

Tests: `go build ./...`, and `go test` of `pkg/keepjson`, `pkg/wallet`,
`pkg/bounty` and `pkg/wire`, the bounty tests in `pkg/p2p`, and the
bounty, GREEN-061 and payment tests in `pkg/cli`, all pass at `ecc4cac`.

## Verdicts

| Item | Verdict | Notes |
| --- | --- | --- |
| D195 W-23: payment file `kind` `poon-payment`, `format` 1, unknown keys kept (top level and legs) | compatible | Done as asked. A file with neither field (dev builds, before D195) still reads. A higher format is refused with a message to use a later release |
| D195 W-24: wallet keeps unknown top-level keys | compatible | Done. Same limits as `node.json` (256, 64 KiB, depth 64) |
| D195 `pkg/keepjson` | compatible | Matches keys the way `encoding/json` does, with case folded (`strings.EqualFold`), so `"Format"` is not kept beside `format`. See Proposal 4 |
| D196: feature `bounty` version 1, listed only when node policy turns the market on | compatible | The name is registered in `wire.go` and wire.md and is never reused. Peers are matched with `wire.Has(..., 1)`, so a version 2 can follow (W-29) |
| D196: wire types 70 to 76 | compatible | They are new types, and a node that does not know them ignores them, as every release has. They are in the wire.md table |
| D196: message layouts | compatible with conditions | Every decoder reads the fields it knows and ignores trailing bytes (`r.Err()`, not `Finish`), and a list holds each offer as `Bytes` of at most 1 KiB. That's right for later phases. **Condition:** W-27 (offer id) |
| D196: unknown offer `kind`, unknown ack `outcome` | compatible | An unknown kind is refused by `Offer.Check`, so the offer is ignored. An unknown outcome is shown as `outcome N` and counts as neither accepted nor paid. W-29 makes both rules |
| D196: `bounties.json` (`kind` `poon-bounties`, `format` 1) | compatible with conditions | A format or kind it does not know stops the node from starting (`daemon.go:149`). That fails safe, but the message should say the file is from a later release. **Condition:** W-28 |
| D196: node.json `bounty` section | compatible | It is an object, so an older release keeps it on save (D184, W-11). It is off when absent, and every limit has a floor and a ceiling |
| D196: RPC `/v1/bounty*`, `poon bounty ...` | compatible | New endpoints and commands only. The GUI uses the terminal for now (GUI/White's to track) |
| D196: a watch-only wallet cannot pay a bounty yet | compatible | Refused with a message. Adding it later changes no file |
| GREEN-061 (#299): `find_pool` in `POST /v1/bounty/post`, and the pool checked against the entry | compatible | Optional (`omitempty`). An older node refuses the unknown field (`DisallowUnknownFields`), so a newer client never posts on pool 0 by mistake |
| BLUE-037 (#304, D197): random start, outcomes 9 `known` and 10 `no lease`, leases per address, `From` kept, acks and records bounded, `bounties.json` checked on open | compatible | The outcomes are new codes in a feature no release has shipped yet. Entries that fail the check on open are dropped and logged, never paid, so an older file keeps loading |
| ORANGE-046 (#301) | n/a (measurements) | Answers to the proposals below |

## W-27: the offer id over the bytes as received

`Offer.ID` is `SHA-256(o.Encode())`. `DecodeOffer` keeps only the fields
it knows and drops trailing bytes, so a node from this release
re-encodes a later offer without its new fields. For the same offer, it
then works out a different id from the one the sender (and every newer
node) uses. Every take and claim names the offer by id. Its takes would
come back `closed`, and the two releases could not trade at all, even
when the new field is one the old node could safely ignore.

**Ask (before the tag; it is cheap now and a feature version later):**

- `DecodeOffer` keeps the offer's bytes as received (the `Bytes` inside
  type 70 or the list entry). `ID` hashes those bytes, and an offer the
  node posts itself hashes what it sends.
- `bounties.json` and the held book keep the raw bytes beside the
  fields (hex), so the id survives a restart.
- wire.md: "the offer id is the SHA-256 of the offer's bytes as sent,
  including fields this release does not know".
- Test: an offer with 8 extra trailing bytes has the same id on decode
  as its sender worked out, and a take naming that id is granted.

## W-28: `bounties.json` keeps what a later release adds

`bounties.json` is the one record of accepted claims a node still has to
pay. Today, a release that rolls back drops any field a later release
added on its next save. That includes fields inside each offer and claim,
which are list entries (the W-11 case). After W-24 the wallet keeps
them, and this file needs it more.

**Ask (before the tag):**

- Keep unknown keys at the top level, in each offer (`Mine`) and in each
  claim (`ClaimRecord`), with `keepjson` as the payment file does for
  its legs.
- A `format` above 1 is refused with: "bounties.json is from a later
  release (format N); run that release, or move the file aside to start
  with no posted offers".
- Test: an unknown key at each of the three levels survives `Open` then
  save.

## W-29 (rule): how later bounty phases change the messages

Phase 2 (signed offers) and phase 3 (passing offers on) are planned.
Write these rules in wire.md under the `bounty` feature:

1. A field added at the end of a bounty message, which an older node
   can ignore and still act correctly, stays in feature version 1. An
   older node reads up to its fields and ignores the rest.
2. A change an older node must not ignore bumps the feature to version
   2. A node sends the new form only to peers whose hello lists
   `bounty` 2 or above, and the old form to the rest. Examples: a
   signature an older node would pass on without checking, or a field
   that changes what a claim is paid for.
3. A new offer `kind` is ignored by a node that does not know it. A new
   ack `outcome` must be safe to read as "not accepted, not paid".
4. A node that passes an offer on (phase 3) passes on the bytes it
   received, never a re-encoding (W-27).

## ORANGE-046: White's answers

**Proposal 1 (move paid, closed offers out of `bounties.json`):
compatible with conditions.**

- An **older release** reading a trimmed `bounties.json` sees fewer
  offers. A closed offer with every claim paid needs nothing more, so it
  loses only history. The file stays `format` 1: a subset of what it
  held, with no new meaning.
- A **newer release** reading an older, full file moves the paid
  offers out on its first save. That works.
- Conditions:
  - The done file names itself. If it is JSON lines, make the first line
    `{"kind":"poon-bounties-done","format":1}`, and refuse a later format.
  - Append to the done file and sync it before removing the offer from
    `bounties.json`. A crash in between then leaves the offer in both
    files, never in neither. The reader drops an offer it finds twice,
    by id.
  - Move only offers that are closed and fully paid. Anything still
    owed stays in `bounties.json`, so a rollback never loses a debt.

**Proposal 2 (save off the inbox worker): compatible**, with one
condition. No file changes. An `accepted` ack must still go out only
after the claim is on disk, so a crash never forgets a layer the node
said it would pay for.

**Proposal 3 (`Receive` cost flat at any `held_offers`): compatible.**
It touches memory only, with no file or message change.

**Proposal 4 (`keepjson` walking tokens): compatible with conditions.**
The bytes in and out must stay the same:

- A key matches a field the way `encoding/json` matches it: with case
  folded (`strings.EqualFold`, as `Split` does today), embedded structs
  included, and a `json:"-"` field never matched.
- When a key appears twice, `encoding/json` takes the last value, and
  `Split` keeps the last one today. The walker must do the same, or
  refuse the file. It must never keep the first.
- The same limits and messages (256 keys, 64 KiB each, depth 64).
- Test: run the old and new `Split` and `Join` side by side over
  fixtures that hold a key in another case, a repeated key, nested
  unknown values and unicode keys, and check that the output bytes are
  identical.

## What Grey should take into Round 37 Part A

1. Merge #304 (with #299), #301 (docs), and Black's Part D PR.
2. **W-27:** offer id over the bytes as received. Before the tag.
3. **W-28:** `bounties.json` keeps unknown keys at three levels, with a
   clear message for a later format. Before the tag.
4. **W-29:** the rule in wire.md.
5. ORANGE-046 Proposals 1 and 4 under the conditions above. Proposals 2
   and 3 are Grey's call.

## Release readiness after this round

The owner's major change (the bounty market) is in. What blocks
`v0.1.0-alpha.1` from White's side is now W-27 and W-28, both small,
then the final review round and freeze. Still with the owner: license,
at least one registry node and its URL, optional signing keys in
`docs/allowed_signers`, and making the repo public. The two matching
local release builds happen at tag time (W-9).

## Addendum: W-30, the market's state as a field (from GUI/White)

GUI/White (#308) says the proposed Bounty view (#305) can tell the
market is off only by matching the text of `p2p.ErrNoBounty`. Error text
is not an interface: a reworded message or the "poon" rename would break
the view without anyone noticing. **Ask (Round 37A, small):** add
`bounty: {"post": bool, "take": bool}` to `GET /v1/status`
(`StatusReply`) and `poon status`, read from node policy and always
present. It is an added field, so older clients ignore it. The GUI reads
the field and keeps the text match only as a fallback for older nodes.
