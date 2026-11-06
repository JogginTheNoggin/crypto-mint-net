# pkg/keepjson

## Summary

Lets a JSON file keep what a later release adds (W-23, W-24, D195).
`Split(raw, v)` decodes `raw` into `v` and returns the top-level fields
`v` does not know, matched without regard to case as `encoding/json`
does. `Join(b, kept)` writes those fields back after the known ones, in
sorted key order, skipping any key `b` already has.

A file with more than 256 unknown keys, more than 64 KiB of them, or
nesting deeper than 64 is refused, so a hostile file cannot make a tool
hold or write back an unbounded amount.

`SplitValid` and `JoinValid` do the same for bytes already known to be
valid JSON, as an `UnmarshalJSON` method receives them or `json.Marshal`
returns them, without checking them again (ORANGE-047 P2, D200). The
wallet's `Open` and `Save` decode and encode the file directly, so its
bytes are checked once, and write the same bytes `json.MarshalIndent`
would (ORANGE-047 P3).

Used by the wallet file (`wallet.File`) and the payment file
(`wallet.Unsigned` and its legs).
