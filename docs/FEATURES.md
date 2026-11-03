# Features

## Summary

This is the list of what the project offers today, one feature per row, in
two parts:

- **Source** is the headless node, `poo`.
- **GUI** is `poo-gui`, a visual layer over the same `poo` commands.

The GUI never has a feature the Source lacks. Every GUI feature runs a
`poo` command (see [gui.md](gui.md)).

It is a plain list, kept for the later docs site and download page:

- Source/Grey updates the Source part at the end of each Source Part A.
- GUI/Grey updates the GUI part at the end of each GUI Part A.

Each row names the command, the package that holds the code, and the
decision that records it. Each package has its own document in
[modules/](modules/), which says how it can be extended.

## Source (headless node and poo)

### Node and node policies

| Feature | Commands | Code | Decisions |
| --- | --- | --- | --- |
| Full node: follows series, pools and mints, keeps their ledgers, takes and forwards transactions, entries and layers | `poo init`, `poo node`, `poo status` | `pkg/engine`, `pkg/node`, `pkg/daemon` | M1, M2 |
| Node policies: what a node knows, accepts and follows, how many branches it keeps, its limits, logs and ports, in `node.json` | `poo init`, `poo follow`, `poo unfollow`, `poo reload` | `pkg/nodepolicy` | D93, D139 |
| Several node policies side by side: save, use, archive, unarchive, delete | `poo policy list` and the `use`, `archive`, `unarchive` and `delete` subcommands | `pkg/nodepolicy` (Manager) | M2 |
| **Policy export and import:** one shareable JSON file holding a series, a node policy or every saved policy. Import shows a plan first, merges, lists conflicts, puts every node-setting line first and counts the pools each follow takes on. By default it takes only what to follow; `-settings` takes the node's settings too, and `-full` this machine's own. A node bundle is planned whole before anything is saved, so a bad policy in it changes nothing; if a save then fails, the error names what was already saved. The plan prints the file's digest, and `-apply -if-digest` applies only the file previewed. `-` reads standard input. A running node applies it live | `poo policy export`, `poo policy import` | `pkg/nodepolicy` (bundle.go), `pkg/cli` (policyport.go), `POST /v1/policy/import` | D149, D150, D151 |
| **Settings export and import:** the node's settings (everything but what it follows: peers, registries, limits, p2p switches, tracking, logs, grind defaults and jobs) in one file, to carry to another machine, keep a backup or share a setup. No key, seed, passphrase or token is in it, and a plain export leaves out what names the owner or opens a door: registry files, registry URLs with a user, password or query, grind jobs on one payment or entry (D156). It says how many peer addresses and registries it holds. Machine-local settings and those go in only with `-full`. Import is the same as policy import: plan first, conflicts, defences first, `-replace`, `-if-digest`, applied live by a running node. It never takes what to follow. `-map-path OLD=NEW` points the file's paths at this machine's, and the preview lists what may not fit here (a device, threads, a file, an address) | `poo settings export`, `poo settings import` | `pkg/nodepolicy` (settings.go), `pkg/cli` (settingsport.go), `POST /v1/policy/import` | D155 |
| Live reload of a changed policy without stopping the node | `poo reload` | `pkg/daemon`, `pkg/rpc` | GREEN-011 |
| Setup check that says what to fix | `poo doctor` | `pkg/cli` | GREEN-035 |
| Structured logs: plain, text or JSON | `poo node -log-format json` | `pkg/logging` | D104 |

### Network

| Feature | Commands | Code | Decisions |
| --- | --- | --- | --- |
| Framed binary wire protocol, forwarding and catch-up from any depth | | `pkg/wire`, `pkg/p2p`, `pkg/chase` | M3, D116 |
| Peer discovery, relaying, address registries | | `pkg/p2p`, `pkg/registry` | M4 |
| Peer categories, ranking, tracking, timeouts and blocks | `poo peers`, `poo peer`, `poo tracking`, `poo chases` | `pkg/peers`, `pkg/tracking` | M5 |
| **Peer book by hand:** add an address (`-keep` also lists it in `p2p.peers`), remove it, give it a label, dial it now | `poo peer add`, `poo peer remove`, `poo peer label`, `poo peer connect` | `pkg/peers`, `pkg/p2p` (admin.go), `POST /v1/peer` | D157 |
| **Peer filters:** the book by category (book state), mint policy, label or address; the tracking view by blocked or timed-out | `poo peers -category -mint-policy -label -addr`, `poo tracking -blocked -timed-out` | `pkg/cli` (peersport.go) | D157 |
| **Peer export and import:** the whole book or chosen categories (candidate, priority, backup, archived, unlisted, blocked, timed-out) in one file with labels and blocks. Imported addresses always land unlisted; blocks are taken only with `-blocks` (adding or lengthening, never on a listed address), ranges only with `-ranges`. Imported addresses take at most a quarter of the book and give way to addresses the node learns; `poo peer remove -imported` takes them out (D158). Plan first, with what it will dial and how much it blocks; `-if-digest` binds the plan applied; applied live by a running node | `poo peers export`, `poo peers import`, `poo peer remove -imported` | `pkg/peerport`, `pkg/cli` (peersport.go), `POST /v1/peers/import` | D157 |
| **Imported addresses are dialled when needed:** when a mint policy is short of peers the manager promotes imported (unlisted) addresses to candidates, oldest first, before asking peers and registries; `p2p.no_dial_imported` turns this off and `poo peer connect -imported` does it now | `poo peer connect -imported` | `pkg/p2p` (discovery.go), `pkg/peers` | D157 |
| **Settable peer limits:** the peer book's size and per-range share, verified peers per range, retry and registry pacing, and the tracker's address count and memory, each with a floor and a ceiling; out of range is refused with the setting's name | `p2p.max_book`, `p2p.book_per_range`, `p2p.verified_per_range`, `p2p.retry_seconds`, `p2p.registry_seconds`, `tracking.max_addresses`, `tracking.flag_history_seconds`, `tracking.log_seconds`, `tracking.log_size` in node.json | `pkg/nodepolicy` (limits.go) | D159 |
| A regular weight check of every priority peer that cannot be turned off (at most an hour apart), so a node learns of a heavier ledger its peers hold even when they send no notices | `check_seconds` in node.json | `pkg/p2p`, `pkg/nodepolicy` | D153 |

### Viewing and watching

| Feature | Commands | Code | Decisions |
| --- | --- | --- | --- |
| One filter (series, pool, mint, address, address policy, transaction, kind) for every view | the `-series`, `-pool`, `-serial`, `-address`, `-tid` flags | `pkg/filter` | M10 |
| Views: pools, ledger pages, pending, unspent, one transaction, an address | `poo pools`, `poo ledger`, `poo pending`, `poo unspent`, `poo tx`, `poo address` | `pkg/watch`, `pkg/rpc` | M10 |
| Older forms kept for scripts: a mint's unspent portions, where a transaction stands | `poo holdings`, `poo payment` | `pkg/cli` | M6 |
| Live events, one JSON object per line | `poo watch` | `pkg/watch` | M10 |
| Table output for reading | `poo pools -o table` | `pkg/cli` | |
| View-only mode: reads only, with a separate token | `poo -view-only status` | `pkg/cli`, `pkg/rpc` | D101 |
| Prometheus metrics | `GET /v1/metrics` | `pkg/rpc` | GREEN-002 |

### Work (grinding)

| Feature | Commands | Code | Decisions |
| --- | --- | --- | --- |
| Building entries and adding layers | `poo build`, `poo work`, `poo submit` | `pkg/engine`, `pkg/grind` | M1, D143 |
| Grinders on any hardware, for any hash: devices, benchmarks, jobs | `poo grind devices`, `poo grind bench`, `poo grind run`, `poo grind jobs` | `pkg/grind` | M11, D109 |
| Scheduled and automatic jobs in the node policy, applied live | `poo grind schedule` | `pkg/nodepolicy` (grindedit.go) | D113, D125 |
| Layer floor: keep every layer above a strength (node policy) | `poo grind schedule` (set, with a layer floor) | `pkg/grind` | D118 |

### Wallet

| Feature | Commands | Code | Decisions |
| --- | --- | --- | --- |
| Keys from one sealed seed, any allowed key algorithm | `poo wallet create`, `poo wallet seed`, `poo wallet passwd` | `pkg/wallet` | M6 |
| Claims, payments with coin selection, change | `poo wallet claim`, `poo wallet pay` | `pkg/wallet` | M6 |
| Payment requests and receiving | `poo wallet request`, `poo wallet receive`, `poo wallet status` | `pkg/wallet` | M6 |
| Co-signing and light mode | `poo wallet cosign`, `poo wallet watch` | `pkg/wallet` | M6 |
| Safe concurrent use: no command undoes another's change, and a passphrase change cancels every command that was already waiting, `seed` and `cosign` included | | `pkg/wallet` | D145, D146, D149, D151 |

### Series and cryptography

| Feature | Commands | Code | Decisions |
| --- | --- | --- | --- |
| Series of mints and pools, with an issuer witness | `poo series new`, `poo series info`, `poo series example` | `pkg/series` | M1 |
| Choice of hash, key algorithms and address policy types per series | `poo crypto`, `poo keygen` | `pkg/hash`, `pkg/policy` | M9 |

### Tools

| Feature | Commands | Code | Decisions |
| --- | --- | --- | --- |
| In-process demo of a payment, a double spend and its resolution | `poo demo` | `pkg/cli`, `pkg/sim` | M1 |
| Deterministic network simulator: load, partition, attack, sizing | `poo sim load` | `pkg/sim` | M7 |
| Shell completion | `poo completion bash` | `pkg/cli` | |
| Fuzzing of every decoder, benchmarks, reproducible release builds | `scripts/fuzz.sh`, `scripts/release.sh` | | M7, M8 |

## GUI (`poo-gui`)

GUI/Grey keeps this part. Every row runs `poo` commands through `cli.Main`
(D84). The views are listed in [gui.md](gui.md#views).

### Foundation

| Feature | What it does | Code | Decisions |
| --- | --- | --- | --- |
| Same code as headless | Every action is a `poo` command line run through `cli.Main`. The GUI may import only `pkg/cli`, and a test enforces it | `gui/`, `gui/imports_test.go` | D84 to D89 |
| Shows its command | Each panel and form shows the exact line it runs, quoted for the platform's shell, with Run in terminal and Copy (off while the line holds a secret flag, and on Windows off for a word PowerShell or cmd.exe would run or change) | `gui/web/static/views.js`, `gui/web/static/api.js` (`copyLine`) | GUI-GREEN-046, GUI-024 |
| One file per view | Each view is a module in `views/` listed once in `views/index.js`; navigation and the e2e walks read that list | `gui/web/static/views/` | GUI-GREEN-044 |
| Terminal | Runs `poo` itself: a typed line runs exactly as in a shell, split the way the platform's shell splits it, with history, Ctrl-C and Ctrl-D | `gui/shell`, `gui/web/static/terminal.js` | D86, GUI-021 |
| Local web app | One Go binary on loopback that opens the system browser, or an app window with `-app`. No CGO and no JavaScript build | `gui/cmd/poo-gui`, `gui/server` | D85 |
| Locked down | One-time sign-in address, session cookie, cross-origin requests refused, strict content security policy | `gui/server` | GUI-001 to GUI-005 |
| View-only mode | `poo-gui -view-only` runs every command in the core's view-only mode | `gui/server` | D101 |
| Masked secrets | A passphrase or seed is masked exactly when `poo` reads one, and asked for again on a second prompt. No form puts a seed on its command line: Make a key and Restore ask for it (D98) | `gui/jobs`, `gui/web/static/views/crypto.js` | D101, GUI-020, GUI-GREEN-047 |
| Jobs | Every command run, with live output, Stop, Run again and bounded scrollback. Node, Watch and grind jobs keep running across views | `gui/jobs`, `gui/server` | ORANGE-001 to ORANGE-003 |
| Long input | A pasted entry of any size reaches `poo`: it is sent in parts, and end of input waits for all of it | `gui/jobs`, `gui/web/static/api.js` | ORANGE-013 |
| Settings | `-root` and `-policy`, set once and added by every view | `gui/web/static/where.js` | |

### Views

| Feature | What it does | Commands |
| --- | --- | --- |
| Getting started | First steps, each a command line to run | |
| Node | Start and stop the node, status, follow (with `-replace`), unfollow, reload, init; a notice when a node.json change waits for a reload, with Reload now | `poo node`, `status`, `follow`, `unfollow`, `reload`, `init` |
| Doctor | `poo doctor`'s report, where each problem offers its fix command exactly as printed | `poo doctor` |
| Watch | Live events, newest first, with the filter flags, pause, counts and each event in full | `poo watch` |
| Work | Devices, benchmarks and jobs; a live run shows the plan, the rate and each layer or entry found | `poo grind` |
| Grind schedule | A table of scheduled jobs with Remove (guarded against stale changes), Add a job, and the defaults: device, threads, layer floor and rate, with the node's limits as hints | `poo grind schedule` |
| Explore | Pools, the ledger page by page, pending, unspent, a transaction and an address, with the shared filter. Ids and addresses link to each other | `poo pools`, `ledger`, `pending`, `unspent`, `tx`, `address` |
| Pools and payments | Status, holdings, payment, build an entry or add a layer (optionally printing the hex), submit as hex or from a file | `poo build`, `work`, `submit` |
| Wallet | Every wallet command, with one series choice shared by all of them | `poo wallet` |
| Peers | Peers, the peer book filtered (categories, mint policy, label, address), add, remove, label or connect a peer now (with every imported address), peer actions, tracking (only blocked or timed out) and chases | `poo peers`, `peer`, `tracking`, `chases` (D157) |
| Peer export and import | Export the peer book, or some categories, and the blocks, with Save as file. Import a path or a picked file: a preview of the addresses, mint policies, labels and blocks it adds, with the blocks read first (a Read first line, leading the table, counted; GUI-025), then Apply pinned with `-if-digest`. Blocks only with Take blocks | `poo peers export`, `poo peers import` (D157) |
| Node policies | List, use, archive, unarchive and delete saved policies | `poo policy` |
| Policy export and import | Export this policy, one series or every saved policy, to a file or the screen with Save as file. Import a path or a file picked in the browser (streamed as it is read): a preview that shows first what changes a defence, then what is new, changed, in conflict, refused or left alone. Only what to follow is taken unless node settings (`-settings`) or every setting (`-full`) is asked for. Apply carries out exactly the previewed file (`-if-digest`), and any change to the form takes Apply away. Registry paths from another machine are mapped (`-map-path`). Long plans draw 500 lines at a time | `poo policy export`, `poo policy import` (D149, D150, D151, GUI-023, ORANGE-014) |
| Settings export and import | Export this node's settings (not what it follows) to a file or the screen, with Save as file; `-full` keeps machine-local ones. Import shows the same preview, with what may not fit this machine, series now used and defences first, then Apply pinned with `-if-digest` | `poo settings export`, `poo settings import` (D155, D156) |
| Series and keys | Crypto choices, keygen, new series (with an example) and series info | `poo crypto`, `keygen`, `series` |
| Simulate | The network simulator's scenarios | `poo sim` |

### Quality checks

| Feature | What it does | Code |
| --- | --- | --- |
| Flag parity | CI fails if a subcommand `poo help` lists is run by no view, a flag `poo CMD -h` lists is sent by no form (unless listed with a reason), a field's stated default differs from `poo -h`, or a field's fixed set of values (book states, event kinds) differs from the one `poo -h` lists | `gui/e2e/parity.mjs` (GUI-GREEN-042, 043, 045, 048) |
| End-to-end tests | Nine browser scripts run in CI against real nodes | `gui/e2e` |
| Performance re-checks | GUI/Orange measures a soak, page load, Watch and Work at high rates and large results every round | `gui/perf`, `docs/proposals/gui/README.md` |

## Adding a feature

Each feature lives in its own package or file behind a small API. The CLI
command and the RPC endpoint are thin layers over it, and the GUI is a
form over the CLI command. A new feature follows the same pattern:

1. Write the code in the package that owns the data, with tests.
2. Add a `poo` command, and an RPC endpoint if a running node must apply
   it. A test fails when an RPC call has no command.
3. Add a row here, the module doc, and a decision.

Then the GUI, where a feature is a form over its command:

4. Add the form to the view it belongs in, in `gui/web/static/views/`,
   with `form()` and `flag()` from `views.js`. Its fields send the
   command's flags. For a new view, add a file there that exports
   `{ id, title, mount }`, and list it in `views/index.js`.
5. Run `gui/e2e/parity.mjs`. It walks every view in the navigation and
   fails if a flag of the command is sent by no form, if a field states
   a default that differs from `poo -h`, or if a field that offers a fixed
   set of values (`type: 'choices'` or a select, with `flag:` set) offers
   other values than `poo -h` lists. Add a reason to its `LEFT_OUT`
   only for a flag that belongs in the terminal.

The policy bundle is built to grow in the same way: its format carries a
version and a scope (series, policy, node, settings), and it has a
reserved slot for settings kept outside node.json.
