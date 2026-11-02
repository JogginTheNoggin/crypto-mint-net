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

## Commands

| Command | Does |
| --- | --- |
| `poo init [-example] [-series FILE]... [-listen ADDR] [-p2p ADDR] [-advertise ADDR] [-peer ADDR]... [-registry LOC]... [-force]` | Writes a node policy that knows and follows the given series, and makes it current |
| `poo node [-log-level L] [-log-format F]` | Runs the node until interrupted; SIGHUP makes it reload its node policy. The log flags override the policy's `log` section for this run ([logging.md](logging.md)) |
| `poo peers` | The node's id, its p2p address, its connected peers with their categories and points, and its peer book |
| `poo peer timeout\|block\|unblock\|clear\|template\|delete ADDR [-seconds N] [-range] [-what flags\|measures\|all] [-template NAME]` | Acts on one peer: pause it, block it (or its range; `-seconds -1` until unblocked), lift either, clear its records, choose its tracking template, or delete it from the book (flags before ADDR) |
| `poo tracking [ADDR]` | Every tracked address's status and the blocked ranges, or one address's whole record |
| `poo chases` | The chase queue: heavier stated weights waiting or being checked |
| `poo status` | Every followed pool and the replay stats; `poo pools` shows the same per pool, with filters |
| `poo doctor [-json \| -o json\|table]` | Checks the node's setup (policy, file permissions, series, grinding devices, the local RPC, peers) and prints ok, warn, fail or skip for each, with the fix; `fix_args` gives the fix as a command line when it is one command (GREEN-022), such as `grind schedule set -layer-rate 50` for a `layer_rate` lowered at load (D135); read-only and local only; exits 1 if a check fails |
| `poo claim -seed-file FILE -serial N` | Claims a whole mint for the key's 1-of-1 address; the seed comes from `POO_SEED` or the file (`-` for standard input), never an argument (D98) |
| `poo pay -seed-file FILE -serial N -to ADDR -amount A` | Pays from the key's address, change back to it |
| `poo build -pool K \| -serial N [-work N] [-max-tries N] [-device D] [-threads N] [-for 30s]` | Builds an entry from the pending transactions, optionally adding a layer; `-serial` picks the pool holding that mint; `-max-tries` bounds the nonces tried when the series sets a minimum entry strength, `-for` bounds the time instead (up to 4m). The reply's `grind` says the device, grinder, threads, fallback and why, and the tries and rate ([grind.md](grind.md)) |
| `poo work -pool K \| -serial N [-entry HASH] [-start S] [-tries N \| -for 30s] [-device D] [-threads N]` | Adds the strongest layer found (default target: the selected tip); `-serial` as for build; with `-for`, as many nonces as fit in that time |
| `poo submit entry\|layer [-series ID] -pool K \| -serial N HEX` | Hands the node an entry or a layer made elsewhere, in the hex its API prints (`entry` of an entry reply, `layer` of a layer reply). It is decoded first, then checked exactly as one from a peer: submitting adds no rule and skips none. The reply is the API's, without the bytes (D143) |
| `poo grind devices [-o FORMAT]` | The devices (asked of the running node, else detected here: `local`), their features, the hashes each grinds fast and those `auto` picks it for |
| `poo grind bench [-series ID] [-device D] [-threads N] [-seconds S] [-o FORMAT]` | Tries a second on each device for the series' hash, or for every hash; local |
| `poo grind start TARGET [JOB FLAGS] [-lease 30s]` | Starts a job in the running node and prints it; a device that cannot grind is refused at once unless the job has `-hours`. TARGET is `tip`, `pending`, `tx:TID` (one payment, until `-until-from`) or an entry hash; JOB FLAGS: `-series` `-pool` `-serial` `-device` `-threads` `-for` `-until-from W` (a from(e) goal) `-hours 22-06`; `-lease` stops the job unless it is renewed |
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
| `poo pending [filter]` | Pending transactions |
| `poo unspent [filter]` | Unspent portions |
| `poo tx [-in-serial N] TID` | One transaction: where it stands, the weight behind it, rivals, its contents |
| `poo address [-below H] [-limit N] [filter] ADDRESS` | An address's balance, holdings, history and pending transactions; `next` gives the `-below` for each pool's older history |
| `poo watch [-notices] [-once] [filter]` | Prints events as they happen, one JSON line each, until interrupted; reconnects after a reload unless `-once` |
| `poo follow [-issuer HEX] [-series ID] [-pool K]... [-mint N]... [-replace] [-no-reload]` | Without flags, what is followed; with them, adds to the follow list (with `-replace` and `-series`, puts exactly those pools and mints in place of the series' rule in one change, as `POST /v1/follow`'s `replace` does for the GUI: GUI-018, GREEN-038) and applies it to the running node (or, when no node answers, edits `node.json` under the data directory's lock, refused with nothing changed while a node or another command holds it: RED-030, D136) |
| `poo unfollow (-issuer HEX \| -series ID) [-pool K]... [-mint N]... [-no-reload]` | Removes from the follow list, the same way |
| `poo reload` | Makes the running node apply its node policy again (as SIGHUP to `poo node` does) |
| `poo policy list\|use\|archive\|unarchive\|delete [-root DIR] [NAME]` | Manages node policies (flags before NAME) |
| `poo wallet COMMAND` | The wallet; see below |
| `poo completion bash\|zsh\|fish` | A shell completion script. It asks `poo __complete WORDS... CURRENT` (hidden) for candidates: commands and subcommands from the usage texts, each command's flags from its own flag set, and saved policy names after `-policy` |
| `poo help [COMMAND]` | The usage, or one command's usage lines; a mistyped command gets the nearest names ("did you mean status?") |
| `poo version [-v]` | Prints the version; `-v` adds the Go version, platform, commit (when the build recorded it) and dependency versions, for a bug report |
| `poo crypto` | Lists the hashes, key algorithms (with key and signature sizes) and policy types, with their numbers ([../cryptography.md](../cryptography.md)) |
| `poo keygen [-alg NAME] [-hash NAME] [-seed-file FILE]` | A new key (default Ed25519; `-seed HEX` is kept for test vectors only, since other users can read a command line) and its 1-of-1 address under a hash (default SHA-256; `address_sha256` is kept for SHA-256) |
| `poo series example` | The example series parameters as JSON |
| `poo series new [-hash NAME] [-algs LIST] [-types LIST] [-mints N] [-supply S] [-pool-size P] [-min-layer-strength] [-min-entry-strength] [-entry-hash-credit] [-max-witness]` | Checked parameters of a new series as JSON; names or numbers, lists comma-separated |
| `poo series info FILE` | Validates a series file and prints its id and pools |
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
Hashes are shortened to their first 8 and last 4 hex digits unless `-wide`
is given. The table is built from the JSON, so it never shows a field the
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
`reverted`, `dropped`; `watch` only). Each takes comma-separated values
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
