# RED-043: a default `policy import` takes every setting the node leaves unset, including the ones that switch off its defences or reach outside, and the plan shows them like any other line

| Field | Value |
| --- | --- |
| Severity | medium |
| Status | fixed |
| Found on | `main` @ b17e892 (Source Round 17 Part B) |
| Component | pkg/nodepolicy/bundle.go (`Merge`, `mergeInto`, `MachineLocal`), pkg/cli/policyport.go, pkg/rpc/policyimport.go |
| Surface | a bundle file from someone else; `poon policy import -apply`, `POST /v1/policy/import` |
| Fixed by | `claude/project-thread-ggo8dt` @ cfd1cbd (BLUE-018) |

## Summary

D149 keeps a shared file from opening someone's control interface or picking
their GPU by listing "machine-local" settings (`MachineLocal`: rpc, no_sync,
p2p listen/advertise, grind device/threads) and stripping them unless
`-full`. That list is right, and I could not get the RPC section, the
listen addresses or a depth limit in through an import (checked below). But
the same merge adds, with no flag at all, every other setting the node has
not set ("every single setting local leaves unset"), and a node that never
set a bool or number has it omitted from its file (`omitempty`), so the file
wins. Several of those are not about what the node follows; they are the
node's defences:

- `p2p.allow_local_peers` (off by default on purpose, D96: so peers cannot
  steer the node at loopback or private addresses), `p2p.no_confirm`,
  `p2p.strict_return`, `p2p.no_notices`, `p2p.no_relay`,
  `p2p.request_rate`, `p2p.unlisted_conns`;
- `tracking.*` (`no_timeout_misses`, `starting_points`, limits, groups and
  per-peer groups), which decide how peers are punished or trusted;
- `p2p.registries`, which the node fetches at start (http(s) URLs or local
  files, `registry.Fetch` has no check for loopback, private or
  link-local addresses): a bundle can make the node GET
  `http://169.254.169.254/...` or `http://127.0.0.1:PORT/` and read a local
  file, with the result visible only as a parse error in the log;
- `p2p.peers`, which are always dialled, `grind.jobs` (a bundle can start
  CPU or GPU work on a schedule, as many as `max_jobs` allows), and
  `max_peers`, `entry_limit`, `k` and other knobs a bundle can set to a
  value that makes the node poor or useless.

The plan lists each as `+ field` (the same mark as a harmless
`+ follow[...]`), and a bundle of benign settings with a handful of these
among twenty other lines is easy to apply. `-prefer-file` also overwrites the
operator's own values of the same settings.

## Reproduction

`TestRedDefaultMergeTakesDefenceSwitches` (scratchpad
`red_r17b_nodepolicy_test.go.txt`, goes in `pkg/nodepolicy`): `Merge` of a
default local policy with a file that sets those settings, `MergeOptions{}`:

```
taken without -full or -prefer-file: allow_local_peers=true no_confirm=true
strict_return=true no_notices=true no_relay=true request_rate=1073741824
unlimited unlisted_conns=-1 max_peers=1 entry_limit=1 no_timeout_misses=true
starting_points=1e+09 k=0 registries=[http://169.254.169.254/latest/meta-data/
http://127.0.0.1:8080/] peers=[198.51.100.7:9000]
plan skipped (machine-local): []
```

All of it is in the result, `Skipped` is empty.

## Impact

Node policy only: nothing here changes what is valid, and an operator who
reads the plan can refuse it. But the safeguard against a hostile bundle is
the person reading a long plan, and the plan does not tell the dangerous
lines from the others. The node then runs with peer-steering protection off,
punishment off, or a request to its own internal network on every start.
The same settings are reachable through `POST /v1/policy/import` with the
full token (no new authority there).

## Recommendation

1. Add a second class beside machine-local, "exposure and defence"
   (the settings above), and treat it as `MachineLocal` is treated: left out
   of an export unless `-full`, skipped on import unless a new
   `-trust-security` (or `-full`), listed in `plan.Skipped` with the name
   of the flag.
2. Or at least a plan section "Risky" that names them, printed first, with
   the node's current value, and refuse `-apply` with such lines unless
   they are named in a flag, so the GUI and the CLI cannot apply them by
   default.
3. Give `registry.Fetch` the same check as peers (no loopback, private or
   link-local host unless `allow_local_peers`), and refuse a local file path
   in a registry that arrived through an import.

For Black: nothing here touches validity, but `k`, `entry_limit`,
`hash_limit`, `layer_limit` and `max_pending` do reach a node through a
bundle, so please confirm none of them can make a heavier ledger
unacceptable (my reading is that they only set request sizes and what is
kept, not what is valid).

## Tried and found fine

- `rpc` never changes through the endpoint: with `Full` the file's section is
  replaced by the node's own; without it `Strip` clears it; `Replace` keeps
  the local one. `p2p.listen` and `advertise` follow the same rule (they
  need `Full`).
- `depth_limit` and `fast_catch_up` are zeroed on import (D116).
- A node bundle over an archived name is refused before anything is changed;
  two policies of one name, more than one marked current, an unknown field,
  a second JSON value, more than 256 policies and a bundle over 16 MiB are
  refused; names must match `^[A-Za-z0-9_-][A-Za-z0-9._-]{0,63}$`, so none
  escapes the root. The POST needs the full token (a view token gets 403).
  The import runs under `policyMu`, as follow and schedule edits do, and the
  CLI's file fallback takes the node's directory lock, so it cannot run while
  a node holds the directory. Replace with a series `series.New` refuses is
  saved but harmless (`Accepted` lists it as refused, it does not stop the
  node).

## Resolution

Fixed (D150), recommendations 1 and 3, in the stricter form. A default
import takes only what to follow: series, follows and accepted issuers
(`nodepolicy.Followed`). Every other setting of the file is a node
setting, listed in `Plan.SkippedSettings` ("skipped, node settings
(-settings takes them)") and left alone, with merge, `-prefer-file` and
`-replace` alike. An allow-list rather than a list of risky settings: a
switch added to node.json later is skipped by default too. `-settings`
(`MergeOptions.Settings`, also implied by `-full`) takes them, and then
the plan names first, under "read first", each line that touches one of
`nodepolicy.Defences` (the p2p switches, the request rate, connection
counts, peers, registries, `entry_limit`, `k`, `issuers.any`, tracking,
grind jobs). A registry that is a local file is skipped unless `-full`.
The node's registry fetch (`p2p.registryClient`) refuses, after name
resolution and on redirects, an address that is not public unless
`allow_local_peers`. A registry reached through an environment proxy has
its literal address checked. Export is unchanged: the receiving node
decides. For Black: `k`, `entry_limit` and the other limits are no longer
taken from a file by default; whether any could make a heavier ledger
unacceptable is your call in Part D. Tests: `TestDefaultImportTakesOnlyWhatToFollow`,
`TestSettingsImportNamesDefences` (pkg/nodepolicy),
`TestRegistryRefusesLocalAddresses` (pkg/p2p), `TestPolicyExportImport`
(pkg/cli).
