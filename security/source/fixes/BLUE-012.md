# BLUE-012: A learned pace is probed before it holds; trickled parts rank slow; follow writes take the lock

| Field | Value |
| --- | --- |
| Fixes | RED-028 (medium), RED-029 (medium), RED-030 (low) |
| Branch | `claude/project-thread-ggo8dt` |
| Base | `claude/m10-watch` @ `1ae02ca` (Source Round 11 Part C) |
| Decisions | D136 |
| Review | No consensus change (Red: none needs Black). RED-028 refines D133, and RED-029 adds a key ahead of D132's order. |

## Summary

This fixes Round 11's three findings. It adds no size or depth limit and
no heavier ledger is refused (D115). B4 (D132), C-6 and D133 still hold.

- **RED-028.** Two lost requests no longer pin an idle asker to one
  request a window. A pace learned from them is a probe: it doubles each
  window and ends at the normal pace. It holds only once the sender drops
  again, which is what a real budget does. A held pace is not lowered by
  drops that came with little sent.
- **RED-029.** A sender that trickles a part a chase, or lets parts
  requests time out, ranks as slow at the disk reserve, whatever size it
  claims, so an honest fetch that is moving takes its room.
- **RED-030.** `poon follow`, `poon unfollow`, `poon wallet follow` and
  `poon init -force` no longer write `node.json` behind a running node.

## What changed

- `pkg/p2p/pace.go`:
  - `reqWindow` gains `probing`, `raised` and `known`.
  - `dropped` decides between holding, probing and lowering. It lowers
    only with half of the pace sent.
  - `answered(now, pace, window)` doubles a probe once a window, and ends
    it at `pace`.
- `pkg/p2p/peer.go`: passes `RequestPace` and its window.
- `pkg/p2p/chase.go`:
  - `progress.partsSlow`, set at the save from `partial.got` and
    `partial.timedOut`; `partial.idle` is gone.
  - `partsMoving` (1 MiB).
  - `partsRank.slow` replaces `idle`, and the waiting chase ranks by its
    own `partsSlow`.
- `pkg/cli/policylock.go`: `lockPolicy`, which `grind.go` (D134) now uses.
- `pkg/cli/view.go`: `followCmd` takes the lock when it writes.
- `pkg/cli/wallet.go`: `walletFollow` goes through the node's follow RPC
  when the node answers, and takes the lock otherwise.
- `pkg/cli/node.go`: `init -dir` takes the lock.
- Docs: D136, `docs/modules/p2p.md` and `poon.md`.

## Tests added

- `pkg/p2p`:
  - `TestLearnedPaceRisesUnlessShown` (RED-028).
  - `TestTricklerJustUnderCannotHoldTheReserve` (RED-029). It fails when
    only "took nothing" counts as slow.
  - `TestReqWindowLearnsThePace` is updated for the new signature and
    behaves as before.
- `pkg/cli`: `TestPolicyWritesTakeTheLock` (RED-030).
- The D133 tests `TestSlowSenderIsReachedOverManyChases` and
  `TestChaseBelowSenderBudgetIsReached` passed 20 of 20 runs in a row
  under `-race`. Across all my runs after the rework, one earlier run
  timed out once in 23.
- My first version of the probe doubled on every reply. It overshot a's
  budget in `TestSlowSenderIsReachedOverManyChases` and failed it, so
  the rise is now once per window.

## How Grey can check it

```sh
go test -race -run 'ReqWindow|LearnedPace|Slow|BelowSender|Trickler|IdleConnected|Departed|DiskWait|Heavier' -v ./pkg/p2p
go test -race -count=10 -run 'TestSlowSenderIsReachedOverManyChases|TestChaseBelowSenderBudgetIsReached' ./pkg/p2p
go test -count=1 -run TestHeavierLedgerHuge ./pkg/p2p
go test -race -run 'PolicyWrites|OfflineSchedule|Wallet' ./pkg/cli
go test -race ./...
GOOS=windows GOARCH=amd64 go build ./... && GOOS=darwin GOARCH=arm64 go build ./... && GOOS=freebsd GOARCH=amd64 go build ./...
```

staticcheck and govulncheck are not installed here, so please run them.

## Open

- **RED-028.**
  - Red's `RequestPace/4` floor is not added. With a sender whose budget
    is below the floor, every learning would overshoot, and the flags
    would add up to a time-out (D133).
  - The pace still applies to every request to the peer, including the
    confirm and weight queries.
- **RED-029.** An honest sender on a link so poor that each chase takes
  in under 1 MiB, or times out, ranks with tricklers.
- **Black.** Red says none of this needs Black. RED-028 changes D133's
  "never rises while it lasts" for an unconfirmed learning only, so
  Black may want to look.
