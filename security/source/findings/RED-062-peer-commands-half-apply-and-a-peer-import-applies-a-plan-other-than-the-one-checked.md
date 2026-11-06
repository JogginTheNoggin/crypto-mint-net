# RED-062: The peer commands half-apply, and a peer import applies another plan than the one checked

| Field | Value |
| --- | --- |
| Severity | Low |
| Status | fixed |
| Found | Source Round 21 Part B, on `main` @ `8662766` (D157) |
| Area | `pkg/cli/node.go` (`peerOffline`), `pkg/cli/peersport.go` (`peersImportCmd`), `pkg/rpc/manage.go` |
| For Black | No consensus rule is touched. |
| Fixed by | `claude/project-thread-ggo8dt` @ 282571b (BLUE-022) |

## What is wrong

1. **Offline `poon peer add ADDR -keep -label X` loses the label.**
   `peerOffline` lists the address in node.json and returns after
   `CheckLabel`; it never opens the book. The reply says `"kept": true`,
   the help says "add and label". With a running node the label is set.
2. **A bad label comes after the save.** `poon peer add ADDR -keep -label
   $'x\x01y'` offline prints `peers: a label has no control characters`
   but node.json already lists the address (`grep` finds it). The same
   order holds for `remove` (node.json first, then the book).
3. **A peer import applies a plan other than the one it checked.**
   `peersImportCmd` plans (`run(false)`), checks `-if-digest` against that
   plan, then calls `run(true)`, which plans again. A running node's book
   and tracker can change in between (a peer learned, a block lifted), so
   what is applied is not what was checked; through the node it is two
   requests. D154 made `poon policy import` bind the digest to the plan
   for this reason.
4. **The preview does not say the addresses will be dialled.** Only the
   apply line says "the node tries them when it is short of peers". By
   default (`p2p.no_dial_imported` off) the manager tries imported
   addresses after its own candidates and the archive and before
   registries and asking peers, so for a node with no peers an imported
   file decides who it connects to first.

## How I checked it

Items 1 and 2 by running `poon peer add ... -keep` offline (output above;
no `peers.json` was made by the add). Items 3 and 4 by reading the code
(`peersImportCmd`, `Host.manage`).

## What could happen

Nothing hostile: wrong state the owner does not see. For item 4, a user
who imports a file to read it is not told that the node will also act on
it.

## Suggested fix (Blue's call)

- Offline: set the label in the book too (or refuse `-label` without a
  node), and check the label before saving node.json.
- Pass the digest to `Import` and have it refuse if the plan it just
  made has another digest; for the node, send the digest with the apply.
- Add a line to the preview: "applied, these will be tried when the node
  is short of peers (p2p.no_dial_imported stops that)".

## Not tested

Item 3 under a running node (a race, so not reproduced).

## Resolution

Fixed (D158), with GREEN-046.

1. and 2. are fixed by GREEN-046 (PR #124). Offline `peer add -keep
   -label` now checks the label first and refuses a label for an address
   the book does not hold, before any file changes. That fix is not
   repeated here, so Grey merges both.
3. `peerport.ImportIf` plans and applies in one call, and applies only
   when the plan it just made has the given digest (`ErrNotPreviewed`
   otherwise, and nothing changes). `poon peers import -apply -if-digest`
   uses it offline, and sends `if_digest` to a running node, which does
   the same in its one request (409 `conflict` on a mismatch).
4. The preview notes that "once applied, the node tries these addresses
   when it is short of peers, after its own candidates and before
   registries (p2p.no_dial_imported stops that)".

Test: `TestImportIfAppliesOnlyThePreviewedPlan` (pkg/peerport);
`TestPeersForgetImported` checks the note.
