# WHITE-002: Nodes tell each other their version and what they support

| Field | Value |
| --- | --- |
| Status | proposed |
| Based on | origin/main @ 4c0f708 |
| Area | `pkg/wire` (hello, ping, pong), `pkg/p2p` (handshake, `PeerInfo`), `pkg/cli` (`poon peers`), `pkg/nodepolicy` (one setting), `docs/protocol/wire.md` |
| Effort | medium |
| Decided by | The owner chose the shape (2026-11-05: hello only, both extensible); Grey builds; Black to confirm it is not consensus |

## Summary

The owner asked for a way for nodes to know which version each peer runs
and what it supports, and (2026-11-05) to lock it into the protocol
**before the first release**, while nothing is in the field to break.
The owner chose: the version goes in the **hello only**, once per
connection, and **both hello and ping become extensible**. So, before
the first tag:

1. The hello gets two new fields: the sender's **release** (for example
   `0.1.0-alpha.1`) and a **features** list of named abilities, each with
   a number (for example `layers-from 1`).
2. The hello, ping and pong each end with an **extensions** list
   (tag, value), empty today. A decoder skips tags it does not know, so
   later releases can add fields to these messages without cutting off
   older nodes. Today they refuse any extra byte, which is why this has
   to happen before release.

The wire version byte stays 1: this becomes the frozen wire version 1
layout at release (D76 says v1 is stable "as of release 0.1.0", and no
release exists). Features only switch on optional behaviour between two
peers that both list them. They never change what is valid, heavier or
selected, and a node never refuses or ranks a peer because of its version.

## Problem

Today a node learns nothing usable about a peer's software. The hello
carries `Protocol` (always 1, never used to choose anything) and `Agent`
(always `poon`, "for logs only"). When a later release adds a new request
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

### Revision (2026-11-05)

The first draft avoided any layout change (step 1: put the version in the
`Agent` text; step 2: a separate `Features` message, type 4). The owner
pointed out that nothing is released, so the layout can still change,
and asked to lock it in. The owner then chose "hello only, both
extensible" over repeating the version in every ping: a peer's version
cannot change during a connection, since an upgrade restarts the node and
reconnects. This section replaces both steps. Type 4 stays unused.

### Byte forms (before the first tag)

```
Hello = U16 Protocol ∥ Hash NodeID ∥ Bytes Agent
      ∥ Bytes Release                      (new; up to 64 bytes)
      ∥ U8 n ∥ n × (Bytes Name ∥ U16 Ver)  (new; Features)
      ∥ Ext                                (new)

Ping  = U64 Nonce ∥ U64 Time ∥ Ext         (Ext new)
Pong  = its current fields ∥ Ext           (Ext new)

Ext   = U8 m ∥ m × (U16 Tag ∥ Bytes Value)
```

- `Release`: the sender's release version, empty when the operator hides
  it. When set it is 1 to 64 bytes of `0-9 A-Z a-z . - +`, otherwise the
  hello is malformed.
- `Features`: at most 64 entries, names up to 32 bytes of `a-z 0-9 -`,
  sorted and unique, otherwise malformed. Names are registered in
  `pkg/wire` like type numbers (never reused, never repurposed), each with
  a line in `docs/protocol/wire.md`. The first list names abilities that
  already exist (for example `parts`, `layers-from`, `many-layers`,
  `release`), so later releases can tell old from new.
- `Ext`: at most 32 entries, tags strictly increasing, each value up to
  1 KiB. A decoder keeps the tags it knows and skips the others. No tag is
  defined yet. A tag number, like a type number, is never reused.
- Decoders still refuse bytes after `Ext`, so strict decoding and the fuzz
  targets keep working. The new message is only a few bytes larger.
- `Agent` stays `poon`, the dial-back hello keeps `poon-self-check/TOKEN`
  (BLUE-032), and neither carries the release. `Protocol` stays 1; later
  versions raise it when a hello extension changes what peers may send.

### Showing it

- `PeerInfo` and `poon peers -o json` get `release` (left out when the peer
  sent none: never a guess) and `claimed_features`. `poon peers` and
  `poon peer` label both as the peer's own claim ("says it runs", "says it
  supports"). The GUI reads them from that JSON and parses nothing of its
  own (GUI/White).
- `poon peers` table output strips control and invisible characters from
  peer text (`plainText`).
- A count of connected peers per release (in `poon peers -counts` or a
  `-versions` view), so the owner can see what the network runs.
- `p2p.announce_version`: `release` (default) or `none` (send an empty
  `Release`), for operators who do not want to tell peers which release
  they run (a known-vulnerable version is easier to find). Machine-local,
  so a shared settings file never changes it.

### What features and versions may and may not do

- **May**: choose between two ways of asking for the same data, send an
  optional message only to peers that understand it, pick a bigger chunk
  or a faster path, show the owner what peers run.
- **May not**: change what is valid, how weight is counted or which
  ledger is selected; refuse, block or rank a peer down because of its
  version or features; stop asking a peer for a heavier ledger because it
  lacks a feature (it falls back to the baseline requests). A peer's
  release and features are its own claim and are never trusted for
  anything that matters: a lie can only make the node use the baseline,
  or send a message the peer then ignores.

With those limits, two nodes with the same ledgers still pick the same
one whatever their versions, so this is not a consensus change and adds
no hard-fork path. Black is asked to confirm.

### Steps for Grey

1. `pkg/wire`: the new hello, ping and pong fields and `Ext`, with fuzz
   targets and limits; `docs/protocol/wire.md` updated; a decision that
   amends D76's frozen layout before the first tag and says plainly that
   builds from before the change cannot talk to builds after it (Black).
   An unknown extension tag is skipped; a malformed one closes only that
   connection and is never an offence that outlives it (Black's Part D
   check).
2. `pkg/p2p`: fill `Release` and `Features` from `node.SetRelease` and a
   registered list; keep them on the peer; a helper
   `peer.has(name, ver)`.
3. `pkg/cli` and `pkg/nodepolicy`: the JSON fields, labels, counts and
   the `p2p.announce_version` setting.
4. Done in one Part A, before WHITE-001's tag. After the tag, the layout
   is frozen like the rest of wire version 1, and new fields go in `Ext`.

## Compatibility

| Case | What happens |
| --- | --- |
| Before the first tag | Builds from before this change and builds after it cannot talk to each other. Nothing is released and no outside nodes run, so that is acceptable. It is the last time the hello layout can change freely |
| After the tag, a newer node adds an `Ext` tag | Older nodes skip it; both keep working |
| A peer sends no release or no features | Shown as not shared; the node uses baseline behaviour |
| A peer lies about its features | The node falls back when a request is ignored or refused, as with today's timeouts |
| A future release drops a feature | It stops listing it and peers fall back. A feature is not removed without a deprecation release (W-8) |

## Cost and risk

Medium: three message layouts, their fuzz targets and vectors if any, a
field on the peer, JSON fields and one setting. The risk is doing it after
the tag, when it would break every released node, so it belongs in the
Part A before WHITE-001's release. Other risks: the privacy of the release
string (the setting covers it), and teams using features to gate
something that must not be gated (the rules above, checked by White and
Black each round).

## Prototype

None yet.
