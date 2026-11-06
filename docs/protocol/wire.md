# Wire protocol (wire version 1)

## Summary

This document is the byte-level specification of what full nodes send each
other over TCP: frames, the common header, the hello, pushed items and
notices, every ledger data request and reply, and the peer discovery
messages (follow requests and their relaying, peer lists, pings, dropped
policy updates, missing chunks). It implements the design's "Forwarding",
"Ledger Data" and "Peer Discovery and Management" documents. Items inside messages
(transactions, entries, layers) use the byte forms of
[encoding.md](encoding.md) unchanged. The code is `pkg/wire` (encoding) and
`pkg/p2p` (behaviour). Peer ranking, tracking and chasing (milestone 5)
added no message types, only the order requests are sent in; numbers are
never reused, and a node ignores types it does not know. The reasons for
the choices here are decisions D30 to D39, D40 to D49 and D50 to D62 in
[../decisions.md](../decisions.md).

Wire version 1 is stable as of release 0.1.0 (decision D76). Existing
message types, headers and bodies keep their bytes and meaning. New
behaviour comes as new message types, which older nodes ignore, or as a new
wire version that the hello offers. Defaults such as reply limits and peer
counts are node policy and may change between releases.

## Primitives

Integers are big-endian, as in [encoding.md](encoding.md). `Bytes` is a
`U32` length and the bytes; `List<T>` is a `U32` count and the items;
`Bool` is `0x00` or `0x01`. Decoders refuse trailing bytes, unknown flag
bits, and counts above the limits in `pkg/wire`.

## Frames

A connection is a stream of frames:

    U32 length ∥ message (length bytes)

No frame may exceed 16 MiB (`wire.MaxFrame`). A node closes a connection
whose next frame is larger. The first frame, the hello, may not exceed
3 MiB (`wire.MaxHelloFrame`: the longest hello D190's limits allow is
about 2.1 MiB). A reader allocates as the bytes arrive, not the size a
frame declares (D191).

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
| hops | `U16` | On a broadcast follow request, how many times it has been relayed; on a reply, how many relays it passed back through. 0 otherwise. |

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
| 2 | ping | `U64` nonce ∥ `U64` time ∥ extensions |
| 3 | dropped policy update | empty |
| 10 | tx | a transaction's byte form |
| 11 | entry | an entry's byte form |
| 12 | layer | a layer's byte form |
| 13 | notice | `Hash` tip |
| 14 | release | a release's byte form: `Hash` series ∥ `U64` part ∥ `Hash` previous ∥ witness (D168) |
| 20 | extension request | Common ∥ `Hash` from ∥ `Bytes` weight to beat ∥ `U8` variant |
| 21 | hash chain request | Common ∥ `Bool` has anchor ∥ [`Hash` anchor ∥ `U8` direction] ∥ `U32` count |
| 22 | entries by hash request | Common ∥ `U8` variant ∥ `List<Hash>` |
| 23 | full ledger request | Common ∥ `U8` variant |
| 24 | layers request | Common ∥ `List<Hash>` entries |
| 25 | weight check request | Common |
| 26 | parts request | Common ∥ `Hash` entry ∥ `U8` variant ∥ `U64` offset |
| 27 | layers-from request | Common ∥ `Hash` entry ∥ `U32` from |
| 30 | follow request | Follow request |
| 31 | peer list request | `U64` nonce ∥ `U64` time ∥ `U8` peer variant (1 or 2) ∥ discovery limits |
| 32 | missing chunks request | `List<U32>` positions (header: the original request id) |
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
| 50 | large | `Hash` entry ∥ `U64` size of its entry item |
| 51 | part | `U64` total ∥ `U64` offset ∥ the bytes to the end of the body |
| 52 | many layers | `Hash` entry ∥ `U32` count of its layers |
| 53 | layers from | `U32` total ∥ `U32` from ∥ `List<U64>` nonces |
| 60 | pong | Contact ∥ `List<Section>` (empty) ∥ extensions |
| 61 | partial pong | Contact ∥ `List<Section>` followed within the one asked about ∥ extensions |
| 62 | yes | Follow reply |
| 63 | partial yes | Follow reply |
| 64 | peer list | `List<Peer>` |

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

    U16 protocol ∥ Hash node id ∥ Bytes agent ∥ Bytes release
      ∥ U8 n ∥ n × (Bytes name ∥ U16 ver)  -- features
      ∥ U8 m ∥ m × (U16 tag ∥ Bytes value) -- extensions

(D186.) The release is the sender's version, such as `0.1.0-alpha.1`, at
most 64 bytes of `0-9 A-Z a-z . - +`, or empty when `p2p.announce_version` is `none`.
The features are what the sender supports beyond the protocol 1
baseline: at most 255, each a name of at most 32 bytes (`a-z`, `0-9`,
`-`) and a version, sorted by name with no repeats. The extensions are
optional fields a later release adds: at most 32, tags strictly
increasing, values at most 64 KiB (room for a post-quantum key or signature, D190). Ping, pong and partial pong end with
the same extension list. Nothing may follow the lists. These limits and
the release's characters are frozen with wire version 1 from the first
public tag (W-17, D190).

Feature names and extension tags are registered in `pkg/wire` (`hello.go`)
and never reused. Registered features: `layers-from`, `many-layers`,
`parts`, `release`. No extension tag is registered yet. A node skips a
feature or tag it does not know, and keeps no extension value of a tag
it does not know.

Release, features and extensions are for display and optional extras
only. They never change a verdict, weight, selection, a peer's standing
or ranking, or chasing, and no peer is refused for them; every node
speaks the whole protocol 1 baseline whatever it claims. A malformed
release, features or extensions list in a hello, ping or pong closes
that connection only, with no lasting offence (`wire.ErrBadExtras`,
D190); any other malformed message is an offence as usual.

Each side sends its hello first and sends nothing else until it has read
the other's, within 10 seconds. The node id is random per run. A node
closes the connection when the node id is its own, when it is already
connected to that node id (two nodes that dial each other at once both keep
the connection dialled by the node with the lower id), when it has too many
peers, or on a second hello.

The hello says nothing about what the sender follows. After it, each side
sends the other a direct follow request for each mint policy it follows (a
whole series as a series section, otherwise each followed pool). The
policies answered with a yes or partial yes are what the other node is a
peer for; a node closes a connection on which none was answered within
the follow timeout.

## Forwarding

A node forwards an item only after it has checked and kept it, only to
peers it verified for the item's pool, and never back to the peer
that sent it. It forwards its own items and items pushed to it; it never
forwards data it requested (or read back from its store). The seen list
stops an item going round a loop twice.

After a switch of its active ledger that was not reached by an item it
forwards, or that displaced entries, a node sends its peers a **notice**:
its new tip in the body and its new weight in the header. A peer decides
for itself whether to look into it. Notices are on by default
(`no_notices` in the node policy turns them off).

A **release** (variable supply phase 3, D168) carries the newest release
a node knows of a variable series, in the series' section (depth 2). A
node sends one for each such series it follows when a peer connects, and
passes on each new one it takes. The receiver drops a release of a series
it does not follow or of a part it already knows, with no hashing, and
checks any later one in full, witness first; one that does not check is
an offence. A node that does not know the type ignores it, as it ignores
every unknown type, so the protocol version stays 1. The release form is
in [encoding-variable.md](encoding-variable.md).

## Replies

Every reply is sent as one or more chunks with positions 1, 2, 3 … and the
last flag on the final one, including one-message replies such as "nothing
heavier". Chunks hold whole items: a chunk is cut only between entries,
hashes or the layers of one entry, and an item larger than the chunk size
is sent alone. The more marker on the last chunk says the server holds
more than the limits let it send.

An entry item larger than one frame can carry is sent **in parts** (D117):
an extension, full ledger or entries reply stops before it, and when it is
the first item the reply is a single **large** chunk naming its hash and
size instead. The asker sends **parts requests** for that entry from
offset 0, each answered by **part** chunks of the encoded entry item in
order (the more marker says it goes on past them), puts the bytes
together, decodes the item and checks its hash is the one named, then
asks on from that entry. Large items are never pushed: a node announces
one with a notice. There is no size at which a valid entry cannot travel.

An entry with more layers than one layers reply carries whole is treated
the same way (D121): a layers reply stops before it, and when it is the
first entry asked about the reply is a single **many layers** chunk with
its count. The asker sends **layers-from requests** from index 0, answered
by **layers from** chunks holding the nonces strongest first (the lower
nonce first among equals), with the more marker when more remain, and
asks again from where they stopped. Every reply carries the server's own
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

Milestone 5 adds no message types; it changes when and in what order the
catch-up requests are sent (decisions D52 to D54, D59, D60). Every stated
weight above the node's own, from a pushed item, a notice, a reply or a
**weight** request (sent to priority peers on each regular check, every
minute by default, and when a peer is verified), only queues a chase. A
chase then asks, from cheap to costly:

1. a priority peer, with an **extension** request from the node's tip,
   whether it sees something heavier too (skipped when the sender is a
   priority peer);
2. a **full ledger** request, when the node holds nothing for the pool;
3. **hash chains**: the latest hashes first; when none is held, the split
   is found by halving over the node's own ledger, asking for one hash
   after an entry it holds (anchor not held means the sender does not
   hold it), about log₂(height) requests at any depth (D116). There is no
   depth limit;
4. **layers** on the hashes after the split, as provisional work: their
   credit must beat the weight of the node's own ledger after the split
   before anything else is asked; they are not kept;
5. **extensions** from the split, each entry matching the hash listed for
   it. If the sender's ledger ends where the node's does, it holds more
   layers on the same entries, and the node asks for the layers on its
   latest entries instead.

It continues after a more marker with an extension from the last entry it
received. A chase that runs past its time slice yields its turn and goes
on later from where it stopped; nothing about a branch's length or depth
ends it (D116). Received entries are checked in chain order, each must name the
one before it (or the request's starting point) as parent, and they are
taken in through the engine as requested data: checked exactly like
pushed data, switched to only on computed weight, never forwarded.

## Peer discovery

### Discovery bodies

**Contact**: `Bool` reachable ∥ [`Bytes` "host:port"]. Only a reachable
node offers one; it is an offer, never a return address.

**Discovery limits**: `U32` chunk size ∥ `U32` list limit ∥ `U32` peer list
limit. As for ledger requests, 0 means the asker sets none, except the peer
list limit, where 0 asks for no peers.

**Peer variant**: 0 no peers, 1 full peers (peers that follow all of the
mint policy, or all of the part the replier follows), 2 all peers (each
with its own list of mint policies within the one asked about).

**Follow request**: `U64` nonce ∥ `U64` time ∥ `U8` peer variant ∥ `Bool`
broadcast ∥ Contact ∥ discovery limits. The header names the one mint
policy asked about. Broadcast marks a request followers may relay; it is in
the body, so in the request id, and no relay can change it.

**Follow reply**: Contact ∥ `List<Section>` (a partial yes: the replier's
mint policies within the one asked about, sorted) ∥ `Bool` more (the list
was cut short) ∥ `List<Peer>`. Later chunks of a direct reply carry only
more peers, with no contact and no list.

**Peer**: Contact (reachable) ∥ `List<Section>` (all peers variant; empty
for full peers).

Request ids, the seen list and the time window work as for ledger
requests. A node also drops discovery requests over its rate limit per
connection (120 a minute by default).

### Answering

A node answers a follow request or a ping with a yes (pong) when one of
its mint policies contains the one asked about, a partial yes (partial
pong) listing its policies inside it when it follows only part of it, and
silence otherwise. There is no "no". A follow request that arrived
directly (hop count 0) may get the peers its variant asks for, within the
peer list limit, in chunks; one that was relayed gets a single message
without peers that fits the asker's chunk size, cutting the list (and
setting more) if it must. A peer list request gets the replier's verified
peers for the mint policy, in chunks; the replier keeps the chunks for 2
minutes for a missing chunks request, which names the original request id
and the positions lost. Listed peers and contacts are candidates only.

### Relaying

A node relays a broadcast follow request only when it follows some or all
of the mint policy, only to its peers for that policy, never back to the
sender, and only while the hop count is below its hop limit (8 by
default); it adds 1 to the hop count. It remembers for 30 seconds which
connection the request came from and passes each reply back over it,
adding 1 to the reply's hop count. A relayed reply in chunks or with peers
is dropped. No other message is relayed.

### Pings and dropped policies

A node pings each peer for each verified mint policy (every 2 minutes by
default). After 3 silences in a row the peering for that policy ends; a
partial pong narrows it to the parts listed. A dropped policy update (empty
body, the header naming the policy) ends the peering for the policies
inside it. A peer with no verified policy left is disconnected.

## Misbehaviour

What a node does about a peer is node policy and never visible on the
wire beyond a closed connection. The reference node (decisions D55 to D58):
flags the address for an entry that breaks a rule or a layer below the
series' minimum strength, for going over its budget of a message kind,
for repeating held items or request ids, and for a provably false branch;
flags it and closes the connection for a malformed message or a request
whose id does not match; times out or blocks the address when its flags
pass the limits, refusing its connections until then; and closes a
connection whose send queue is full or that brings more reply chunks than
it can queue. A repeated request id is answered with refused. A refused
transaction is never a reason: a transaction can be refused for honest
reasons, such as a race with another spend.
