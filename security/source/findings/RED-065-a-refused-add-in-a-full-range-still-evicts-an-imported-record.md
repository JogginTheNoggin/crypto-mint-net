# RED-065: A refused add in a full range still evicts an imported record

| Field | Value |
| --- | --- |
| Severity | Low |
| Status | open |
| Found | Source Round 22 Part B, on `main` @ `b997eb0` (the RED-058 fix in D158) |
| Area | `pkg/peers` (`Book.add`, `evictImportedLocked`) |
| For Black | No consensus rule is touched. |

## What is wrong

In `Book.add` the book-full check and the range-full check each evict an
imported record as a side effect while they test their condition:

```go
if len(b.recs) >= b.lim.MaxRecords && (st == Unlisted || !b.evictImportedLocked("")) { return ErrFull }
if b.inRangeLocked(...) >= b.lim.PerRange && (st == Unlisted || !b.evictImportedLocked(...)) { return ErrRangeFull }
```

When the book is full, the first check evicts the oldest imported record
anywhere and passes. If the new address's range is also full of records
that are not imported, the second check finds nothing to evict and returns
`ErrRangeFull`. The imported record is already deleted and nothing was
added.

## How I checked it

A book with `Limits{MaxRecords: 256, PerRange: 16, MaxUnlisted: 64}`: 64
imported records, 16 candidates from one range (66.6.1.x), and the rest of
the 256 from other ranges. Then `Add("66.6.2.0:1", ..., FromPeerList)`:

```
imported before: 64 book: 256
add 0 peers: too many from this network range imported: 63 book: 255
```

One imported record is gone, and the add failed. After that the book has a
free slot, so the next refused adds do not evict (255 < 256); the sender
would put one more address in elsewhere to fill the slot and repeat. That
costs the sender one new address a time, so it is slow, but each refused
message still removes a record the owner imported.

## What could happen

An address from a range that is already full can wear down the owner's
imported peers one record at a time, without ever adding to the book. It is
small next to RED-064 (where the same sender can fill the book outright),
but it is a plain correctness bug: a refused call changed state.

## Suggested fix (Blue's call)

Decide both checks first, then change state: work out whether the book is
full and whether the range is full, find the records to evict (one in the
range may also free the book slot), and only delete when the add will
succeed. A test: any call that returns an error leaves `Book.Records()`
equal to what it was.
