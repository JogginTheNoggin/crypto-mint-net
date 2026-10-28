# BLUE-004: No IPv4-carrying IPv6 dial targets, and a bounded address view

| Field | Value |
| --- | --- |
| Fixes | RED-007 (low), RED-008 (medium) |
| Branch | `claude/project-thread-ggo8dt` |
| Base | `claude/m10-watch` @ `5c86670` (Source Round 2 Part C) |
| Decisions | D102, D103 |

## Summary

Two fixes from Source Round 2. A peer can no longer get past the
public-address check by naming a loopback or private IPv4 address in an
IPv6 transition form such as NAT64 (RED-007). One address lookup, which
the read-only view token can make, can no longer walk whole ledgers while
holding the engine's lock (RED-008): it is now paged like the ledger
view. No byte form, wire or consensus change.

## What changed

- **RED-007** (`pkg/p2p/addr.go`): `notPublic` gains `64:ff9b::/96`,
  `64:ff9b:1::/48`, `2002::/16`, `2001::/32`, `::/96` and
  `::ffff:0:0:0/96`, so both the learn-time and the dial-time checks
  refuse them.
- **RED-008** (`pkg/watch/query.go`): `AddressPage(ctx, …, below, limit)`
  walks at most 50×limit entries in all, at most limit items per pool,
  from height `below` down, sets `Address.Next` (pool, below) for each
  pool it did not finish, and stops on a done context. `AddressOf` calls
  it from the tips. `pkg/rpc`: `/v1/address` takes `below` and passes the
  request context; `Client.AddressPage`. `pkg/cli`: `poo address -below`.
- Docs: D102, D103, `docs/modules/rpc.md`, `watch.md`, `poo.md`.

## Tests added

- `pkg/p2p`: NAT64, local-use NAT64, 6to4, Teredo, IPv4-compatible and
  IPv4-translated cases in `TestPublicIP` and `TestLearnable`;
  `TestCandidateDialerRefusesEmbeddedIPv4` (the dial-time check refuses
  them, a public IPv6 address passes, and a listed address is not
  checked).
- `pkg/watch`: `TestAddressPageIsBounded`.
- `pkg/rpc`: an `AddressPage` case with `below`.

## How Grey can check it

```sh
go test -race -run 'PublicIP|Learnable|CandidateDialer' -v ./pkg/p2p
go test -race -run AddressPage -v ./pkg/watch
go test -race ./pkg/rpc ./pkg/cli
```
