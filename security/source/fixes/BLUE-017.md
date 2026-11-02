# BLUE-017: Durable writes everywhere; submit sends its length; waiters say why

| Field | Value |
| --- | --- |
| Fixes | RED-041 (medium), RED-040 (low), RED-042 (low) |
| Branch | `claude/project-thread-ggo8dt` (PR against `main`) |
| Base | `main` @ `b502960` (Source Round 16 Part C) |
| Decisions | D148 |
| Review | No consensus change and no limit. |

## Summary

- **RED-041.** A new `pkg/fsutil.WriteFile` writes a temporary file of
  its own, syncs it, renames it over the target and syncs the directory.
  The wallet, the peer book, the tracker, the policy manager and
  `nodepolicy.Save` all use it now, so a crash leaves the old file or the
  new one and never an empty one.
- **RED-040.** `Client.SubmitEntryHex` sets `Content-Length`. `poo
  submit` no longer sends a chunked body, so the node's ORANGE-025
  sizing applies. This is independent of ORANGE-026 (PR #74), which
  changes the node's side.
- **RED-042.**
  - `status` and `receive` no longer hold the wallet while they ask the
    node. They hold it only to save, and `status` keeps a leg state that
    another command changed in the meantime.
  - Any command that has to wait for the lock prints "waiting for another
    poo wallet command (or the GUI) to finish with …" on its error
    output, then waits.

## What changed

- `pkg/fsutil` (new) with `fsutil.md`.
- `pkg/wallet/wallet.go`: `Save` uses `fsutil.WriteFile`. New `OnBusy`.
  `Hold` and `Save` pass the busy notice to `lockFile`.
- `pkg/wallet/lock_unix.go` and `lock_windows.go` try the lock without
  waiting first (`LOCK_NB`, `LOCKFILE_FAIL_IMMEDIATELY`).
- `pkg/peers/peers.go`, `pkg/tracking/tracking.go`,
  `pkg/nodepolicy/manager.go` and `nodepolicy.go` use `fsutil.WriteFile`.
- `pkg/rpc/client.go`: `doReader` takes a length, and `SubmitEntryHex`
  passes it.
- `pkg/cli/wallet.go`:
  - `holdWallet` replaces `w.Hold()` and adds the notice.
  - `status` and `receive` hold the wallet only around their saves.
- Docs: D148, plus `wallet.md`, `nodepolicy.md`, `fsutil.md` and the
  modules index.

## Tests added

- `TestWriteFile` (pkg/fsutil).
- `TestSubmitEntryHexSendsItsLength` (pkg/rpc). It goes through the real
  client. On `b502960` it fails with "Content-Length -1, chunked true".
- `TestHoldSaysWhenBusy` (pkg/wallet).
- `TestHeldWalletWaitersSayWhy` (pkg/cli), against a running node. While
  another process holds the wallet, `status` answers without waiting.
  `address` says it is waiting, finishes only after the release, and
  then saves.

## Not taken

- A time limit on the wait. The holder is saving a payment's record, so
  the waiter says why it waits and keeps waiting. D148 gives the
  reasons.

## What is left

- A `status` that has a leg to save still waits for a holder, and says
  so.
- Not tested: a real crash, and Windows (built and vetted only).
- staticcheck and govulncheck are not installed here, so Grey runs them.
