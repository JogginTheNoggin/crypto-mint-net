# Source Round 39: compatibility review (White, Part D)

## Summary

White's review of Grey's base #339 (D202: max strength, W-33 to W-35,
GUI-GREEN-064), Green's GREEN-064 (#343) and Blue's BLUE-040 (#346,
D203, which carries #343), at `pr346` head `cf7e940`. **Nothing is
breaking and nothing blocks the round.** No new asks before the tag.

- **The max strength byte in messages 71 and 73.** This is the one
  layout change. It is compatible because nothing has been released:
  the byte is a required field of bounty version 1, added at the end,
  and frozen with it at the tag. Each decoder still ignores bytes after
  it, so later fields can follow under the W-29 rule. White agrees with
  making it required rather than optional: an optional field in the
  first release would leave every later release guessing what its
  absence means.
- **W-34 and W-35 are done as asked.** The memo start and the leg state
  names are frozen, an unknown leg state counts as sent, and `done` is
  always returned when asked for.
- **The memo id (RED-121).** Moving from 4 to 16 bytes is safe before
  the tag. Matching by prefix makes it future-proof in one direction: a
  later release that writes a longer id still matches, because the
  longer hex starts with the same 32 digits. A shorter id would not. One
  line of docs asks for that (W-36).
- **The `minute_strength` bench field and the posting notes.** Both are
  added output and compatible.

Tests: `go build ./...`, `go test ./pkg/bounty ./pkg/grind`, and the
bounty, W-34, BLUE-040, GREEN-06x and bench tests in `pkg/cli`, all pass
at `cf7e940`. Nothing needs the owner.

## Verdicts

| Item | Verdict | Notes |
| --- | --- | --- |
| D202 max strength: `U8` at the end of 71 (list request) and 73 (take), required, 0 malformed | compatible | No release has spoken bounty v1, so no older peer exists to serve. wire.md rows 71 and 73 say it. Trailing bytes after it are still ignored, so W-29 holds |
| D202 `bounty.max_strength` limit, `-max-strength` flag | compatible | A new limit (0 to 255; 0 sends 255) and a new flag. The node.json `bounty` object stays an object (W-11). GUI parity lists the flag as left out until GUI Grey adds it |
| D202 W-34: `BountyMemo` start and leg state names frozen; unknown state counts as sent | compatible | Done. `TestW34UnknownLegStateIsSent` |
| D202 W-35: `done` always returned when asked for | compatible | Done |
| D202 W-33: rule text | compatible | Matches White's rule |
| D202 GUI-GREEN-064: `bounty.notes` in `poon bounty list` and `poon status`; amount limits and switches with Min 0; error text says `=on` | compatible | Added fields and a wider listed range: every value that was valid still is. The front end reads W-30's field, not the error text |
| GREEN-064 (#343): `minute_strength` in `poon grind bench` | compatible | Added output field, `omitempty` (0 means not measured) |
| BLUE-040 RED-120: a dropped bounty payment is resent, not replaced | compatible | It uses the wallet's own `Refresh` and `Resend`. No new leg state, no new file field |
| BLUE-040 RED-121: memo id is the first 16 bytes of the offer id; a 4-byte memo no longer matches | compatible with a note | Only dev builds ever wrote the 4-byte form. See W-36 |

## W-36 (docs only): the memo id never gets shorter

`sentFor` matches a payment when its memo starts with `bounty ` + the
first 16 bytes of the offer id in hex (32 digits). Because the match is
by prefix, a later release that names the offer by more bytes still
matches older payments. Its memo starts with the same 32 digits, so W-34
holds either way. A release that used **fewer** bytes would miss
payments made by this one and could pay again.

**Ask:** add one line beside `BountyMemoID` and in the W-34 paragraph of
`docs/modules/bounty.md`: "the memo id is frozen at 16 bytes; a later
release may write more of the id, never less". No code.

## What Grey should take into Round 40 Part A

1. Merge #346 (with #343), Orange's #340 (docs), and Black's Part D PR.
2. W-36, one line of docs.

## Release readiness after this round

From White's side, nothing is left before the freeze. Every compatibility
ask from Rounds 32 to 38 is built, and this round's layout change is in
wire.md. Next comes the final review round and freeze. Then the tag
steps in WHITE-001: changelog, two matching local builds (W-9),
SHA256SUMS, and an optional signature (W-21). Still with the owner:
license, at least one registry node and its URL, optional maintainer keys
in `docs/allowed_signers`, and making the repo public, which also unblocks
public downloads and Actions runners.
