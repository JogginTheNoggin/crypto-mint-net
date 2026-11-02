# ORANGE-026: Entry submit re-measured after D147; let the buffer end at the stated size

| Field | Value |
| --- | --- |
| Status | adopted (D149) |
| Based on | `main` @ `b502960` |
| Area | `pkg/rpc` (`readSized`) |
| Effort | one function |
| Decided by | Grey, with Black (D147's 64 MiB ceiling is kept) |

## Summary

I re-measured the node's peak memory on a 128 MiB entry submitted through
`POST /v1/entry`, after D147. GUI/Red measured about 1.3 GB (`VmHWM`)
before ORANGE-025.

| 128 MiB entry (256 MiB of hex) | Peak RSS (`VmHWM`) | Heap peak | Allocated | Time |
| --- | --- | --- | --- | --- |
| `46bcb9e` (before ORANGE-025) | 888 MiB | 1,180 MiB | 1,436 MiB | 4.4–5.7 s |
| `b502960` (D147) | 727–856 MiB | 668–773 MiB | 1,117 MiB | 1.6–5.1 s |
| This change | 542–547 MiB | 517–522 MiB | 893 MiB | 1.3–2.2 s |

My harness measures only the node's side, so its numbers are lower than
GUI/Red's 1.3 GB. (Its client streams the hex from a file.)

**What is left after D147.** 63% of the bytes the node allocates are
`readSized`'s buffer: 704 MiB to read 256 MiB.
- The buffer starts at the 64 MiB hint and doubles: 64, 128, 256 MiB.
  The stated length is 256 MiB plus the JSON around the hex, so the room
  has to grow once more, to the stated length.
- That last step copies the whole 256 MiB body to make room for its last
  98 bytes. At that moment both copies are held (512 MiB), and that
  moment is the peak.
- Any entry just over a power of two pays it, so any hex body just over
  64, 128 or 256 MiB does.

**The fix.** When the stated length is past the hint, `readSized` starts
at the stated length halved until it fits under the hint. The doublings
then end at exactly the stated length, and no step copies the body for
its last bytes. For this entry the buffer is 32, 64, 128 and then
256 MiB plus 99 bytes. That is 480 MiB allocated instead of 704, with
128 + 256 MiB held at the last step instead of 256 + 256.

D147's bounds are unchanged:
- A stated length sets aside at most the hint (64 MiB) before bytes
  arrive.
- Each step at most doubles what has arrived.
- The room never grows past the stated length once it has reached it.

**What is left after this change: about 520 MiB.** That is the body
(256 MiB of hex), the decoded bytes (128 MiB) and the decoded entry,
each held once. Decoding the hex over its own buffer would save another
128 MiB. I did not build that. It writes into bytes the JSON decoder
handed over, which `UnmarshalJSON`'s contract does not allow (as in
ORANGE-025). I also tried letting the decoded bytes go before
`HandleEntry`. It changed nothing: the peak is reached while decoding,
not after.

No consensus change. It changes only how the node's own API sizes one
buffer.

## Regressions

None. The only code that changed since ORANGE-025's measurement is in
`pkg/rpc` and the CLI, and the long p2p tests pass.

## Measurements

Same machine as ORANGE-012: 4 vCPU Xeon at 2.10 GHz, Go 1.27.3.

How they were taken:
- A test-only harness builds an entry of 191 claims of another series,
  704 KiB each: 128 MiB, written as 256 MiB of hex to a file.
- A second process runs `rpc.NewHandler` on an `httptest` server with
  the full token. It posts the JSON body streamed from that file, with
  its `Content-Length` set. The node decodes the entry and checks it,
  and refuses it with "transaction of another series", so the whole
  path up to `HandleEntry`'s check is measured.
- `VmHWM` is read from `/proc/self/status` before and after. The heap
  peak is sampled every millisecond, and the bytes allocated come from
  `runtime.MemStats`.
- Three runs each of `b502960` and of this change, alternated, and two
  of `46bcb9e`.

Without a `Content-Length` (chunked), `b502960` allocated 1,372 MiB
with a 924 MiB heap peak. This change does not touch that path: with no
stated length, the buffer doubles from the hint as before.

Where the bytes went on `b502960` (alloc profile, one run):

| Where | Share |
| --- | --- |
| `readSized`: 64 + 128 + 256 + 256 MiB | 63% |
| `hexBytes.UnmarshalJSON`: the decoded bytes | 11% |
| `tx.TID`'s body encoding (`encoding.NewWriter`), short-lived | 11% |
| `tx.DecodeFrom` and `Reader.Raw`: the decoded entry | 15% |

## Idea

- **`readSized`:** for `stated > hint`, the first room is `stated + 1`
  halved (rounding up) until it is at most `max(hint, 512) + 1`.
  Otherwise it is as before.
- **Tests:**
  - `TestReadSizedEndsAtStated`: a body of 1 MiB + 100 bytes with a
    64 KiB hint ends with room for exactly its size. It sets aside at
    most 2.25× its size over the read. On `b502960` it set aside 3.0×
    and fails.
  - `TestReadSized` (D147's bounds: a false length sets aside at most
    the hint or twice what came) passes unchanged.

## Cost and risk

- About 10 lines and a test.
- The full suite passes: `go test -race ./...`,
  `TestHeavierLedgerHuge`, `TestHeavierLedgerLongBareRun`,
  `TestTwoSlowFetchesBothFinish`, and `GOEXPERIMENT=simd` for `pkg/rpc`.

## Prototype

On `claude/source-orange-r16`; see the commit "ORANGE-026".
