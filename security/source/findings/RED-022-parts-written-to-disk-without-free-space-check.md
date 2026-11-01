# RED-022: Parts of a claimed large entry are written to disk with no check of the free space, at the speed of the link

| Field | Value |
| --- | --- |
| Severity | medium |
| Status | open |
| Found on | `claude/m10-watch` @ ae8a103 (Source Round 8 Part B) |
| Component | pkg/p2p/chase.go (`fetchParts`, `partial`, `progress.part`), pkg/p2p/host.go (`Config.PartsDir`) |
| Surface | peers (a sender the node chases) |
| Fixed by | |

## Summary

BLUE-008 (RED-016, D120) moved the parts of a large entry from memory to
a temporary file, and decided that no size is refused: the sender pays for
every byte by sending it. The file lives in `Config.PartsDir`, whose default
is empty, so `os.CreateTemp` uses the system's temporary directory (`/tmp`,
which is memory on many systems and is not the node's data disk). Nothing
looks at the free space, at the other files, or at what the node needs for
its own store. The work check that now comes first (step 4) is passed by a
sender that lists one made-up hash and shows one layer, if the series'
minimum layer strength is low, or when the node holds nothing. A sender then
claims a size of 1 TB (`Large.Size` is a u64) and sends the parts. The
yielded fetch keeps its file in `h.prog`, up to 256 files, one for each node
id the sender uses.

## Reproduction

A temporary test in `pkg/p2p` (not committed): `PartsDir` is a test
directory; a raw peer states a huge weight, lists one hash, shows one layer,
claims a 1 TB `Large` entry and answers parts requests with 8 MiB of data
(the first starting with the pool id) as fast as it is asked:

```
parts dir: 1 file, 2738 MB after 15.0 s (claimed 1024 GB)
```

That is 180 MB a second to the disk with nothing to slow it, on one
connection. The same would fill any disk, and the node's append-only store
then cannot write its own ledger. After the last part `fetchParts` also
reads the whole file back with `os.ReadFile` and decodes it, so a real entry
of S bytes takes about 2S of memory at the end (the known open gap): the
buffer and the decoded entry.

## Impact

Medium. A remote peer can fill the disk the temporary directory is on, and
with the default the node's data disk or its memory, at the speed of its
link, for no more than the bandwidth. The node then fails where it keeps its
own ledger. The no-size-cap rule is right and stays; what is missing is a
guard for the node's own room.

## Recommendation

Keep accepting every size, but treat free space as a thing to wait for, not
a reason to refuse (the way `errRevertBudget` waits):

- before each write, check the free space of `PartsDir`'s disk (and of the
  store's) against a reserve (for example the larger of 1 GiB and a few
  percent); below it the fetch yields with a reason, as for the revert
  budget, and goes on later from the offset it reached. A real large entry
  gets through as soon as there is room, a lying one never takes the room
  from the store;
- default `PartsDir` to a directory inside the node's data directory, so it
  is on the disk the operator planned for, and remove `poo-parts-*` files
  there at start;
- count the bytes written against the sender's budget in the tracking, so
  one address cannot hold the disk with many progress slots;
- decode from the file in a streaming way so that the end does not need the
  whole item twice (the open gap).

Test: a `PartsDir` on a small filesystem (or a fake free-space function) and
a sender claiming a larger size; the fetch yields at the reserve, the store
can still be written, and when room is made it goes on from its offset.

## Resolution

(Blue fills in.)
