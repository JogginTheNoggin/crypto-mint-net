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
| [RED-009](RED-009-preverify-verifies-what-checks-would-reject.md) | Pre-lock signature verification runs on items the cheap checks would reject, does not stop at the first bad signature, and never remembers a failure | medium | open |
| [RED-010](RED-010-failed-fsync-retried-then-reported-ok.md) | A failed fsync of the store is retried and a later waiter is told the record is on disk | low | open |
