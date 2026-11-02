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
| [GREEN-021](GREEN-021-grinder-view-sketch.md) | GUI ideas for the grinders (M11), and what the commands should print | medium | no | adopted |
| [GREEN-022](GREEN-022-output-conventions.md) | Output conventions: `-o` on doctor, fix as argv, stable error codes (Source) | small | no | done by Source (D110) |
| [GREEN-023](GREEN-023-grind-jobs-table.md) | The grind jobs as a table, with Stop on each job | small | yes | adopted |
| [GREEN-024](GREEN-024-device-choices.md) | Device fields offer the devices `poo grind devices` found, and auto | small | yes | adopted |
| [GREEN-025](GREEN-025-rates-read-the-same.md) | Rates read as H/s everywhere, and found hashes render like all others | small | yes | adopted |
| [GREEN-026](GREEN-026-grind-schedule-commands.md) | The grind schedule: `poo grind schedule` commands first, then GUI forms | medium | no | done: `poo grind schedule` by Source (D113); Schedule panel and forms in Work (Round 7) |
| [GREEN-027](GREEN-027-grind-loose-ends.md) | Grinding loose ends for Source: uncounted entry tries, two start lines, a waiting reason | small | no | done by Source (D112) |
| [GREEN-028](GREEN-028-flags-per-action.md) | Forms show only the flags the chosen action takes (Peers, Node policies), as `poo` checks them | small | yes | adopted |
| [GREEN-029](GREEN-029-crypto-suggestions.md) | Key and hash fields suggest what `poo crypto` lists | small | yes | adopted |
| [GREEN-030](GREEN-030-jump-list.md) | Long views list their panels at the top; a title scrolls to its panel | small | yes | adopted |
| [GREEN-031](GREEN-031-schedule-remove-by-index.md) | Schedule Remove goes by position and can drop the wrong job; ask Source for `remove N -target T` | small | no (needs Source) | done: `remove -target` by Source (D115-D119), the GUI sends each row's target (Round 8) |
| [GREEN-032](GREEN-032-hidden-fields-and-until-from.md) | Fields `only` hides were still drawn (CSS beat `hidden`); Until from shows only for entry and tx: targets | small | yes | adopted |
| [GREEN-033](GREEN-033-defaults-show-current.md) | Schedule defaults fields show what is set now (and the layer rate the summary hides) | small | yes | adopted |
| [GREEN-035](GREEN-035-layer-rate-loose-ends.md) | The Schedule summary shows a layer rate kept with no floor; ask Source for doctor `fix_args` when the rate was lowered | small | yes (GUI part) | done: GUI part (Round 11), doctor `fix_args` by Source (D135) |
| [GREEN-036](GREEN-036-getting-started-reaches-grinding.md) | Getting started covers `doctor`, `watch` and `grind`, and each node step links its view | small | yes | adopted |
| [GREEN-038](GREEN-038-reload-due-notice.md) | The Node view says when a node.json change waits for a reload, with Reload now (needs Source GREEN-037) | small | yes | adopted (with Source GREEN-037) |
| [GREEN-039](GREEN-039-wallet-parity.md) | Every `poo wallet` subcommand has a form (shared policy, co-sign, submit, resend, follow, passwd, seed), checked against `poo wallet help` | small | yes | adopted (with Source `follow -replace`) |
| [GREEN-040](GREEN-040-whole-help-parity.md) | Every `poo help` command is in a view or listed as terminal-only; adds `sim` and `series example` forms | small | yes | proposed |

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
| [ORANGE-009](ORANGE-009-work-view-paced-drawing.md) | Work view draws at most 10 times a second and reuses its chart (46% → 12% busy at 200 lines/s; full rate 9 → 46 fps) | small | yes | adopted |
| [ORANGE-010](ORANGE-010-close-streams-on-leave.md) | Close a view's event streams when it is left: the Node view leaked one per visit and the GUI stopped answering after about 2 minutes | small | yes | adopted |
| [ORANGE-011](ORANGE-011-one-crypto-ask-in-flight.md) | Ask `poo crypto` once while a key field waits for suggestions (8 focuses: 8 jobs → 1) | small | yes | adopted |

### Re-checks each round

Each Part B runs every scenario again on Grey's new base, so a change that
undoes an earlier gain shows up. Numbers are from a cloud container with
Playwright Chromium, so compare them with each other, not with a laptop.

| Scenario | Round 7 (b20bf8f) | Round 8 (a93ccb6) | Round 9 (6fae281) | Round 10 (4108866) | Round 11 (4be2230) | Round 12 (c9c9e1f) | Round 13 (376e964) | Round 14 (3a7a39c) |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| 5-minute soak: heap, nodes, open streams, server RSS, slowest click | 3.1 MB, 27.6k, 1, 40 MB, 72 ms | 3.2 MB, 27.6k, 1, 40 MB, 36 ms | 3.1 MB, 27.7k, 1, 42 MB, 51 ms | 3.2 MB, 27.7k, 1, 41 MB, 134 ms | 3.2 MB, 27.7k, 1, 40 MB, 50 ms | 3.1 MB, 27.7k, 1, 40 MB, 33 ms | 3.2 MB, 27.6k, 1, 42 MB, 50 ms | 3.2 MB, 27.6k, 1, 42 MB, 56 ms |
| Page load: DOMContentLoaded, first view, bytes | not run | 30 ms, 58 ms, 136 KB in 15 requests | 29 ms, 56 ms, 140 KB in 15 requests | 39 ms, 75 ms, 141 KB in 15 requests | 33 ms, 64 ms, 142 KB in 15 requests | 30 ms, 60 ms, 144 KB in 15 requests | 32 ms, 62 ms, 145 KB in 15 requests | 33 ms, 62 ms, 149 KB in 15 requests |
| Watch at 200 events/s | 60 fps, 19% busy | 60 fps, 19% busy | 60 fps, 20% busy | 60 fps, 21% busy | 60 fps, 21% busy | 60 fps, 19% busy | 60 fps, 18% busy | 60 fps, 20% busy |
| Watch at full rate (20k events) | 32 fps, 0.55 s | 31 fps, 0.55 s | 32 fps, 0.65 s | 22 to 30 fps, 0.66 to 0.90 s | 29 fps, 0.72 s | 32 fps, 0.67 s | 35 fps, 0.51 s | 30 fps, 0.67 s |
| Terminal, `poo watch` (20k events) | 0.60 s | 0.59 s | 0.63 s | 0.55 to 0.78 s | 0.63 s | 0.65 s | 0.62 s | 0.70 s |
| Node log at 50 lines/s for 60 s | 2 to 6 ms answers, flat | 4% busy, flat | 4% busy, flat | 5% busy, flat | 5% busy, flat | 5% busy, flat | 4% busy, flat | 5% busy, flat |
| Work at 200 lines/s | 60 fps, 12% busy | 60 fps, 11% busy | 60 fps, 12% busy | 60 fps, 13% busy | 60 fps, 12% busy | 60 fps, 12% busy | 60 fps, 12% busy | 60 fps, 13% busy |
| A 1.5 MB JSON result shown | not recorded | 199 ms | 199 ms | 257 ms | 192 to 306 ms | 261 ms | 237 ms | 235 ms |
| Reload a running watch (40k events) | 238 ms | 257 ms | 245 ms | 237 ms | 217 ms | 279 ms | 226 ms | 219 to 384 ms |
| Work with a found layer on every line (a low layer floor), 2000 lines/s | not run | not run | 60 fps, 30% busy, found table held at 500 rows | 60 fps, 32% busy, 500 rows | 60 fps, 30% busy, 500 rows | 60 fps, 27% busy, 500 rows | 60 fps, 27% busy, 500 rows | 60 fps, 32% busy, 500 rows |

Round 8 found nothing new to fix. The jump list, one job per form and
the Schedule Remove guard did not show up in any measurement.

Round 9 found nothing new to fix either. The layer floor in Schedule
defaults can make `poo grind run` print a found layer on most lines, so
Round 9 measured that case (`FOUND=1` in the kit): the Work view stays at
60 fps and keeps its found table at 500 rows. The kit used to count the
plan table's rows as found rows; it now counts the found table.

Round 10 found nothing new to fix. It ran in a new container, and the
fast-stream numbers moved by up to a third with no change to that code.
Running the Round 9 and Round 10 builds back to back, twice each, gave
the same spread for both (Watch at full rate 0.55 to 0.69 s, terminal
0.55 to 0.87 s). Compare rounds by their trend, not by one run.

Round 11 found nothing new to fix. The 1.5 MB JSON result took 306 ms
in the full run. Running the Round 10 and Round 11 builds back to back
twice each gave 192 to 240 ms for both, so that is noise too.

Round 12 found nothing new to fix. The new Getting started steps add
links only; their `poo watch` step runs in the terminal, which the
terminal row above already measures.

Round 13 found nothing new to fix. The reload notice comes from
`policy_changed` in `poo status`, which the Node view asks for every
5 s. To answer it the node reads node.json again and compares it with the
policy it runs: 17 µs and 40 allocations for the example policy (a Go
benchmark of `nodepolicy.Load` and `SameApplied`), so the 5 s refresh
costs the node nothing worth changing.

Round 14 found nothing new to fix. The Wallet view grew from 11 to 18
panels and opens in about 15 ms (14 ms before), with 332 elements, and
it still runs no command on opening. Reload took 384 ms in the full run,
but the Round 13 and Round 14 builds back to back gave 219 to 385 ms for
both, so that is noise.

## How to try the prototypes

```sh
git checkout claude/gui-green
go -C gui build -o /tmp/poo-gui ./cmd/poo-gui && /tmp/poo-gui
node gui/e2e/qol.mjs /tmp/poo-gui     # the checks for 001 to 003, 007 to 010, 016, 017, 019, 020, 028, 029, 032, 036, 039 and 040
node gui/e2e/watch.mjs /tmp/poo-gui   # the checks for 012 to 014 and 018
node gui/e2e/work.mjs /tmp/poo-gui    # the checks for 023 to 025, 030, 032, 033, 035 and 038
```
