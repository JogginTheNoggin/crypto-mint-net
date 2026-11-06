# RED-080: An in-place `limits -set` marks a hand-edited grind schedule and manual peers as applied, though only a reload applies them

| Field | Value |
| --- | --- |
| Severity | Low |
| Status | fixed |
| Found | Source Round 26 Part B, on `main` @ `439b369` (D168) |
| Area | `pkg/nodepolicy` (`SameButInPlace`), `pkg/rpc` (`applyInPlace`), `pkg/node` (`SetRunning`) |
| For Black | No consensus rule is touched. |
| Fixed by | `claude/project-thread-ggo8dt` @ `a117710` (BLUE-027) |

## What is wrong

`applyInPlace` takes a change in place only when `SameButInPlace(running,
saved)` says the two policies differ only in what a running node applies
without a reload. `SameButInPlace` strips `Grind`, `P2P.Peers` and `Lowered`
before comparing, with the comment that these are "applied by the grind
manager" and "already in the running book". That is so when the RPC changed
them; it is not so when the owner edited node.json by hand and then ran
`poon policy limits -set`. The change is then applied in place, `SetRunning`
makes the running policy equal to node.json, and from then on nothing
shows that the hand edit was never applied. The doc comment of `applyInPlace`
promises the opposite: "anything else edited by hand leaves it to the reload".

## How I checked it

A scratch node (`poon init -example`), running:

```
$ # edit node.json by hand: p2p.peers = ["127.0.0.1:19999"], grind.jobs = [{"target":"tip"}]
$ poon policy limits -dir n1 -set p2p.max_book=2000
{ ..., "saved": true, "in_place": true }
$ poon peers -dir n1 -counts       addresses 0       (the manual peer is not in the book)
$ poon grind jobs -dir n1          []                (the job is not running)
$ poon doctor -dir n1              ok p2p ... 0 in the peer book; ok grind ... jobs in the policy: 0
$ poon reload -dir n1              reloaded
$ poon peers -dir n1 -counts       addresses 1
$ poon grind jobs -dir n1          [ { "id": 1, "spec": { "target": "tip", "source": "policy" }, ... } ]
```

(`poon doctor` reads the running policy, so it also shows nothing wrong.)

## What could happen

An owner who edits node.json to add a grind job or a manual peer, then
changes a limit, believes the node has both. It has neither until some later
reload, and no command says so. Not a security hole in itself; the risk is a
node that grinds or connects less than the owner configured, with no warning.

## Suggested fix (Blue's call)

- Do not strip `Grind` and `P2P.Peers` in `SameButInPlace`; instead, have
  the RPC paths that change them (`peer add`, `peer remove`, grind edits)
  write the running policy too (`SetRunning`), so a difference means a hand
  edit.
- Or keep the strip and make `applyInPlace` compare those two against the
  book and the manager, and reload when they differ.
- Test: hand-edit `p2p.peers` and `grind.jobs`, run `-set p2p.max_book`, and
  expect `reloading` (not `in_place`).

## Resolution

Fixed in BLUE-027 (D169), as the first suggestion.

- `SameButInPlace` no longer strips `grind` and `p2p.peers`.
- The RPC edits that apply them at once write them into the running policy (`node.UpdateRunning`). These are `peer add -keep` and `peer remove`, and a grind schedule that the grind manager started.
- A hand edit of either is now a difference, so `limits -set` reloads.
- Test: `TestInPlaceLeavesHandEditsToTheReload` covers three cases. A hand-edited `p2p.peers` or `grind.jobs` gives `reloading`. A peer kept by the RPC still gives `in_place`.
