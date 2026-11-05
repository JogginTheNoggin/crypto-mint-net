# RED-098: A ledger import call has no bound on its body (the node holds about three times what is sent), and `poo ledger import` reads the whole file into memory, though export streams

| Field | Value |
| --- | --- |
| Severity | Low |
| Status | open |
| Found | Source Round 33 Part B, on `main` @ `941049d` (D188) |
| Area | `pkg/rpc/rpc.go` (`ServeHTTP`, the `/v1/ledger/import` case), `pkg/rpc/ledgerport.go`, `pkg/cli/ledgerport.go` (`ledgerImportCmd`) |
| For Black | Not a rule. |

## What is wrong

1. **The node.** `/v1/ledger/import` is left out of the body limit, like
   `/v1/entry` (RED-038, D144). `MaxImportItems` (256) bounds the number of
   items, not their size, so a call may hold 256 items of any size. `decodeBody`
   reads the whole value into the decoder's buffer and then decodes the
   base64 into a second slice, before `wire.DecodeItem` or any check sees a
   byte. An item over the series' size limits is refused only after that.
2. **The command.** `ledgerImportCmd` does `json.NewDecoder(r).Decode(&f)`
   on the whole file, so every item of the file is in memory at once,
   though `poo ledger export` takes care to stream ("a long ledger never sits
   whole in memory"). A ledger that exports fine can be too big to import, and
   a file from someone else of any size is read whole before the first
   check.
3. The header and per-entry summary of a file (`height`, `tip`, `weight`,
   `hash`, `tx_count`) are never compared with the items, and there is no way
   to look at a file without importing it (no dry run). A reader of the
   summary can be told something else than the items hold. Nothing is
   trusted by the import, so this only misleads a human reviewer.

## How I checked it

`pkg/rpc` test (throwaway, removed; copy kept with Red's notes): a POST to
`/v1/ledger/import` with one item streamed as base64 text (the item is not
valid, so the node answered 400 after reading it all), heap measured while
the call ran:

```
item 32 MiB:  body 42 MiB   → peak heap 128 MiB over baseline
item 128 MiB: body 170 MiB  → peak heap 512 MiB over baseline
```

About three times the body. The call came from the full token's holder, so
this is a bound on what a file can cost, not an open door. The command's
read-everything behaviour is from reading the code; I did not run it on a
large file.

## Suggested fix (Blue's call)

- Bound the call: a body cap that suits 256 items of the series' largest
  entry plus their layers, or stop reading once the items so far exceed the
  series ceiling. If an unbounded entry is wanted (D144), say so for
  import, and cap the total.
- Make the command read the file as a stream (`json.Decoder.Token`) and
  send a batch when it is full, as export writes it.
- Let `poo ledger import -dry-run` read and check a file against the node
  without keeping anything, or print the summary computed from the items.

## Not tested

The command on a file of several GiB; peak memory of the real
`poo ledger import`.
