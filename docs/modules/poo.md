# pkg/cli and cmd/poo

## Summary

The command-line tool. Every command is in package `cli`, behind
`cli.Main(ctx, args, Stdio{In, Out, Err})`; `cmd/poo` is only the process
around it (arguments, standard streams, and an interrupt that cancels the
context). The GUI's terminal and every GUI action call the same `Main`, so
the headless tool and the GUI cannot drift apart (decision D84). `poo node` runs the full node ([daemon.md](daemon.md)) with its store,
its peer connections, its peer tracker (`tracking.json`) and local RPC; client commands talk to it; `poo policy` manages node
policies; offline commands make keys, inspect series and run an in-process
demo. `poo wallet` keeps a holder's keys and pays, requests, co-signs and
follows payments through the node ([wallet.md](wallet.md)). `claim` and
`pay` sign with a single raw key and exist so a node can be exercised
without a wallet. See
[../running-a-node.md](../running-a-node.md) for a walk-through.

Exit codes: 0 done, 1 refused or failed, and 3 (`cli.ExitWaiting`) when a
command is refused only because its mint's part is not released at this
node yet, the RPC's 409 `waiting` (D166). That is not an invalid mint: the
same command works once the release arrives, from a peer or from
`poo series add-release` (D168). `cli.ExitCode` maps an error to its code
for a caller of `cli.Run`, such as the GUI.

## Commands

| Command | Does |
| --- | --- |
| `poo init [-example] [-reserve] [-no-series] [-series FILE]... [-any-ceiling] [-listen ADDR] [-p2p ADDR] [-advertise ADDR] [-port-map MODE] [-peer ADDR]... [-registry LOC]... [-force]` | Writes a node policy that knows and follows the given series, and makes it current. `-port-map auto` (or `pcp`, `natpmp`, `upnp`) asks the router to forward the peer port, off by default (D182; see running-a-node.md for the privacy cost). With no `-series`, `-example` or `-reserve` it follows the built-in reserve series and prints its full id (D170); `-reserve` adds it beside others, `-no-series` follows none. A `-series` file whose witness ceiling is not set or above 4 MiB is refused without `-any-ceiling` |
| `poo node [-log-level L] [-log-format F] [-reverify]` | Runs the node until interrupted; SIGHUP makes it reload its node policy. The log flags override the policy's `log` section for this run ([logging.md](logging.md)). It starts each pool from its snapshot and replays only newer records; `-reverify` replays every log whole on this start and reports whether each snapshot matched (D178) |
| `poo peers [-category LIST] [-mint-policy TEXT] [-label TEXT] [-addr TEXT]` | The node's id, its p2p address, its connected peers with their categories and points, and its peer book; the filters keep the addresses in one of the listed book states, with a mint policy, label or address holding the text (D157). Runs offline on the files when the node is stopped |
| `poo peers -counts` | `{node_id, listen, peers, counts}`: the connections, and the peer book's size and addresses per state, without the book's records, for a view that refreshes often (D164, D166). Does not go with the filters |
| `poo peers export [-category LIST] [-out FILE]` | Writes the peer book, or only the chosen categories (the book states, `blocked`, `timed-out`), to a peer file with labels and blocks ([peerport.md](peerport.md)) |
| `poo peers import FILE\|- [-apply] [-blocks [-ranges]] [-json] [-if-digest D]` | Shows the plan, then with `-apply` adds the file's addresses to the book as unlisted, with labels where the book has none; blocks only with `-blocks`, adding or lengthening only, never on an address the node policy lists; network ranges only with `-ranges` |
| `poo peer timeout\|block\|unblock\|clear\|template\|delete ADDR [-seconds N] [-range] [-what flags\|measures\|all] [-template NAME]` | Acts on one peer: pause it, block it (or its range; `-seconds -1` until unblocked), lift either, clear its records, choose its tracking template, or delete it from the book (the address may come before or after the flags) |
| `poo peer add\|remove\|label\|connect ADDR [-label TEXT] [-keep]`, `poo peer connect -imported` | Adds an address by hand (as a candidate, `-keep` also lists it in `p2p.peers`), removes it from the book (and from `p2p.peers`), sets or clears its label, or dials it now. With the node stopped, `add -keep`, `remove` and `label` work on the files; `add -keep -label` labels only an address the book already holds, and refuses before changing anything otherwise (GREEN-046); `connect -imported` lets the manager dial the imported (unlisted) addresses at once (D157) |
| `poo tracking [ADDR] [-blocked] [-timed-out]` | Every tracked address's status and the blocked ranges, or one address's whole record; `-blocked` and `-timed-out` show only those (D157) |
| `poo chases` | The chase queue: heavier stated weights waiting or being checked |
| `poo status [-counts]` | Every followed pool and the replay stats, the entry cache, the file limit and whether node.json changed; `-counts` adds the pools up into one row per followed series, with the same notes, for a view that refreshes often (GUI ORANGE-021, D177); `poo pools` shows the same per pool, with filters |
| `poo doctor [-json \| -o json\|table]` | Checks the node's setup (policy, file permissions, series, the open file limit (`open-files`, D172), grinding devices, the local RPC, entry bodies the running node failed to read back from its logs (`bodies`, D173), pool snapshots: age, records a start would replay, any not used or differing under `-reverify` (`snapshots`, D178), the port mapping when `p2p.port_map` asks for one (`port-map`, D182), whether the running node's release is this `poo`'s (`release`, GREEN-057), peers; the `policy` line also names settings node.json holds that this release does not know, warning for those the next save drops, GREEN-057) and prints ok, warn, fail or skip for each, with the fix; `fix_args` gives the fix as a command line when it is one command (GREEN-022), such as `grind schedule set -layer-rate 50` for a `layer_rate` lowered at load (D135); read-only and local only; exits 1 if a check fails |
| `poo claim -seed-file FILE [-alg NAME] -serial N [-parent HASH\|root\|before-claim [-max-tries N] [-work N]]` | Claims a whole mint for the key's 1-of-1 address; the seed comes from `POO_SEED` or the file (`-` for standard input), never an argument (D98). `-alg` (default `ed25519`) names the key's algorithm, as `poo keygen -alg` made it; a key the series does not list in `key_algs` is refused before anything is sent (GREEN-051). With `-parent` the claim is not submitted as pending but built into an entry on that parent, so a mint the selected ledger has claimed can be claimed on a rival branch: `before-claim` is the parent of the selected entry that holds the mint's claim, `root` the pool's first entry. The reply has the entry, the transactions left out, and `next`: the command that adds weight to it (D161) |
| `poo pay -seed-file FILE [-alg NAME] -serial N -to ADDR -amount A` | Pays from the key's address, change back to it; `-alg` as for `poo claim` |
| `poo build -pool K \| -serial N [-parent HASH\|root\|before-claim] [-work N] [-max-tries N] [-device D] [-threads N] [-for 30s] [-bytes]` | Builds an entry from the pending transactions, optionally adding a layer; `-parent` builds it on that entry instead of the selected tip, from the pending transactions that fit there (`before-claim` needs `-serial`, D161); `-bytes` also prints the encoded entry (and layer) for `poo submit` on another node (GREEN-039); `-serial` picks the pool holding that mint; `-max-tries` bounds the nonces tried when the series sets a minimum entry strength, `-for` bounds the time instead (up to 4m). The reply's `grind` says the device, grinder, threads, fallback and why, and the tries and rate ([grind.md](grind.md)); an entry no layer selected gets a `next` naming the command that adds weight to it, by `-pool K` (GREEN-048) |
| `poo work -pool K \| -serial N [-entry HASH] [-start S] [-tries N \| -for 30s] [-device D] [-threads N] [-bytes]` | Adds the strongest layer found (default target: the selected tip); `-bytes` also prints the encoded layer; `-serial` as for build; with `-for`, as many nonces as fit in that time. Without `-tries` or `-for` it tries 2^16 nonces, or 2^(floor + 2) for a series whose layer floor makes that larger, so a default run can meet the floor (D168) |
| `poo submit entry\|layer [-series ID] -pool K \| -serial N HEX \| -f FILE \| -` | Hands the node an entry or a layer made elsewhere, in the hex its API prints (`entry` of an entry reply, `layer` of a layer reply; `poo build -bytes` and `poo work -bytes` print them, and their whole reply may be given instead: the file `poo build -bytes -work N` writes feeds both `poo submit entry -f FILE` and `poo submit layer -f FILE` as it is (GREEN-040)). The hex is the one argument, or the whole of a file (`-f FILE`) or of standard input (`-`, or `-f -`), trimmed; one argument may be at most 128 KiB on Linux, so an entry over 64 KiB goes through a file or standard input, and the node takes an entry of any size (RED-038, D144). It is decoded first, then checked exactly as one from a peer: submitting adds no rule and skips none. The reply is the API's, without the bytes (D143) |
| `poo grind devices [-o FORMAT]` | The devices (asked of the running node, else detected here: `local`), their features, the hashes each grinds fast and those `auto` picks it for |
| `poo grind bench [-series ID] [-device D] [-threads N] [-seconds S] [-o FORMAT]` | Tries a second on each device for the series' hash, or for every hash; local |
| `poo grind start TARGET [JOB FLAGS] [-lease 30s]` | Starts a job in the running node and prints it; a device that cannot grind is refused at once unless the job has `-hours`. TARGET is `tip`, `pending`, `tx:TID` (one payment, until `-until-from`) or an entry hash; JOB FLAGS: `-series` (default: every followed series) `-pool` `-serial` `-device` `-threads` `-for` `-until-from W` (a from(e) goal) `-hours 22-06`; `-lease` stops the job unless it is renewed |
| `poo grind run TARGET [JOB FLAGS]` | Starts a job and follows it: one JSON line per event (`start` with the spec, `plan` per pool, `progress` at most once a second, `found`, `waiting` with a hint, `end`) until it ends. The job has a lease the command renews, so it stops if the command dies; a failed renewal is retried while the lease may hold, with one warning, and only a job the node no longer knows or that is done ends the renewals (RED-019); an interrupt or a hangup stops it, prints an `end` line and exits 0 |
| `poo grind jobs [-o FORMAT] [ID]` | The node's jobs, running and recently ended, with their plans, tries, rate, layers and entries; with an `ID`, that one job (D143) |
| `poo grind stop ID \| -all` | Stops a job, or every job, keeping what it found |
| `poo grind schedule [-o FORMAT]` | The jobs `node.json`'s `grind` section starts, its defaults, and its `version`. `add`, `remove` and `set` take `-if-version V` to change only that version, so a script never edits a schedule someone else changed since it looked (GREEN-034) |
| `poo grind schedule add TARGET [-series ID] [-pool K \| -serial N] [-device D] [-threads N] [-hours H] [-until-from W]` | Adds a job to the schedule; a running node saves it and starts it at once, with no reload, and says if it could not; otherwise `node.json` is edited under the data directory's lock, and the edit is refused, with nothing changed, while a node or another command holds it (GREEN-026; GUI-017, D134) |
| `poo grind schedule remove N... \| -all` | Removes scheduled jobs, numbered from 1 as listed; a running node stops them. `-target T` also checks that each numbered job grinds T, as a check on what was meant. The removal names the whole job, so it is refused if the schedule changed meanwhile, and the add, remove and set commands change the section on the node, so two at once both apply (D125) |
| `poo grind schedule set [-device D] [-threads N] [-layer-floor S] [-layer-rate R]` | Sets the defaults of every job, build and work; `-device auto` clears the device; `-layer-floor` sets `grind.layer_floor` (D118) and `-layer-rate` sets `grind.layer_rate`, the most layers a second the floor adds (D122) |
| `poo payment -serial N -tid HASH` | Where a transaction stands, and the weight behind it. The older form of `poo tx -in-serial N TID`, kept for scripts |
| `poo holdings -serial N` | Unspent portions of a mint. The older form of `poo unspent -serial N`, kept for scripts |
| `poo pools [filter]` | The followed pools the filter covers |
| `poo ledger -pool K [-below H] [-limit N] [filter]` | A page of a pool's selected ledger, newest first, with each entry's transactions |
| `poo ledger export -pool K [-series ID] [-from H] [-to H] [-out FILE]` | A stretch of the selected ledger, oldest first, as a ledger file (`"kind": "poo-ledger"`, format 1): the header (series, pool, range, height, tip, weight) and each entry's summary with its raw item (the entry's bytes and layer nonces, base64). Read page by page, every page after the first up to the header's `to`; fails if the node switches ledgers part way, including to a shorter one, and the file must end at the header's `to` (and tip, when it runs to the tip) (D189). `-out` writes a file only when the export is whole (D188). A `-from` past the tip is refused rather than written as an empty file that claims a range (GREEN-058) |
| `poo ledger import [-dry-run] FILE\|-` | Hands a ledger file's entries to the node in batches (at most 256 entries, and 8 MiB past the first, refused while it is read, D191), reading the file as it streams (ORANGE-043); the node checks every entry and layer in full, as from a peer. Prints what was kept, already held and refused, with why (D188). When the node does not know the file's series or follow its pool, the error names them and the command to follow the pool; when a later batch fails, it says how many entries earlier batches sent and kept (GREEN-058). On a failure or a stop after a batch was kept, it prints the totals so far with `sent` and `stopped` and exits non-zero; a stop ends it between batches (GUI-031, D189). The file's header may hold only the keys format 1 defines (`kind`,
`format`, `series`, `pool`, `from`, `to`, `height`, `tip`, `weight`,
`exported`), each at most 4 KiB and each once, and `entries` once;
entries that come before the fields naming the file and pool (as a tool
that sorts keys writes them) go to a temporary file one at a time and are
sent after the header (RED-102, D191). `-dry-run` sends nothing and needs no running node: it checks that every item decodes, matches its summary and chains, and that the file ends where its header says, with the hashes worked out when node.json knows the series, and exits non-zero on a problem (D189). It also says whether node.json follows the file's pool (`pool_followed`), with a `note` naming what a real import needs when it would be refused (GREEN-059) |
| `poo pending [filter]` | Pending transactions |
| `poo unspent [filter]` | Unspent portions |
| `poo tx [-in-serial N] TID` | One transaction: where it stands, the weight behind it, rivals, its contents |
| `poo address [-below H] [-limit N] [filter] ADDRESS` | An address's balance, holdings, history and pending transactions; `next` gives the `-below` for each pool's older history |
| `poo watch [-notices] [-once] [filter]` | Prints events as they happen, one JSON line each, until interrupted; reconnects after a reload unless `-once` |
| `poo follow [-issuer HEX] [-series ID] [-pool K]... [-mint N]... [-replace] [-no-reload]` | Without flags, what is followed; with them, adds to the follow list (with `-replace` and `-series`, puts exactly those pools and mints in place of the series' rule in one change, as `POST /v1/follow`'s `replace` does for the GUI: GUI-018, GREEN-038) and applies it to the running node (or, when no node answers, edits `node.json` under the data directory's lock, refused with nothing changed while a node or another command holds it: RED-030, D136) |
| `poo unfollow (-issuer HEX \| -series ID) [-pool K]... [-mint N]... [-no-reload]` | Removes from the follow list, the same way |
| `poo reload` | Makes the running node apply its node policy again (as SIGHUP to `poo node` does) |
| `poo policy list\|use\|archive\|unarchive\|delete [-root DIR] [NAME]` | Manages node policies (flags before NAME) |
| `poo policy limits [-set NAME=VALUE]... [-o FORMAT] [-policy NAME \| -dir DIR]` | The node policy's settable limits (D159): each one's place in node.json, its range, its default, the value node.json sets (0: left out) and the value used. Without `-set` it reads node.json only and works in view-only mode (GREEN-047). `-set` (repeatable; 0 puts a limit back to its default) checks every value against its range before anything is saved, then changes node.json through the running node, which applies the peer book's limits, the tracker's retention and `seen_size` in place (`in_place` in the reply) and reloads for the others (`reloading`), or directly with the node stopped (D161, D168); a `-set` sent while the node is still reloading waits for it (up to 20s) instead of failing (D166) |
| `poo policy export [-all \| -series ID] [-full] [-out FILE] [-policy NAME \| -dir DIR]` | Writes one shareable file, a bundle: the node policy, one series of it with how it is followed, or with `-all` every saved node policy. Machine-local settings are left out unless `-full` (D149) |
| `poo policy import FILE [-apply] [-prefer-file] [-replace] [-settings] [-full] [-if-digest D] [-no-reload] [-json] [-policy NAME \| -dir DIR]` | Merges a bundle (or a node.json; `-` reads it from standard input, as the GUI hands over a picked file) into the node policy, or, for an `-all` file, into each saved node policy by name, creating the missing ones. Takes only what to follow unless `-settings` (or `-full`), and then names the defence lines first (D150). Plans every node policy before changing any. Prints the plan and changes nothing without `-apply`; a preview with something to apply prints the file's digest (and only then has `digest` in `-json`, D180), and `-if-digest D` then applies only that file (D151). A running node takes the change through `POST /v1/policy/import` and reloads; otherwise node.json is changed under the directory's lock. Refused in view-only mode |
| `poo settings export [-full] [-out FILE] [-policy NAME \| -dir DIR]` | Writes the node policy's settings, everything but what it follows, to a bundle of scope `settings`. Machine-local settings are left out unless `-full`. Refused in view-only mode (D155) |
| `poo settings import FILE [-apply] [-prefer-file] [-replace] [-full] [-map-path OLD=NEW]... [-if-digest D] [-no-reload] [-json] [-policy NAME \| -dir DIR]` | `poo policy import` for settings only. It reads a settings bundle, or takes the settings of a policy bundle or node.json, and never what it follows. `-replace` puts the file's settings in place of the node's. `-map-path` rewrites registry file paths (taken only with `-full`; the text plan notes a map that cannot change anything, GREEN-045), and the plan lists what may not fit this machine (`fit`). `poo policy import` takes `-map-path` too, and refuses a settings file (D155) |
| `poo wallet COMMAND` | The wallet; see below |
| `poo completion bash\|zsh\|fish` | A shell completion script. It asks `poo __complete WORDS... CURRENT` (hidden) for candidates: commands and subcommands from the usage texts, each command's flags from its own flag set, and saved policy names after `-policy` |
| `poo help [COMMAND]` | The usage, or one command's usage lines; a mistyped command gets the nearest names ("did you mean status?"). A command made of subcommands followed by `help`, `-h` or `--help` alone prints the same lines and exits 0 (GREEN-045) |
| `poo version [-v]` | Prints the version; `-v` adds the Go version, platform, commit (when the build recorded it) and dependency versions, for a bug report |
| `poo crypto [-max-witness N] [-o FORMAT]` | Lists the hashes, key algorithms (with key and signature sizes) and policy types, with their numbers ([../cryptography.md](../cryptography.md)). `max_signatures_per_witness` is each algorithm's largest k-of-k under the ceiling `max_witness` (default the reserve series' 131072; 0: none) (D170) |
| `poo keygen [-alg NAME] [-hash NAME] [-seed-file FILE]` | A new key (default Ed25519; `-seed HEX` is kept for test vectors only, since other users can read a command line) and its 1-of-1 address under a hash (default SHA-256; `address_sha256` is kept for SHA-256). When the current node policy lists series, `series_addresses` gives the key's address in each (`series`, `builtin`, `hash`, `allowed`: whether the series lists the key's algorithm), and `note` says when `address` is the address in none of them, as for the default reserve series, whose hash is SHA-512/256 (GREEN-052) |
| `poo series example` | The example series parameters as JSON |
| `poo series new [-hash NAME] [-algs LIST] [-types LIST] [-mints N] [-supply S] [-pool-size P] [-min-layer-strength] [-min-entry-strength] [-entry-hash-credit] [-max-witness N] [-any-ceiling] [-issuer ADDR] [-variable -part-size M [-max-parts K]]` | Checked parameters of a new series as JSON; names or numbers, lists comma-separated. `-max-witness` defaults to 131072 (128 KiB, the reserve series'); 0 or above 4 MiB is refused without `-any-ceiling` (D170) |
| `poo series info FILE` | Validates a series file (params, or the signed file `poo series sign` prints) and prints its id and pools; a variable series adds `supply_type`, `part_size`, `max_parts` (left out when uncapped) and its first parts (D166). `"builtin": "reserve"` names the built-in reserve series, on its full id only (D170) |
| `poo series sign FILE [-seed-file F] [-alg NAME]` | Signs a series as its issuer (a 1-of-1 key, of the `-alg` algorithm, default Ed25519, whose address is the series' issuer; when the seed's key under another algorithm is the issuer, the refusal names that `-alg`, GREEN-051) and prints `{series_id, params, issuer_witness}`, a signed series file that `poo init -series` and `poo series info` read (D166) |
| `poo series release FILE -part K [-seed-file F] [-alg NAME]` | Releases part K (from 2) of a variable series as its issuer and prints the release (D163); refuses a key that is not the issuer's, part 1 and a part past the largest number of parts |
| `poo series release FILE -check RELEASE\|-` | Checks a release file (`-` reads it from standard input, GUI-027) against the series and prints the part, its id, its serials and the serials it releases |
| `poo series parts FILE [-from K] [-to K] [-o FORMAT]` | A variable series' parts, released or not: serials, pools and part ids (at most 1,000 at a time), and `mints_released`, the mints that exist once the part is released (D168); `-o table` prints a row per part |
| `poo series add-release RELEASE\|-` | Gives the node a release of a variable series: checked, kept in node.json when newer than the one held, and taken in place: the new part's pools are followed and the release is passed on to peers (`in_place`), or the node reloads when it cannot (D168); with the node stopped it changes node.json directly. A release the node already has adds a `note` saying which release covers it. Refused in view-only mode (D166, GREEN-050) |
| `poo series followed [-o FORMAT]` | The series the running node follows; a variable series adds `variable{released, next_part, mints, units, release}`: part K is released when K ≤ `released` (D166). `-o table` prints one row per series: id, supply, pools, mints, units, released_through, next_part (GREEN-050). `"builtin": "reserve"` names the built-in reserve series (D170) |
| `poo sim load\|partition\|attack\|sizing [flags]` | Runs a scenario of the network simulator and prints JSON; `-seed` and `-runs` pick the seeds, other flags shape the network and the attack ([sim.md](sim.md), [../simulation.md](../simulation.md)) |
| `poo demo [-quiet]` | Four in-process nodes: claims, payments, a rival double spend that wins, a carried payment, and the honest ledger restored |

Node and client commands choose a data directory with `-dir`, or with
`-root` (default `$POO_HOME` or `~/.poo`) and `-policy` (default: the current
node policy, else `default`). Client commands take `-series ID` when the
node follows more than one series.

The read commands (`status`, `pools`, `ledger`, `pending`, `unspent`, `tx`,
`address`, `holdings`, `payment`, `peers`, `tracking`, `chases`, `version -v`, and `wallet addresses`, `balance` and `status`) print JSON, which
scripts and the GUI read. `-o table` prints the same reply for reading
instead: a list becomes aligned columns named after its JSON fields, and an
object becomes one field per line followed by a table for each list in it.
Hashes are shortened to their first 8 and last 4 hex digits, times to the
second, and the zero time (never, as a peer not yet tried) to `-`, unless
`-wide` is given (GREEN-020, GREEN-046). The table is built from the JSON, so it never shows a field the
JSON lacks.

**View-only mode.** `poo -view-only COMMAND`, or any command with
`POO_VIEW_ONLY=1` set, only looks (D101). Commands that talk to the node
use `rpc.view-token`, which the node lets read and nothing else, and every
command that would change something is refused before it runs with
`view-only: poo X changes things`. The commands that run are those of
`readCommands` in `pkg/cli/scope.go`: the views and `watch`, `status`,
`peers`, `tracking`, `chases`, `payment`, `holdings`, `follow` without
flags, `doctor`, `policy list`, `wallet addresses` and `wallet balance`,
and the offline commands (`crypto`, `keygen`, `series`, `sim`, `demo`,
`help`, `version`, `completion`). `poo-gui -view-only` is this mode.

**Secrets.** Passphrases and a seed given as `-seed-file -` are read
through `Stdio.Secret` when the caller sets it; `poo` itself sets it when
standard input is a terminal, so a typed passphrase is not echoed and its
prompt goes to standard error. Piped input is read as a line, as before.

`[filter]` is the shared filter ([filter.md](filter.md)): `-series`,
`-pool`, `-serial`, `-address`, `-address-policy` (hex of a policy's byte
form), `-tid` and `-kind` (`tx`, `entry`, `layer`, `switch`, `selected`,
`reverted`, `dropped`, `grind`; `watch` only: the views do not offer it, GREEN-041). Each takes comma-separated values
and may repeat; fields combine with "and", values with "or". For example
`poo watch -serial 5 -kind tx,selected` prints mint 5's transactions as
they arrive and are selected. Event lines are described in
[watch.md](watch.md). Flags go before positional arguments.

## Wallet commands

The wallet file is `-wallet FILE`, default `wallet.json` in `-root`. The
passphrase is read from `$POO_PASSPHRASE`, `-passphrase-file FILE`, or a
line on standard input (which is echoed: prefer the file or the
variable in scripts). The series comes from the node policy, so commands
that do not need the node (`create`, `seed`, `address`, `addresses`,
`watch`, `request`, `policy`, `cosign`, `follow`) work with the node
stopped. Flags go before positional arguments.

| Command | Does |
| --- | --- |
| `create [-restore -\|HEX] [-gap N] [-alg NAME]` | Makes a wallet with a new random seed, or restores one from its seed: `-restore -` reads it hidden, like a passphrase; `-alg` fixes its key algorithm (default Ed25519) |
| `seed` | Prints the seed: everything needed to restore the wallet |
| `passwd [-new-passphrase-file FILE]` | Seals the seed under a new passphrase (`$POO_NEW_PASSPHRASE`) |
| `address [-label TEXT]` | Gives out a fresh address, with its public key for co-signers |
| `addresses` | Every address the wallet watches: derived keys (the gap included) and shared policies |
| `watch [-serial N]... [-pool K]...` | Adds mints to look for portions in |
| `follow` | Rewrites the node policy to follow only the pools of the watched mints (light mode); a running node applies it at once |
| `balance` | Spendable units per watched mint |
| `claim -serial N` | Claims a whole mint for a fresh address |
| `pay -to ADDR -amount A [-mint N]... [-memo TEXT] [-out FILE]` | Pays an address; with a shared policy, writes the payment for co-signers to `-out` |
| `pay -request FILE [-out FILE]` | Checks a payment request and pays it |
| `request -amount A [-memo TEXT] [-out FILE]` | Writes a payment request to a fresh address |
| `receive (-address ADDR \| -request FILE) [-serial N -tid HASH]... [-threshold W]` | What has arrived for a request, and whether it is settled |
| `status [-threshold W]` | Every payment sent: each leg's state, from(e), rivals, and settled |
| `resend` | Sends dropped legs again |
| `policy -name NAME -k K -key self\|ALG:HEX\|HEX... [-label TEXT]` | Records a shared k-of-n policy and prints its address; `ALG` is an algorithm's name or number, plain `HEX` is Ed25519 |
| `cosign FILE` | Adds this wallet's signatures to a payment file, in place |
| `submit FILE` | Sends a payment whose signatures are complete |

## Code

`pkg/cli/main.go` has `Main` and `Run`, which take a context and
`Stdio`, and dispatches through `run(args, out)`. The few commands that
read input (passphrases) or run until stopped (`poo node`) get them from
the invocation's out, with `inputOf(out)` and `contextOf(out)`, so every
command keeps the signature `(args, out)` and no state is shared between
invocations: the GUI can run several at once. Cancelling the context is
what an interrupt does in a shell. In `pkg/cli`, `node.go` holds the daemon, client and policy commands;
`view.go` the view, watch, follow and reload commands; `wallet.go` holds the wallet commands; `sim.go` the simulator command; `demo.go` holds the demo. `node_test.go` runs a daemon in-process and drives
it only through the commands, across a restart; `TestTwoNodes` runs two
daemons where the second lists the first as a peer. `view_test.go`
drives the views, a watcher, and a live unfollow with its reload.
