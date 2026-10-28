# GUI proposals (Green and Orange)

## Summary

Green's ideas for poo-gui: quality-of-life improvements, nice-to-haves and
explorations. Each file is one proposal with a status that Grey sets.
Prototypes live on `claude/gui-green`. Every idea keeps the GUI a visual
layer: each action is still a `poo` command line run through `cli.Main`.

| ID | Proposal | Effort | Prototype | Status |
| --- | --- | --- | --- | --- |
| [GREEN-001](GREEN-001-getting-started.md) | A Getting started guide of runnable poo commands | small | yes | adopted |
| [GREEN-002](GREEN-002-terminal-completion.md) | Tab completion in the terminal, read from poo's help | small | yes | adopted |
| [GREEN-003](GREEN-003-accessibility.md) | Accessibility fixes: skip link, aria-current, quiet live output | small | yes | adopted |
| [GREEN-004](GREEN-004-machine-readable-help.md) | `poo help -json`, so the GUI never parses usage text | medium | no | accepted, later |
| [GREEN-005](GREEN-005-command-palette.md) | A command palette (Ctrl-K) over views and forms | small | no | deferred |
| [GREEN-006](GREEN-006-linked-ids-and-pool-picture.md) | Linked ids and a live pool picture (after G4) | medium | no | deferred |
| [GREEN-007](GREEN-007-error-next-steps.md) | Next steps under common errors, and helpful empty states | small | yes | adopted |
| [GREEN-008](GREEN-008-jobs-filters.md) | Jobs view: filter, failed only, start times, grouped repeats | small | yes | adopted |
| [GREEN-009](GREEN-009-narrow-menu.md) | A view menu instead of scrolling tabs in narrow windows | small | yes | adopted |
| [GREEN-010](GREEN-010-high-contrast.md) | Honour the system's high contrast setting | small | yes | adopted |
| [GREEN-011](GREEN-011-node-banner-and-job-churn.md) | One node banner instead of an error per panel; stop refresh jobs evicting user jobs | medium | yes | adopted |
| [GREEN-012](GREEN-012-pools-view-on-unspent-and-tx.md) | The Pools view's lookups on `poo unspent` and `poo tx` (Source GREEN-008 step 2) | small | yes | adopted |
| [GREEN-013](GREEN-013-lookup-links.md) | "Look up" links from tids and addresses to Explore | small | yes | adopted |
| [GREEN-014](GREEN-014-watch-consistency.md) | Watch matches the rest: kind boxes, local times, one hash style | small | yes | adopted |
| [GREEN-015](GREEN-015-watch-explore-loose-ends.md) | Loose ends: Watch in the background, paging, a flaky e2e, `-kind` on non-stream commands | small to medium | partly | 1 and 2 adopted; 3 and 4 with Source/Grey |
| [GREEN-016](GREEN-016-one-wallet-file.md) | One wallet file for the whole Wallet view, not one per form | small | yes | adopted |
| [GREEN-017](GREEN-017-guide-follows-settings.md) | Getting started uses the Settings -root and -policy, like the views | small | yes | adopted |
| [GREEN-018](GREEN-018-ledger-picture.md) | A picture of the selected ledger in Explore (rest of GREEN-006) | small | yes | adopted |
| [GREEN-019](GREEN-019-links-and-copy-output.md) | Themed links, and Copy output on every result | small | yes | adopted |
| [GREEN-020](GREEN-020-health-check.md) | A Health check panel on `poo doctor`, with runnable fixes | small | yes | adopted |
| [GREEN-021](GREEN-021-grinder-view-sketch.md) | GUI ideas for the grinders (M11), and what the commands should print | medium | no | after M11 |
| [GREEN-022](GREEN-022-output-conventions.md) | Output conventions: `-o` on doctor, fix as argv, stable error codes (Source) | small | no | with Source/Grey |

## Orange (performance)

GUI/Orange measures the GUI and proposes changes with before and after
numbers. Every change is to how the page draws or streams what `poo`
printed, never to what runs. Prototypes are on `claude/project-thread-f0df7u`.
ORANGE-005 explains how to measure.

| ID | Proposal | Effort | Prototype | Status |
| --- | --- | --- | --- | --- |
| [ORANGE-001](ORANGE-001-watch-table-rendering.md) | Watch draws once per frame and lays out only the newest 200 of 1000 rows (5 → 17 fps at full rate, 37 → 60 at 200/s) | small | yes | adopted |
| [ORANGE-002](ORANGE-002-stream-joined-chunks-and-resume.md) | Join adjacent writes into one stream event; resume with `Last-Event-ID` instead of replaying everything | small | yes | adopted |
| [ORANGE-003](ORANGE-003-bounded-scrollback.md) | Bounded scrollback for the terminal, node log and running commands (`poo watch` in the terminal: 126 s frozen → 1.1 s) | small | yes | adopted |
| [ORANGE-004](ORANGE-004-refresh-timers-and-jobs-list.md) | No timed refreshes in a hidden tab; Jobs redraws only on change | small | yes | adopted |
| [ORANGE-005](ORANGE-005-measuring-kit.md) | A measuring kit (`gui/perf`, benchmarks), and what measured fine | small | yes | adopted |
| [ORANGE-006](ORANGE-006-watch-draw-rate.md) | Draw a busy Watch table at most 10 times a second (61% → 18% busy at 200 events/s) | small | yes | adopted |
| [ORANGE-007](ORANGE-007-batched-scrollback-writes.md) | Batch frequent writes in scrollback panes (node log 11% → 4% busy) | small | yes | adopted |
| [ORANGE-008](ORANGE-008-long-text-results.md) | Show the last 1 MiB of a long text result (3.9 MB: 792 → 181 ms) | small | yes | adopted |

## How to try the prototypes

```sh
git checkout claude/gui-green
go -C gui build -o /tmp/poo-gui ./cmd/poo-gui && /tmp/poo-gui
node gui/e2e/qol.mjs /tmp/poo-gui     # the checks for 001 to 003, 007 to 010, 016, 017, 019 and 020
node gui/e2e/watch.mjs /tmp/poo-gui   # the checks for 012 to 014 and 018
```
