# ORANGE-017: Let the Peers panel's 10 s refresh leave out the peer book

| Field | Value |
| --- | --- |
| Status | proposed |
| Based on | main @ 2cacdd5 |
| Area | gui/web/static/views/peers.js (the Peers panel), and one flag for `poo peers` in pkg/cli/peersport.go (Source) |
| Effort | small |
| Decided by | |

## Summary

The Peers panel runs `poo peers` every 10 s. That prints the connected
peers and the whole peer book. Round 23 made the book's size settable
(D159 to D161): `p2p.max_book` now goes from 4,096 up to 65,536. The
panel's cost grows with the book, every 10 s, for as long as the view is
open, whether or not anyone is reading the book.

At 16,384 addresses, the largest book an import can fill alone (a
quarter of 65,536), `poo peers` prints 12.8 MB on each refresh. With the
view open:

- poo-gui holds about 240 MB, against 16 MB with an empty book;
- the page pauses for up to 0.5 s on each refresh;
- frames are up to 0.85 s apart.

The proposal: a `poo peers` flag that leaves the book out and prints only
its size. The timed Peers panel would use it, so it shows the connected
peers live. The book stays one press away in Find peers, which already
prints the whole book when no field is filled in, and runs only when
asked.

## Problem

The panel's job is to show who the node is connected to, and that
changes every few seconds. The book changes far less often and is far
larger. Every refresh still sends the book through poo-gui's job
stream, parses it in the page and draws it again (`render.js` draws 500
rows, each with its mint policy table).

Before Round 23 the book stopped at 4,096 (3 MB). Round 22 measured
that and called it in line with the other views. Now it can be 16 times
that size.

## Measurements

The setup was a real `poo node` run here and the real `poo-gui` at
2cacdd5, with Chromium through Playwright. The probe opened the Peers
view and sampled it for 60 s, and for 180 s at 16,384. Each book entry
had three mint policies and a label. The books were filled with
`poo peers import`, with the node stopped (see "Found on the way"). The
limits were set with `poo policy limits -set p2p.max_book=65536 -set
p2p.book_per_range=4096`.

| Peer book | `poo peers` prints, in | Peers view opens | Page busy | Longest pause, longest frame gap | poo-gui memory (RSS) |
| --- | --- | --- | --- | --- | --- |
| empty | 141 bytes, 8 ms | 0.09 s | 1% | none, 17 ms | 16 MB |
| 4,096 (the old limit, Round 22) | 3.0 MB, 0.14 s | 0.6 to 1.1 s | 5% | 0.33 to 0.49 s, 0.37 to 0.67 s | 71 to 84 MB |
| 16,384 | 12.8 MB, 0.44 to 0.52 s | 1.1 to 2.0 s | 6 to 7% | 0.48 to 0.49 s, 0.75 to 0.85 s | 236 to 243 MB |
| 65,536 (the new limit) | about 51 MB, about 2 s (estimated) | | | | |

The last row is a straight-line estimate from the two measured books. It
was not measured: past 16,384 the book holds only addresses the node
learned for itself, and a test node here learns none. poo-gui's memory
held steady over 180 s, so it is the cost of each refresh, not a leak.

With the change, the timed panel would cost about what the empty-book
row shows, at any book size: its output would be the connected peers
and a count. The book would cost its row above once, when Find peers is
pressed, and never on a timer.

## Idea

- Source: `poo peers -book=false` (or `-connected`) prints the
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
nothing new, because the flag is poo's own and the GUI only sends it.

## Prototype

None. The change starts in Source, so this proposal goes to Source
through the coordinator, and to GUI Grey for the panel.

## Modularity

Nothing to challenge. The flag keeps the command one command with one
more output choice.

## Found on the way (not performance)

On 2cacdd5, `poo peers import -apply -if-digest D` against a running
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
