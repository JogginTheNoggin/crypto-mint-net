# GREEN-064: A fast and a slow grinder against offers of several strengths

| Field | Value |
| --- | --- |
| Status | proposed (prototyped) |
| Based on | `main` @ `fa8dad7` |
| Area | `pkg/p2p` (`AskBounties` refuses a `-max-strength` over the node's own; `NotReceived`), `pkg/rpc` (work uses `NotReceived`), `pkg/cli` (`poon bounty list -max-strength` lists only offers at or under it), `pkg/grind` (`minute_strength` in `poon grind bench`), docs (running-a-node, modules/bounty, modules/poon) |
| Effort | small |
| Consensus | none: refusals, an error message, an output filter and one bench field. Messages 71 and 73 and the payer's checks are unchanged |
| Decided by | Grey; Black to confirm no rule changed; White may note the bench field below |

## Summary

BASE.md asked Green to walk a fast and a slow grinder against one
payer's offers of several strengths, as a new user. Green set up three
nodes:

- a payer posting offers at 12, 20 and 26 bits;
- a fast grinder with `bounty.max_strength` unset;
- a slow grinder with `bounty.max_strength=16`.

The core works. The slow grinder's ask came back with only the 12-bit
offer. The fast grinder held all three and worked the 26-bit one, while
the slow one worked the 12-bit one; both claims were accepted. Four
things got in a new user's way:

1. **`-max-strength` above the node's setting did nothing, silently.**
   The slow grinder ran `poon bounty list -ask -max-strength 22` and
   still got only the 12-bit offer. The payer sent the 20-bit one, but
   `Receive` dropped it because it was over the node's own 16. The
   command succeeded and listed nothing new.
2. **Working that offer gave a misleading error.** `poon bounty work
   <20-bit id>` on the slow grinder said "no offer with that id was
   received; poon bounty list -ask asks peers for theirs". It was
   received, then dropped. Asking again changes nothing.
3. **`list -ask -max-strength 14` still listed 20- and 26-bit offers**
   held from an earlier, wider ask. That is not what the person asked
   for.
4. **Nothing helps choose the value.** The docs show
   `bounty.max_strength=30`. To pick their own, a person has to run `poon
   grind bench` and work out log2(rate × seconds) by hand.

## The change

- `AskBounties` refuses a `-max-strength` above a set
  `bounty.max_strength`: "-max-strength 22 is over this node's
  bounty.max_strength 16, and offers above that are not kept; poon policy
  limits -set bounty.max_strength=22 raises it". Work is unchanged:
  every offer it can work is already at or under the setting.
- `p2p.NotReceived(cfg)` is the error for an offer id the node does not
  hold. When the node has them set, it adds "(this node does not keep
  offers over bounty.max_strength 16)" and the same for
  `bounty.min_price`. Both work paths (RPC and host) use it.
- `poon bounty list -max-strength S` lists only peers' offers at or
  under S, with or without `-ask`. Wider offers stay held. The flag's
  help says so.
- `poon grind bench` adds `minute_strength` to each result:
  log2(rate × 60), rounded down. That is the strength found in about a
  minute on average. Each bit more doubles the time.
- Docs: running-a-node.md adds `poon grind bench` before the setting and
  a paragraph on choosing it. modules/bounty.md and modules/poon.md
  describe the refusals, the filter and the field.

## How it was tested

- `TestBountyMaxStrengthFlow` (pkg/cli/green064_test.go) runs three
  real nodes, connected. On main it fails at the list filter, and its
  later checks (the refusal and the error) fail too. It checks:
  - the fast grinder holds both offers;
  - `list -ask -max-strength 10` shows only the 8-bit one;
  - the slow grinder (16) is refused `-ask -max-strength 22`;
  - working the 20-bit id names `bounty.max_strength 16`.
- `TestMinuteStrength` (pkg/grind).
- By hand, with three nodes: the walk above, the messages above, and
  bench showing `minute_strength` 30 for SHA-256 at about 27 M/s.
- gofmt, go vet, staticcheck and `go test -race ./...` pass, as does
  GUI e2e `parity.mjs`; no flag was added.

## For Black

No rule changes. The byte in messages 71 and 73 is sent as before, and
the payer's `Listable` and take checks are untouched. Refusing the ask
only stops this node from asking for offers it would drop.

## For White

`poon grind bench`'s output gains an optional `minute_strength` field.
The bench runs locally, with no RPC. There are no file or wire format changes.

## Not done (ideas for later)

- A payer cannot see whether any connected grinder can work an offer.
  Payers could remember each peer's last stated strength from its list
  requests, and `poon bounty list` could flag an open offer that no
  connected grinder states it can work.
- The `bounty.max_strength` row in `poon policy limits` reads min 0, max
  255. The docs say 1 to 255, with 0 meaning unset.
