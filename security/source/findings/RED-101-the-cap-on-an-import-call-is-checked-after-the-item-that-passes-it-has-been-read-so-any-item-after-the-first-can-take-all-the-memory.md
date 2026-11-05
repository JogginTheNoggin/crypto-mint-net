# RED-101: The cap on an import call is checked after the item that passes it has been read, so a second item of any size still takes the memory

| Field | Value |
| --- | --- |
| Severity | Low |
| Status | open |
| Found | Source Round 34 Part B, on `main` @ `f1ac406` (D189, RED-098's fix) |
| Area | `pkg/rpc/ledgerport.go` (`decodeImport`) |
| For Black | Not a rule. |

## What is wrong

`decodeImport` reads the body item by item and refuses the item that takes
the call over `MaxImportBatchBytes` (8 MiB) past the first. It does that
check **after** `dec.Decode(&b)`, so the decoder has already buffered the
whole JSON string and decoded it. The refusal message says "over 8 MiB past
the first item in one call", but the node has by then held the item, at
about four times its size. Only the first item was meant to be of any
size (D144); with the check in this place every item is.

## How I checked it

`pkg/rpc` test (throwaway, removed; copy kept with Red's notes): a call with
a first item `"AAAA"` and a second item of 32 and then 128 MiB of base64,
streamed, heap measured while the call ran:

```
second item 32 MiB:  status 400 (over 8 MiB past the first item)   peak heap 128 MiB over baseline
second item 128 MiB: status 400 (over 8 MiB past the first item)   peak heap 512 MiB over baseline
```

The caller holds the full token. This is the same cost RED-098 measured,
unchanged for the second item.

## Suggested fix (Blue's call)

- Put a limit under the decoder after the first item: wrap the body in an
  `io.LimitedReader` whose `N` is set, once the first item is read, to the
  batch bytes (times 4/3 for base64, plus the JSON around them), and
  refuse when it runs out. Then an over-long item is cut off at the limit
  and never held.
- Or read each later item's string token by token and stop at the cap.

## Not tested

A first item of any size (by decision D144).
