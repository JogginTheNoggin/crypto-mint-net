# Changelog

## Unreleased (0.1.0)

The first release: a full node for Proof of Ownership, with its wallet and
simulator. See [docs/roadmap.md](docs/roadmap.md) for what each milestone
built.

- **Consensus core:** series, pools, threshold policies,
  transactions, entries, layers and credit, pool state, the per-pool ledger
  tree with selection, switching and top-K pruning, and the pending pool.
- **Node:** an append-only, checksummed store replayed on start-up, node
  policies, and a local RPC on loopback with a token.
- **Network:** a framed binary wire protocol, forwarding, catch-up, peer
  discovery and relaying, address registries, peer categories, ranking,
  tracking and chasing.
- **Wallet:** keys from one sealed seed, claims, payments with coin
  selection, payment requests, co-signing and light mode.
- **Simulator:** many real engines over a modelled network, with load,
  partition, attack and sizing scenarios (`poo sim`).
- **Hardening:** fuzzing of every decoder, benchmarks, a security review
  ([docs/security.md](docs/security.md)), a separate cap on inbound
  connections, byte form v1 and wire version 1 declared stable, a
  supported Go toolchain, and reproducible release builds.
- **GUI:** `poo-gui`, a local web app over the same `poo` commands
  (`pkg/cli`), with a terminal that is `poo` itself, a job list, and a
  locked-down loopback server ([docs/gui.md](docs/gui.md)). `poo-gui
  -view-only` runs every command in poo's view-only mode, and the GUI
  masks a passphrase or seed exactly when poo reads one (D89). Its Work view
  runs and follows `poo grind` jobs: devices, plan, rate and finds.
- **Cryptographic choice:** a series can use SHA-256, SHA-512/256,
  SHA3-256, BLAKE2b-256 or BLAKE3; allow Ed25519, ECDSA (P-256, P-384,
  P-521, secp256k1), BIP-340 Schnorr and ML-DSA-44/65/87 keys; and allow
  hashlock and nested k-of-policies addresses
  ([docs/cryptography.md](docs/cryptography.md)). Wallets choose their key
  algorithm (`poo wallet create -alg`); `poo crypto`, `poo keygen -alg
  -hash` and `poo series new` are new. Requires Go 1.27.
- **Viewing and watching:** one filter (series, pool, mint, address,
  address policy, transaction, event kind) for views of pools, ledger
  pages, pending transactions, unspent portions, a transaction and an
  address, and for a live event stream (`poo pools`, `ledger`, `pending`,
  `unspent`, `tx`, `address`, `watch`; `GET /v1/watch` as NDJSON).
- **Live following:** `poo follow` and `poo unfollow` (and
  `POST /v1/follow`) change what a running node follows and apply it at
  once by reloading the node in place; `poo reload` and SIGHUP do the
  same after a hand edit. `poo wallet follow` applies at once too.
- **Wallet:** new wallets seal the seed with Argon2id (D95); older
  PBKDF2 wallets still open, and `poo wallet passwd` upgrades them.
- **Read-only use:** `rpc.view-token` reads and nothing else (D100), and
  `poo -view-only` / `POO_VIEW_ONLY=1` runs only the commands that read
  (D101). `poo help`, typo suggestions, `poo doctor`, `-o table`, shell
  completion (`poo completion`) and `GET /v1/metrics` are new. A
  passphrase typed at a terminal is no longer echoed.
- **Logs and diagnostics:** levelled, structured node logs (`poo node
  -log-level -log-format`, plain, text or JSON; D104), and `poo version -v`
  with the Go version, platform, commit and dependencies.
- **Performance:** signatures are verified once per witness and before
  the node's lock, items share fsyncs, and branch ranking and hashing
  are cheaper (D105): several times the transactions a second, with no
  rule change.
- **Safer operation:** a reload onto a broken `node.json` is refused and
  the node keeps running (D106); `poo wallet create -restore -` reads
  the seed hidden.
- **Grinding:** `pkg/grind` detects the CPU (and its hashing features)
  and GPUs, picks a grinder for the series' hash on a chosen device or
  auto, and runs jobs in the node on the tip, pending transactions or an
  entry, by hand, within hours or from `node.json` (D109). `poo grind`
  (devices, bench, start, run, jobs, stop), `/v1/grind`, watch kind
  `grind`, and `-device`, `-threads`, `-for` on `poo build` and `poo work`
  are new; grinding uses every core but one, with the same result.
- **Grind jobs, refined (D112, D113):** a `tx:TID` target grinds for one
  payment until it is safe; a job can carry a lease, and `poo grind run`
  keeps one, so a job whose follower dies stops; a hangup stops a `poo`
  command as an interrupt does (but reloads `poo node`); a device that
  cannot grind is refused at start; progress comes at most once a second
  with a steadier rate; one `start` per job, then a `plan` per pool;
  waiting jobs say what would help; job-wide events reach watchers of
  their series. `poo grind schedule` (list, add, remove, set) and
  `/v1/grind/schedule` change the jobs `node.json` starts, applied at once
  with no reload. A native BLAKE3 grinder keeps chaining values (5 times
  faster entry tries), and `-o table` spreads nested objects into columns.
- **For front ends:** `poo doctor -o json|table` and a `fix_args` per
  check, and `cli.ErrorCode` and `cli.SecretOutput` so the GUI handles
  errors and secret output the way `poo` does (D110).
- **Library:** the commands are package `pkg/cli`, so the GUI runs the
  same code as `poo` (D84).
- **Fixed:** a node catching up a heavier ledger one entry at a time could
  prune each entry before its work arrived and never switch (D79).
- **A heavier ledger is always reachable (D115 to D117):** no node policy
  can stop a node reaching a heavier valid ledger. A chase finds a split
  at any depth by halving, takes in any run of entries without layers,
  and yields and resumes past its time slice instead of giving up; only
  hashes shown false are bad-listed. Entries and transactions have no
  size or count cap: an entry larger than a frame travels in parts (new
  wire types: parts request, large, part), and the store splits large
  records. `p2p.depth_limit` and `fast_catch_up` are ignored.
- **Grinding (D118, D119):** `grind.layer_floor` keeps every layer above a
  floor (off by default; several times the credit for the same work);
  SIMD lane grinders in a `GOEXPERIMENT=simd` build; jobs stop at once
  and report progress each second; `poo grind schedule remove -target`
  guards against a stale row number.
- **Held branches and many layers (D121, D122):** a node that holds a
  heavier branch's entries without their layers now switches to it; layers
  added deep in a shared ledger are fetched; an entry with any number of
  layers is counted (wire types 27, 52, 53); budgets count only what they
  let through. Witnesses of large transactions are checked on every core,
  events no longer walk every branch, and `grind.layer_rate` bounds what
  `layer_floor` sends (`poo grind schedule set -layer-rate`).
- **Long chases always go on (D124 to D126):** a chase paces its requests
  under the sender's budget and keeps its progress through a timeout;
  paging one entry's layers yields each page and checks their order; step
  4 moves on only past what it counted; a chase waiting for disk space
  shows in `poo chases`, `poo doctor` and the metrics. Grind schedule
  changes are made on the node (`POST /v1/grind/schedule/edit`, `If-Match`
  on `PUT`), so two at once both apply. Large items are encoded once, and
  large-entry parts live in the data directory; `grind.layer_rate` is at most 50
  a second, so a node's own layers stay inside peers' default budget
  (D127).
- **Slow senders and larger pages (D128 to D130):** kept progress and
  parts files of departed senders are cleaned up, while a stalled chase's
  progress outlasts its sender's backoff and a stall that made progress
  does not double it; chases ask for hashes in pages of 4,096; the grind
  schedule shows its version and limits, and edits take `-if-version`.
- **The disk reserve and paced askers (D131 to D135):** kept progress
  holds nothing per layer, and a fetch at the disk reserve reclaims the
  parts of an idle sender, a departed one, or one with more still to come,
  so a sender trickling a huge claim never holds the room an honest entry
  needs; an asker learns a sender's request budget from dropped requests
  and stays under it; catch-up checks signatures on every core; a full
  grind schedule leaves room for four user jobs, an edit restarts only the
  jobs it changes, and offline edits take the data directory's lock; `poo
  doctor` offers a command for a lowered value.
- **Turns at the disk reserve and faster restarts (D136 to D138):** a
  learned pace is probed back up; a slow fetch waiting for disk gets its
  turn, so one lost request or a slow honest link never leaves an entry
  of the heavier ledger waiting for good behind a trickler; `node.json`
  writes from the CLI take the data directory's lock; restart checks the
  store's signatures on every core; `poo doctor`'s fixes name the policy
  they checked, and floor or rate changes restart no grinding job.
- **Paid cuts at the disk reserve (D139 to D141):** a fetch waiting for
  disk takes room only for the bytes that arrive, cut from the end of a
  file whose sender left, stalled, or delivers at least 1.5 times more
  slowly over a whole chase, so neither a trickler nor colluders cutting
  each other in bursts can hold the room against an honest sender;
  `poo wallet follow` replaces the series' rule in one change; status and
  `poo doctor` say when `node.json` waits for a reload.
- **Headless and GUI level (D143):** `poo follow -replace` sets a
  series' followed pools in one change, as the GUI does; `poo submit
  entry|layer` hands the node an entry or layer made elsewhere;
  `poo grind jobs ID` shows one job; a test fails when an RPC call has no
  `poo` command.
- **Safer wallet writes (D144, D145):** a `poo wallet` command never undoes
  another's change, a new passphrase included (it saves only over the file
  it read, under a short lock); a typed new passphrase is asked for twice;
  `poo submit` reads a file or standard input, `build`/`work -bytes` print
  what it reads, and the node's own RPC takes an entry of any size with the
  full token.
- **Round 16 (D146, D147):** a payment holds the wallet from before it is
  sent until it is saved, and every wallet command that saves reads the
  file again after its prompts, so nothing saved meanwhile is lost;
  `poo submit` reads a `-bytes` reply as printed and sends a large entry's
  hex as read; the node reads an entry body once, setting aside at most
  64 MiB before the bytes arrive; Windows locks a data directory as Unix
  does.
- **Sharing node policies (D148, D149):** `poo policy export` writes one
  JSON file with a series, a node policy or every saved node policy, without
  this machine's own settings unless `-full`; `poo policy import` shows what
  it would add, change or keep and applies it only with `-apply`, live on a
  running node; a wallet command waiting since before a passphrase change
  is refused; wallet, peer and node files are written durably; help lists
  only the flags a command takes; a large entry's buffer ends at its size.
- **Safer imports (D150, D151):** a default import takes only what to
  follow, `-settings` the node's settings with the lines that switch off
  defences listed first; a node bundle is planned whole before anything is saved;
  `-if-digest` applies only the file that was previewed; plans show
  values; and a wallet command opened before a passphrase change, `seed`
  and `cosign` included, no longer unlocks with the old one.
- **Round 19 (D152, D153):** plans count the pools a follow takes on and
  list every node setting first under `-settings`; `-if-digest` needs
  `-apply` and a full digest; a node finds its stores from disk; and the
  weight check of priority peers can no longer be turned off (at most an
  hour apart).
- **Settings export and import (D154, D155):** `poo settings export` and
  `poo settings import` carry a node's settings, without what it follows
  or anything personal, to another machine. The preview lists what may
  not fit there, and `-map-path` remaps file paths. The weight check asks
  within each peer's budget. A whole-series follow of a huge series costs
  little in p2p. Taking a pool off a large follow writes `except`. The
  import digest covers the options too.
- **Peer book (D156, D157):** `poo peer add|remove|label|connect` edit the
  peer book by hand; `poo peers` filters by category, mint policy, label
  or address and `poo tracking` by blocked or timed-out; `poo peers
  export|import` carry the book or chosen categories, landing unlisted,
  with blocks only on `-blocks`. The manager dials imported addresses when
  short of peers, unless `p2p.no_dial_imported`. A plain export leaves out
  personal registries and grind jobs (D156).
- **Settable limits (D158, D159):** node.json sets the peer book's size
  (`p2p.max_book`, 256 to 65536) and per-range share, verified peers per
  range, retry and registry pacing, and the tracker's address count and
  memory, each with a floor and a ceiling. Imports take at most a quarter
  of the book, and `poo peer remove -imported` forgets them (D158).
- **Rival claims and settable limits (D160, D161):** `poo claim -parent
  before-claim` claims a mint the selected ledger has claimed on a branch
  from before that claim, to grind until it outweighs the other; `poo
  build -parent` builds on any held entry. `poo policy limits` lists the
  bounded settings and `-set` changes them, live in a running node.
  `seen_size`, `max_pending` and `ties` have bounds, and a full book makes
  room by the oldest unreached candidate (D160).
- **Variable supply series, phase 1 (D163):** a series can be variable.
  Its issuer releases more parts of M mints later, in order, and each
  release proves every part before it (PoO section 16.8). This round
  builds the definition (format version 2), part ranges and ids, release
  records and checks, the issuer tools `poo series new -variable`,
  `poo series sign`, `poo series release` and `poo series parts`, and
  test vectors. A node does not follow a variable series yet: it reports
  it unreadable until phase 2 builds its transactions
  ([docs/variable-supply.md](docs/variable-supply.md)). Fixed series keep
  their bytes and ids exactly.
- **Variable supply series, phase 2 (D166):** nodes follow variable
  series. A variable mint names its part, which its transaction's body
  and tid cover; fixed transactions keep their bytes and tids. A node
  keeps the newest release of each series in node.json, and a
  transaction on a part it has no release for waits (409 `waiting`):
  it is never invalid and is taken once the release arrives. New:
  `poo series add-release`, `POST /v1/series/release`, `poo series
  followed`, and `variable` in `GET /v1/series`. `poo series sign`
  prints a signed series file that `poo init -series` reads.
- **Variable supply series, phase 3 (D168):** nodes pass releases on. A
  node sends a peer the newest release of each variable series it
  follows when they connect (wire message type 14, ignored by nodes that
  do not know it) and passes on each new one; the receiver checks it in
  full, keeps it in node.json and opens the new part's pools with no
  reload. `poo series add-release` takes a release in place too
  (`in_place` in the reply). `poo series parts` adds `mints_released`,
  and a command refused only because its part is not released yet exits
  with code 3.
- **Round 32 (D184):**
  - `poo status` names the node's release (`release.version` and
    `commit`), so a front end can tell when it talks to another version.
  - After an upgrade, status lists the first 8 snapshots not used and
    their count, not one line per pool; the start log names each.
  - Going back a release no longer loses settings a newer one wrote in
    node.json: they are kept on every save, and the node warns at start.
- **Round 31 fixes (D183):**
  - A router's address is confirmed only when the node's own listener
    receives the dial-back's random token, so an echo or a copied hello
    no longer passes.
  - A mapping request that fails leaves nothing at the router: UPnP asks
    for the address before mapping, and an unusable or unanswered request
    is removed. A PCP lifetime of 0 is no mapping.
  - Renewal comes at half the asked lease when the router states a longer
    lifetime, a lifetime of 30 s or less is refused, and a stop no longer
    waits on a silent dial-back. Green's port-map status (#234) is
    carried.
- **Round 31 (D182):**
  - **Port mapping, opt in.** `p2p.port_map` (`poo init -port-map`) asks
    the router to forward the peer port, by PCP, NAT-PMP or UPnP. The
    router's address is offered to peers only after the node dials it and
    reaches itself. The lease is renewed and removed at a clean stop. It
    is off by default because peers then learn the network's public
    address. `poo status` shows `port_map`, and `poo doctor` has a
    `port-map` check.
  - Snapshots of large pools are taken every tenth of the log rather than
    every `snapshot_every` records, and gathering one walks the ledger
    once rather than twice (ORANGE-040).
- **Round 30 fixes (D181):**
  - Each release's snapshots are its own: the build a snapshot names now
    includes the release version and commit, so an upgrade replays once.
  - A start checks that a snapshot restores the pool it was written from,
    and replays the log if not. `-reverify` says when it compared nothing
    because a snapshot was behind its log.
  - A temporary snapshot file left by a crash is removed at start.
- **Round 30 (D178 to D180):**
  - **Fast starts from pool snapshots.** A node saves each pool's
    skeleton, tip state, pending transactions and the place of every
    entry record in its log beside the log, at a clean stop, after each
    compaction and every `snapshot_every` records (default 50,000). A
    start restores it and replays only what came after, so it no longer
    re-checks the whole history. A snapshot that is torn, damaged, from
    another pool or build, or no longer fits its log is not used, and the
    log is replayed whole as before. `poo node -reverify` replays
    everything and reports whether the snapshot matched; `no_snapshots`
    turns them off.
  - A body that fails to read back, or a node that stopped after an
    internal error, removes the snapshots, so the next start checks the
    logs and cuts a damaged one.
  - `poo status` and `poo doctor` report the snapshots' age and the most
    records a start would replay; `poo status -counts` rows name the
    built-in reserve series (`builtin`).
  - `poo policy import -json` and `poo settings import -json` leave out
    `digest` when nothing would change.
- **Round 29 fixes (D177):**
  - A failed read of a cold entry while checking a pushed entry, a chase
    page or a build on a chosen parent now stops the node cleanly; it used
    to leave the engine locked, so the node hung and could not shut down.
  - A revert's cost counts the older entries it reads back, so a
    transaction with many inputs is charged for them.
  - `poo status -counts` prints one row of totals per followed series.
- **Round 29 (D174 to D176):**
  - **Items and undo on disk too.** Entries of the selected ledger more
    than `recent_window` (default 10,000) below the tip keep only their
    skeleton in memory; a deep switch or view reads their transactions
    back and recomputes their undo, so it is slower, never refused.
  - Ledger replies to peers read entry bodies after the engine lock is
    released.
  - A panic while a pool is being changed now stops the node, which
    rebuilds from its stores on the next start.
  - `poo policy import -replace` honours the node's `series_ceiling` and
    keeps each series once.
- **Round 28 fixes (D173):**
  - An entry body that cannot be read back from a damaged log no longer
    crashes the node when a peer asks for it: it is not held for that
    peer. A panic while handling a peer's message closes that peer only.
  - `GET /payment` says `unreadable`, and ledger and address pages count
    `unreadable` entries, instead of answering as if complete; rivals are
    found from memory and are always complete. `poo doctor` warns
    (`bodies`).
  - `entry_cache_mib` counts bodies as they really take, with a 256 MiB
    default (ORANGE-038).
- **Round 28 (D172):**
  - **Entry bodies on disk.** A node keeps every held entry's skeleton in
    memory and reads entry bodies back from its pool logs when it needs
    them, through one cache bounded by node policy `entry_cache_mib`
    (default 512 MiB, applied in place). Start-up reads the logs one
    window at a time, and compaction copies records from the old file,
    so neither holds the whole history. What a node decides is
    unchanged. `poo status` shows `bodies` and `file_limit`.
  - A node warns at start, and `poo doctor` checks (`open-files`), when
    the open file limit is below the followed pools plus 256.
- **Round 27 (D170):**
  - **Per-series witness size limits.** The network-wide caps (4,096-byte
    keys, 8,192-byte signatures, 64 KiB params and data) are gone: a
    series' witness ceiling, its algorithms' exact sizes and its types'
    bounds decide. Series with a ceiling at or below 64 KiB decide as
    before; looser series now take, for example, 16-of-16 ML-DSA-87. A
    pinned cross-version test, with verdicts from a build of the code
    before the change, shows nothing valid became invalid.
  - A new key algorithm, `slh-dsa-sha2-256s` (FIPS 205, number 11).
  - The default reserve series: `poo init` with no series follows it, and
    tools name it `"builtin": "reserve"` on its full id. Its id is pinned
    (`fe52c1ac…f139`).
  - `poo series new` writes a 128 KiB ceiling by default and, like
    `poo init -series`, refuses a series with no ceiling or one above
    4 MiB without `-any-ceiling`. Node policy `series_ceiling` lets a node
    decline such a series on import.
  - An address whose largest witness would not fit its series is refused
    where it is made; `poo crypto -max-witness N` shows each algorithm's
    largest k.
  - `poo policy limits` says for each limit whether a running node
    applies it in place (`in_place`).
  - `poo series release` and `poo series parts` on a part far out can be
    cancelled (a GUI job's Cancel, or Ctrl-C).
- **Round 26 (D168):**
  - `poo policy limits -set` applies the peer book's limits, the
    tracker's retention and `seen_size` in place, with no reload
    (`in_place` in the reply); the other limits still reload.
  - `poo work` with no `-tries` or `-for` tries 2^(floor + 2) nonces for
    a series whose layer floor needs more than the old 2^16, and the
    node's cap on one work call grows with the floor (2^(floor + 4), at
    most 2^40); `poo build -work` gets the same cap.
  - A new key algorithm, `slh-dsa-sha2-128s` (FIPS 205, number 10), for
    series that allow it.
  - The tracker trims records with the same age in address order, so the
    same records and bounds always trim alike.
- **Round 25 tooling (D166):** `poo policy limits -set` during a reload
  waits for the node instead of failing with EOF; `poo series info`
  shows a variable series' supply type, part size and cap and reads
  signed files, and `-h` prints its usage; `poo peers -counts` leaves
  out the empty book list; `poo series parts -o table`;
  `poo series release -check -` reads standard input.
- **Round 24 fixes (D162, D164):**
  - `poo peers import -apply -if-digest` works against a running node
    again.
  - `poo peers -counts` gives the book's size without its records.
  - `poo claim` and `poo build` refuse an empty `-parent`.
  - **Restart after upgrading.** `rpc.Dial` sends the token only after
    `GET /v1/hello` proves the node (D162). A node that was started
    before this upgrade has no `/hello`, so restart it.
