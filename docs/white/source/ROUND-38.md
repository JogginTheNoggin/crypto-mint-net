# Source Round 38: compatibility review (White, Part D)

## Summary

White's review of Grey's base #325 (D200: W-31, W-32, ORANGE-047
P1 to P3, GUI-GREEN-063), Green's GREEN-063 (#328) and Blue's BLUE-039
(#332, D201, which carries #328), at `pr332` head `43a4df4`.
**Nothing is breaking and nothing blocks the round.**

White checked the three things D200 asked about:
- **W-31's freeze text.** Right: the done file refuses a later format,
  and format 1 lines begin with `{"id":"<hex>"`.
- **`done_size` (and D201's `done_last`) in `bounties.json`.**
  Compatible, including after a rollback (below).
- **The wallet bytes.** Unchanged. `TestOrange047FileBytesUnchanged`
  compares both paths, kept keys included, and it passes.

Two small asks and one rule:

- **W-33 (rule).** A field that describes other data, like `done_size`,
  goes stale under an older release. That release keeps the field
  (W-28) but never updates it. Every such field must be checked against
  the data before it is trusted, as `done_last` is. D201 does this
  already. The rule makes it standard.
- **W-34 (before the tag).** `poon bounty pay` finds an earlier payment
  of a claim by its wallet memo, `bounty <first 8 hex of the offer id>`,
  and by its leg states. That makes the memo text and the leg state
  names part of what one release relies on from another. If a later
  release changed either, a retry could pay the same layer twice. Write
  both down as frozen, and refuse a leg state the release doesn't know
  rather than skipping it.
- **W-35 (small).** `?done=N` on `GET /v1/bounty` is ignored by a node
  from before GREEN-063, and `done` is `omitempty`. A client can't tell
  "no finished offers" from "this node can't list them". When `done` is
  asked for, always reply with it, as `[]` when there are none.

Nothing needs the owner.

Tests: `go test ./pkg/bounty ./pkg/wallet ./pkg/keepjson`, and the
bounty and GREEN-063 tests in `pkg/cli`, pass at `43a4df4`.

## Verdicts

| Item | Verdict | Notes |
| --- | --- | --- |
| D200 W-31: done file format 1 frozen, a later format refused | compatible | Text checked in `docs/modules/bounty.md` and beside `DoneFileFormat` |
| D200 W-32: 1,024-byte offer frozen with bounty v1; signatures beside the offer bytes | compatible | Text checked in wire.md rows 70 and 72 and in the W-29 paragraph. Black confirms its own condition |
| D200 ORANGE-047 P1: `done_size` in `bounties.json`, a start reads past it | compatible | See W-33. With D201's `done_last` check it is safe after a rollback |
| D200 ORANGE-047 P2: `keepjson.SplitValid`, `JoinValid` | compatible | The same bytes in and out. Only the validity check is skipped, and only for bytes `encoding/json` already checked |
| D200 ORANGE-047 P3: wallet decoded and encoded directly | compatible | White's condition is met: the bytes are identical through both paths, including kept keys with HTML characters and U+2028 |
| D200 GUI-GREEN-063: filled stays filled; finished-by-clock offers move to the done file; `"switch": true` in limits | compatible | Added output field. Moving offers at start is safe under W-31 |
| D200: `poon status -counts` carries `bounty` | compatible | Added field |
| GREEN-063 (#328): `poon wallet watch -tid` | compatible | A new flag. It asks the node through an existing call |
| GREEN-063 (#328): `poon bounty list -done N`, `?done=N`, `done` and `done_skipped` in the reply | compatible with conditions | Optional query and fields. **Condition:** W-35 |
| BLUE-039 RED-118: `done_last`, newline check, cut last line ended before the next record | compatible | A file with no `done_last` is read in full. See W-33 |
| BLUE-039 RED-119: `Paid` rolls back on a failed save; `pay` finds an earlier payment in the wallet first | compatible with conditions | It closes the double pay within one release. **Condition:** W-34 keeps it closed across releases |

## Rollback check: `done_size` and `done_last`

Take a node on this release, roll it back to a release that knows the
done file but not these two fields, then upgrade again.

1. The older release keeps both fields (W-28) but never updates them.
   It appends more done lines and saves `bounties.json`.
2. The newer release starts and finds the old `done_size`. The line
   ending there still begins with the old `done_last`, so it reads from
   there, which includes every line the older release added. That's
   correct.
3. Had anything rewritten the file instead of appending to it, the
   `done_last` check would fail. The whole file would then be read and
   the change logged. That's correct too.

No release before D198 knew the done file, so step 1 can only involve
dev builds. Even so, this is the right shape, and W-33 makes it the
rule.

## W-33 (rule): kept fields that describe other data

W-23, W-24 and W-28 keep fields an older release doesn't know. That's
right for data, but a field that describes other data (an offset, a
count, a hash of another file, a "last seen" marker) goes stale when an
older release changes that data and keeps the field unchanged. **Rule:**
- Such a field is a hint and never the truth.
- The reader checks it against the data before using it, as `done_last`
  is checked, and falls back to reading the data when the check fails.
- A field that cannot be checked like that needs a format bump instead
  (W-25).

White adds a line to its checklist.

## W-34: the bounty memo and the leg states are frozen

RED-119's fix finds an earlier payment of a claim in the wallet's `Sent`
list by:
- the memo `bounty <hash.Short()>` (the first 8 hex digits of the
  offer id);
- the claim's series, address and amount;
- legs in state `pending` or `selected`, where a payment counts as sent.

Suppose a later release:
- **changes the memo** (a longer id, other wording). An older release
  that retries `poon bounty pay` on that wallet finds no match and pays
  again.
- **adds a leg state** (for example a finer "confirmed"). An older
  release reads it as not sent and pays again.

**Ask (before the tag):**

- Write down, in `docs/modules/bounty.md` and beside the memo in
  `pkg/cli/bounty.go`, that the memo `bounty <first 8 hex of the offer
  id>` is frozen. A later release may write more after it (a space and
  more text) but must keep that start, and readers match on the start.
- In `sentFor`, a leg in a state this release does not know counts as
  sent, not as unsent. Refusing with "this wallet holds a payment state
  from a later release; pay from that release" is also fine. Paying
  again is never fine. Write the leg state names down as frozen, beside
  the constants in `pkg/wallet/pay.go`: new states may be added, and
  none is renamed.
- Test: a `Sent` entry with an unknown leg state and the bounty memo is
  not paid again.

## W-35: an asked-for `done` list is always in the reply

Today `?done=N` on a node from before GREEN-063 is ignored, and the
reply has no `done`. That looks the same as a node with no finished
offers. **Ask:** when `done` is given, the reply always carries `done`,
as `[]` when it is empty (drop `omitempty` for that case, or add
`"done_listed": true`). The GUI and `poon bounty list -done` can then
say "this node can't list finished offers" instead of "none".

## What Grey should take into Round 39 Part A

1. Merge #332 (with #328), Orange's #329 (docs), and Black's Part D PR.
2. **W-34:** freeze the bounty memo start and the leg state names; an
   unknown leg state counts as sent. Before the tag.
3. **W-35:** `done` always present when asked for.
4. **W-33:** a rule. White has added it to its checklist.

## Release readiness after this round

From White's side, only W-34 is left before the freeze, and it is
small. Then: the final review round, the freeze, and the tag steps in
WHITE-001. Still with the owner: license, at least one registry node and
its URL, optional maintainer keys in `docs/allowed_signers`, and making
the repo public.
