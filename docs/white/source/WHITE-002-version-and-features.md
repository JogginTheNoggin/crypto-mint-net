# WHITE-002: Nodes tell each other their version and what they support

| Field | Value |
| --- | --- |
| Status | proposed |
| Based on | origin/main @ 4c0f708 |
| Area | `pkg/wire` (hello, a new message type), `pkg/p2p` (handshake, dispatch, `PeerInfo`), `pkg/cli` (`poo peers`), `pkg/nodepolicy` (one setting) |
| Effort | small (step 1), medium (step 2) |
| Decided by | Grey; Black to confirm it is not consensus |

## Summary

The owner asked for a way for nodes to know which version each peer runs
and what it supports, so later releases can add optional abilities
without guessing. White suggests two steps. **Step 1** (for the first
alpha, no wire change): the hello's free-text `Agent` becomes
`poo/0.1.0-alpha.1`, shown per peer in `poo peers`. **Step 2** (a later
round): a new optional message, `Features`, sent right after the hello,
listing named abilities with a number each (for example `layers-from 1`);
nodes that speak it say so by sending hello `Protocol` 2. Older nodes
already ignore message types they do not know and accept any `Protocol`
of 1 or more, so nothing breaks for them. Features only switch on
optional behaviour between two peers that both list it; they never change
what is valid, heavier or selected, and a node never refuses or ranks a
peer for its version.

## Problem

Today a node learns nothing usable about a peer's software. The hello
carries `Protocol` (always 1, never used to choose anything) and `Agent`
(always `poo`, "for logs only"). When a later release adds a new request
or a new optional reply, it cannot tell whether a peer would understand
it, so it has to send and wait for a timeout, or not use the new ability
at all.

Constraints found in the code:

- `DecodeHello` and `DecodePing` refuse trailing bytes (`finish`), so
  adding a field to either would make every older node drop the
  connection as malformed. Changing the hello or ping layout is out.
- The wire header version byte and header flags must match exactly, so a
  header change is out too (W-4).
- Unknown message types are ignored before any budget is charged
  (`pkg/p2p` dispatch: "Unknown types are ignored, so later versions can
  add messages"), and type numbers 4 to 9 are unused. A new type is
  therefore safe with older peers.
- `Hello.Protocol` is documented as "the highest protocol version the
  sender speaks", and older nodes accept any value of 1 or more.

## Idea

### Step 1: a structured agent (alpha, no wire change)

- `Agent` becomes `poo/VERSION`, where VERSION is the release version
  (`pkg/cli.Version`, through the existing `node.SetRelease`), for example
  `poo/0.1.0-alpha.1`. Older nodes accept any agent up to 256 bytes.
- `PeerInfo` already has `agent`; `poo peers` (table and JSON) shows it.
  Add a parsed `version` field next to it in the JSON, so the GUI reads
  it from `poo peers -o json` without parsing anything itself (GUI/White).
- `poo status -counts` (or a `poo peers -versions` view) gives a count of
  connected peers per agent, so the owner can see what the network runs.
- A setting `p2p.announce_version`, `release` (default) or `name`
  (`poo` only), for operators who do not want to tell peers which release
  they run (a known-vulnerable version is easier to find). Machine-local,
  so a shared settings file does not change it.

### Step 2: a Features message (later round)

- A node that supports it sends hello `Protocol` 2.
- After both hellos, a node sends `Features` (type 4, not a request, no
  reply) to a peer whose hello said `Protocol` 2 or more. It is sent once
  per connection; a second one is ignored.
- Body: `U8 count` then `count` entries of `Bytes name ∥ U16 version`,
  names up to 32 bytes of `a-z 0-9 -`, at most 64 entries, sorted and
  unique. Anything else is malformed. That keeps it under about 2.2 KB.
- Each node keeps the peer's list in its peer record. An ability is used
  with a peer only when both list it with a version both support (the
  lower of the two). A peer that sends no `Features` has none: baseline
  protocol 1 behaviour, which every node keeps forever.
- Names are registered in `pkg/wire` (like type numbers: never reused,
  never repurposed), each with a one-line meaning in
  `docs/protocol/wire.md`. The first list would name abilities that
  already exist, so later releases can tell them apart from new ones
  (for example `parts`, `layers-from`, `many-layers`, `release`).
- `poo peers -o json` shows each peer's features list; `poo peer` shows
  it for one peer.

### What features may and may not do

- **May**: choose between two ways of asking for the same data, send an
  optional message only to peers that understand it, pick a bigger chunk
  or a faster path, show the owner what peers run.
- **May not**: change what is valid, how weight is counted or which
  ledger is selected; refuse, block or rank a peer down because of its
  version or features; stop asking a peer for a heavier ledger because it
  lacks a feature (it falls back to the baseline requests). A peer's
  agent and features are its own claim and are never trusted for
  anything that matters: a lie can only make the node use the baseline,
  or send a message the peer then ignores.

With those limits, two nodes with the same ledgers still pick the same
one whatever their versions, so this is not a consensus change and adds
no hard-fork path. Black is asked to confirm in Part D.

## Compatibility

| Peer pair | What happens |
| --- | --- |
| old (protocol 1) and new | Old ignores the new agent text; new sees `Protocol` 1 and sends no `Features`; both use baseline |
| new and new | Both send `Features`, use what both list |
| new node, peer lies about features | Falls back when a request is ignored or refused; same as today's timeout handling |
| a future node drops a feature | It stops listing it; peers fall back. A feature is never removed without a deprecation release (W-8) |

## Cost and risk

Step 1 is a few lines (the agent string, a JSON field, one setting).
Step 2 is one small message type with a fuzzed decoder, a field on the
peer, and a helper `peer.has(name, version)`. Risk: privacy of the
version string (the setting covers it), and teams using features to
gate something that must not be gated (the rules above, checked by White
and Black each round).

## Prototype

None yet. White suggests step 1 for the first alpha (WHITE-001 R6) and
step 2 when the first new optional ability needs it.
