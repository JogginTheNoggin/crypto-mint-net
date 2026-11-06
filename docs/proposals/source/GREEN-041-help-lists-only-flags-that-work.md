# GREEN-041: Help lists only the flags a command takes

| Field | Value |
| --- | --- |
| Status | adopted (D149) |
| Based on | `main` @ `b502960` |
| Area | `pkg/filter` (`ItemFlags`), `pkg/cli` (views, `grind start`/`run`), `docs/modules/poon.md`, `docs/modules/filter.md` |
| Effort | small |
| Consensus | none: the command line only |
| Decided by | Grey (Black may skip it) |

## Summary

GUI Green found that `pools`, `pending`, `unspent`, `ledger`, `tx` and
`address` all list `-kind` in their `-h`, then refuse it with "-kind
selects events and applies to poon watch only". The views and `watch`
share one flag set, and the refusal came after parsing. Green found the
same slip in `grind run`: it listed `-lease` (whose help began with a
stray "start:") and refused it with "-lease is for poon grind start".
GREEN-015 fixed this kind of flag for `peer`, `policy` and `sim`. Here
the fix is to never register the flag, so help, completion and the GUI's
flag lists all show only what works.

## Idea

- `filter.ItemFlags(fs)` adds every filter flag except `-kind`. `filter.Flags` is unchanged and is still used by `watch`. `addView(fs, events)` picks between them. The refusal in `viewCmd` is gone, because the flag no longer exists there. A `-kind` given to a view now fails at parsing like any unknown flag.
- `addJob(fs, lease)` adds `-lease` only for `grind start`, with the stray "start:" removed. `grind run` keeps its job's lease itself (RED-019), as before.
- `-until-from`'s help now says it works with an entry or a `tx:TID` target, and that a `tx:TID` target needs it. It used to say "with an entry target".
- Docs: `poon.md`'s filter paragraph adds the `grind` kind (it was missing) and says the views do not offer `-kind`. `filter.md` names `ItemFlags`.

## Testing

- `TestKindOnlyOnWatch`: none of the six views lists `-kind` in `-h`, `watch` does, `grind start` lists `-lease`, and `grind run` does not.
- The existing tests that expect `pending -kind` and `grind run -lease` to fail still pass. Both now fail at parsing.
- gofmt, vet, staticcheck, and `go test -race ./...` pass.

## Risks

A script that passed `-kind` to a view got an error before and still
does, so behaviour is the same. The GUI, if it builds a view's flag list
from `-h` or the completion probe, now offers one flag fewer, which is
the point.

## Still open from GREEN-020

These were not taken in D119 and are still true on `main`:

- Errors name the JSON field (`until_from`) where the person typed `-until-from`.
- No command renews a lease, so `grind start -lease` from a shell only means "stop after this long".
