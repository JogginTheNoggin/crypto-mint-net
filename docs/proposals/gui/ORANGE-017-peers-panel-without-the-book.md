# ORANGE-017: Let the Peers panel's 10 s refresh leave out the peer book

| Field | Value |
| --- | --- |
| Status | adopted (Round 25 Part A) |
| Based on | main @ 2cacdd5 |
| Area | gui/web/static/views/peers.js (the Peers panel), and one flag for `poon peers` in pkg/cli/peersport.go (Source) |
| Effort | small |
| Decided by | |

## Summary

The Peers panel runs `poon peers` every 10 s. That prints the connected
peers and the whole peer book. Round 23 made the book's size settable
(D159 to D161): `p2p.max_book` now goes from 4,096 up to 65,536. The
panel's cost grows with the book, every 10 s, for as long as the view is
open, whether or not anyone is reading the book.

At 16,384 addresses, the largest book an import can fill alone (a
quarter of 65,536), `poon peers` prints 12.8 MB on each refresh. With the
view open:

- poon-gui holds about 240 MB, against 16 MB with an empty book;
- the page pauses for up to 0.5 s on each refresh;
- frames are up to 0.85 s apart.

The proposal: a `poon peers` flag that leaves the book out and prints only
its size. The timed Peers panel would use it, so it shows the connected
peers live. The book stays one press away in Find peers, which already
prints the whole book when no field is filled in, and runs only when
asked.

## Problem

The panel's job is to show who the node is connected to, and that
changes every few seconds. The book changes far less often and is far
larger. Every refresh still sends the book through poon-gui's job
stream, parses it in the page and draws it again (`render.js` draws 500
rows, each with its mint policy table).

Before Round 23 the book stopped at 4,096 (3 MB). Round 22 measured
that and called it in line with the other views. Now it can be 16 times
that size.

## Measurements

The setup was a real `poon node` run here and the real `poon-gui` at
2cacdd5, with Chromium through Playwright. The probe opened the Peers
view and sampled it for 60 s, and for 180 s at 16,384. Each book entry
had three mint policies and a label. The books were filled with
`poon peers import`, with the node stopped (see "Found on the way"). The
limits were set with `poon policy limits -set p2p.max_book=65536 -set
p2p.book_per_range=4096`.

| Peer book | `poon peers` prints, in | Peers view opens | Page busy | Longest pause, longest frame gap | poon-gui memory (RSS) |
| --- | --- | --- | --- | --- | --- |
| empty | 141 bytes, 8 ms | 0.09 s | 1% | none, 17 ms | 16 MB |
| 4,096 (the old limit, Round 22) | 3.0 MB, 0.14 s | 0.6 to 1.1 s | 5% | 0.33 to 0.49 s, 0.37 to 0.67 s | 71 to 84 MB |
| 16,384 | 12.8 MB, 0.44 to 0.52 s | 1.1 to 2.0 s | 6 to 7% | 0.48 to 0.49 s, 0.75 to 0.85 s | 236 to 243 MB |
| 65,536 (the new limit) | about 51 MB, about 2 s (estimated) | | | | |

The last row is a straight-line estimate from the two measured books. It
was not measured: past 16,384 the book holds only addresses the node
learned for itself, and a test node here learns none. poon-gui's memory
held steady over 180 s, so it is the cost of each refresh, not a leak.

With the change, the timed panel would cost about what the empty-book
row shows, at any book size: its output would be the connected peers
and a count. The book would cost its row above once, when Find peers is
pressed, and never on a timer.

## Idea

- Source: `poon peers -book=false` (or `-connected`) prints the
  connected peers and the book's size, never its records. The filters
  stay as they are. This is a CLI output choice and changes nothing the
  node accepts, selects or sends.
- GUI: the Peers panel's timed run adds that flag. Find peers says that
  with no field filled in it lists the whole book. The panel's About
  line points to it.

## Cost and risk

The cost is one flag in `pkg/cli/peersport.go`, and one argument and
one sentence in `peers.js`. The risk is low. Someone who opened the
Peers view to read the book presses Find once. The parity check needs
nothing new, because the flag is poon's own and the GUI only sends it.

## Prototype

None. The change starts in Source, so this proposal goes to Source
through the coordinator, and to GUI Grey for the panel.

## Modularity

Nothing to challenge. The flag keeps the command one command with one
more output choice.

## Measured after adoption (Round 25)

Measured on main @ 88bd534, which has `poon peers -counts` and the panel
that uses it, with the same 16,384-address book and the same probe as
above, for 60 s, twice:

| Peers panel, 16,384 addresses | Round 23 (whole book) | Round 25 (`-counts`) |
| --- | --- | --- |
| `poon peers` prints, in | 12.8 MB, 0.44 to 0.52 s | 333 bytes, 24 ms |
| Peers view opens | 1.1 to 2.0 s | 0.09 to 0.15 s |
| Page busy | 6 to 7% | 1% |
| Longest pause | 0.48 to 0.49 s | none |
| Longest frame gap | 0.75 to 0.85 s | 33 to 50 ms |
| poon-gui memory (RSS) | 236 to 243 MB | 16 MB |

The panel with 16,384 addresses now costs what it did with an empty
book. Find peers with no field filled in prints the whole book when
pressed: 1.7 to 2.6 s, one pause of 0.4 to 0.6 s, and 500 of 16,384
rows drawn with the note. poon-gui then holds about 125 MB until Go gives
the memory back, and the timed refresh does not add to it.

## Found on the way (not performance)

On 2cacdd5, `poon peers import -apply -if-digest D` against a running
node always refuses with "this is not the one previewed", even right
after its own preview, for 100 to 16,384 addresses. It works with the
node stopped, and it works online with a build from f4f7945. So the
GUI's Import peers Apply fails while the node runs, unless the file
changes nothing.

The likely cause is that `peerport.File.Digest` is `json:"-"`. The CLI
hashes the plan with the file's digest, but the node gets an empty one
over RPC. This was sent to Source through the coordinator. The GUI e2e
check applies a file that changes nothing, so it passes.

## Consensus

None.
