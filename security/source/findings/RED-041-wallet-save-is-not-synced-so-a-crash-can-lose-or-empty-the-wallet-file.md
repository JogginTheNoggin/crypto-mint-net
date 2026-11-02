# RED-041: `Wallet.Save` writes and renames without a sync, so a crash or power loss can leave an empty or old wallet file after a payment was sent

| Field | Value |
| --- | --- |
| Severity | medium |
| Status | fixed |
| Found on | `main` @ b502960 (Source Round 16 Part B) |
| Component | pkg/wallet/wallet.go (`Save`); also pkg/peers/peers.go, pkg/tracking/tracking.go |
| Surface | local (crash, power loss, kernel panic, VM stop) |
| Fixed by | `claude/project-thread-ggo8dt` @ c2964ed (BLUE-017) |

## Summary

`Save` does `os.WriteFile(w.path+".tmp", b, 0o600)` then `os.Rename`. It
never calls `Sync` on the file, and never syncs the directory. On a crash
the rename can reach the disk before the data does (ext4 without
`auto_da_alloc`, XFS, btrfs and others make no promise for a file that was
not synced), so wallet.json can be empty, short, or the old version after a
`Save` that returned nil. D145/D146 now make `Save` the place the record of
a payment lives: `pay` and `claim` submit to the node, then `Save`. A
crash after the node took the payment but before the data is on disk
brings back RED-039's shape (the wallet has no record, a rerun pays again)
and in the worse case an unreadable wallet file (`wallet: ...: unexpected
end of JSON input`).

The node policy writer already does this properly: `nodepolicy.go` (RED-013,
D114) writes a temporary file, `Sync`s it, renames, and syncs the directory.
The store has the same care (`store.go` `Sync` calls). The wallet, the
peer book (`peers.go:258`) and the tracker (`tracking.go:489`) use the
bare `WriteFile` + `Rename`.

## Reproduction

By reading; a crash cannot be made in-process. `Save`'s write path is
`WriteFile`, `Rename`, `w.read = b` (wallet.go ~lines 305-311); there is no
`Sync`, `CreateTemp`, or directory sync anywhere in pkg/wallet
(`grep -rn Sync pkg/wallet` is empty). I did not run a power-loss test.

## Impact

Only after a crash in a short window, but what is lost is the wallet's one
copy of its sent payments, new addresses and expected requests, and the
failure is silent until the next run. The seed is not lost (it is in the
sealed field written at creation, and a mnemonic backup restores it), so
this is loss of records and a possible double payment, not of keys, unless
the file is emptied and no backup exists. The peer book and tracker are
caches; losing them costs relearning, not money.

## Recommendation

Give the wallet the nodepolicy writer's steps: write the temp file, `Chmod`,
`Sync`, `Close`, `Rename`, then sync the directory (ignore the error where
the platform cannot, as on Windows). Prefer one shared helper (`atomicWrite`)
for wallet, peers, tracking and the schedule file, so the next writer is
durable by default. A fixed `.tmp` name is fine under the wallet lock.

## Resolution

Fixed (D148). A shared `pkg/fsutil.WriteFile` (temporary file of its own,
mode, `Sync`, `Close`, `Rename`, directory sync) is used by the wallet,
the peer book, the tracker, the policy manager and `nodepolicy.Save`.
Test: `TestWriteFile`. Not run: a real crash or power loss.
