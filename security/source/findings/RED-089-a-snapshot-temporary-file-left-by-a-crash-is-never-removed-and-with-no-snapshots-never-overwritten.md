# RED-089: A `.snap.tmp` left by a crash is never removed, and under `no_snapshots` it is never overwritten either

| Field | Value |
| --- | --- |
| Severity | Low (disk space only) |
| Status | open |
| Found | Source Round 30 Part B, on `main` @ `cf8c859` (D178) |
| Area | `pkg/node/snapshot.go` (`writeCollected`), `pkg/node/node.go` (`loadPool`) |
| For Black | Not a rule. |
| Fixed by | |

## What is wrong

`writeCollected` writes `<index>.log.snap.tmp`, syncs it and renames it. A
crash between the first write and the rename leaves the temporary file, which
is as large as the snapshot (about 1 to 2 GB for 10^7 held entries, by
D178's own size per entry). Nothing at start looks for it. The next
successful snapshot write truncates and reuses the same path, so it goes away
then. With `no_snapshots`, or for a pool that stays below `snapshot_every`
records and is not closed cleanly, it stays for ever, and nothing reports it
(`poo status -counts`, `poo doctor`).

## How I checked it

Test (throwaway, removed): a 1 MiB `.snap.tmp` beside a pool log, the snapshot
removed, `no_snapshots` on, start and clean stop: the file is still there. With
`no_snapshots` off, the clean stop wrote a snapshot and the file was gone.

## Suggested fix

Remove `<index>.log.snap.tmp` when a pool's log is opened, whatever the policy
is (the rename is the only way it becomes a snapshot, so a leftover is never
wanted), and count it in `poo doctor`'s `snapshots` check if one is found.

## Not tested

A real crash during a write (the file was created by hand).
