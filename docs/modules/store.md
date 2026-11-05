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
- `AppendLater(Record)` writes and returns a ticket; `Durable(ticket)`
  waits until that record is on disk. `Durable` may run beside the other
  methods, and callers waiting together share one fsync (group commit,
  ORANGE-005, D105).
- `Rewrite(records)` replaces the contents through a temporary file, fsync
  and rename, so a crash leaves the old log or the new one.
- A failed write or fsync stops the log (`ErrFailed`, `Err`): it takes no
  more appends, `Durable` never reports a later record durable and never
  retries the sync, and `Rewrite` and `Close` return the error. Only
  reopening, which replays what is on disk, brings it back (RED-010, D108).
- `Records`, `Path`, `Close`. `ErrNotLog` for a file without the header.

### Locations (D172)

- Every record has a `Loc{Gen, Off, Len}`: the file's generation (each
  compaction starts a new one), the offset of its first frame and the
  bytes its frames take, parts included.
- `OpenFunc(path, opt, fn)` hands each record, its location and the log
  to `fn` as it reads them, holding none; `fn` may `ReadAt` a record
  already handed over. `Open` is `OpenFunc` collecting the records.
- `AppendLaterAt(Record)` is `AppendLater` that also returns the location.
- `ReadAt(loc)` reads one record back, checking every frame's checksum,
  beside appends and `Durable`. A location from an older generation or
  past the end gives `ErrStaleLoc`; bytes that are not one whole record
  give `ErrBadRecord`. Neither is ever read as another record.
- `RewriteFunc(n, next)` asks for the n records one at a time (`next` may
  `ReadAt` the old file, which is replaced only at the end) and returns
  their locations in the new generation. `Rewrite` wraps it.

- `OpenFuncAt(path, opt, From{Ready}, fn)` (D178): `Ready` runs once the
  file is open, may `ReadAt`, and returns the offset and record count a
  snapshot covers; only the records after it are read. `Tag(path, off)`
  digests the offset and the first and last 4 KiB before it, so a
  snapshot can tell whether the log still holds what it covers.
  `Size()` is the log's length.

## Guarantees

- A crash mid-append loses at most the record being written. `Open` keeps
  every record up to the first incomplete or corrupt one, cuts the rest,
  and reports how many bytes it cut.
- A corrupt length never causes a large allocation (`MaxRecordSize`, and
  the length must fit in the file).
- No item is too large to store: one larger than `MaxRecordSize` is written
  as `KindPart` records and a last record of its own kind, read back as
  one record. A crash that leaves only some parts loses that item and cuts
  the log before them (D117).

## Tests

`store_test.go` cuts the last record at every possible byte, flips bytes,
writes impossible lengths, and checks rewrite and reopen.
`readat_test.go` reads records back by location, parts included, refuses
locations off by one, too long, stale after a compaction or over corrupt
bytes, and checks that locations match after a reopen and appends.
