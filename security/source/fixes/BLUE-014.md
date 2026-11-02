# BLUE-014: A probe throws away the whole reply; outages never hold a pace; waiting probes back off

| Field | Value |
| --- | --- |
| Fixes | RED-035 (high), RED-036 (medium), RED-037 (low) |
| Branch | `claude/project-thread-ggo8dt` |
| Base | `claude/m10-watch` @ `97456c1` (Source Round 13 Part C), with ORANGE-023 (`claude/source-orange-r13` @ `28f9931`) merged |
| Decisions | D142 |
| Review | No consensus change. For Black (Part D): RED-036 changes when D133's learned pace holds. |

## Summary

This fixes Round 13's three Source findings. It adds no size, count or
depth limit, and no heavier ledger is refused (D115). B4, B5, B6, C-6 and
D133 still hold.

- **RED-035.** ORANGE-023's fix, as Orange wrote it: once a probe throws
  away one part of a reply it throws away the rest, and every part is
  checked against the offset it should start at, kept or not. An honest
  sender whose replies hold many chunks is no longer flagged false.
- **RED-036.** Drops with nothing sent in the window answered are an
  outage, not a budget. They never hold, lower or confirm a pace, so two
  short outages at low load no longer pin the chase at one request a
  window.
- **RED-037.** A waiting chase whose measured chase found no room asks
  again after `partsStale`, doubled for each such chase in a row, up to
  eight times. A holder that slows, stalls or leaves is still cut at once.

## What changed

- `pkg/p2p/chase.go`: ORANGE-023's `want` offset in `fetchParts`;
  `progress.partsMiss`, counted in `fetchParts`; `measureLasts`, used by
  `mayTake`.
- `pkg/p2p/pace.go`: `dropped` checks `served` (more requests in the
  window than misses in a row) before it holds, lowers or uses `known`.
- `pkg/p2p/serve_test.go`: a raw test peer's writes stop quietly once
  the test's cleanup has closed its connection. This is the B5 failure
  on PR #59's CI (`write tcp ...: use of closed network connection` at
  `chase_test.go:35`, after the test had passed): the honest fetch
  reached its end, the cleanup closed the trickler's connection before
  its sender goroutine stopped, and that goroutine's write failed the
  test. The "entry item: encoding: trailing bytes" line before it is the
  expected end of a fake fetch, whose filler parts never decode as an
  entry; it is not RED-035.
- Docs: D142 and `docs/modules/p2p.md`.

## Tests added

- `TestProbeNeverFlagsAMultiChunkSender` (RED-035): replies of 4 chunks,
  with the probe beginning on the first, a middle and the last chunk. It
  fails on `97456c1` ("part 262144+65536 ..., want 196608") and passes.
- `TestProbeKeepsAnHonestSender` (ORANGE-023, Orange's).
- `TestOutageAtLowLoadNeverHolds` (RED-036): peak 2, a second outage with
  2 sent, then back to the configured pace; a peer that served some of the
  window still holds; an outage at the known pace does not.
- `TestMeasureLastsLongerAfterMisses` (RED-037).
- Ten runs in a row each, all passing: B5, B6 (`TestBlackColludingTricklersHoldTheReserve`), the trickler tests,
  `TestTwoSlowFetchesBothFinish`, `TestSlowSenderIsReachedOverManyChases`,
  `TestChaseBelowSenderBudgetIsReached`, the disk-wait tests and the two
  probe tests. The full `-race` suite, `TestHeavierLedgerHuge` and the
  cross-builds pass.

## Not taken

- RED-036's `RequestPace/4` floor for a hold: a sender whose budget is
  below it would be overshot every few windows and time this node out
  (D133).
- RED-037's record of a cut file's old offset: a cut file already resumes
  where it was cut, and the cut bytes are gone either way.

## What is left

- A sender that answers one request in a window, then loses two, looks
  like a budget of about that load (D133).
- A waiting sender that sped up waits up to 8 × `partsStale` to show it,
  unless the holder slows, stalls or leaves.
- A fast fresh sender cuts a slow file at its own rate: the bandwidth
  contest D140 names.
- staticcheck and govulncheck are not installed here; Grey runs them.
