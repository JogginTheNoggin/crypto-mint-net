# GREEN-039: Make `poon submit` usable with `poon` alone

| Field | Value |
| --- | --- |
| Status | adopted (D145), merged with BLUE-015 |
| Based on | `claude/m10-watch` @ `184bc54` |
| Area | `pkg/cli` (`build`, `work`, `submit`, `grind jobs`), `docs/modules/poon.md` |
| Effort | small |
| Consensus | none: the command line only. A submitted item is checked as one from a peer, as D143 says |
| Decided by | Grey (Black may skip it) |

## Summary

Green used Round 14's new commands as a person would. `poon submit entry`
and `poon submit layer` take "the hex the API prints", but no `poon`
command printed it. `poon build` and `poon work` remove the bytes from
their replies ("the hash is what a user needs; the bytes are for tools").
So the only way to feed `poon submit` was a tool calling the API, and
`TestSubmitEntryAndLayer` got its hex from `rpc.Client` directly. Three
small fixes, all prototyped with tests:

1. `poon build -bytes` and `poon work -bytes` keep the encoded entry, and the layer (`build -work`'s too), in the reply. Without the flag the replies are as before.
2. When the receiving node lacks the entry's parent, `poon submit entry` only said `ledger: parent not held`. It now names the missing parent and says what to do: submit that parent first, or let the node take the pool from a peer.
3. `poon grind jobs 3 -o table` said "usage: poon grind jobs [ID]", because flags after the id were not read. It now uses `parseMixed`, as `grind schedule` does.

## Problem and idea

**1.** The flow GREEN-038 had in mind was: make an entry or a layer on one
node, then hand it to another with `poon submit`. With poon alone:

```
poon build -pool 1 -bytes -work 4000 -root A   # prints entry and layer.layer
poon submit entry -pool 1 -root B HEX           # the entry
poon submit layer -pool 1 -root B HEX           # its layer
```

`addBytes` adds the same `-bytes` flag to both commands. `poon.md` lists it
on `build` and `work`, and the `submit` row says where the hex comes from.

**2.** The node's error has code `not_found` and the ledger's text.
`submit` matches the text of `ledger.ErrUnknownParent`, because the
error crosses the RPC as text. It returns that error with the parent's
hash and the two ways forward.

**3.** `grind jobs ID` was the only command with a positional argument
that read its flags with `fs.Parse` alone.

## Testing

- `TestSubmitEntryAndLayer` builds with `poon build -bytes -work` on node A, submits the entry and the layer on node B with `poon submit`, and checks the hashes. A `build` without `-bytes` prints no bytes. On a fresh node C, the same entry is refused with the hint.
- `TestGrindCommands`: `grind jobs ID -root R` reads the flag after the id.
- gofmt, vet, staticcheck, and `go test -race ./...` pass.

## Risks

None for the node. `-bytes` replies can be large for a big entry, which
is why it is opt-in.

## Smaller note (not prototyped)

There is no way to export an entry the node already holds, for example
the parent the hint names, by its hash. A `poon ledger -entry HASH -bytes`
(or an `entry` field on an existing read) would close that. It needs a
read of one stored entry through the API, so it is left to Grey.
