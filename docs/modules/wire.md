# pkg/wire

## Summary

The byte form of the messages full nodes send each other: frames, the
common header with the mint policy section and stated weight, request ids,
the hello, every push, ledger request and reply body, and the peer
discovery messages. It only encodes and
decodes; what a node does with a message is in `pkg/p2p`. The full
specification is [../protocol/wire.md](../protocol/wire.md).

## API

- `Message{Header; Body}`, `Header{Type, Section, RequestID, Chunk, Flags,
  Stated, Hops}` with `Last()` and `More()`; `Encode`, `Decode`.
- `WriteFrame(w, m)`, `ReadFrame(r, max)`; `MaxFrame` is 16 MiB. A frame
  bounds a message, not an item: an entry larger than a frame travels in
  parts (`PartsReq`, `Large`, `Part`, `EncodeItem`, `DecodeItem`, D117).
- `Type` and its constants, with `IsRequest`, `IsReply`, `String`.
- `Section` with `All`, `IssuerSection`, `SeriesSection`, `PoolSection`,
  `MintSection`; `Covers` (None, Partial, Full) and `Contains`.
- `RequestID(type, section, body)`.
- Bodies: `Hello`, `Notice`, `DecodeTx` / `DecodeEntry` / `DecodeLayer`;
  requests `ExtensionReq`, `HashChainReq`, `EntriesReq`, `FullLedgerReq`,
  `LayersReq`, `WeightReq`, `PartsReq` (all with `RequestCommon{Nonce, Time, Limits}`)
  and `DecodeRequest`; reply chunks `EntriesChunk`, `HashChainChunk`,
  `HeldEntriesChunk`, `LayersChunk` with their `Decode*`, and
  `DecodeEmpty` for replies without a body.
- Discovery (`discovery.go`): `FollowReq`, `PeerListReq`, `Ping`,
  `MissingChunksReq`, `FollowReply`, `PongReply`, `PeerListChunk`, with
  `Contact`, `DiscoveryLimits`, `PeerVariant` and `PeerInfo`;
  `SortSections` / `LessSection` (the design's order), `Section.Text` and
  `ParseSection` (the text form used by registries), `CheckAddr`.
- `ErrMalformed` wraps every decoding failure; a peer that sends one is
  disconnected.

## Tests

`wire_test.go` and `discovery_test.go`: section coverage cases, round trips of the header and
every body, malformed inputs (bad version, flags, depth, trailing bytes,
oversized frames), request id stability, section text and order, and two fuzz targets:
`FuzzDecode` (a message that decodes encodes back to the same bytes) and
`FuzzBodies` (no body decoder panics on any input).

## Extending

A new message is a new type number (never reuse one) and a body type with
`Encode` and a `Decode` function; add it to the round-trip and fuzz tests
and to the protocol document. Receivers ignore unknown types, so new
messages need no version change; a change to an existing body does.
