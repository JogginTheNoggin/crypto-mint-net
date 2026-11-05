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
| [GREEN-040](GREEN-040-whole-help-parity.md) | Every `poo help` command is in a view or listed as terminal-only; adds `sim` and `series example` forms | small | yes | adopted, with Build/Work `-bytes` and Submit `-f` |
| [GUI-GREEN-041](GUI-GREEN-041-flag-parity.md) | Forms take the flags poo takes: Build/Work device, threads, for, serial; one wallet series for every wallet command; Receive's named transactions | small | yes | adopted |
| [GUI-GREEN-042](GUI-GREEN-042-flag-parity-check.md) | A check (parity.mjs, in CI) that every flag `poo CMD -h` lists is sent by a form or left out for a stated reason; fixes Explore's address -below/-tid and Pools' filters | small | yes | adopted |
| [GUI-GREEN-043](GUI-GREEN-043-stated-defaults.md) | parity.mjs also checks every default a field states against `poo -h`; Schedule a job's device and threads say the section's default | small | yes | adopted |
| [GUI-GREEN-044](GUI-GREEN-044-one-file-per-view.md) | One file per view in `static/views/`, one list in `views/index.js`; the e2e walks every view from the navigation | small | yes | adopted |
| [GUI-GREEN-045](GUI-GREEN-045-subcommand-parity.md) | parity.mjs also checks that every subcommand poo lists (peer actions, policy, sim, submit, grind, wallet) is run by a view | small | yes | adopted |
| [GUI-GREEN-046](GUI-GREEN-046-copy-command-line.md) | Copy beside every command line, to run it in a shell; off while the line holds a secret flag | small | yes | adopted |
| [GUI-GREEN-047](GUI-GREEN-047-keygen-seed-masked.md) | Make a key asks for its seed in a masked field (`keygen -seed-file -`, D98) instead of putting it on the line; a secret flag set to `-` is not a secret line | small | yes | adopted |
| [GUI-GREEN-048](GUI-GREEN-048-stated-choices.md) | parity.mjs also checks that a field offering a fixed set of values offers exactly the ones `poo -h` lists; Find and Export peers take book states as boxes, as Watch takes kinds | small | yes | adopted |
| [GUI-GREEN-049](GUI-GREEN-049-limit-fields.md) | Set limits draws a field for each limit `poo policy limits` lists, with its value in effect, range and default, instead of typed `NAME=VALUE` pairs | small | yes | adopted |
| [GUI-GREEN-050](GUI-GREEN-050-variable-supply-review.md) | Review of the variable supply plan against the design and D84 (four plan lines to word so the GUI holds no rule; a release cannot be undone); a required field says "(required)" in its label | small | yes | adopted |
| [GUI-GREEN-051](GUI-GREEN-051-series-pool-size.md) | New series takes units per mint and pool size, so a variable series' "multiples of the pool size" refusal can be fixed in the form; notes for Source on `series info` | small | yes | adopted |
| [GUI-GREEN-052](GUI-GREEN-052-one-file-picker.md) | One `pickFile` helper for every form that reads a file (imports, Add a release), and a picker on Check a release (`-check -`); for Source, a distinct exit code for a waiting refusal | small | yes | adopted |
| [GUI-GREEN-053](GUI-GREEN-053-say-what-a-change-did.md) | One line over a change's result saying what poo's reply says it did (in place, reloading, saved for the next start, nothing changed) on Set limits and Add a release; Tries states poo's default; the plan names SLH-DSA-SHA2-128s; for Source, mark which limits apply in place | small | yes | adopted |
| [GUI-GREEN-054](GUI-GREEN-054-newcomers-on-the-reserve.md) | Newcomers on the reserve default: Getting started writes `poo init` (the reserve) instead of `-example`; a Followed series panel names the built-in reserve series from poo's `builtin`; notes for Orange (two 1,024-row pool tables) and Grey (a first-mint step) | small | yes | adopted (R29A) |
| [GUI-GREEN-055](GUI-GREEN-055-exact-amounts.md) | Amounts above 2^53 were shown rounded (JSON.parse); `parseJSON` keeps them exact as BigInt for results, series panels and Watch events; for Source, print amounts, balances and totals as decimal strings as weight and units already are | small | yes | adopted (R30A; the Source part is queued) |
| [GUI-GREEN-056](GUI-GREEN-056-node-view-on-demand.md) | On the reserve the Node view still drew 1,008 rows and 23,550 elements on every open (Each pool ran `poo status` on mount; the follow list drew 1,024 pools); `panel({ start: false })` and a folded follow pools list bring it to 451 elements; for Source, `builtin` on `status -counts` rows | small | yes | adopted (R31A) |

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
| [ORANGE-012](ORANGE-012-submit-large-entry-as-read.md) | `poo submit entry` sends the hex as read (128 MiB entry: 12 s → 5 to 7 s, client heap 1 GB → 625 MB); Round 15's ask and wallet-lock measurements | small | yes | adopted by Source (D147) |
| [ORANGE-013](ORANGE-013-long-paste-in-parts.md) | Send a long pasted line in parts and end the input only once it arrived (largest pasteable entry: about 500 KiB → any size; 8 MiB of hex in 4.7 s) | small | yes | adopted |
| [ORANGE-014](ORANGE-014-picked-file-as-read.md) | Send a picked policy file as it is read and stop when `poo` stops (500 MB picked by mistake: 5.3 s and +491 MB of browser memory → 0.9 s and +47 MB) | small | yes | adopted |
| [ORANGE-015](ORANGE-015-long-import-plan.md) | Draw a long import plan 500 lines at a time (50,000 issuers: no plan and a page error → plan in 2.2 s; 20,000: 4.6 s → 0.9 s) | small | yes | adopted |
| [ORANGE-016](ORANGE-016-short-read-first-line.md) | Name the first 20 items of a plan's "Read first" line, with Show all (50,000 peers: a 1.5 million-character alert, 183,000 px tall → 685 characters, 151 px) | small | yes | adopted |
| [ORANGE-017](ORANGE-017-peers-panel-without-the-book.md) | Let the Peers panel's 10 s refresh leave out the peer book, with a `poo peers` flag (16,384 addresses: 12.8 MB every 10 s, poo-gui 240 MB, pauses up to 0.5 s; up to 65,536 now settable) | small | no | adopted (`poo peers -counts`) |
| [ORANGE-018](ORANGE-018-parts-as-ranges.md) | Print a variable series' parts as ranges, not lists of every serial and pool, and say when a list is cut at 500 (100 parts of 1,000 as lists: 2.9 s and 121,000 elements → as ranges: 30 ms and 6,500) | small | the note on a cut list | adopted |
| [ORANGE-019](ORANGE-019-variable-series-table-in-parts.md) | Draw the Variable series table 500 rows at a time, with Show more (4,096 followed series: 32,800 elements, panel in 1.1–1.25 s → 4,000 elements, 0.6–0.8 s) | small | yes | adopted |
| [ORANGE-020](ORANGE-020-output-as-printed-when-opened.md) | Draw "Output as printed" only when it is opened (Node view with 4,096 followed series: 60,970 → 28,887 elements; Variable series panel 0.45–1.0 s → 0.35–0.42 s) | small | yes | adopted |
| [ORANGE-021](ORANGE-021-followed-pools-refresh-without-every-pool.md) | Let the Followed pools panel's 5 s refresh leave out the per-pool rows, with a `poo status` option (reserve series, 1,024 pools: 352 KB every 5 s, 5% busy, 150 ms pauses; 8,192 pools: 2.8 MB, 280 ms pauses, poo-gui 140 MB) | small | no | adopted (R30A, built on `poo status -counts`, D177) |
| [ORANGE-022](ORANGE-022-parsejson-fast-path.md) | Let `parseJSON`'s fast path look for 16 digits only where a JSON number can start, so hex strings no longer send every answer through the slow scan (`poo status`, 1,024 pools: 7 → 1 ms; 4,096 followed series: 20 → 9 ms; 20,000 Watch events: 38 → 10 ms) | small | yes | adopted (R31A) |

### Re-checks each round

Each Part B runs every scenario again on Grey's new base, so a change that
undoes an earlier gain shows up. Numbers are from a cloud container with
Playwright Chromium, so compare them with each other, not with a laptop.

| Scenario | Round 7 (b20bf8f) | Round 8 (a93ccb6) | Round 9 (6fae281) | Round 10 (4108866) | Round 11 (4be2230) | Round 12 (c9c9e1f) | Round 13 (376e964) | Round 14 (3a7a39c) | Round 15 (55cf745) | Round 16 (6a1cdce) | Round 17 (052d032) | Round 18 (3804c55) | Round 19 (e71e317) | Round 20 (9628426) | Round 21 (4dc7d3d) | Round 22 (f4f7945) | Round 23 (2cacdd5) | Round 24 (2aceb1a) | Round 25 (88bd534) | Round 26 (eb74797) | Round 27 (b9fed52) | Round 28 (adcf216) | Round 29 (4e1c601) | Round 30 (0ade046) |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| 5-minute soak: heap, nodes, open streams, server RSS, slowest click | 3.1 MB, 27.6k, 1, 40 MB, 72 ms | 3.2 MB, 27.6k, 1, 40 MB, 36 ms | 3.1 MB, 27.7k, 1, 42 MB, 51 ms | 3.2 MB, 27.7k, 1, 41 MB, 134 ms | 3.2 MB, 27.7k, 1, 40 MB, 50 ms | 3.1 MB, 27.7k, 1, 40 MB, 33 ms | 3.2 MB, 27.6k, 1, 42 MB, 50 ms | 3.2 MB, 27.6k, 1, 42 MB, 56 ms | 3.2 MB, 27.7k, 1, 42 MB, 57 ms | 3.2 MB, 27.7k, 1, 41 MB, 54 ms | 3.2 MB, 27.7k, 1, 40 MB, 37 ms | 3.2 MB, 27.7k, 1, 40 MB, 61 ms | 3.2 MB, 27.7k, 1, 39 MB, 51 ms | 3.3 MB, 27.7k, 1, 40 MB, 61 ms | 3.3 MB, 27.7k, 1, 41 MB, 71 ms | 3.5 MB, 27.7k, 1, 42 MB, 68 ms | 3.3 MB, 27.7k, 1, 40 MB, 66 ms | 3.3 MB, 27.6k, 1, 41 MB, 70 ms | 3.3 MB, 27.7k, 1, 40 MB, 51 ms | 3.5 MB, 27.7k, 1, 42 MB, 62 ms | 3.4 MB, 27.7k, 1, 43 MB, 67 ms | 3.4 MB, 27.6k, 1, 39 MB, 52 ms | 3.4 MB, 27.7k, 1, 40 MB, 69 ms | 3.4 MB, 27.7k, 1, 40 MB, 53 ms |
| Page load: DOMContentLoaded, first view, bytes | not run | 30 ms, 58 ms, 136 KB in 15 requests | 29 ms, 56 ms, 140 KB in 15 requests | 39 ms, 75 ms, 141 KB in 15 requests | 33 ms, 64 ms, 142 KB in 15 requests | 30 ms, 60 ms, 144 KB in 15 requests | 32 ms, 62 ms, 145 KB in 15 requests | 33 ms, 62 ms, 149 KB in 15 requests | 31 ms, 59 ms, 150 KB in 15 requests | 39 ms, 76 ms, 155 KB in 15 requests | 38 ms, 74 ms, 155 KB in 15 requests | 30 ms, 62 ms, 164 KB in 15 requests | 36 ms, 68 ms, 171 KB in 24 requests | 39 ms, 72 ms, 171 KB in 24 requests | 43 ms, 79 ms, 177 KB in 24 requests | 50 ms, 88 ms, 186 KB in 24 requests | 35 ms, 60 ms, 191 KB in 24 requests | 44 ms, 83 ms, 194 KB in 24 requests | 47 ms, 80 ms, 199 KB in 24 requests | 47 ms, 97 ms, 208 KB in 24 requests | 49 ms, 85 ms, 209 KB in 24 requests | 43 ms, 88 ms, 215 KB in 24 requests | 43 ms, 70 ms, 220 KB in 24 requests | 39 ms, 66 ms, 223 KB in 24 requests |
| Watch at 200 events/s | 60 fps, 19% busy | 60 fps, 19% busy | 60 fps, 20% busy | 60 fps, 21% busy | 60 fps, 21% busy | 60 fps, 19% busy | 60 fps, 18% busy | 60 fps, 20% busy | 60 fps, 20% busy | 60 fps, 21% busy | 60 fps, 22% busy | 60 fps, 19% busy | 60 fps, 20% busy | 60 fps, 19% busy | 60 fps, 21% busy | 60 fps, 21% busy | 60 fps, 18% busy | 60 fps, 18% busy | 60 fps, 22% busy | 60 fps, 21% busy | 60 fps, 21% busy | 60 fps, 20% busy | 60 fps, 20% busy | 60 fps, 18% busy |
| Watch at full rate (20k events) | 32 fps, 0.55 s | 31 fps, 0.55 s | 32 fps, 0.65 s | 22 to 30 fps, 0.66 to 0.90 s | 29 fps, 0.72 s | 32 fps, 0.67 s | 35 fps, 0.51 s | 30 fps, 0.67 s | 30 fps, 0.57 s | 32 fps, 0.68 s | 17 to 24 fps, 0.70 to 1.07 s | 31 fps, 0.71 s | 30 fps, 0.66 s | 31 fps, 0.57 s | 31 fps, 0.64 s | 27 fps, 0.69 s | 32 fps, 0.65 s | 29 fps, 0.58 s | 32 fps, 0.53 s | 32 fps, 0.67 s | 33 fps, 0.55 s | 33 fps, 0.67 s | 30 fps, 0.68 s | 34 fps, 0.53 s |
| Terminal, `poo watch` (20k events) | 0.60 s | 0.59 s | 0.63 s | 0.55 to 0.78 s | 0.63 s | 0.65 s | 0.62 s | 0.70 s | 0.64 s | 0.63 s | 0.74 to 1.33 s | 0.85 s | 0.62 s | 0.80 s | 0.79 s | 0.81 s | 0.82 s | 0.74 s | 0.56 s | 0.57 s | 0.48 s | 0.55 s | 0.65 s | 0.54 s |
| Node log at 50 lines/s for 60 s | 2 to 6 ms answers, flat | 4% busy, flat | 4% busy, flat | 5% busy, flat | 5% busy, flat | 5% busy, flat | 4% busy, flat | 5% busy, flat | 5% busy, flat | 5% busy, flat | 5% busy, flat | 5% busy, flat | 5% busy, flat | 5% busy, flat | 5% busy, flat | 5% busy, flat | 4% busy, flat | 4% busy, flat | 5% busy, flat | 5% busy, flat | 5% busy, flat | 5% busy, flat | 5% busy, flat | 5% busy, flat |
| Work at 200 lines/s | 60 fps, 12% busy | 60 fps, 11% busy | 60 fps, 12% busy | 60 fps, 13% busy | 60 fps, 12% busy | 60 fps, 12% busy | 60 fps, 12% busy | 60 fps, 13% busy | 60 fps, 13% busy | 60 fps, 14% busy | 60 fps, 14% busy | 60 fps, 13% busy | 60 fps, 13% busy | 60 fps, 12% busy | 60 fps, 12% busy | 60 fps, 14% busy | 60 fps, 12% busy | 60 fps, 12% busy | 60 fps, 14% busy | 60 fps, 12% busy | 60 fps, 13% busy | 60 fps, 12% busy | 60 fps, 13% busy | 60 fps, 11% busy |
| A 1.5 MB JSON result shown | not recorded | 199 ms | 199 ms | 257 ms | 192 to 306 ms | 261 ms | 237 ms | 235 ms | 219 ms | 300 ms | 278 ms | 255 ms | 252 ms | 209 ms | 219 ms | 243 ms | 182 ms | 199 ms | 249 ms | 206 ms | 219 ms | 189 ms | 207 ms | 195 ms |
| Reload a running watch (40k events) | 238 ms | 257 ms | 245 ms | 237 ms | 217 ms | 279 ms | 226 ms | 219 to 384 ms | 237 ms | 241 ms | 259 ms | 232 ms | 231 ms | 228 ms | 205 ms | 296 ms | 253 ms | 219 ms | 237 to 417 ms | 233 ms | 229 to 427 ms | 237 ms | 251 to 366 ms | 225 ms |
| Work with a found layer on every line (a low layer floor), 2000 lines/s | not run | not run | 60 fps, 30% busy, found table held at 500 rows | 60 fps, 32% busy, 500 rows | 60 fps, 30% busy, 500 rows | 60 fps, 27% busy, 500 rows | 60 fps, 27% busy, 500 rows | 60 fps, 32% busy, 500 rows | 60 fps, 30% busy, 500 rows | 60 fps, 35% busy, 500 rows | 60 fps, 35% busy, 500 rows | 60 fps, 31% busy, 500 rows | 60 fps, 32% busy, 500 rows | 60 fps, 30% busy, 500 rows | 60 fps, 30% busy, 500 rows | 60 fps, 32% busy, 500 rows | 60 fps, 29% busy, 500 rows | 60 fps, 27% busy, 500 rows | 60 fps, 30% busy, 500 rows | 60 fps, 28% busy, 500 rows | 59 fps, 32% busy, 500 rows | 60 fps, 28% busy, 500 rows | 60 fps, 28% busy, 500 rows | 60 fps, 29% busy, 500 rows |

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

Round 15 measured what was new: Submit with `-f` on large entries, the
state event for each ask, and the wallet lock (ORANGE-012). Every row
above held.

Round 16 ran on main after the stack was merged, and every row held
within the spread above. Its finding is ORANGE-013: a paste over 1 MiB
never reached the command.

Round 17 found nothing new to fix. It ran on a new and slower container.
The full-rate Watch and terminal rows moved, but the Round 16 and
Round 17 builds run back to back, twice each, gave the same spread
(0.69 to 1.07 s and 0.90 to 1.33 s), and that code did not change. The
merged streaming submit (ORANGE-012, D147) holds its gain with a real
node: 128 MiB in 6.4 s, client heap peak 643 MiB, 635 MiB allocated
(Round 15 before it: 12 s, 1 GB, 2 GB). The node's own peak fell from
1.3 GB to 1.0 GB.

Round 18 measured the new policy import with a picked file, and every
row above held. Its finding is ORANGE-014: the picked file was read
whole into the tab before `poo` refused one over 16 MiB (500 MB: 5.3 s,
+491 MB of browser memory).

Round 19 found nothing new to fix. The views now load as one file
each, so the page fetches 24 files where it fetched 15, and the views'
files are found one import later. On this computer that costs nothing
measurable: the Round 18 and Round 19 builds run back to back gave 34 to
42 ms to DOMContentLoaded for both. The kit's new `LATENCY` setting adds
round-trip time to every request, for a page reached over a slower link
such as an SSH tunnel. At 20 ms the split adds about 55 ms to the first
load, and at 50 ms about 110 ms (236 ms against 348 ms). Listing every
module as `modulepreload` in index.html won back about half of that at
50 ms (289 ms). poo-gui listens only on loopback, so the split is kept
as it is and Orange makes no modularity challenge. If the GUI is ever
served beyond this computer, the server could add those preload links
itself from the files it serves, so no list has to be kept by hand.

Round 20 had no change to the GUI's code, and every row above held. Its
finding is ORANGE-015: the import plan drew every line at once. 20,000
issuers took 4.6 s, and 50,000 never showed a plan or offered Apply,
because of a page error.

Round 21 measured the new settings import, and every row above held.
Its finding is ORANGE-016. The plan's "Read first" line named every
defence. 50,000 peers taken with the settings made that line 1.5 million
characters long and 183,000 pixels tall, above the table and Apply.

Round 22 measured the new Peers view against a real node whose peer book
was full: 4,096 addresses, the book's default limit (`peers.Limits`),
each with three mint policies. Every row above held. Page load and
reload ran slower in the full run, but the Round 21 and Round 22 builds
run back to back, twice each, gave the same spread for both (39 to 57 ms
and 247 to 396 ms).

- `poo peers` printed 3.0 MB in 0.14 s. The Peers view opened in 0.6 to
  1.1 s. Its 10 s refresh used about 5% of the page's time, and each
  refresh paused the page for 0.3 to 0.5 s at worst. That is in line
  with the other refreshing views.
- Find peers runs poo's own filter, so a narrow filter prints and draws
  little. A filter that matches the whole book costs what the panel
  does.
- Import peers previewed a file of 10,000 addresses in 0.6 s and one of
  50,000 (4.3 MB) in 0.9 s. The plan table drew its first 500 lines
  (ORANGE-015), and Apply was pinned to the file's digest. Export of the
  full book printed 1.35 MB, drawn like the 1.5 MB JSON row.

Tried and not kept: drawing a timed refresh only when its output changed,
which made the refresh run out of sight. With the full book it removed
the refresh's layout (0.8 s a minute) and halved the time the page was
busy (5% to 2 to 3%). The longest pause fell only from about 0.36 s to
0.28 s, because the result is still read and built. The page also held
5 to 11 MB more. That gain did not justify changing how every panel
refreshes, so nothing is proposed.

Round 23 measured the settable limits and the rival claim and build
forms, and every row above held. GitHub Actions was not running, so
every e2e check was run here on 2cacdd5, and all passed.

- The Settable limits panel and Set limits run `poo policy limits`,
  which takes 7 to 10 ms and prints 1.7 KB.
- Build a rival claim and Build with a parent run what Build and Add a
  layer already ran, with one flag more. Their cost is the grinding
  asked for, as measured for the Work view.

The finding is ORANGE-017. `p2p.max_book` now goes up to 65,536, and the
Peers panel prints the whole book every 10 s. At 16,384 addresses that
is 12.8 MB each time: poo-gui holds about 240 MB and the page pauses for
up to 0.5 s.

Round 24 measured Grey's Part A changes and the variable supply plan,
and every row above held. GitHub Actions was not running, so every e2e
check was run here on 2aceb1a.

- Set limits now draws one field per limit from the panel's run. That is
  about 15 fields from the same 1.7 KB `poo policy limits` output, drawn
  once per run, and the panel has no timer.
- The required-field check (GUI-026) reads the form's values once per
  press, which does not show in any measurement.

The finding is ORANGE-018, for the variable supply plan, which is not
built yet. If poo prints each part's serials and pools as lists, Series
info takes seconds and hundreds of thousands of elements to draw. As
ranges, it takes about 30 ms for any number of parts. On the way it
found that a plain list longer than 500 was cut without a note, so a
part's serials would seem to end at the 500th. That note is built here.

Round 25 measured Grey's Part A, which adopted ORANGE-017 and
ORANGE-018, and every row above held. The reload row read 417 ms once,
and 237 to 266 ms when run three more times, so that is noise. GitHub
Actions was not running, so every e2e check was run here on 88bd534.

- **ORANGE-017, adopted:** with 16,384 addresses, the Peers panel now
  costs what an empty book did. Each refresh prints 333 bytes instead of
  12.8 MB. The page is 1% busy instead of 6 to 7%, with no pauses
  instead of 0.5 s, and poo-gui holds 16 MB instead of 240 MB. Find
  peers prints the whole book only when pressed (1.7 to 2.6 s).
- **ORANGE-018, adopted:** `poo series parts` prints ranges. Its largest
  run, 1,000 parts, is 228 KB and shows in 0.2 s with 500 rows drawn.
- **Set limits** keeps unsaved typing across Refresh. That is the same
  one redraw per run, so it costs nothing new.
- **New series, Sign a series, Release a part and Check a release** each
  run one short poo command: 8 to 10 ms, under 500 bytes of output.

Nothing new to propose this round.

Round 26 measured the Variable series panel, Add a release and the
waiting state, and every row above held. GitHub Actions was not running,
so every e2e check, including variable.mjs, was run here on eb74797.

- The finding is ORANGE-019. A node can follow up to 4,096 series, and
  the Variable series panel drew a row for every one: 32,800 elements,
  more than twice the rest of the Node view. Drawn 500 rows at a time,
  the table is 4,000 elements and the panel shows in 0.6 to 0.8 s
  instead of 1.1 to 1.25 s. The fix is built and qol.mjs checks it.
- Add a release took 209 ms, including the save, on a node with 4,096
  series.
- The waiting status is one test of a finished run's error text.

Round 27 measured GUI Grey's Part A and the lazy "Output as printed"
that Grey asked about. Every row above held. The re-check ran on this
branch, so the results it draws go through the lazy path. The 1.5 MB
JSON row is a result that nothing folds, and it drew in the same time
as before. The reload row read 427 ms once, then 229 to 251 ms in three
more runs. GitHub Actions was not running, so all ten e2e checks were
run here on b9fed52.

The finding is ORANGE-020. Five panels fold poo's JSON into "Output as
printed", but each run drew that JSON first, then hid it. With 4,096
followed series, the hidden copy was 32,000 of the Node view's 61,000
elements. Drawn only when opened, the view has 28,900 elements, and the
Variable series panel shows in 0.35 to 0.42 s instead of 0.45 to 1.0 s.
Opening "Output as printed" takes as long as it did before. This is
built here, and qol.mjs checks it.

Round 28 measured GUI Grey's Part A: key algorithms (`-alg`), the
witness ceiling and reserve fields, and ORANGE-020 as adopted. Every row
above held, and nothing new is proposed. GitHub Actions was not running,
so all ten e2e checks were run here on adcf216.

- **SLH-DSA-SHA2-256s signing:** the probe pressed Sign a series three
  times for each key. With an SLH-DSA-SHA2-256s issuer key, the time
  from pressing Enter on the seed to the drawn result was 0.82 s, and
  0.95 to 1.0 s with a click on another field in between. With ed25519
  it was 0.17 to 0.19 s. On the command line, `poo series sign` and
  `poo series release` with that key took 0.69 to 0.74 s and printed
  60 KB. The witness hex is shortened on screen by `render.js`.
- **The page while poo signs:** poo signs in poo-gui, not in the page.
  The page kept 60 fps (largest frame gap 17 to 33 ms) with no long
  tasks, and a click on another field answered in 41 to 48 ms.
- **The new fields and the far-part question:** these add no commands
  and no measurable work. Making an SLH-DSA key took 55 ms, and checking
  its release took 11 ms.

Round 29 measured GUI Grey's Part A, and every row above held. The
reload row read 366 ms once and 251 to 266 ms in three more runs. GitHub
Actions was not running, so all ten e2e checks were run here on 4e1c601.

Part A added the entry cache and file limit lines, the Followed series
panel with its ceiling column, and the import warnings. None of them
cost anything measurable. On the reserve series, the Node view settled
in 0.63 to 0.79 s on 4e1c601, against 0.61 to 1.25 s on adcf216, with
23,477 elements against 23,385.

The finding is ORANGE-021, about the two 1,024-row pool tables that
Green pointed to. What the node follows runs once. Followed pools runs
`poo status` every 5 s and prints every pool. With the reserve series,
which every new node follows (D170), that is 352 KB every 5 s: the page
is 5% busy and pauses about 150 ms on every refresh. With 8,192 pools it
is 2.8 MB, with pauses up to 280 ms, and poo-gui holds 140 MB. A series
of 2 pools costs 1%. The fix starts in Source, like ORANGE-017: a `poo
status` option without the pool rows for the timed panel.

Round 30 measured GUI Grey's Part A, which adopted ORANGE-021 on `poo
status -counts` (D177) and GUI-GREEN-055's `parseJSON`. Every row above
held. GitHub Actions was not running, so all ten e2e checks were run
here on 0ade046 with this branch's change.

- **ORANGE-021, adopted:** on the reserve series, Followed pools now
  runs `poo status -counts` every 5 s, which prints 629 B in 21 ms
  instead of 352 KB. Over 60 s the page is 1 to 2% busy with no long
  tasks and frame gaps of 33 ms at most, and poo-gui holds 20 MB
  (before: 5%, a 150 ms pause on every refresh, 30 MB). The Node view
  settles in 0.54 to 0.57 s with 23,593 elements.
- **At 4,096 series (8,192 pools):** the counts answer still has a row
  per series, 910 KB in 157 ms every 5 s. The page is 4% busy with 12
  long tasks a minute of 84 to 103 ms and frame gaps up to 117 ms, and
  poo-gui holds 81 to 85 MB (before: 6%, pauses up to 280 ms, 140 MB).
  That is the most a node can follow, so nothing more is proposed for it.
- **The Node view on opening:** Each pool (10,596 elements) and What
  the node follows (12,546) run once when the view opens and make up
  nine tenths of its elements on the reserve series. They are not on a
  timer, and the view settles in about half a second, so this is noted
  only.

The finding is ORANGE-022. `parseJSON` skipped its slow scan only when
the text had no 16 digits in a row, but poo's hex hashes nearly always
have such a run, so every long answer was scanned. Looking for the 16
digits only where a JSON number can start brings `poo status` with 1,024
pools from 7 ms back to 1 ms, as `JSON.parse`, and 20,000 Watch events
from 38 ms to 10 ms. This is built here, and qol.mjs checks it.

## How to try the prototypes

```sh
git checkout claude/gui-green
go -C gui build -o /tmp/poo-gui ./cmd/poo-gui && /tmp/poo-gui
node gui/e2e/qol.mjs /tmp/poo-gui     # the checks for 001 to 003, 007 to 010, 016, 017, 019, 020, 028, 029, 032, 036, 039, 040, GUI-GREEN-041 and 046
node gui/e2e/watch.mjs /tmp/poo-gui   # the checks for 012 to 014 and 018
node gui/e2e/parity.mjs /tmp/poo-gui  # the checks for GUI-GREEN-042, 043, 045 and 048
node gui/e2e/work.mjs /tmp/poo-gui    # the checks for 023 to 025, 030, 032, 033, 035 and 038
```
