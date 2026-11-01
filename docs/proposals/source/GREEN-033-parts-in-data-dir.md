# GREEN-033: Put large-entry parts in the data directory, and document `layer_rate`

| Field | Value |
| --- | --- |
| Status | proposed (prototyped) |
| Based on | `claude/m10-watch` @ `ae8a103` |
| Area | `pkg/daemon`, docs (`p2p.md`, `nodepolicy.md`) |
| Effort | small |
| Consensus | none. It changes where the parts of a large entry are written, not what is fetched, checked, accepted or selected. It touches the D117 and D120 fetch path, so Black should look first |
| Decided by | Black (Part D), then Grey |

## Summary

D120 writes the parts of a large entry to a temporary file in
`p2p.Config.PartsDir`, so that a big entry does not sit in memory. But
`poo node` never sets `PartsDir`, so the parts go to the system's
temporary directory (`/tmp`). That has two problems:

- On many systems `/tmp` is a RAM disk (tmpfs) that is small and shared with other users, which works against the reason the parts go to disk.
- A node that crashes or is killed leaves its `poo-parts-*` files there, and nothing removes them. An entry can be gigabytes (D117 sets no size limit).

The prototype makes `poo node`, and the GUI, which runs the same daemon,
use `parts/` in the node's data directory (mode 0700, beside the store),
and remove leftover `poo-parts-*` files there on start. Prototyped, with
a test.

The same pass found that `grind.layer_rate` (D122) was in the code and in
`poo.md`, but not in `nodepolicy.md`, which documents every node.json
field. It is added there.

## Problem

`pkg/p2p/host.go` says `PartsDir ""` means "the system's temporary
directory". `pkg/daemon/daemon.go` builds the host config from
`p2p.FromPolicy` and sets only the logger, book and tracker. So every
node writes its parts to `os.TempDir()`. The host removes a file when its
fetch ends or the host stops (RED-016), but a `kill -9`, a power cut or a
panic skips that. On the next start the node creates new files and never
looks at the old ones. On a tmpfs `/tmp`, a large entry also takes RAM,
and it can fill a disk that other programs share.

## Idea

- `daemon.partsDir(dir)` makes `<data dir>/parts` with mode 0700 and removes the `poo-parts-*` files in it. Only that pattern is removed, never other files. It logs how many it removed.
- `runOnce` sets `cfg.PartsDir` to it before `p2p.New`. A reload runs it again, which is safe, because the old host has stopped and removed its own files by then.
- Docs: `p2p.md` says where the parts go, and `nodepolicy.md` adds `layer_rate` to the example and to the text about the floor.

A later step could add a `poo doctor` check of the free space on the data
directory's disk. This proposal leaves that out.

## Testing

- `TestPartsDir` (`pkg/daemon/parts_test.go`): a leftover `poo-parts-*` file is removed, another file is kept, and the directory is 0700.
- By hand: `poo node` creates `parts/` beside `node.json`.
- gofmt, vet, staticcheck, and `go test -race ./...` pass.

## Risks

The parts now use the data directory's disk, which must have room for the
largest entry being fetched. Before this change, that load fell on `/tmp`.
No consensus rule changes, and a fetch behaves exactly as before.
