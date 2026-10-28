# RED-007: Public-address filter misses IPv4-embedding IPv6 transition ranges (NAT64 / 6to4 / Teredo)

| Field | Value |
| --- | --- |
| Severity | low |
| Status | fixed |
| Found on | `claude/m10-watch` @ 5ea6828 (after BLUE-002 / D96) |
| Component | pkg/p2p/addr.go (publicIP, candidateDialer, learnable) |
| Surface | peer |
| Fixed by | `claude/project-thread-ggo8dt` @ 78cea9c (BLUE-004) |

## Summary

BLUE-002 (the RED-001 fix, D96) is a good fix: `candidateDialer` validates
the **resolved** address at connect time through `ControlContext`, which
closes the literal-address and DNS-rebinding cases, and `publicIP` unmaps
IPv4-mapped IPv6 (`::ffff:a.b.c.d`) so that form cannot smuggle a private
IPv4 past the check.

It still misses the IPv6 transition addresses that embed an IPv4 address in
a non-mapped form. `publicIP` only special-cases `Unmap()` (IPv4-mapped) and
a fixed `notPublic` prefix list; that list does not include:

- **NAT64** `64:ff9b::/96` (RFC 6052) — the low 32 bits are an IPv4 address.
  `64:ff9b::7f00:1` embeds `127.0.0.1`.
- **6to4** `2002::/16` (RFC 3056) — bits 16–48 are an IPv4 address.
  `2002:7f00:1::` embeds `127.0.0.1`.
- **Teredo** `2001:0::/32` (RFC 4380) — the last 32 bits are an (obfuscated)
  IPv4 address.

For all three, `netip.Addr` reports `IsGlobalUnicast() == true` and
`IsPrivate() == false`, and none of the `notPublic` prefixes match, so:

```go
func publicIP(ip netip.Addr) bool {
    ip = ip.Unmap()                                   // only ::ffff:/96
    if !ip.IsValid() || !ip.IsGlobalUnicast() || ip.IsPrivate() {
        return false
    }
    for _, p := range notPublic { if p.Contains(ip) { return false } }
    return true                                       // 64:ff9b::7f00:1 → true
}
```

returns `true` for `64:ff9b::7f00:1`, `2002:7f00:1::`, etc. The
`ControlContext` guard calls the same `publicIP`, so a hostname that resolves
to one of these also passes.

## Reproduction

1. On a node whose network has NAT64/DNS64 (common on IPv6-only mobile and
   some cloud networks) or 6to4/Teredo configured, and with
   `AllowLocalPeers` off (the default).
2. As a connected peer, send a follow request whose `Contact` is
   `[64:ff9b::7f00:1]:<port>` (or a hostname resolving there). `learnable`
   accepts it (public per `publicIP`) and it enters the book as a candidate.
3. When the manager dials it, `candidateDialer`'s `ControlContext` checks the
   resolved `64:ff9b::7f00:1` with `publicIP`, which returns true, so the
   dial proceeds. The host's NAT64 translator rewrites it to `127.0.0.1`,
   and the node connects to its own loopback — the exact internal-reach
   RED-001/D96 set out to prevent.

A `pkg/p2p` unit test that fails today: assert `publicIP(netip.MustParseAddr(
"64:ff9b::7f00:1"))` is `false` (and the same for `2002:7f00:1::` and a
Teredo address embedding a private v4). Today they return `true`.

## Impact

Low. It is a residual of the RED-001 fix and only bites on a host where a
v6→v4 transition mechanism is actually deployed (NAT64 most plausibly on
IPv6-only/cloud networks); on a host without one, these addresses simply
fail to route. Where one is present, a remote peer regains the RED-001
internal-reach / loopback-probe primitive against the node's own network,
bypassing D96. No remote code exec or value theft, so low rather than
medium, but it reopens a fixed finding under a realistic deployment.

## Recommendation

In `publicIP`, treat the IPv4-embedding transition ranges as their embedded
IPv4 and re-check that, or refuse them outright unless `AllowLocalPeers`:

- `64:ff9b::/96` and the configurable NAT64 well-known/custom prefixes:
  extract the trailing 32 bits as an IPv4 and return `publicIP(thatV4)`.
- `2002::/16` (6to4): extract bytes 2–5 as the IPv4 and re-check.
- `2001:0::/32` (Teredo): extract and de-obfuscate the embedded client IPv4
  and re-check; simplest is to add `2001::/32` to `notPublic`.
- Simplest conservative fix: add `64:ff9b::/96`, `2002::/16` and
  `2001::/32` to `notPublic` so the node never dials a transition address
  learned from a peer (the owner can still list one explicitly, which
  bypasses the check via `h.configured`).

Test: extend the `pkg/p2p` address test with the three forms above (each
embedding a private/loopback v4) and assert `publicIP` and `learnable`
reject them, while a genuine global v6 and a `configured` address still
pass.

## Resolution

Fixed by BLUE-004 (decision D102), with the conservative option: the
public-address check refuses `64:ff9b::/96`, `64:ff9b:1::/48`,
`2002::/16`, `2001::/32`, `::/96` and `::ffff:0:0:0/96` outright, at
learn time and at dial time. Decoding the embedded IPv4 was not done:
Teredo obfuscates it, a network may use its own NAT64 prefix, and a peer
reachable this way has a native IPv4 address to give. Listed `peers` and
`allow_local_peers` still bypass it. Tests: new cases in `TestPublicIP`
and `TestLearnable`, and `TestCandidateDialerRefusesEmbeddedIPv4`
(`pkg/p2p`).
