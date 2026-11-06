# GREEN-040: `poon submit` takes the reply as printed; a missing wallet says what to do

| Field | Value |
| --- | --- |
| Status | adopted (D147) |
| Based on | `claude/m10-watch` @ `46bcb9e` |
| Area | `pkg/cli` (`submit`, `wallet`), `docs/modules/poon.md` |
| Effort | small |
| Consensus | none: the command line only |
| Decided by | Grey (Black may skip it) |

## Summary

Green used Round 15's merged work as a person would: BLUE-015's `submit
-f` and stdin input, GREEN-039's `-bytes`, and the wallet's lock and
passphrase checks. The passphrase checks behave as D145 says: an empty
passphrase is refused, and a piped one is read once. Two rough edges
remain, and both are prototyped with tests:

1. `poon build -bytes` prints a JSON reply, but `poon submit` read only bare hex. So the natural `poon build -bytes ... > r.json; poon submit entry -f r.json` failed with "not hex". The person had to pull out `.entry`, or `.layer.layer`, by hand or with `jq`.
2. Every wallet command run with no wallet failed with the bare `open .../wallet.json: no such file or directory`, without saying how to make one.

## Idea

**1.** `fromReply`: when the input starts with `{`, it is read as a reply.
An entry comes from the top-level `entry`. A layer comes from a work
reply's `layer` string, or from a build reply's `layer.layer`. A reply
with no bytes says "make it with -bytes". Bare hex is unchanged. One file
from `build -bytes -work` now feeds both submits:

```
poon build -pool 1 -bytes -work 3000 -root A > r.json
poon submit entry -pool 1 -root B -f r.json
poon submit layer -pool 1 -root B -f r.json
```

Checked by hand on two running nodes: the entry was taken, and the layer
switched B's tip.

**2.** `walletFlags.open` wraps a missing file: "no wallet (open ...):
make one with poon wallet create (or -restore), or name another with
-wallet". It keeps `errors.Is(err, os.ErrNotExist)` for any caller that
tests for it.

## Testing

- `TestSubmitFromReply`: entry and layer from a build reply, a layer from a work reply, bare hex unchanged, and a reply without the bytes or with broken JSON refused.
- `TestNoWalletSaysWhatToDo`: the hint is there, and the error still matches `os.ErrNotExist`.
- gofmt, vet, staticcheck, and `go test -race ./...` pass.

## Risks

None. Hex never starts with `{`, so no valid input changes meaning.

## Smaller note (not prototyped)

`poon submit entry|layer` (hand in an item) and `poon wallet submit FILE`
(send a co-signed payment) share a verb for different things. Calling
the wallet one `send` would read better, but it is a rename, so it is
left to Grey and GUI/Grey.
