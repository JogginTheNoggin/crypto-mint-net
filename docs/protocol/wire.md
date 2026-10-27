# Wire protocol (wire version 1)

## Summary

This document is the byte-level specification of what full nodes send each
other over TCP: frames, the common header, the hello, pushed items and
notices, and every ledger data request and reply. It implements the
design's "Forwarding" and "Ledger Data" documents. Items inside messages
(transactions, entries, layers) use the byte forms of
[encoding.md](encoding.md) unchanged. The code is `pkg/wire` (encoding) and
`pkg/p2p` (behaviour). Peer discovery, relaying and registries (milestone
4) and peer ranking (milestone 5) will add message types; numbers are never
reused, and a node ignores types it does not know. The reasons for the
choices here are decisions D30 to D39 in [../decisions.md](../decisions.md).

## Primitives

Integers are big-endian, as in [encoding.md](encoding.md). `Bytes` is a
`U32` length and the bytes; `List<T>` is a `U32` count and the items;
`Bool` is `0x00` or `0x01`. Decoders refuse trailing bytes, unknown flag
bits, and counts above the limits in `pkg/wire`.

## Frames

A connection is a stream of frames:

    U32 length ∥ message (length bytes)

No frame may exceed 16 MiB (`wire.MaxFrame`). A node closes a connection
whose next frame is larger.

## Header

Every message starts with the header, so a node can decide whether the
message concerns it before reading the body:

| Field | Bytes | Meaning |
| --- | --- | --- |
| version | `U8` | Wire version, 1. Any other value is malformed. |
| type | `U16` | Message type (table below). |
| section | Section | The mint policy the message is about. |
| request id | `Hash` | The request a reply answers, or the request's own id; zero on pushed items. |
| chunk | `U32` | Position of a reply chunk (1, 2, 3 …); 0 on other messages. |
| flags | `U8` | Bit 0: last chunk. Bit 1: more marker. Other bits must be 0. |
| stated weight | `Bytes` (≤ 64) | The sender's computed weight of its active ledger for the pool, minimal big-endian, empty for 0. |
| hops | `U16` | 0. For relaying, milestone 4. |

The body is the rest of the frame.

**Stated weight is never weight.** It is only a reason to ask (see
Catching up). A node switches ledgers only on weight it computed itself.

### Section

The mint policy section is the design's issuer / series / pool / mint path,
with wildcards from the right:

    U8 depth (0–4) ∥ issuer Hash (depth ≥ 1) ∥ series Hash (≥ 2) ∥ pool_id Hash (≥ 3) ∥ serial U64 (= 4)

A series that names no issuer has the zero hash as its issuer. Depth 0 is
"everything". A followed section F covers a message section S fully when,
at every level, F's field equals S's or F is a wildcard there; partially
when at every level the fields are equal or either side is a wildcard; not
at all otherwise.

Pushed items, notices, requests and replies about a pool carry its pool
section (depth 3). A node answers a request only for a pool section of a
pool it follows, and replies "not held" otherwise.

### Request ids

    request id = SHA-256("PoO-request-v1" ∥ U16 type ∥ section ∥ body)

SHA-256 always, whatever the series' hash. The hop count is left out
because it changes at each relay. The body holds a random nonce and the
time, so ids do not repeat. A request whose id does not match is
malformed. A node remembers ids for its request window (10 minutes by
default) and refuses a repeat, a request older than the window or more than
the clock skew (2 minutes) ahead.

## Message types

| Number | Type | Body |
| --- | --- | --- |
| 1 | hello | Hello |
| 10 | tx | a transaction's byte form |
| 11 | entry | an entry's byte form |
| 12 | layer | a layer's byte form |
| 13 | notice | `Hash` tip |
| 20 | extension request | Common ∥ `Hash` from ∥ `Bytes` weight to beat ∥ `U8` variant |
| 21 | hash chain request | Common ∥ `Bool` has anchor ∥ [`Hash` anchor ∥ `U8` direction] ∥ `U32` count |
| 22 | entries by hash request | Common ∥ `U8` variant ∥ `List<Hash>` |
| 23 | full ledger request | Common ∥ `U8` variant |
| 24 | layers request | Common ∥ `List<Hash>` entries |
| 25 | weight check request | Common |
| 40 | extension | entries chunk |
| 41 | nothing heavier | empty |
| 42 | heavier, not from this entry | empty |
| 43 | hash chain | `Bool` from start ∥ `List<Hash>` |
| 44 | anchor not held | empty |
| 45 | entries | `List<Hash ∥ Bool held ∥ [entry item]>` |
| 46 | layers | `List<Hash entry ∥ Bool held ∥ [List<U64> nonces]>` |
| 47 | weight | empty (the answer is the header's stated weight) |
| 48 | not held | empty |
| 49 | refused | empty |

**Common** starts every request: `U64` nonce ∥ `U64` time (Unix seconds) ∥
the asker's limits `U32` chunk size ∥ `U32` entry limit ∥ `U32` hash limit
∥ `U32` layer limit. A limit of 0 means the asker sets none; the server
then applies its own cap.

**Variant**: 0 entries only, 1 entries with layers. Direction: 0 before,
1 after.

**Entry item**: the entry's byte form ∥ `Bool` has layers ∥ [`List<U64>`
nonces]. A layer is an entry hash and a nonce, so a reply sends only the
nonces of the layers on each entry; the receiver rebuilds each layer and
checks its strength as usual.

**Entries chunk**: `List<entry item>`, in chain order.

## Hello

    U16 protocol ∥ Hash node id ∥ Bytes listen ∥ Bytes agent ∥ List<Section> coverage

Each side sends its hello first and sends nothing else until it has read
the other's, within 10 seconds. The node id is random per run. A node
closes the connection when the node id is its own, when it is already
connected to that node id (two nodes that dial each other at once both keep
the connection dialled by the node with the lower id), when the two
coverages share nothing, when it has too many peers, or on a second hello.
Coverage is the list of sections the sender follows: a whole series as a
series section, otherwise each followed pool.

## Forwarding

A node forwards an item only after it has checked and kept it, only to
peers whose coverage includes the item's pool, and never back to the peer
that sent it. It forwards its own items and items pushed to it; it never
forwards data it requested (or read back from its store). The seen list
stops an item going round a loop twice.

After a switch of its active ledger that was not reached by an item it
forwards, or that displaced entries, a node sends its peers a **notice**:
its new tip in the body and its new weight in the header. A peer decides
for itself whether to look into it. Notices are on by default
(`no_notices` in the node policy turns them off).

## Replies

Every reply is sent as one or more chunks with positions 1, 2, 3 … and the
last flag on the final one, including one-message replies such as "nothing
heavier". Chunks hold whole items: a chunk is cut only between entries,
hashes or the layers of one entry, and an item larger than the chunk size
is sent alone. The more marker on the last chunk says the server holds
more than the limits let it send. Every reply carries the server's own
stated weight. Replies come from the active ledger only.

| Request | Answer |
| --- | --- |
| Extension (from, beat) | Nothing heavier if the active ledger's weight is not above beat. Heavier, not from this entry if the ledger does not hold from (the pool_id counts as the start). Otherwise the entries after from, up to the limits. |
| Full ledger | The entries from the first, up to the limits. |
| Hash chain, no anchor, count | The latest count hashes, ending at the tip. |
| Hash chain, no anchor, no count | The hashes from the first entry, up to the hash limit. |
| Hash chain, anchor before, count | Up to count hashes just before the anchor. |
| Hash chain, anchor after | The hashes after the anchor, up to count or the tip. |
| Hash chain, anchor not in the active ledger | Anchor not held. |
| Entries by hash | Each hash with its entry, or not held. |
| Layers | Each entry's layer nonces, or not held. |
| Weight check | Weight, with the stated weight in the header. |

The "from start" flag of a hash chain is set when its first hash is the
pool's first entry.

## Catching up

A node asks:

- with a **full ledger** request when it holds nothing for the pool;
- with an **extension** request from its tip, beating its own computed
  weight, when it connects to a peer, on each regular check (every minute
  by default), when a pushed entry or layer builds on something it lacks,
  and when any message states a weight above its own.

It continues after a more marker with an extension from the last entry it
received. On "heavier, not from this entry" it finds the split with hash
chains: the latest hashes first, then pages back with the oldest hash as
anchor, until it finds the newest hash it holds; if the chain reaches the
first entry with no shared hash, the split is the pool_id. It then asks
for an extension from the split. If the reply brings no entries but the
peer still states more weight, the peer holds more layers on the same
entries, and the node asks for the layers on its latest entries.

Received entries are checked in chain order, each must name the one
before it (or the request's starting point) as parent, and they are taken
in through the engine as requested data: checked exactly like pushed data,
switched to only on computed weight, never forwarded.

## Misbehaviour

Until peer ranking (milestone 5), a node disconnects a peer that sends a
malformed message, a request whose id does not match, an entry that breaks
a rule, a layer below the series' minimum strength, a reply that does not
fit the request, or more reply chunks than it can queue; and a peer whose
send queue is full. It does not disconnect for a refused transaction: a
transaction can be refused for honest reasons, such as a race with another
spend.
