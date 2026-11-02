# ORANGE-025: Wallet lock and entry submit measured; read an entry body once

| Field | Value |
| --- | --- |
| Status | adopted (D147): read once as proposed; the buffer starts at 64 MiB and grows toward the stated length instead of reserving up to 1 GiB |
| Based on | `claude/m10-watch` @ `46bcb9e` |
| Area | `pkg/rpc` (`submitEntry`, new `readEntryBody`) |
| Effort | small |
| Decided by | Grey, with Red (the full token's body has no bound, D144) |

## Summary

This round I measured the two new paths of D145, and checked for
regressions against ORANGE-024.

**Wallet `Save` lock (GUI-019): no problem.** The lock (`flock`), the
re-read of the file and the comparison cost about 0.1 ms on a small
wallet. That is within noise for large wallets, where JSON encoding is
almost all of the time.

| `Save`, 4 vCPU, tmpfs | Before (`471de80`) | After (`46bcb9e`) |
| --- | --- | --- |
| 20 keys | 0.38–0.44 ms | 0.46–0.55 ms |
| 1,000 keys | 1.51–1.57 ms | 1.41–1.64 ms |
| 20,000 keys | 19–28 ms | 25–26 ms |

**The hex decoded from the body (D145) saves little.** D145 decodes the
hex as the JSON is read, so the entry is not also held as a string. But
`json.Decoder` still copies the whole body into its own buffer, growing
it step by step as it reads (`jsontext.(*decoderState).fetch`). That
buffer is most of the cost: 61% of the bytes allocated for an entry
submit.

| `POST /v1/entry` (junk entry, refused once decoded) | Before D145 (`471de80`) | D145 (`46bcb9e`) | This change |
| --- | --- | --- | --- |
| 7 MiB entry: time; peak heap | 168 ms; 70 MiB | 126 ms; 63 MiB | 47–52 ms; 21 MiB |
| 64 MiB entry | 2.15 s; 576 MiB | 0.85 s; 512 MiB | 0.46–0.49 s; 192 MiB |
| 256 MiB entry | 9.3 s; 2.3 GiB | 4.8 s; 2.0 GiB | 1.7–1.9 s; 768 MiB |

The peak heap includes the client's own copy of the body: 2× the entry,
as hex. A 256 MiB entry took 8× its size in memory; now it takes 3×
(the client's hex, the node's hex, the bytes). Submitting is also 2.5×
faster.

**The fix.** `submitEntry` reads the body once into one buffer and
decodes it there (`readEntryBody`):
- The buffer is sized by the request's `Content-Length`, up to
  `entryHint` (1 GiB). Beyond that, it grows as bytes arrive.
- The JSON is decoded with `encoding/json/v2`'s `Unmarshal`, using
  `encoding/json`'s own options (`DefaultOptionsV1`) and
  `RejectUnknownMembers`. That is the same JSON `decodeBody` accepts:
  unknown fields are refused, and so is anything after the value.
  `Unmarshal` reads straight from the buffer, so `hexBytes` gets the hex
  without another copy.
- Every other request still goes through `decodeBody` and `MaxBody`.

No consensus change. It changes only how the node's own API reads one
request.

## Regressions against ORANGE-024

None found. No `pkg/p2p` code changed between `184bc54` and `46bcb9e`,
and the long p2p tests pass (`TestHeavierLedgerHuge`,
`TestHeavierLedgerLongBareRun`, `TestTwoSlowFetchesBothFinish`).

## Measurements

Same machine as ORANGE-012: 4 vCPU Xeon at 2.10 GHz, Go 1.27.3.

How they were taken:
- **Wallet:** a test-only benchmark of `Wallet.Save` on a wallet made
  with `Gap` 20, 1,000 or 20,000 keys and 1 iteration, under `t.TempDir`.
  It ran 3 times each on `471de80` (before GUI-019) and on `46bcb9e`.
- **Entry submit:** a test-only harness on `rpc.NewHandler` with an
  `httptest` server and the full token. It posts
  `{"series":…,"pool":1,"entry":"00…"}` with N MiB of zeros. That is not
  an entry, so it is refused once decoded, which measures reading and
  decoding the body.
- The peak heap is sampled every 2 ms. The bytes allocated come from
  `runtime.MemStats`. Each case ran on `471de80`, `46bcb9e` and this
  change.

Where the bytes went on `46bcb9e` (alloc profile, entries of 1, 7, 64
and 256 MiB):

| Where | Share |
| --- | --- |
| `jsontext.(*decoderState).fetch`: the decoder's buffer, grown step by step | 61% |
| The test's client: building the body | 30% |
| `hexBytes.UnmarshalJSON`: the decoded bytes | 8% |

After the change, the node's share is one buffer of the body's size and
the decoded bytes.

## Idea

- **`readEntryBody(r, v)`:** described above.
- **`submitEntry`:** calls `readEntryBody` in place of `decodeBody`.
  `hexBytes` and the "entry: not hex" message are unchanged.
- **Tests:**
  - `TestEntryBodyReadOnce`: a 4 MiB entry allocates at most 20 MiB,
    against 36 MiB on `46bcb9e`, where the test fails. The check is
    skipped under `-race`. An unknown field, a second value, bad hex
    and a cut-off body are each refused as before.
  - `TestSubmitEntryBodySize` (RED-038) passes unchanged.

## For Red

- **The `Content-Length` hint.** A full-token request that states a
  large length and sends little makes the node set aside up to
  `entryHint` (1 GiB) before the bytes arrive. Go does not touch that
  memory until it is written, but it counts against the heap. Only the
  full token reaches this path, and D144 already lets it send a body of
  any size. If Red prefers, a smaller hint (64 MiB) bounds it. That
  costs the largest entries the growth again: 256 MiB took 1.0 GiB at
  its peak in 5.9 s, against 768 MiB in 1.9 s.
- **In-place decoding** would save another 1× the entry, by decoding the
  hex over its own buffer. I did not build it: it writes into bytes the
  JSON decoder handed over, which `UnmarshalJSON`'s contract leaves to
  the decoder.

## Cost and risk

- About 30 lines and a test.
- The full suite passes: `go test -race ./...`,
  `TestHeavierLedgerHuge`, `TestHeavierLedgerLongBareRun`,
  `TestTwoSlowFetchesBothFinish`, and `GOEXPERIMENT=simd` for `pkg/rpc`.

## Prototype

On `claude/source-orange-r15`; see the commit "ORANGE-025".
