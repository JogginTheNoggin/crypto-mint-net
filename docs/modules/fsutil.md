# pkg/fsutil

## Summary

`WriteFile(path, data, perm)` writes a file so that a crash leaves either
the old file or the new one, whole. It writes a temporary file of its own
beside `path` (so concurrent writers never tear each other's), sets the
mode, syncs it, closes it, renames it over `path`, and syncs the
directory so the rename is on disk too. Where a directory cannot be
synced (Windows) that step is skipped; the file itself is still synced.
A missing parent directory is made with mode 0700.

The node policy (`node.json`, D114), the policy manager's files, the
wallet, the peer book and the tracker all write through it (RED-041,
D148). Without the syncs, many file systems may put a rename on disk
before the data, so a crash could leave a file empty or short after a
write had returned.

## Tests

`TestWriteFile`: the file is replaced whole with the mode asked, no
temporary file is left, a missing directory is made, sixteen concurrent
writers never tear the file, and a failed write leaves no temporary file.
