# RED-015: A chase pages the sender's hashes for ever, keeping them all, with no work shown

| Field | Value |
| --- | --- |
| Severity | high |
| Status | fixed |
| Found on | `claude/m10-watch` @ 0fd59fa (code e99e3e4, Source Round 7 Part B) |
| Component | pkg/p2p/chase.go (checkWork, findCommon, progress, saveProgress), pkg/p2p/host.go (Config.ChaseSlice) |
| Surface | peer |
| Fixed by | `claude/project-thread-ggo8dt` @ 58e4e42 (BLUE-008) |

## Summary

D116 removed every bound on a chase so that "a heavier ledger is always
reachable": `DepthLimit` (16 pages) and `ProvisionalLayers` (65,536) are gone,
and a chase that outlives `ChaseSlice` (2 minutes) yields and goes on later
with its progress kept. `checkWork` still comes before any entry is fetched
and still stops once the provisional layers beat the node's weight, but it now
**asks for more hashes whenever it has none left to check**:

```go
// pkg/p2p/chase.go, checkWork
for !st.total.Greater(beat) {
    ...
    if st.checked == len(st.after) {
        if st.complete { break }
        hs, _, more, kind, err := h.hashPage(pi, p, q)   // next page of the sender's hashes
        ...
        st.after = append(st.after, hs...)
        st.complete = !more || len(hs) == 0
        continue
    }
    ... LayersReq for after[checked:end] ...
    st.checked += len(items)                              // a reply with no layers counts as checked
}
```

A page whose hashes have **no layers** adds nothing to `st.total`, and the
reply "held, no layers" is accepted (`items` is not empty; the old
"no layers sent" refusal only fires on an empty reply). So the loop pages
and checks for ever. Each hash costs the node a slot in `st.after`, in
`st.listed` and in `st.branch` (three copies, two maps), and the sender
pays 32 bytes. With `MinEntryStrength` 0, the default, any hash is a valid
entry hash, so the sender needs no work for any of it. The node keeps
all of it: a yielded chase is saved in `h.prog` (up to 256 of them) and goes
on from it. The sender's only claim is a **stated weight** above the node's
own (RED-003, won't fix), and the optional confirmation by a priority peer is
skipped when the node has none (`askPeerIn` returns nil), as on a new node.

## Reproduction

A temporary test in `pkg/p2p` (not committed) using the `rawPeer` harness:
an `a` node with two entries, a raw peer that sends a notice with a stated
weight of 2^62, answers every hash chain request (probe or page) with fresh
random hashes (256 and a "more" flag for pages), answers every layers
request with "held, no layers", and never sends an entry. After 15 seconds
(loopback, so a fast sender):

```
probes=2  pages=14,570  layer requests=14,570  hashes served=3,730,178
119.4 MB on the wire   heap +948.4 MB   open chase groups=1 (the chase had not ended)
```

The heap grows about 8 bytes for every byte sent and linearly with time. At
a 100 Mbit/s link (12.5 MB/s) that is about 100 MB/s of heap, or 6 GB a
minute; a node with 4 GB is killed in about 40 seconds. The test node never
asked for an entry, as `TestEmptyBranchIsCheckedBeforeEntries` expects,
because it never got that far. That existing test passes only because its
three made-up hashes come with `FromStart`, which ends the paging.

## Impact

High. Any peer that can connect can, without any work, a valid signature or
a single real hash, make a node hold gigabytes in a few minutes and keep a
chase slot busy for ever, on every node that has no priority peer that
disagrees with the stated weight (new nodes, nodes whose priority peers are
away, or `no_confirm`). The chase queue backs a sender off only when a chase
*ends*; this one does not end. It is a regression from the bounded chase of
Round 6 and a direct result of "no limit on length".

## Recommendation

Keep a heavier ledger reachable and make the sender pay for the length in
the one thing the node can check, work:

- **Tie the hashes held to the work shown.** Let the chase hold a base
  allowance of hashes (for example 4,096 pages' worth at most, or a number of
  megabytes), and past it allow one more page only for each additional
  provisional weight above what the last page brought. A real heavier ledger
  shows weight on its layers all along, so it is never refused; a list of
  weightless hashes stops at the allowance. Count weight from layers and,
  where the series credits entry hashes, from `eh.Strength()`.
- **Do not store three copies.** `listed` and `branch` can share one map, or
  be dropped for hashes already checked (`after[:checked]`) once their layers
  are counted: step 5 compares each fetched entry with `after[idx]` and needs
  only the list, and `branch` is needed for the tip check.
- **Bound what a yielded chase keeps** in bytes, not only in count
  (`maxProgress`), and forget a progress whose sender is backed off or gone.
- Make a reply with **no layers at all** for a whole page of hashes after
  the first few a reason to end the chase as `Nowhere` (back-off), as the
  empty reply was before D116.

This touches the owner's rule G3 ("a heavier ledger is always reachable"),
so it should go to Black first: the recommendation does not refuse any ledger
that shows its work, it limits what a sender that shows none can make the
node hold.

Tests: a raw peer that serves unbounded weightless hash pages must be ended
(back-off) within a bounded number of pages and bytes, with the node's heap
growth bounded; `TestHeavierLedgerHuge` (a real long ledger) must still pass.

## Resolution

Fixed by BLUE-008 (decision D120), in the form the recommendation gives,
without a cap that a heavier ledger could hit:

- **One map, not three, and only for work.** A chase's progress now holds
  the page of hashes being checked, the listed hashes that carried layers
  (`marks`, by index, so step 5 still checks that each is the entry sent at
  its place), and the entries fetched. A hash with no layers is let go once
  its layers are counted.
- **Pacing, not an allowance.** After `p2p.Config.WeightlessPages` (64)
  pages in a row that bring no work, the chase yields and goes on later
  from its progress. A real heavier ledger whose work sits on its last
  entries is reached, slowly; a weightless listing costs a page of memory
  and a paced share of chase time.
- **The bound on a yielded chase** follows from the above: what it keeps
  grows only with the layers and entries the sender has really shown.
- The first page after the pool_id is now the sender's first hashes (no
  anchor, no count). Before, it was its newest hashes.

Ending as `Nowhere` after weightless pages was not taken, because work may
sit on any entry of a heavier ledger (D115). Test:
`TestWeightlessListingIsPaced` (`pkg/p2p`). It serves endless pages of
random hashes with no layers and checks four things: the chase yields every
4 pages, goes on from its progress (one first page in 60 pages), never
holds more than 16 hashes, and never asks for an entry.
`TestHeavierLedgerHuge` and every catch-up test still pass.
