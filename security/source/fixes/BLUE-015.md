# BLUE-015: `poon submit` reads a file or standard input; the full token may submit an entry of any size

| Field | Value |
| --- | --- |
| Fixes | RED-038 (low) |
| Branch | `claude/project-thread-ggo8dt` |
| Base | `claude/m10-watch` @ `184bc54` (Source Round 14 Part C), with GREEN-039 (`claude/source-green` @ `2455f5e`) merged |
| Decisions | D144 |
| Review | No consensus change. For Black (Part D): the RPC's body bound is lifted for one route with the full token. |

## Summary

- `poon submit entry|layer` takes the hex as one argument, as before, or
  from `-f FILE` or standard input (`-`, or `-f -`), read whole and
  trimmed. An entry over 64 KiB no longer hits Linux's 128 KiB limit on
  one argument.
- Testing a large body through the real RPC, as asked, found a second
  bound: `MaxBody` (8 MiB) refused any entry over about 4 MiB, from both
  `poon submit` and the GUI. `POST /v1/entry` with the full token is no
  longer bounded, since the node takes an entry of any size from a peer.
  Every other request, and every view-token request, is bounded as
  before.
- The node's own refusal of a view-token POST to `/v1/entry` is checked:
  403 before the body is read, however large the body.
- It is consistent with GREEN-039: `poon build -bytes` and `poon work
  -bytes` print what `poon submit` reads.

## What changed

- `pkg/cli/submit.go`: `-f`, `-`, and `submitHex`.
- `pkg/rpc/rpc.go`: `ServeHTTP` skips `MaxBytesReader` for a full-token
  `POST /v1/entry`.
- Docs: D144, `docs/modules/poon.md`, `docs/modules/rpc.md`.

## Tests added

- `TestSubmitEntryAndLayer` (pkg/cli), in-process through the real RPC:
  - a built entry through standard input, and the next one through `-f`,
    to a fresh node, which takes both;
  - 300 KB of hex on standard input, read whole and refused as an entry;
  - `-f` together with an argument, refused with the usage.
- `TestSubmitEntryBodySize` (pkg/rpc): an entry body over `MaxBody` with
  the full token is decoded (it fails on `184bc54` with "body: http:
  request body too large"); the same with the view token gets 403; a
  layer body over `MaxBody` is still refused; no token gets 401.

## What is left

- A submitted entry is decoded whole in memory, as one from a peer is
  (D117). Only the full token can send one.
- Not run with a valid entry over 64 KiB, since none can be built cheaply
  in a test. The paths are the same ones the tests use.
- staticcheck and govulncheck are not installed here; Grey runs them.
