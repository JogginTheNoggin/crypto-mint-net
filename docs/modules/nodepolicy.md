# pkg/nodepolicy

## Summary

The node policy: every choice a full node makes for itself, as a
`node.json` file. It lists the known series (series information comes from
outside the network), which issuers are accepted, what the node follows (an
issuer, a series, some pools or some mints), K and the other limits, write
durability, the RPC address, the p2p settings (including peer slots and
chasing) and how peers are tracked. A `Manager` keeps several node policies side
by side with the design's save, archive and delete operations (decision
D25). Nothing here changes what is valid.

## node.json

```json
{
  "format": 1,
  "series": [{"params": {"hash": 1, "mints": 8, "...": "..."}, "issuer_witness": "hex, optional"}],
  "issuers": {"any": true, "accept": ["issuer address hex"]},
  "series_ceiling": 0,
  "follow": [
    {"series": "series id hex"},
    {"series": "series id hex", "pools": [1], "mints": [9]},
    {"issuer": "issuer address hex"}
  ],
  "k": 8,
  "ties": 64,
  "max_pending": 10000,
  "seen_size": 65536,
  "no_sync": false,
  "rpc": {"listen": "127.0.0.1:8645", "allow_remote": false},
  "p2p": {
    "listen": ":8646",
    "advertise": "node.example.org:8646",
    "peers": ["203.0.113.5:8646"],
    "max_peers": 32,
    "chunk_size": 1048576, "entry_limit": 512, "hash_limit": 4096, "layer_limit": 65536,
    "no_notices": false,
    "check_seconds": 60,
    "registries": ["https://registry.example/poo.json"],
    "max_outbound": 8,
    "max_inbound": 24,
    "allow_local_peers": false,
    "no_dial_imported": false,
    "no_relay": false,
    "hop_limit": 8,
    "request_rate": 120,
    "ping_seconds": 120,
    "priority_slots": 4, "backup_slots": 4, "margin": 1,
    "review_seconds": 600,
    "unlisted_conns": 8,
    "strict_return": false,
    "no_confirm": false,
    "max_book": 4096, "book_per_range": 16, "verified_per_range": 4,
    "retry_seconds": 60, "registry_seconds": 3600, "candidate_seconds": 86400
  },
  "tracking": {
    "template": "basic",
    "custom": {"lean": {"from": "basic", "off": ["rtt"]}},
    "groups": {"priority": "standard", "unlisted": "lean"},
    "peers": {"203.0.113.5": "detailed"},
    "escalate_at": 5,
    "priority_limit": 5, "timeout_limit": 10, "block_limit": 30,
    "timeout_seconds": 600, "block_seconds": 86400,
    "weights": {"pings": 2, "rtt": 1, "first_deliveries": 1, "requests": 2, "reply_timing": 1, "chases": 2, "denials": 2, "cost": 0.1, "large": 0.5},
    "starting_points": 0,
    "no_timeout_misses": false,
    "max_addresses": 8192, "flag_history_seconds": 604800,
    "log_seconds": 3600, "log_size": 256
  },
  "log": {"level": "info", "format": "plain"},
  "grind": {
    "device": "auto", "threads": 0, "layer_floor": 0, "layer_rate": 10,
    "jobs": [
      {"target": "pending"},
      {"target": "tip", "series": "…", "pools": [1], "hours": "22-06"}
    ]
  }
}
```

`grind.layer_floor`, when above 0, keeps every layer the node's own
grinding finds at or above that strength (raised to the series' minimum),
not only the strongest of each search: several times the credit for the
same work, and more layers sent (ORANGE-010, D118). 0 keeps today's
behaviour. `grind.layer_rate` bounds the layers a second the floor adds,
strongest first, so peers' layer budgets take every one: 0 means 10, and
at most 50, half of the 100 a second peers take from one node by default (ORANGE-016, D122; GUI-015, D127). Both are set by `poo grind schedule set
-layer-floor S -layer-rate R`. A node.json written while a bound was
higher still opens: `Load` brings the value down to the bound and notes
it in `Policy.Lowered` (a `Lowered` each: the field, the value written,
the value used and `FixArgs`, the command that writes the value used),
which the node logs as a warning and `poo doctor` shows with that command
as its fix_args (GREEN-035, D135); every write (Save, the RPC, the CLI, the GUI) refuses it. A bound
may only be lowered this way, so an older file never stops a node from
starting (RED-025, D128). `p2p.fast_catch_up` and `p2p.depth_limit` are no longer used: a
chase finds a split at any depth (D116). They still load and are ignored.

Every field but `format` may be left out; the defaults are the provisional
ones in [decisions.md](../decisions.md) (D12, D22, D24, D26). `k: -1` keeps
every branch. `rpc.listen: "off"` disables the interface. `p2p.listen:
"off"` accepts no connections (listed peers are still dialled);
`ping_seconds: -1` turns off pings. The weight check cannot be turned off:
`check_seconds: -1`, or a value over 3600 (`MaxCheckSeconds`), is read as
one hour and noted as lowered, so a node whose peers send no notices still
finds a heavier ledger they hold (D153).
Without `advertise` the node is never offered to others as a contact. The p2p defaults are in
`p2p.DefaultConfig` (decisions D37, D47, D62). `unlisted_conns: -1` keeps
no unlisted connections; `review_seconds: -1` never reviews.
`max_inbound` defaults to `max_peers` less `max_outbound`, so inbound
connections cannot take the outbound slots (D75); `-1` caps inbound only
by `max_peers`. `allow_local_peers` (off by default, D96) lets addresses
learned from peers and registries be loopback, private, link-local or
otherwise not public; turn it on only for a network on one machine or LAN.
Addresses in `peers` are always dialled. `no_dial_imported` (off by
default, D157) keeps the unlisted addresses an imported peer file gave
out of the connection manager, so only `poo peer connect -imported`
tries them.

The peer book, tracker and discovery limits, and the engine's memory
limits, are the user's to set (D159, D160). Left out or 0, each is its default; any other value must lie in
its range, or `Validate` refuses the policy and names the setting
(`nodepolicy.Bounds`):

| Setting | Default | Range | What it bounds |
| --- | --- | --- | --- |
| `p2p.max_book` | 4096 | 256 to 65536 | addresses in the peer book; a quarter of it is for imported ones (D158) |
| `p2p.book_per_range` | 16 | 1 to 4096, at most `max_book` | addresses from one network range |
| `p2p.verified_per_range` | 4 | 1 to 64 | priority and backup peers from one range, per mint policy |
| `p2p.retry_seconds` | 60 | 1 to 86400 | the first wait before trying a failed candidate again, doubled each failure |
| `p2p.registry_seconds` | 3600 | 60 to 86400 | the least time between two loads of the registries |
| `p2p.candidate_seconds` | 86400 | 60 to 31536000 | how long an address learned from the network and never reached keeps its place against a newer one when the book or its range is full (D160) |
| `tracking.max_addresses` | 8192 | 1024 to 131072 | addresses the tracker keeps records for |
| `tracking.flag_history_seconds` | 604800 | 3600 to 31536000 | how long an expired flag is remembered |
| `tracking.log_seconds` | 3600 | 60 to 86400 | how long each address's detailed log keeps events |
| `tracking.log_size` | 256 | 16 to 4096 | events in each address's detailed log |
| `seen_size` | 65536 | 1024 to 4194304 | items on the seen list; it grows as items arrive (D160) |
| `max_pending` | 10000 | 16 to 1048576 | pending transactions per pool (D160) |
| `ties` | 64 | 1 to 4096 | off-chain entries kept that tie with the selected ledger's weight, beyond `k` (D160) |

`Policy.Limits` lists them with the value node.json sets and the value used,
from the one list `Validate` checks; `poo policy limits` prints it
(GREEN-047). Each limit's `in_place` says whether a running node applies a
change of it without a reload, from the `InPlace` list below
(GUI-GREEN-053, D170).
`poo policy limits -set NAME=VALUE` (`Policy.SetLimits`,
`POST /v1/policy/limits`) changes them, checked whole first (D161).
A running node applies the ones `InPlace` names without a reload (D168):
`p2p.max_book`, `p2p.book_per_range`, `p2p.retry_seconds`,
`p2p.candidate_seconds`, the four `tracking.*` limits and `seen_size`.
Each part ends as a reload onto the same node.json would leave it: the
book takes its limits and runs the same `Fit` as on start, the tracker
its retention, and the engine a new, empty seen list. `ChangedLimits`
lists what changed and `SameButInPlace` says whether node.json holds
anything else the node does not run yet, in which case the reload is
used. `verified_per_range`, `registry_seconds`, `max_pending` and `ties`
stay on the reload.

What the ceilings cost, as Orange measured them (ORANGE-032): a tracker
at `tracking.max_addresses` 131072 takes about 420 MiB (3.3 KB an
address), a 171 MiB `tracking.json`, and 6.9 s to save, holding the
tracker meanwhile (at shutdown, and on a peer import with `-blocks`); a
book at `p2p.max_book` 65536 takes 0.7 to 0.9 s for each save of the
manager's pass. The defaults cost a small part of that. Raise a limit
toward its ceiling only on a machine that has the memory and disk.

None of them changes which ledger a node selects: a smaller book or a
shorter memory only means the node knows fewer peers, which is a lack of
information (AGENTS.md). In
`tracking`, `escalate_at: -1` never escalates; every template name must
be known, custom templates may not form a cycle, and groups must be
priority, backup, candidate, unlisted, inbound or outbound (see
[tracking.md](tracking.md)). `peers_per_policy`, from milestone 4, is ignored:
the slots replace it. `log.level` is `debug`, `info` (default), `warn` or
`error`, and `log.format` is `plain` (default), `text` or `json`
([logging.md](logging.md)); `poo node -log-level` and `-log-format` override
them for one run. `grind` ([grind.md](grind.md), D109) sets the device
(`auto`, `cpu`, `gpu:0`, ...) and threads (0: every core but one) that
grinding uses unless told otherwise, and up to 16 jobs the node runs by
itself from start to stop: `target` is `tip`, `pending`, `tx:TID` or an
entry hash, with optional `series`, `pools`, `device`, `threads`, `hours`
(local, such as `22-06`) and, for an entry or `tx:`, `until_from` (which
`tx:` needs). `Validate` checks the section, so a reload onto a bad one
is refused (D106). `poo grind schedule` edits it, and a running node
applies the change at once without a reload (D113).

## API

- `Policy`, `Default`, `Validate`, `Load`, `Save` (atomic: its own
  temporary file per call, synced, renamed, then the directory synced, so
  concurrent writers never tear the file; RED-013; through `fsutil.WriteFile` since D148),
  `LedgerConfig`, `TrackingConfig`, `ListenAddr`, `P2PListenAddr`
  (`p2p.FromPolicy` turns the p2p section into host settings).
- `SeriesInfo.Build()` builds a listed series with the release it keeps,
  and reports a kept release that does not check (left out, never the
  series; RED-072, RED-073, D167). Every reader of a listed series uses it.
- `CheckRelease(r)` checks a release in full without changing anything.
  `AddRelease(r)` checks every release in full, also one of a known part,
  and replaces a kept one that does not check (RED-074). `Load` and `Save`
  read and write the node's own cache of checked releases,
  `releases-checked.json` (`CheckedFileName`), beside node.json, so a
  later process checks only a kept release's witness. It is never exported,
  and a cache that does not match is ignored (RED-075).
- `Accepted()` builds each listed series and applies the issuer rule,
  returning accepted series (with the verified issuer) and refused ones with
  reasons. One bad series never stops a node.
- `AddFollow(f)` / `RemoveFollow(f)` edit the follow list with merge
  rules (decision D93). Adding merges: an issuer appears once, a whole
  series replaces any parts of it, and pools and mints of one series join
  one entry (`joinFollow`: what either follows). Removing an issuer or
  whole series takes it off entirely. Removing pools from a series
  followed whole puts them on the follow's `except` list, so "all but
  these" stays one entry for a series of any size. When fewer pools are
  left than are taken off, the pools left are listed instead (RED-050,
  D154). Adding a pool, or a mint, of an excepted pool takes it off the
  list. Removing mints takes them off the listed mints, and a mint
  followed only through its pool cannot be removed alone, since a mint is
  checked with its whole pool. Both refuse a series not listed in
  `series`. `poo follow`, `poo unfollow`, `poo wallet follow` and
  `POST /v1/follow` all use them, so the file and the running node always
  agree. A change after which the follow list would need more than
  `MaxSections` (4096) separate sections, the wire's limit on what a node
  announces, is refused with the way out, when it is made. A whole series,
  with or without `except`, is one section (RED-050, D154).
- `ReplaceFollow(f)` puts f's pools and mints (or the whole series, when
  f names none) in place of the series' rule in one change, leaving other
  rules alone; `poo wallet follow` uses it through `POST /follow` with
  `replace` (GUI-018, D139).
- `GrindEdit{Remove, RemoveAll, Add, Set, Version}.Apply(g)` changes a
  grind section where it is kept, so two changes made at once both apply.
  A removal names the whole job at its number (`SameJob`), so a stale
  number is refused with `ErrGrindChanged` rather than removing another
  job. `Grind.Version()` names a section's content, for `If-Match` and
  `GrindEdit.Version` (GUI-014, D125). The node applies edits under its
  lock (`POST /v1/grind/schedule/edit`); `poo grind schedule` applies the
  same edit to node.json when no node runs, under `Lock(dir)`, so it is
  refused while a node or another command holds the directory (GUI-017,
  D134).
- `EncodeWitness` / `DecodeWitness`: the hex form of an issuer witness.
- `Manager{Root}`: `Save`, `Load`, `List`, `Current`, `Use`, `Archive`,
  `Unarchive(name, fresh)`, `Delete`, `Dir`.
- `Lock(dir)`: an exclusive lock on a data directory, released by the
  operating system if the process dies, refused at once if another process
  holds it: flock on Unix, LockFileEx on Windows (D147), and only a
  directory check on other systems.

## Sharing node policies: bundles

One file, a *bundle*, carries node policies between nodes: a series to
follow, one node policy, or every saved node policy of a node (decision
D149). It is JSON:

```json
{
  "kind": "poo-node-policies",
  "format": 1,
  "scope": "series | policy | node | settings",
  "exported": "2026-11-02T21:00:00Z",
  "full": false,
  "policies": [{"name": "default", "current": true, "policy": {"format": 1, "...": "node.json"}}]
}
```

A `settings` bundle holds one node policy's settings and nothing it
follows (see "Settings" below). The `settings` field is reserved for
settings kept outside node.json in a later format; this version writes
none and only notes that a file has them.

- **Export** (`ExportPolicy`, `ExportSeries`, `NewBundle`, `Bundle.Write`)
  leaves out the machine-local settings, `MachineLocal`: the `rpc`
  section, `no_sync`, `p2p.listen`, `p2p.advertise` and the grind device
  and threads (the section's and each job's). A shared file then never
  opens someone's control interface or picks their GPU. `full` keeps them,
  for a backup or a clone of the same machine. The settings no longer
  used (`p2p.fast_catch_up`, `p2p.depth_limit`) are always left out. No
  token, key or seed is ever in a bundle: they are not in node.json.
- **Reading** (`ReadBundle`) takes at most 16 MiB and 256 node policies.
  It refuses a field it does not know rather than dropping it, refuses a
  newer format, and checks every node policy as `Load` does, every series'
  parameters, and the names. A plain node.json is read as a bundle of one.
- **Merging** (`Merge(local, file, MergeOptions)`) returns the result and
  a `Plan`. By default it takes only what to follow (`Followed`, D150):
  - series new to local (a witness local lacks is added; two different
    witnesses are a conflict), except those `series_ceiling` declines;
  - followed issuers, series, pools and mints, merged as `AddFollow` does
    (in time linear in the entries, RED-044);
  - accepted issuers, as a set.

  Every other setting of the file is a node setting. The plan lists them
  in `SkippedSettings` and leaves them alone, so a file from someone else
  never turns off a defence of the node, points it at an address or
  starts work on it (RED-043). A setting added to node.json later is a
  node setting unless it is put in `Followed`. `Settings` takes them too:
  - peers, registries and grind jobs, as sets. A registry that is a local
    file is still skipped unless `Full`;
  - every single setting local leaves unset. A custom tracking template
    and the tracking weights count as one setting each.

  With `Settings`, the plan's `Defences` names first every node setting
  line it changes, except the ones in `Cosmetic` (`log.format`). A
  setting added later is named until someone shows it changes only how
  the node reports (RED-048, D152).

  The plan's `Follows` gives, for each series whose follow it adds to or
  changes, the series' pool count and how many pools the result follows.
  A whole-series follow of `LargeFollow` (65,536) pools or more is named
  first in `Defences`, even in a default import. This limits nothing; it
  tells the reader what the line costs (RED-046, D152).

  Where both set a value and they differ, local's is kept and the plan
  lists the conflict; `PreferFile` takes the file's. `Replace` takes the
  file's node policy whole, keeping local's machine-local settings, or
  without `Settings` only what it follows. `Full` takes the file's
  machine-local settings and node settings too. `SeriesOnly` (how a
  series bundle imports) takes only series, follows and accepted issuers.
  The plan lists what is added, changed and removed, the conflicts, the
  machine-local settings and node settings skipped, the defence lines, how
  many of the file's settings were already the same, and any series the
  result lists but would not accept (`Accepted`). A result that does not
  validate is refused, and the inputs are never changed.
- **Names.** A node policy made here or read from a bundle may not be a
  Windows device name (`con`, `nul`, `com1`, ...) or end in `.`, so it can
  be kept on any system (`checkNewName`). One already saved under such a
  name still loads.

## Settings: export, import and fit

A node's *settings* are everything in its node policy but what it follows
(`Followed`): peers, registries, limits, the p2p switches, tracking, logs,
the grind defaults and jobs, and `issuers.any` (decision D155). They are
there to be carried to another machine, kept as a backup, or shared.
node.json has no key, seed, passphrase or token. What else in it is
personal stays out of a plain export (`Personal`, RED-052, D156): a
registry that is a local file, a registry URL with a user name,
password, query or fragment, and a grind job on one payment or entry.
`-full` keeps them. Peers and `tracking.peers` are exported, as a setup
is made of them, and `ExportNotes` says what the file holds and left
out, so the person sharing it sees that first.

- **Export** (`ExportSettings(p, full)`) is `ExportPolicy` without the
  series, follows and accepted issuers. It goes in a bundle of scope
  `settings` (`ScopeSettings`). `ReadBundle` refuses a settings bundle
  that holds any of those.
- **Import** is `Merge` with `MergeOptions.SettingsOnly`. It drops what
  to follow from the file, even a whole node.json, and takes the
  settings as `Settings` does: the same conflicts, `Defences`, skipped
  machine-local settings and digest. With `Replace`, the file's settings
  replace local's, and local keeps what it follows and, unless `Full`,
  its machine-local settings. `SettingsOnly` with `SeriesOnly` is
  refused.
- **Paths** (`MapPaths`, `ParsePathMap`). The only local paths in
  node.json are registry files. Each `PathMap{From, To}` rewrites one
  that starts with `From` at a path boundary (`/data` never rewrites
  `/database`), before the merge. URLs are never rewritten. The rest of
  the path is joined to `To` with one separator, `To`'s own, and a
  `From` written the Windows way (a drive letter or a backslash) matches
  in any case and with either separator. A registry file a map rewrote
  is taken without `-full`; one it did not is skipped, as before
  (RED-055, D156).
- **Text** (`checkText`, in `Validate`). No text in a node policy may
  hold a control character, a bidirectional override or bytes that are
  not UTF-8: none is valid there, and a file's text reaches a terminal
  in the preview and the log. The error quotes the value escaped. The
  CLI also prints every value from a file through `termSafe` (RED-053,
  D156).
- **Series now used** (`Plan.NowUsed`). A series local lists but refuses
  (`Accepted`) that the result uses, because a setting such as
  `issuers.any` changed, is named in `NowUsed` and under the lines to
  read first (RED-056, D156).
- **Empty** (`Plan.Empty`). A conflict where the file's value was taken
  is a change, so a plan made of such conflicts alone is saved (RED-054,
  D156).
- **Fit** (`Fit(before, after, Machine)`) lists what the import brings
  in that may not fit the machine:
  - a grind device it lacks, or more threads than it runs;
  - a registry file that is not there (a network or device path, such
    as `\\host\share`, is named but never opened: a preview reaches
    nothing, RED-053);
  - an RPC or p2p listen address that is none of its interfaces';
  - an advertised address from another machine;
  - a grind job for a series the node policy does not list.

  Lines that `before` already had are not listed. The caller describes
  the machine (`grind.ThisMachine`) and puts the lines in `Plan.Fit`.
  Fit refuses nothing.

Nothing in a bundle changes what is valid, and nothing in a merge can make
a node refuse a heavier ledger: it writes only node.json, through the same
checks as any other change, and node.json has no depth limit.

## Extending

A new setting is a new optional field with a documented default. Bump
`Format` only for a change that old nodes would misread.

## Series witness ceilings (D170)

`series_ceiling` (0, the default, is off) lets a node decline series whose
witness ceiling it finds too large to check. When it is set, a policy
import leaves out any series it would add whose `max_witness` is 0 (not
set) or above it, with that series' follow rules, and lists each in the
plan's `declined` with the reason (`Policy.DeclinesSeries`). It never
drops a series the policy already lists. Declining a series is a lack of
information, not a verdict on any ledger, so it cannot split nodes that
have the same information. It is a node setting, so an import takes it
only with `-settings`. `poo init` and `poo series new` refuse such a
series on their own, unless `-any-ceiling` forces it.
