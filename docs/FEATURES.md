# Features

## Summary

This is the list of what the project offers today, one feature per row, in
two parts:

- **Source** is the headless node, `poon`.
- **GUI** is `poon-gui`, a visual layer over the same `poon` commands.

The GUI never has a feature the Source lacks. Every GUI feature runs a
`poon` command (see [gui.md](gui.md)).

It is a plain list, kept for the later docs site and download page:

- Source/Grey updates the Source part at the end of each Source Part A.
- GUI/Grey updates the GUI part at the end of each GUI Part A.

Each row names the command, the package that holds the code, and the
decision that records it. Each package has its own document in
[modules/](modules/), which says how it can be extended.

## Source (headless node and poon)

### Node and node policies

| Feature | Commands | Code | Decisions |
| --- | --- | --- | --- |
| Full node: follows series, pools and mints, keeps their ledgers, takes and forwards transactions, entries and layers; `poon status -counts` gives one row of totals per series | `poon init`, `poon node`, `poon status` | `pkg/engine`, `pkg/node`, `pkg/daemon` | M1, M2, D177 |
| Node policies: what a node knows, accepts and follows, how many branches it keeps, its limits, logs and ports, in `node.json` | `poon init`, `poon follow`, `poon unfollow`, `poon reload` | `pkg/nodepolicy` | D93, D139 |
| Several node policies side by side: save, use, archive, unarchive, delete | `poon policy list` and the `use`, `archive`, `unarchive` and `delete` subcommands | `pkg/nodepolicy` (Manager) | M2 |
| **Policy export and import:** one shareable JSON file holding a series, a node policy or every saved policy. Import shows a plan first, merges, lists conflicts, puts every node-setting line first and counts the pools each follow takes on. By default it takes only what to follow; `-settings` takes the node's settings too, and `-full` this machine's own. A node bundle is planned whole before anything is saved, so a bad policy in it changes nothing; if a save then fails, the error names what was already saved. The plan prints the file's digest, and `-apply -if-digest` applies only the file previewed. `-` reads standard input. A running node applies it live | `poon policy export`, `poon policy import` | `pkg/nodepolicy` (bundle.go), `pkg/cli` (policyport.go), `POST /v1/policy/import` | D149, D150, D151 |
| **Settings export and import:** the node's settings (everything but what it follows: peers, registries, limits, p2p switches, tracking, logs, grind defaults and jobs) in one file, to carry to another machine, keep a backup or share a setup. No key, seed, passphrase or token is in it, and a plain export leaves out what names the owner or opens a door: registry files, registry URLs with a user, password or query, grind jobs on one payment or entry (D156). It says how many peer addresses and registries it holds. Machine-local settings and those go in only with `-full`. Import is the same as policy import: plan first, conflicts, defences first, `-replace`, `-if-digest`, applied live by a running node. It never takes what to follow. `-map-path OLD=NEW` points the file's paths at this machine's, and the preview lists what may not fit here (a device, threads, a file, an address) | `poon settings export`, `poon settings import` | `pkg/nodepolicy` (settings.go), `pkg/cli` (settingsport.go), `POST /v1/policy/import` | D155 |
| **Entry bodies on disk:** every held entry's skeleton stays in memory; its body is read back from the pool log by location, through one cache bounded in MiB, so memory no longer grows with every body a node holds. Start-up and compaction stream the log. A small cache costs reads, never changes a verdict. The cache counts what bodies really take (256 MiB by default). A body that cannot be read back (a damaged log) is not held for peers and queries, which say so, and `poon doctor` warns | `entry_cache_mib` in node.json (`poon policy limits -set entry_cache_mib=N`, in place), `bodies` in `poon status` and `poon doctor`, `poon_entry_cache_*` metrics | `pkg/store` (ReadAt), `pkg/ledger` (bodies.go), `pkg/node` (bodies.go) | D172, D173 |
| **Items and undo on disk:** entries more than `recent_window` below each selected tip keep only their skeleton; a switch at any depth reads back what it reverts before changing anything (slower, never refused), and a failed read stops the node rather than deciding anything, on every path that reads under the engine lock; a revert's cost counts the maker bodies it reads | `recent_window` in node.json (`poon policy limits -set recent_window=N`, on reload), `bodies.cold` in `poon status` | `pkg/ledger` (cold.go) | D175, D177 |
| **Pool snapshots:** each pool's skeleton, tip state, pending transactions and body index (the sidecar index) saved beside its log, checksummed and renamed into place, every `snapshot_every` records or a tenth of the log, whichever is more (ORANGE-040); a start restores it and replays only newer records (6 times faster on 20,000 entries, more with post-quantum keys). A torn, damaged, swapped, stale or other-release snapshot, or one that does not restore the pool it was written from, is never used, and a damaged log or a failed engine removes it, so the next start replays and cuts the damage. `-reverify` replays everything and checks the snapshot matched, and says when it compared nothing. A temporary file a crash left is removed at start | `poon node -reverify`; `snapshot_every` and `no_snapshots` in node.json (`poon policy limits -set snapshot_every=N`, in place); `snapshots` in `poon status` and `poon doctor` | `pkg/ledger` (snapshot.go), `pkg/node` (snapshot.go), `pkg/store` (Tag) | D178, D181, D182, D184 |
| Open file limit check: a warning at start and in `poon doctor` when the limit is below the followed pools plus 256 | `poon doctor` (`open-files`), `file_limit` in `poon status` | `pkg/nodepolicy` (filelimit.go) | D172 |
| **Rollback-safe node.json:** settings a newer release wrote are kept on every save by an older one (at the top level and inside sections, up to 256 settings of 64 KiB and 64 levels each, written compacted), with a warning at start naming the first ten; a key that is a setting's name in another letter case is that setting and is saved under its own name; an import keeps them too | | `pkg/nodepolicy` (unknown.go) | D184, D185 |
| **Release in status:** the running node's version and commit, so a front end of another release can tell | `release` in `poon status` and `GET /v1/status` | `pkg/node`, `pkg/rpc` | D184 |
| **Default ports 7732 and 7733:** local commands on `127.0.0.1:7732`, peers on `:7733`; `poon init` writes both into a new node.json, so a later default never moves an existing node | `poon init` | `pkg/nodepolicy`, `pkg/cli` | D187, D190 |
| Live reload of a changed policy without stopping the node | `poon reload` | `pkg/daemon`, `pkg/rpc` | GREEN-011 |
| Setup check that says what to fix | `poon doctor` | `pkg/cli` | GREEN-035 |
| Structured logs: plain, text or JSON | `poon node -log-format json` | `pkg/logging` | D104 |

### Network

| Feature | Commands | Code | Decisions |
| --- | --- | --- | --- |
| Framed binary wire protocol, forwarding and catch-up from any depth | | `pkg/wire`, `pkg/p2p`, `pkg/chase` | M3, D116 |
| Peer discovery, relaying, address registries | | `pkg/p2p`, `pkg/registry` | M4 |
| Peer categories, ranking, tracking, timeouts and blocks | `poon peers`, `poon peer`, `poon tracking`, `poon chases` | `pkg/peers`, `pkg/tracking` | M5 |
| **Peer book by hand:** add an address (`-keep` also lists it in `p2p.peers`), remove it, give it a label, dial it now | `poon peer add`, `poon peer remove`, `poon peer label`, `poon peer connect` | `pkg/peers`, `pkg/p2p` (admin.go), `POST /v1/peer` | D157 |
| **Peer filters:** the book by category (book state), mint policy, label or address; the tracking view by blocked or timed-out | `poon peers -category -mint-policy -label -addr`, `poon tracking -blocked -timed-out` | `pkg/cli` (peersport.go) | D157 |
| **Peer counts:** the book's size and the addresses in each state, without the book, for a view that refreshes often | `poon peers -counts`, `GET /v1/peers?counts=1` | `pkg/cli` (peersport.go), `pkg/rpc` | D164 |
| **Peer export and import:** the whole book or chosen categories (candidate, priority, backup, archived, unlisted, blocked, timed-out) in one file with labels and blocks. Imported addresses always land unlisted; blocks are taken only with `-blocks` (adding or lengthening, never on a listed address), ranges only with `-ranges`. Imported addresses take at most a quarter of the book and give way to addresses the node learns; `poon peer remove -imported` takes them out (D158). Plan first, with what it will dial and how much it blocks; `-if-digest` binds the plan applied; applied live by a running node | `poon peers export`, `poon peers import`, `poon peer remove -imported` | `pkg/peerport`, `pkg/cli` (peersport.go), `POST /v1/peers/import` | D157 |
| **Imported addresses are dialled when needed:** when a mint policy is short of peers the manager promotes imported (unlisted) addresses to candidates, oldest first, before asking peers and registries; `p2p.no_dial_imported` turns this off and `poon peer connect -imported` does it now | `poon peer connect -imported` | `pkg/p2p` (discovery.go), `pkg/peers` | D157 |
| **Settable peer limits:** the peer book's size and per-range share, verified peers per range, retry and registry pacing, how long an unreached candidate keeps its place, the tracker's address count and memory, and the seen list, pending pool and ties, each with a floor and a ceiling; out of range is refused with the setting's name | `p2p.max_book`, `p2p.book_per_range`, `p2p.verified_per_range`, `p2p.retry_seconds`, `p2p.registry_seconds`, `p2p.candidate_seconds`, `tracking.max_addresses`, `tracking.flag_history_seconds`, `tracking.log_seconds`, `tracking.log_size`, `seen_size`, `max_pending`, `ties`, `entry_cache_mib`, `recent_window`, `snapshot_every`, `p2p.port_map_seconds` in node.json; `poon policy limits` lists them and `poon policy limits -set NAME=VALUE` changes them, live in a running node: the peer book, tracker, seen list and entry cache limits in place with no reload (D168, D172), the others by a reload; each limit says which (`in_place`) | `pkg/nodepolicy` (limits.go), `POST /v1/policy/limits` | D159, D160, D161, D168, D170 |
| **A peer book nobody can fill for good:** when the book or a range is full, an imported address, then a learned one whose last try failed, then a learned one older than `p2p.candidate_seconds` gives way to a new address; the owner's own add takes any learned place, reached or not, but a peer in use; addresses the owner gave or labelled always stay; host names count in the network range they lead to, so one server holds at most a range's share; a lowered `max_book` is applied when the node starts | `poon peer add`, `p2p.candidate_seconds` in node.json | `pkg/peers` (Book.Fit), `pkg/daemon` | D160, D162 |
| **Port mapping (opt in):** asks the home router to forward the peer port by PCP, then NAT-PMP, then UPnP, speaking only to the gateway with capped replies and short waits; the router's address is offered to peers only once the node's own listener receives a dial to it carrying a random token, never on peers' word (D183); renewed at half the lease (or of a shorter lifetime; one of 30 s or less refused) and removed at a clean stop, and a failed request leaves nothing at the router (D183); a manual `p2p.advertise` wins; off by default for privacy, and machine-local, so a shared file never turns it on | `poon init -port-map`, `p2p.port_map`, `p2p.port_map_gateway`, `p2p.port_map_seconds` in node.json; `port_map` in `poon status`, `port-map` in `poon doctor` | `pkg/portmap`, `pkg/p2p` (portmap.go) | D182, D183 |
| **Release and features between peers:** the hello says which release a node runs and which features it supports beyond the protocol 1 baseline, and hello, ping and pong carry extension fields a later release can add; shown per peer and counted by release, never used to refuse, rank or judge a peer; `p2p.announce_version: "none"` hides the release | `poon peers` (`release`, `claimed_features`), `poon peers -counts` (`releases`), `p2p.announce_version` in node.json | `pkg/wire` (hello.go), `pkg/p2p` | D186, D190 |
| A regular weight check of every priority peer that cannot be turned off (at most an hour apart), so a node learns of a heavier ledger its peers hold even when they send no notices | `check_seconds` in node.json | `pkg/p2p`, `pkg/nodepolicy` | D153 |

### Viewing and watching

| Feature | Commands | Code | Decisions |
| --- | --- | --- | --- |
| One filter (series, pool, mint, address, address policy, transaction, kind) for every view | the `-series`, `-pool`, `-serial`, `-address`, `-tid` flags | `pkg/filter` | M10 |
| Views: pools, ledger pages, pending, unspent, one transaction, an address | `poon pools`, `poon ledger`, `poon pending`, `poon unspent`, `poon tx`, `poon address` | `pkg/watch`, `pkg/rpc` | M10 |
| Older forms kept for scripts: a mint's unspent portions, where a transaction stands | `poon holdings`, `poon payment` | `pkg/cli` | M6 |
| Live events, one JSON object per line | `poon watch` | `pkg/watch` | M10 |
| **Ledger export and import:** a stretch of a pool's selected ledger (`-from`, `-to`), oldest first, as raw entries with their layers that anyone can check, plus a readable summary of each; import hands them to a node, which checks every entry and layer in full, as from a peer, and says what it kept, already held or refused and why; an export fails rather than end short, an import that stops partway prints what it kept, and `-dry-run` checks a file with no node (D189); the file (kind `poon-ledger`, D190) is read as it streams, a batch at a time (ORANGE-043), with a bounded header of known keys (D191); a header field from a later release is read under the same bound and dropped (D192) | `poon ledger export`, `poon ledger import [-dry-run]`, `GET /v1/ledger/export`, `POST /v1/ledger/import` | `pkg/watch` (export.go), `pkg/node` (import.go), `pkg/cli` (ledgerport.go) | D188, D189, D190 |
| Table output for reading | `poon pools -o table` | `pkg/cli` | |
| View-only mode: reads only, with a separate token | `poon -view-only status` | `pkg/cli`, `pkg/rpc` | D101 |
| A stale `rpc.addr` never gets the token: a command sends it only after the node proves it knows it | any command that talks to a running node | `pkg/rpc` (hello.go) | D162 |
| Prometheus metrics | `GET /v1/metrics` | `pkg/rpc` | GREEN-002 |

### Work (grinding)

| Feature | Commands | Code | Decisions |
| --- | --- | --- | --- |
| Building entries and adding layers; `poon work` and `build -work` size their default tries and the node's cap from the series' layer floor, so a default run can meet it | `poon build`, `poon work`, `poon submit` | `pkg/engine`, `pkg/grind`, `pkg/rpc` (`WorkTriesFor`) | M1, D143, D168 |
| **Rival claims and building on a branch:** claim a mint the selected ledger has claimed, on a branch from before that claim, and add weight to it until it is selected; build any entry on a chosen parent (an entry hash, the pool's first entry, or before a mint's claim) from the pending transactions that fit there | `poon claim -parent before-claim`, `poon build -parent`, then `poon work -entry` or `poon grind start ENTRY` | `pkg/engine` (BuildEntryOn), `pkg/ledger` (branch.go), `POST /v1/build` | D161 |
| Grinders on any hardware, for any hash: devices, benchmarks, jobs | `poon grind devices`, `poon grind bench`, `poon grind run`, `poon grind jobs` | `pkg/grind` | M11, D109 |
| Scheduled and automatic jobs in the node policy, applied live | `poon grind schedule` | `pkg/nodepolicy` (grindedit.go) | D113, D125 |
| Layer floor: keep every layer above a strength (node policy) | `poon grind schedule` (set, with a layer floor) | `pkg/grind` | D118 |

### Wallet

| Feature | Commands | Code | Decisions |
| --- | --- | --- | --- |
| Keys from one sealed seed, any allowed key algorithm | `poon wallet create`, `poon wallet seed`, `poon wallet passwd` | `pkg/wallet` | M6 |
| Claims, payments with coin selection, change | `poon wallet claim`, `poon wallet pay` | `pkg/wallet` | M6 |
| Payment requests and receiving | `poon wallet request`, `poon wallet receive`, `poon wallet status` | `pkg/wallet` | M6 |
| Co-signing and light mode | `poon wallet cosign`, `poon wallet watch` | `pkg/wallet` | M6 |
| **Cold keys:** a key the node never holds. Import a key made by another tool (its public key and addresses, never the key, are printed); a watch-only wallet with no seed follows keys kept elsewhere and prepares payments, with change back to the key's address or `-change`; `poon sign` signs an unsigned payment on any machine, with no node or wallet, after `-show` says what it pays (the series, hash, inputs spent and outputs; a file whose series is not its transactions' is refused, D194); the signed file is submitted as before | `poon keygen -raw`, `poon wallet create -watch-only`, `poon wallet pay -change`, `poon sign` | `pkg/policy` (raw.go), `pkg/wallet`, `pkg/cli` (sign.go) | D193, D194 |
| **Bounty market (phase 1):** pay peers to grind layers on this node's entries, with no consensus change. Post an offer (entry, least strength, amount per layer, count, how long open) to direct peers; a grinder asks for offers, takes a lease (one grinder at a time, about three times the expected time at its stated rate, more leases as it hands layers in) naming its payout address, grinds and claims; the payer checks the layer, pays with an ordinary wallet payment and tells the grinder. Every offer says its state (open, filled, closed, expired; a peer's: open, leased, taken) and when it closes, and acks name their outcome; an offer is named by its bytes, so later releases agree on its id; offers that need nothing more move to a done file; a grinder states the strongest offer it works, and a payer lists and leases only offers at or under it (D202); `-ask` says how many peers it asked, and a held offer reads filled once every layer it asks for was accepted (D204). Off until turned on as settable limits (`bounty.post`, `bounty.take` and the rest) or in node.json; `poon status` says whether it is on | `poon bounty post`, `list -ask`, `work` (`-max-strength`), `pay`, `cancel`; `poon policy limits -set bounty.*` | `pkg/bounty`, `pkg/p2p` (bounty.go), `pkg/rpc` (bounty.go), `pkg/cli` (bounty.go) | D196, D197, D198, D199, D200, D201, D202, D203, D204 |
| **Files from later releases:** payment files carry a kind and format, and a newer format is refused with a clear message; payment and wallet files keep fields a later release adds | `poon wallet pay -out`, `poon sign` | `pkg/keepjson`, `pkg/wallet` | D195 |
| Safe concurrent use: no command undoes another's change, and a passphrase change cancels every command that was already waiting, `seed` and `cosign` included | | `pkg/wallet` | D145, D146, D149, D151 |

### Series and cryptography

| Feature | Commands | Code | Decisions |
| --- | --- | --- | --- |
| Series of mints and pools, with an issuer witness | `poon series new`, `poon series info`, `poon series example` | `pkg/series` | M1 |
| Choice of hash, key algorithms and address policy types per series: five hashes, eleven signature algorithms (Ed25519, ECDSA on four curves, BIP-340 Schnorr, ML-DSA-44/65/87 and the hash-based SLH-DSA-SHA2-128s and SLH-DSA-SHA2-256s) and three policy types | `poon crypto`, `poon keygen` | `pkg/hash`, `pkg/policy` | M9, D168, D170 |
| **Built-in default series:** a built-in, pinned long-term series (32,768 mints of 10^14 units in 1,024 pools, SHA-512/256, Ed25519, ML-DSA-65/87 and SLH-DSA-128s/256s keys, a 128 KiB witness ceiling, no issuer) that a new node follows unless given others; tools name it on its full id only | `poon init` (`-default-series`, `-no-series`), `"builtin": "default"` in `poon series info` and `poon series followed` | `pkg/series` (reserve.go) | D170 |
| **Per-series witness size limits:** no network-wide size cap; each series' witness ceiling, its algorithms' exact sizes and its types' bounds decide, so 16-of-16 post-quantum custody fits the default series. An address whose largest witness would not fit its series is refused where it is made; `poon crypto` shows each algorithm's largest k under a ceiling. Tools refuse a series with no ceiling or one above 4 MiB unless forced, and a node may decline to add one (`series_ceiling`). Imports, `series info`, `series followed` and `doctor` warn of a series with no ceiling or one above 4 MiB | `poon crypto -max-witness`, `poon series new -max-witness -any-ceiling`, `poon policy limits -set series_ceiling=N` | `pkg/policy`, `pkg/cli`, `pkg/nodepolicy` | D170, D171 |
| **Variable supply series (phases 1 to 3):** a series can be variable: the issuer releases more parts of M mints later, in order, with an optional largest number of parts. Fixed series are unchanged. Nodes follow a variable series once they hold its issuer witness (part 1), keep the newest release, and take claims and spends on released parts; a transaction on a part not released yet waits, never invalid (exit code 3) | `poon series new -variable -part-size -max-parts -issuer`, `poon series sign`, `poon series release -part / -check`, `poon series parts [-o table]` (with `mints_released`), `poon series add-release`, `poon series followed`, `POST /v1/series/release` | `pkg/series` (parts.go, released.go), `pkg/tx`, `pkg/nodepolicy`, `pkg/rpc` (release.go), `pkg/vectors` (variable.go) | D163, D166, D168 |
| **Releases pass between nodes:** a node sends its peers the newest release of each variable series it follows when they connect, and passes on each new one; the receiver checks it in full, keeps it, and opens the new part's pools in place, with no reload, as a release given with `poon series add-release` now does | `poon series add-release` (`in_place` in the reply) | `pkg/p2p` (release.go), `pkg/node` (release.go), `pkg/wire` (type 14) | D168 |

### Tools

| Feature | Commands | Code | Decisions |
| --- | --- | --- | --- |
| In-process demo of a payment, a double spend and its resolution | `poon demo` | `pkg/cli`, `pkg/sim` | M1 |
| Deterministic network simulator: load, partition, attack, sizing | `poon sim load` | `pkg/sim` | M7 |
| Shell completion | `poon completion bash` | `pkg/cli` | |
| Fuzzing of every decoder, benchmarks, reproducible release builds | `scripts/fuzz.sh`, `scripts/release.sh` | | M7, M8 |

## GUI (`poon-gui`)

GUI/Grey keeps this part. Every row runs `poon` commands through `cli.Main`
(D84). The views are listed in [gui.md](gui.md#views).

### Foundation

| Feature | What it does | Code | Decisions |
| --- | --- | --- | --- |
| Same code as headless | Every action is a `poon` command line run through `cli.Main`. The GUI may import only `pkg/cli`, and a test enforces it | `gui/`, `gui/imports_test.go` | D84 to D89 |
| Shows its command | Each panel and form shows the exact line it runs, quoted for the platform's shell, with Run in terminal and Copy (off while the line holds a secret flag, and on Windows off for a word PowerShell or cmd.exe would run or change) | `gui/web/static/views.js`, `gui/web/static/api.js` (`copyLine`) | GUI-GREEN-046, GUI-024 |
| Required fields | A field a command needs is marked "(required)" while it is shown, and a form with one left empty runs nothing, since the command without it could do something else | `gui/web/static/views.js`, `style.css` | GUI-026, GUI-GREEN-050 |
| One file per view | Each view is a module in `views/` listed once in `views/index.js`; navigation and the e2e walks read that list | `gui/web/static/views/` | GUI-GREEN-044 |
| Terminal | Runs `poon` itself: a typed line runs exactly as in a shell, split the way the platform's shell splits it, with history, Ctrl-C and Ctrl-D | `gui/shell`, `gui/web/static/terminal.js` | D86, GUI-021 |
| Local web app | One Go binary on loopback that opens the system browser, or an app window with `-app`. No CGO and no JavaScript build. It comes back on the port it used last, so the browser's saved settings (node policy, wallet file, theme, terminal history) survive a restart (GUI White G1). The header shows the release it runs, the one `poon version` prints (GUI White G4, GUI-GREEN-058) | `gui/cmd/poon-gui`, `gui/server` | D85 |
| Locked down | One-time sign-in address, session cookie, cross-origin requests refused, strict content security policy | `gui/server` | GUI-001 to GUI-005 |
| View-only mode | `poon-gui -view-only` runs every command in the core's view-only mode | `gui/server` | D101 |
| Masked secrets | A passphrase or seed is masked exactly when `poon` reads one, and asked for again on a second prompt. No form puts a seed on its command line: Make a key and Restore ask for it (D98) | `gui/jobs`, `gui/web/static/views/crypto.js` | D101, GUI-020, GUI-GREEN-047 |
| Jobs | Every command run, with live output, Stop, Run again and bounded scrollback. A command still running 3 s after Stop says it is still working and holds a job slot (GUI-028). Node, Watch and grind jobs keep running across views | `gui/jobs`, `gui/server` | ORANGE-001 to ORANGE-003 |
| Exact amounts | A whole number `poon` prints above 2^53 (a balance of 91 reserve mints, say) is shown digit for digit, where a plain JSON parse would round it; results, the Followed and Variable series panels and Watch events read it this way | `gui/web/static/render.js` (parseJSON) | GUI-GREEN-055 |
| Long input | A pasted entry of any size reaches `poon`: it is sent in parts, and end of input waits for all of it | `gui/jobs`, `gui/web/static/api.js` | ORANGE-013 |
| Settings | `-root` and `-policy`, set once and added by every view | `gui/web/static/where.js` | |

### Views

| Feature | What it does | Commands |
| --- | --- | --- |
| Getting started | First steps, each a command line to run; Write a node policy is `poon init`, which follows the built-in default series (GUI-GREEN-054) | |
| Node | Start and stop the node, status, follow (with `-replace`), unfollow, reload, start with `-reverify` as a box (replay every log whole; an alert if a replay differs from its snapshot, D178), init (with no series it follows the built-in default series; `-default-series`, `-no-series` and `-any-ceiling` as boxes, D170; opt-in router port mapping, `-port-map`, with the note that peers then learn the home's public IP, D182); a notice when a node.json change waits for a reload, with Reload now; a Followed series panel with one row per series, naming the built-in default series by poon's `builtin` field and showing poon's witness ceiling `warning` per series (GUI-GREEN-054, D171); Followed pools refreshes every 5 s with poon's totals per series (`poon status -counts`, D177, ORANGE-021), drawn as a table that names the built-in default series (D179), with the node's release beside the GUI's and a note when they differ (D184, GUI White S1), lines for the pool snapshots and how this start used them, counting capped lists from poon's count fields (D178, D184), and a line for the router port mapping from `port_map` in `poon status`, an alert when it failed (D182), and Each pool shows every pool's row only when Show is pressed; What the node follows shows its rules and folds its pools under a count, drawn when opened (GUI-GREEN-056). Followed pools shows the entry cache in use, hits and disk reads from `bodies` in `poon status`, with the older entries read back from the logs outside `recent_window` (D175), and an alert when `file_limit` says the open file limit is too low (D172). `entry_cache_mib`, `recent_window`, `snapshot_every` and `p2p.port_map_seconds` are in Set limits by themselves, as are the `snapshots` and `port-map` checks in `poon doctor`, and `poon doctor`'s open-files check is drawn with the other checks | `poon node`, `status`, `follow`, `unfollow`, `reload`, `init`, `series followed` |
| Doctor | `poon doctor`'s report, where each problem offers its fix command exactly as printed | `poon doctor` |
| Watch | Live events, newest first, with the filter flags, pause, counts and each event in full | `poon watch` |
| Work | Devices, benchmarks and jobs; a live run shows the plan, the rate and each layer or entry found | `poon grind` |
| Bounty market | This node's offers with their claims (unpaid ones counted, Pay and Cancel on each row) and the offers peers sent (Work on each row, which fills the form); ask peers, post, work, pay from the Wallet view's wallet, cancel; each offer's state and closing time, with Work only on an open peer offer; long lists draw 500 rows at a time with Show more; Ask and Work take a strongest offer (`-max-strength`, D202), Ask says how many peers it asked (D204), and the notes `poon` gives on the market's settings show over the tables; when bounty list is refused, the `bounty` field of `poon status -counts` says whether the market is off, and the line says to turn it on with `bounty.post` or `bounty.take` in Set limits, never reading the error's text (GUI-GREEN-062, GUI White S5, W-30). Cancel shows only on an open offer, and closing and received times read in the reader's own time zone, poon's text in the tooltip (GUI-GREEN-063); with no offers, only the steps this node takes are offered (GUI-GREEN-064); Ask and Work take a strongest offer (`-max-strength`), and Ask says it can only narrow `bounty.max_strength` and to ask again after a reload (GUI-GREEN-065) | `poon bounty` |
| Grind schedule | A table of scheduled jobs with Remove (guarded against stale changes), Add a job, and the defaults: device, threads, layer floor and rate, with the node's limits as hints | `poon grind schedule` |
| Explore | Pools, the ledger page by page, pending, unspent, a transaction and an address, with the shared filter. Ids and addresses link to each other | `poon pools`, `ledger`, `pending`, `unspent`, `tx`, `address` |
| Pools and payments | Status (every pool's row, run when Show is pressed, GUI-GREEN-057), holdings, payment, build an entry (on the selected tip or a chosen parent) or add a layer (optionally printing the hex), submit as hex or from a file. Export a pool's ledger (a height range, to a file or the screen: a line says what the file holds, Save as file is offered only for one whole ledger file, and poon's output is folded, GUI-GREEN-059; a cut export is folded with one alert, GUI/Orange R34B; a later file format gets a neutral line and Save as file, White N2) and import a ledger file (a path or a picked file sent as `-`), with poon's count of what was kept, known and refused (D188); Only check the file runs `-dry-run`, which reads it without the node (D189), and an import that stops early shows poon's totals so far as an alert (GUI-031) | `poon build`, `work`, `submit`, `ledger export`, `ledger import` |
| Rival claims | Build a mint's claim into an entry on a chosen parent (before the claim, the pool's first entry, or an entry hash), with an optional layer, to grind and compete; the seed is typed into the masked field, with the key's algorithm (`-alg`). Parent is required, so an empty one never runs the plain claim (GUI-026) | `poon claim -parent` (D161) |
| Wallet | Every wallet command, with one series choice shared by all of them. Cold keys (D193): Create can make a watch-only wallet, Pay takes a change address, Claim a mint can claim to a wallet policy into a claim file the key signs (`-to-policy -out`, GREEN-060), and Sign with a key kept elsewhere runs `poon sign` on a co-signing file (a masked field, or a key file once "The key is in a file" is ticked, and a key pasted as the path is refused, GUI-032; `-raw`, Show only), with a line saying what the key added and what is still needed. Keep the key off this machine lists the seven steps, where each runs, with a button to each form; Pay's confirm warns when no co-signing file is filled in, since `poon` refuses a payment that needs another signature without one (GUI-GREEN-061, Source #291) | `poon wallet`, `sign` |
| Peers | Peers (the timed panel shows the book's counts per state, not the book, so a 65,536-address book costs nothing every 10 s; ORANGE-017), the peer book filtered (categories, mint policy, label, address), add, remove, label or connect a peer now (with every imported address), peer actions, tracking (only blocked or timed out) and chases. Each peer's release and claimed features show as `poon peers` prints them, said to be the peer's own claim, and the timed panel says which releases the connected peers claim, with "not shared" for none (D186, GUI White S3) | `poon peers`, `peer`, `tracking`, `chases` (D157) |
| Peer export and import | Export the peer book, or some categories, and the blocks, with Save as file. Import a path or a picked file: a preview of the addresses, mint policies, labels and blocks it adds, with the blocks read first (a Read first line, leading the table, counted; GUI-025), then Apply pinned with `-if-digest`. Blocks only with Take blocks | `poon peers export`, `poon peers import` (D157) |
| Node policies | List, use, archive, unarchive and delete saved policies | `poon policy` |
| Policy export and import | Export this policy, one series or every saved policy, to a file or the screen with Save as file. Import a path or a file picked in the browser (streamed as it is read): a preview that shows first what changes a defence, then what is new, changed, in conflict, refused or left alone, the series this node's series ceiling declines (D170), and a Read first warning, with poon's reason, for each series the import adds or follows more of that sets no witness ceiling or one above 4 MiB (D171, GUI-029); it never takes Apply away. Only what to follow is taken unless node settings (`-settings`) or every setting (`-full`) is asked for. Apply carries out exactly the previewed file (`-if-digest`), and any change to the form takes Apply away. Registry paths from another machine are mapped (`-map-path`). Long plans draw 500 lines at a time. A plan that changes nothing has no digest and no Apply, and says there is nothing to apply (GUI/Red R28B) | `poon policy export`, `poon policy import` (D149, D150, D151, GUI-023, ORANGE-014) |
| Settings export and import | Export this node's settings (not what it follows) to a file or the screen, with Save as file; `-full` keeps machine-local ones. Import shows the same preview, with what may not fit this machine, series now used and defences first, then Apply pinned with `-if-digest` | `poon settings export`, `poon settings import` (D155, D156) |
| Settable limits | Every limit the node policy can set, with its range, default, the value node.json sets and the one in effect, read from `poon policy limits`, so a new limit shows without a GUI change; Set limits has a field for each limit `poon policy limits` listed, showing its value in effect, range, default and what 0 means (GUI-GREEN-049, D204), and whether a running node takes it at once or on reload, from poon's `in_place` (D170), and saves the ones filled as `NAME=VALUE` pairs, refused whole when one is out of range; a running node takes them at once, and a line over the result says whether it applied them in place or is reloading (GUI-GREEN-053); a limit `poon` marks as a switch (`bounty.post`, `bounty.take`) is a choice of on, off or unchanged, and the notes `poon` gives on the saved settings show as lines (D198, GREEN-062); a switch node.json sets reads on (GUI-GREEN-064) | `poon policy limits`, `-set` (D159, D160, D161, D168) |
| Series and keys | Crypto choices, keygen, new series (with an example) with its mints, units per mint, pool size (GUI-GREEN-051) and witness ceiling (`-max-witness`, poon's default 131072; `-any-ceiling` for 0 or above 4 MiB, D170), with Save as file, and series info. Make a key can also take a key made by another tool (`-raw`, in a masked field) and print only its public key and address (D193), or save a new key to a file and show only its public half (`-out`, GREEN-060) | `poon crypto`, `keygen`, `series` |
| Variable supply series | A new series can be variable: an issuer, the part size and an optional cap on parts (empty: no cap), checked by poon. Sign a series as its issuer; the parts of a variable series as ranges (serials, pools, part id), 1,000 at a time and never on a timer (ORANGE-018); release part K in two steps: Preview names the serials it brings into being (and every part before it), then Sign signs that part only, and any change takes Sign away (GUI-027); a part of 10^8 or more is confirmed first, with its cost (GUI-028); it says it is permanent, with Save as file; check a release file, by path or picked in the browser (GUI-GREEN-052). Seeds go in the masked field. Sign a series and Release a part take the issuer key's algorithm (`-alg`, from `poon crypto`'s list); Release a part keeps it with the preview | `poon series new -variable`, `series sign`, `series parts`, `series release` (D163) |
| Variable series on a node | The Node view's Variable series panel shows each followed series that can grow, as `poon series followed` prints it: parts released, the next part, the mints and units that exist now, and the newest release held. Add a release gives the node an issuer's release, as a path or a file picked in the browser (sent as `-`); it is checked and kept, and the node follows the new mints; a line over the result says whether the node took it in place, is reloading, or already had it (GUI-GREEN-053). After Sign, Release a part checks the release just printed (`-check -`). A claim on a part not released yet is shown as waiting for a release, by the node's own exit code for it (3, D168), not as a failure | `poon series followed`, `series add-release`, `series release -check -` (D166) |
| Simulate | The network simulator's scenarios | `poon sim` |

Planned: views over Source's disk-backed state, once Source merges its commands ([gui-variable-supply-plan.md](gui-variable-supply-plan.md)).

### Quality checks

| Feature | What it does | Code |
| --- | --- | --- |
| Flag parity | CI fails if a subcommand `poon help` lists is run by no view, a flag `poon CMD -h` lists is sent by no form (unless listed with a reason), a field's stated default differs from `poon -h`, or a field's fixed set of values (book states, event kinds) differs from the one `poon -h` lists | `gui/e2e/parity.mjs` (GUI-GREEN-042, 043, 045, 048) |
| End-to-end tests | Ten browser scripts run in CI against real nodes (`variable.mjs`: a node following a variable series) | `gui/e2e` |
| Performance re-checks | GUI/Orange measures a soak, page load, Watch and Work at high rates and large results every round | `gui/perf`, `docs/proposals/gui/README.md` |

## Adding a feature

Each feature lives in its own package or file behind a small API. The CLI
command and the RPC endpoint are thin layers over it, and the GUI is a
form over the CLI command. A new feature follows the same pattern:

1. Write the code in the package that owns the data, with tests.
2. Add a `poon` command, and an RPC endpoint if a running node must apply
   it. A test fails when an RPC call has no command.
3. Add a row here, the module doc, and a decision.

Then the GUI, where a feature is a form over its command:

4. Add the form to the view it belongs in, in `gui/web/static/views/`,
   with `form()` and `flag()` from `views.js`. Its fields send the
   command's flags. For a new view, add a file there that exports
   `{ id, title, mount }`, and list it in `views/index.js`.
5. Run `gui/e2e/parity.mjs`. It walks every view in the navigation and
   fails if a flag of the command is sent by no form, if a field states
   a default that differs from `poon -h`, or if a field that offers a fixed
   set of values (`type: 'choices'` or a select, with `flag:` set) offers
   other values than `poon -h` lists. Add a reason to its `LEFT_OUT`
   only for a flag that belongs in the terminal.

The policy bundle is built to grow in the same way: its format carries a
version and a scope (series, policy, node, settings), and it has a
reserved slot for settings kept outside node.json.
