# GREEN-062: Turning the bounty market on with `poo policy limits -set`

| Field | Value |
| --- | --- |
| Status | proposed (prototyped) |
| Based on | `main` @ `bf90ffd` |
| Area | `pkg/cli` (`poo policy limits -set` takes on/off; the help line for `bounty post`), `pkg/nodepolicy` (`BountyNotes`, `Bound.OnOff`), `pkg/rpc` (`notes` in the limits reply; `bounty work` checks take), `pkg/p2p` (`ErrNoBounty` text, `ErrNoTake`), `pkg/bounty` (posting names unset spending limits), docs (running-a-node, modules/bounty, modules/poo) |
| Effort | small |
| Consensus | none: messages, CLI parsing and one optional reply field. What a node accepts into a ledger does not change |
| Decided by | Grey; Black to confirm no rule changed; White may note the local RPC field below; GUI Grey or Green for the Bounty view's off line |

## Summary

BASE.md asked Green to try turning the market on with `poo policy limits
-set` as a new user. Green used two fresh nodes, with the commands in
running-a-node.md, both with the node stopped and while it ran. Turning
the market on works: the node reloads, `poo status` shows `bounty`, and
the peers see each other's feature again. Five things got in the way:

1. **`-set bounty.post=on` (or `=true`) was refused** with "want
   NAME=VALUE, such as p2p.max_book=8192". That reads as if the shape
   were wrong when it was the value. The list shows the setting as `min
   1, max 1`, which does not suggest on or off either.
2. **Posting on with no spending limits said nothing at `-set`.** The
   first post then failed with "over bounty.max_amount 0". A new user
   reads that as "the amount is over zero", not "max_amount is not set".
3. **The market-off error pointed at node.json.** "set bounty.post or
   bounty.take in node.json", although D198 made both settable with one
   command.
4. **A node that only posts asked peers for offers and dropped every
   answer.** `poo bounty list -ask` came back with an empty `held` and no
   reason. `poo bounty work ID` said "no offer with that id was
   received", when the reason is that `bounty.take` is off.
5. **The `poo help` line for `bounty post` still said `-pool K`.**
   GREEN-061 made the pool optional and updated `poo bounty`'s own usage,
   but missed this one.

## The change

- `poo policy limits -set` takes `on`, `off`, `true`, `false`, `yes` and
  `no` for an on/off setting (`Bound.OnOff`: range 1 to 1, default 0,
  which is `bounty.post` and `bounty.take`). A bad value says what the
  setting takes ("bounty.post is 1 (on) or 0 (off), not "maybe"") or
  that it is not a whole number. A mistyped name still gets its "did you
  mean" hint first.
- The limits reply (through the node and with it stopped) gains `notes`
  from `Policy.BountyNotes`: posting on with `max_amount` or `budget`
  unset ("so no offer can be posted"), and `max_amount` over `budget`.
  Nothing is refused, because a person may set them in the next command.
- `Book.Post` names the unset limits and the command that sets them:
  "bounty.max_amount and bounty.budget are not set, so this node posts no
  offer; poo policy limits -set bounty.max_amount=N -set bounty.budget=N".
  The off message names `-set bounty.post=1`.
- `ErrNoBounty` names the commands instead of node.json. The new
  `ErrNoTake` (wrapping `bounty.ErrOff`) is returned by
  `poo bounty list -ask` and `poo bounty work` on a node whose
  `bounty.take` is off.
- `poo help` shows `bounty post -entry HASH [-pool K | -serial N]`.
- Docs: running-a-node.md, modules/bounty.md and modules/poo.md mention
  on/off and `notes`.

## How it was tested

- `TestBountyTurnedOnWithLimits` (pkg/cli/green062_test.go) covers:
  the off error; `bounty.post=on`; the notes with the node stopped and
  through a running node; a bad on/off value; a word for a numeric
  limit; a mistyped name with a word value; posting with `budget` unset;
  and `-ask` and `work` on a post-only node. On main it does not build
  (no `notes`), and its first check (the off error) fails.
- `TestBountyMarket` and `TestBountyPostFindsTheEntrysPool` pass.
- By hand, with two nodes: the messages above, and the reply
  `{"saved": true, "reloading": true, "notes": [...]}`.
- gofmt, go vet, staticcheck, `go test -race ./...` and GUI e2e
  `parity.mjs` and `qol.mjs` all pass.

## For Black

There are no rule changes. `ErrNoTake` stops a post-only node from
sending list requests and takes it could never use. A node that takes
behaves as before.

## For White

The `poo policy limits` reply (`/v1/policy/limits`) gains an optional
`notes` list. An older node leaves it out, and readers that ignore
unknown fields are unaffected. No file or wire format changes.

## For the GUI

The Bounty view's fixed off line (`gui/web/static/views/bounty.js`,
`offNote`) still says to edit node.json and restart. Since D198, `poo
policy limits -set bounty.take=on` (or the Limits form) does it with a
reload. Green has not edited GUI files. That one line is for GUI Grey
or GUI Green.
