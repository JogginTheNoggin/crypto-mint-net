# RED-040: `Client.SubmitEntryHex` sends the body chunked, so the node never sees a Content-Length and ORANGE-025's sizing does not apply to `poo submit`

| Field | Value |
| --- | --- |
| Severity | low |
| Status | fixed |
| Found on | `main` @ b502960 (Source Round 16 Part B) |
| Component | pkg/rpc/client.go (`SubmitEntryHex`, `doReader`), pkg/rpc/rpc.go (`readSized`) |
| Surface | local operator (CLI to its own node, full token) |
| Fixed by | `claude/project-thread-ggo8dt` @ c2964ed (BLUE-017) |

## Summary

ORANGE-025 sizes the node's body buffer from `Content-Length`: it starts at
min(stated, 64 MiB) and grows "never past the stated size", so an honest
large entry costs about its own size. ORANGE-012 then added
`SubmitEntryHex`, which builds the body with `io.MultiReader(head, hex,
tail)` and hands it to `http.NewRequest`. Go sets `ContentLength` only for
`*bytes.Buffer`, `*bytes.Reader` and `*strings.Reader`; for any other reader
the request goes out with `Transfer-Encoding: chunked` and no length. The
node sees `r.ContentLength == -1`, `readSized` is called with `stated < 0`,
and the buffer doubles with no bound from 64 MiB on, copying at each step.
The one client that sends large entries (`poo submit -f`) is exactly the one
that defeats the sizing.

## Reproduction

`TestRedSubmitEntryHexIsChunked` (scratchpad `red_r16b_rpc_test.go.txt`, goes in
`pkg/rpc`): a server behind `Client.SubmitEntryHex` logs
`ContentLength=-1 TransferEncoding=[chunked]`.

`TestRedChunkedEntryBodyCost`, same file: a real handler, an entry of
100 MiB (200 MiB of hex), posted once with a `strings.Reader` body (length
known) and once wrapped in `io.MultiReader` (what `SubmitEntryHex` sends):

```
Content-Length: allocated 492 MiB (HeapSys 623 MiB)
chunked:        allocated 748 MiB (HeapSys 983 MiB)
```

That is 256 MiB more allocated and 360 MiB more heap for one 100 MiB entry
(the client's own copy of the body is in both numbers). The buffer ends up
at the next power of two above the hex, up to nearly twice the entry's
need, and the last doubling holds the old and the new buffer at once.

## Impact

Memory only, to the operator's own node, behind the full token; the node
does not fail, it just needs about 1.5 times what ORANGE-025 meant it to
need for the big entries it was built for, and more as the entry nears a
power of two. It is also a trap for the next change: a test of ORANGE-025
that posts a `strings.Reader` shows the good number while the real client
never sends that.

## Recommendation

In `SubmitEntryHex` set the length on the request: build it with
`http.NewRequest` and then `req.ContentLength = int64(len(head)+len(hex)+2)`
(`doReader` can take a `length` argument, `-1` for none), and set `GetBody`
if redirects matter (they should not). Add a test that goes through
`Client.SubmitEntryHex`, not a hand-made request, and checks the server saw
a `Content-Length`.

## Resolution

Fixed (D148). `Client.SubmitEntryHex` sets the body's `Content-Length`
(`doReader` takes a length), so `poo submit` no longer sends a chunked
body and the node's sizing applies. Test: `TestSubmitEntryHexSendsItsLength`,
through the client; on b502960 it fails with "Content-Length -1, chunked
true".
