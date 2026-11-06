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

Used by the wallet file (`wallet.File`) and the payment file
(`wallet.Unsigned` and its legs).
