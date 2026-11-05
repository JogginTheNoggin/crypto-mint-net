# pkg/portmap

## Summary

Asks the router a node sits behind to forward an inbound TCP port, for an
owner who turned on `p2p.port_map` (D182). It speaks PCP (RFC 6887)
first, NAT-PMP (RFC 6886) second and UPnP IGD third. It knows nothing of
nodes. Package p2p decides when to map, confirms the address and
advertises it (see [p2p.md](p2p.md#port-mapping-d182)).

## API

| Name | What it does |
| --- | --- |
| `Client{Gateway, Timeout}` | maps through one gateway; a zero `Gateway` means `DefaultGateway()` |
| `Client.Map(ctx, method, internal, lifetime, prev)` | asks for a mapping by `auto`, `pcp`, `natpmp` or `upnp`; `prev` renews one (same method, PCP nonce, external port and UPnP control URL) |
| `Client.Unmap(ctx, m)` | removes a mapping |
| `Mapping` | `Method`, `External` (the router's address and port), `Internal`, `Lifetime` (0: until removed) |
| `DefaultGateway()` | the IPv4 default route's gateway from `/proc/net/route` (Linux); elsewhere `ErrNoGateway` |
| `ValidMethod`, `Methods` | the setting's values |

## Rules it keeps

- **Only the gateway.**
  - PCP and NAT-PMP use a UDP socket connected to the gateway's port,
    so no other host can answer.
  - A PCP reply must carry the request's random nonce.
  - SSDP answers count only from the gateway's address.
  - The description URL, `URLBase` and the control URL must be plain
    `http` on the gateway's IP literal. A host name, `https`, a user in
    the URL or any other address is refused before a request is made.
  - The SOAP service type may hold only letters, digits, `:`, `-` and
    `.`.
- **Small and quick.**
  - Every reply is capped at `MaxReply` (64 KiB); a datagram at 2 KiB.
  - Each method waits at most `Timeout` (3 s by default), resending UDP
    at 250 ms, then 500 ms, then 1 s.
  - A cancelled context ends the wait at once.
  - HTTP uses no proxy, follows no redirect, keeps no connection open
    and caps headers at 16 KiB.
- **A claim, not a fact.** `Mapping.External` is what the router says.
  The caller must confirm it before telling anyone.
- **Permanent leases.** A UPnP router that answers error 725
  (OnlyPermanentLeasesSupported) is asked again with lease 0. The mapping
  then lasts until `Unmap`.

## Tests

`portmap_test.go` runs fake gateways on 127.0.0.1:

- PCP: map, renew (same nonce and port) and unmap; a reply with another
  nonce; a refusal.
- NAT-PMP: a NAT-PMP-only gateway (the fallback).
- Waits: a silent gateway (resends, then gives up in time), a cancel, and
  short replies.
- UPnP: map, renew and unmap; permanent-only leases.
- Eleven ways a UPnP gateway can point elsewhere or misbehave, each
  refused before any SOAP call.
- The route table parser.

## Extending

A new method is a pair of functions, `xMap` and `xUnmap`, added to
`Map`'s order and `Unmap`'s switch, plus a name in `Methods`. It must keep
the rules above: speak only to the gateway, cap every reply, and wait
briefly.
