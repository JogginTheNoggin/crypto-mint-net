# Source findings

## Summary

Red team findings against the headless node (everything outside `gui/`).
The format and the team loop are in [AGENTS.md](../../../AGENTS.md#teams-read-first).
One file per finding, `RED-<NNN>-<slug>.md`. This index lists them all.

| Id | Title | Severity | Status |
| --- | --- | --- | --- |
| [RED-001](RED-001-remote-dial-target-injection.md) | A peer can make the node dial arbitrary addresses (internal-scan / reflection) | medium | fixed |
| [RED-002](RED-002-missing-chunks-amplification.md) | Missing-chunks request resends chunks without dedup or per-chunk cost | low | fixed |
| [RED-003](RED-003-unverified-stated-weight-chase.md) | An unverified inbound peer can start chases and pull in honest priority peers | info | won't fix |
| [RED-004](RED-004-wallet-kdf-memory-ceiling.md) | A crafted wallet file can still force a ~4 GiB allocation on open | low | fixed |
| [RED-005](RED-005-seed-on-command-line.md) | `poo claim`/`pay`/`keygen` take the raw seed on the command line | medium | fixed |
| [RED-006](RED-006-unbounded-build-pow-under-lock.md) | `/v1/build` runs unbounded entry proof-of-work while holding the global node lock | medium | fixed |
| [RED-007](RED-007-ipv6-transition-address-bypass.md) | Public-address filter misses IPv4-embedding IPv6 transition ranges (NAT64 / 6to4 / Teredo) | low | fixed |
| [RED-008](RED-008-address-view-unbounded-ledger-walk.md) | `/v1/address` walks the whole ledger per request under the engine lock, reachable with the read-only view token | medium | fixed |
| [RED-009](RED-009-preverify-verifies-what-checks-would-reject.md) | Pre-lock signature verification runs on items the cheap checks would reject, does not stop at the first bad signature, and never remembers a failure | medium | fixed |
| [RED-010](RED-010-failed-fsync-retried-then-reported-ok.md) | A failed fsync of the store is retried and a later waiter is told the record is on disk | low | fixed |
| [RED-011](RED-011-deep-parent-view-cost-doubled-under-lock.md) | An entry on a deep parent costs a revert of the whole selected chain, now twice, under the engine lock | medium | fixed |
| [RED-012](RED-012-requested-entries-uncharged-quadratic-branch.md) | A branch of entries with no layers costs time quadratic in its length, and the entries a chase fetches are not charged for it | medium | fixed |
| [RED-013](RED-013-node-json-writers-unserialised.md) | Concurrent writes of node.json share one temporary file and can leave it corrupt | low | fixed |
| [RED-014](RED-014-maxjobs-not-atomic.md) | MaxJobs is checked and then used without holding the slot, so concurrent starts exceed it | low | fixed |
| [RED-015](RED-015-chase-pages-hashes-without-limit.md) | A chase pages the sender's hashes for ever, keeping them all, with no work shown | high | fixed |
| [RED-016](RED-016-parts-fetch-trusts-claimed-size.md) | A part fetch buffers whatever the sender sends against a size it claimed, and a node holding nothing fetches it with no work check | high | fixed |
| [RED-017](RED-017-parts-reply-encodes-whole-item-under-lock.md) | Every parts request re-encodes the whole entry item while the engine lock is held | medium | fixed |
| [RED-018](RED-018-layer-floor-has-no-rate-cap.md) | `grind.layer_floor` sends a layer for every 4,096 tries, far above what peers accept | low | fixed |
| [RED-019](RED-019-lease-renew-stops-at-first-error.md) | `poo grind run` stops renewing its lease after one failed request | low | fixed |
| [RED-020](RED-020-layers-from-and-parts-copy-and-sort-all-layers-under-lock.md) | A layers-from or parts request copies, sorts and hashes all of an entry's layers under the engine lock, for a reply of a few bytes | medium | fixed |
| [RED-021](RED-021-many-layers-paged-whole-into-memory.md) | A sender's many-layers entry is paged and counted whole, with no stop once the work beats the node's, at 15 times the memory it sends | high | fixed |
| [RED-022](RED-022-parts-written-to-disk-without-free-space-check.md) | Parts of a claimed large entry are written to disk with no check of the free space, at the speed of the link | medium | fixed |
| [RED-023](RED-023-departed-senders-keep-their-progress-and-parts-files.md) | A sender that leaves after a yield keeps its progress, and its parts file on disk, until the node restarts | medium | fixed |
| [RED-024](RED-024-stalled-chase-throws-away-its-parts.md) | A timeout in the middle of a large entry deletes the parts already fetched, so the next chase starts that entry again from zero | medium | fixed |
| [RED-025](RED-025-lowering-layer-rate-maximum-stops-old-node-json-loading.md) | A node.json with a `layer_rate` that was valid before D127 no longer loads, so the node does not start | low | fixed |
| [RED-026](RED-026-departed-senders-keep-counted-layers-for-an-hour.md) | A sender that stalls and leaves keeps its counted layers in the node's memory for an hour, at about 7 times the bytes it sent | high | fixed |
| [RED-027](RED-027-connected-silent-sender-holds-the-disk-reserve.md) | A connected sender that stalls keeps its parts file, so the disk reserve stays taken and no other large entry can be fetched | medium | fixed |
| [RED-028](RED-028-learned-pace-pins-an-idle-asker-to-one-request-a-window.md) | Two unanswered requests at low load teach the asker a pace of one request a window for half an hour | medium | fixed |
| [RED-029](RED-029-trickler-claims-just-under-the-honest-size-and-keeps-the-reserve.md) | A sender that claims a large entry a little smaller than the honest one, and trickles it, still holds the disk reserve | medium | fixed |
| [RED-030](RED-030-follow-and-wallet-follow-write-node-json-behind-a-running-node.md) | `poo follow`, `poo unfollow` and `poo wallet follow` still write node.json without the data directory lock | low | fixed |
| [RED-031](RED-031-trickler-that-takes-one-mib-a-chase-holds-the-reserve.md) | A trickler that takes 1 MiB a chase and claims a little less than the honest entry is "moving", and no turn ever reaches it | medium | fixed |
| [RED-032](RED-032-fresh-senders-take-the-room-from-a-slow-file.md) | Each fresh sender ranks as moving and drops a slow file, so a stream of them restarts a slow link's fetch for good | medium | fixed |
| [RED-033](RED-033-slow-trickler-sending-a-little-more-starves-a-slow-honest-link.md) | A slow trickler that takes one part more than the honest link each chase wins every turn | low | fixed |
| [RED-034](RED-034-second-outage-while-probing-pins-the-pace-again.md) | A second pair of lost requests while the pace is being probed pins the asker to one request a window again | medium | fixed |
| [RED-035](RED-035-probing-flags-an-honest-multi-chunk-reply-as-false.md) | A fetch that probes at the disk reserve throws the first chunk of a reply away, so the second chunk is "false" and the honest sender is flagged | high | fixed |
| [RED-036](RED-036-second-outage-at-low-load-still-pins-the-pace.md) | The RED-034 fix compares with the first drops' load, so at low load a second outage still pins the pace at one request a window | medium | fixed |
| [RED-037](RED-037-fast-fresh-sender-chews-a-slow-file-and-probing-costs-a-reply.md) | A fast fresh sender cuts a slow file at its own line rate, byte for byte, and each probe costs the sender up to a parts reply | low | partly fixed |
| [RED-038](RED-038-poo-submit-takes-the-hex-as-an-argument-so-no-entry-over-64-kib.md) | `poo submit entry` takes the entry as one command-line argument, so an entry over about 64 KiB cannot be submitted, which the GUI can | low | fixed |
| [RED-039](RED-039-refused-save-after-a-submitted-payment-loses-the-record-and-says-run-it-again.md) | When `Save` is refused after a payment or claim was already submitted, the wallet has no record of it and the message says to run the command again | medium | fixed |
| [RED-040](RED-040-submit-entry-hex-sends-a-chunked-body-so-the-node-cannot-size-its-buffer.md) | `SubmitEntryHex` sends the body chunked, so the node never sees a Content-Length and ORANGE-025's sizing does not apply to `poo submit` | low | fixed |
| [RED-041](RED-041-wallet-save-is-not-synced-so-a-crash-can-lose-or-empty-the-wallet-file.md) | `Wallet.Save` writes and renames without a sync, so a crash can leave an empty or old wallet file after a payment was sent | medium | fixed |
| [RED-042](RED-042-hold-keeps-the-wallet-lock-across-node-calls-and-waiters-wait-forever.md) | `Hold` keeps the wallet lock across node calls and every other wallet command waits for it with no limit and no message | low | fixed |
| [RED-043](RED-043-a-default-import-takes-every-unset-setting-including-the-ones-that-switch-off-defences.md) | A default `policy import` takes every setting the node leaves unset, including the ones that switch off its defences or reach outside, and the plan shows them like any other line | medium | fixed |
| [RED-044](RED-044-merge-time-grows-with-the-square-of-the-series-in-a-bundle.md) | Importing a bundle takes time that grows with the square of its series, so a few MiB makes even a dry run take minutes | low | fixed |
| [RED-045](RED-045-a-node-bundle-applies-its-first-policies-then-fails-and-says-only-the-last.md) | A node bundle that fails on its second policy has already applied its first, and the error names only the second (and the CLI and endpoint size caps differ) | low | fixed |
| [RED-046](RED-046-a-followed-series-with-many-pools-makes-the-node-spend-time-and-memory-per-pool.md) | A series with very many pools, followed whole, makes the node spend time and memory for every pool at load, and a default import carries exactly that | medium | fixed in part |
| [RED-047](RED-047-an-empty-if-digest-turns-the-check-off.md) | `-if-digest ""` applies the file unchecked, so an unset shell variable turns the protection off | low | fixed |
| [RED-048](RED-048-the-defences-list-is-a-list-of-what-was-thought-of-so-other-settings-pass-unnamed.md) | `Defences` is a list of what was thought of, so other settings that blind or starve a node go unnamed under `-settings` | low | fixed |
| [RED-049](RED-049-the-lazy-pool-state-is-made-by-a-rejected-item-and-p2p-still-keeps-every-pool.md) | D152 made `node.Open` cheap for a huge series, but a rejected item still makes a pool's state and `p2p.New` still keeps and walks every pool | medium | fixed |
| [RED-050](RED-050-unfollowing-one-pool-of-a-large-whole-series-leaves-a-follow-the-node-will-not-start-with.md) | Taking one pool off a whole-series follow of more than 4096 pools writes every other pool out, and `p2p.New` then refuses to start | medium | fixed |
| [RED-051](RED-051-the-digest-binds-the-file-and-not-the-options-it-is-applied-with.md) | `-if-digest` ties an apply to the file previewed but not to the options, so the same digest applies with `-settings`, `-full` or `-replace` | low | fixed |
