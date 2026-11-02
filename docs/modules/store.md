# pkg/store

## Summary

An append-only log of records for one pool, so a node can rebuild its
ledgers and pending transactions after a restart. Records are a kind
(entry, layer, transaction) and the item's canonical bytes, framed with a
length and a CRC-32C. The store holds bytes, not meaning: the node replays
them through the engine, which checks everything again (decision D21).

## Format

```
header : "PoO-log" 0x01
record : U8 kind ∥ U32 length ∥ data ∥ U32 CRC-32C(kind ∥ length ∥ data)
```

Kinds: 1 entry, 2 layer, 3 transaction. Numbers are never reused.

## API

- `Open(path, Options{NoSync})` returns the log and `Opened{Records, Cut}`.
  It creates the file and directory if needed (modes 0600 and 0700).
- `Append(Record)` writes and, unless `NoSync`, fsyncs.
- `Rewrite(records)` replaces the contents through a temporary file, fsync
  and rename, so a crash leaves the old log or the new one.
- `Records`, `Path`, `Close`. `ErrNotLog` for a file without the header.

## Guarantees

- A crash mid-append loses at most the record being written. `Open` keeps
  every record up to the first incomplete or corrupt one, cuts the rest,
  and reports how many bytes it cut.
- A corrupt length never causes a large allocation (`MaxRecordSize`, and
  the length must fit in the file).

## Tests

`store_test.go` cuts the last record at every possible byte, flips bytes,
writes impossible lengths, and checks rewrite and reopen.
