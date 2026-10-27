# RED-001: A peer can make the node dial arbitrary addresses (internal-scan / reflection primitive)

| Field | Value |
| --- | --- |
| Severity | medium |
| Status | fixed |
| Found on | `claude/m10-watch` @ 581476e |
| Component | pkg/p2p/discovery.go (handleFollow), pkg/p2p/host.go (dialCandidate), pkg/peers/peers.go (Book.Add/Pick/Range), pkg/wire/discovery.go (CheckAddr) |
| Surface | peer |
| Fixed by | `claude/project-thread-ggo8dt` @ a733b18 (BLUE-002) |

## Summary

Any connected peer — including an unverified, unlisted *inbound* connection
that has only completed the hello — can seed the node's peer book with an
address of the attacker's choosing, which the connection manager will later
dial. The only validation applied to the address is `wire.CheckAddr`, which
checks syntax (`host:port`, port in range) and nothing else: private,
loopback, link-local, reserved and arbitrary third-party addresses are all
accepted. A remote party therefore gets a primitive that makes the victim
node open unsolicited TCP connections to hosts the attacker names, behind
the victim's own firewall.

The injection point is the `Contact` field of a follow request:

```go
// pkg/p2p/discovery.go, handleFollow
full, parts := h.answer(m.Section)
if !full && len(parts) == 0 {
    return nil
}
if q.Contact.Reachable() && !h.isSelf(q.Contact.Addr) {
    // The asker asks only about what it follows: a candidate.
    h.book.Add(q.Contact.Addr, []wire.Section{m.Section}, peers.FromReply, time.Now())
}
```

`Book.Add` only runs `wire.CheckAddr` (plus the `MaxRecords`/`PerRange`
caps and the blocked-address refusal). When the node is short of peers for
the covered mint policy, `manage` picks book candidates and `dialCandidate`
dials them with no address-class check:

```go
// pkg/p2p/host.go, dialCandidate
d := net.Dialer{Timeout: 10 * time.Second, KeepAlive: 15 * time.Second}
c, err := d.DialContext(ctx, "tcp", addr)   // addr is attacker-chosen
```

`CheckAddr` (pkg/wire/discovery.go) does not reject non-global addresses:

```go
func CheckAddr(a string) error {
    host, port, err := net.SplitHostPort(a)
    ...
    if p, err := strconv.Atoi(port); err != nil || p < 1 || p > 65535 {
        return fmt.Errorf("address %q has a bad port", a)
    }
    return nil
}
```

The `PerRange` cap does not meaningfully limit loopback targets, because
`peers.Range` gives every loopback **address** its own range key:

```go
// pkg/peers/peers.go, Range
case ip.IsLoopback():
    return "loopback:" + addr   // per host:port, not per host
```

so `127.0.0.1:1`, `127.0.0.1:2`, … are each a distinct "range" and each
may hold `PerRange` records, bounded only by `MaxRecords` (default 8192).
That turns the primitive into an internal TCP port scanner.

## Reproduction

Steps (two nodes on loopback, the attacker driving a raw connection):

1. Start a victim node that follows some pool `P` and runs a p2p host with
   `ManageEvery` short and its outbound slots not yet full (the default
   state of a fresh node).
2. From the attacker, open a TCP connection to the victim, send a valid
   `wire.TypeHello`, then send a `wire.TypeFollowReq` for pool `P` whose
   body carries `Contact{Addr: "127.0.0.1:9"}` (or any target you want the
   victim to connect to), with a fresh nonce and a current timestamp so it
   passes `admit`.
3. The victim adds `127.0.0.1:9` to its book as a `FromReply` candidate.
4. Within `ManageEvery`, `manage` → `Book.Pick` → `dialCandidate` dials
   `127.0.0.1:9`. Observe the victim's outbound `connect()` to that target
   (e.g. a listener you place there logs an inbound TCP connection and the
   node's hello frame).

A failing unit test can be written in `pkg/p2p` with the existing
loopback-node test harness: register an extra `net.Listener` on an address
that is *not* a node, feed the victim a follow request whose `Contact`
names that listener, and assert the listener accepts a connection within a
manage cycle. Today that connection happens; after the fix it must not,
unless policy opts in to non-global targets.

## Impact

A remote, unauthenticated peer (only a completed hello is required; no peer
verification, no slot) can:

- **Probe the victim's internal network / localhost.** The victim will
  `connect()` to attacker-named private, loopback and link-local addresses
  (e.g. `169.254.169.254:80`, `127.0.0.1:<port>`, RFC 1918 hosts),
  revealing — through connect success/refused/timeout timing visible in the
  node log, book `Failures`, and `/v1/peers` book state — which internal
  services are reachable from the node. Loopback targets are effectively
  unbounded by `PerRange`.
- **Reflect connections at a third party.** Point many victims' books at one
  victim address to make them all dial it. The amplification per node is
  small (one TCP handshake plus a hello frame) and rate-limited by
  `ManageEvery` and the outbound slot count, so this is a nuisance rather
  than a flood, which is why the overall rating is medium rather than high.

Cost to the attacker: one TCP connection and one follow request per target
address; no proof-of-work, no value, no peer standing.

## Recommendation

1. In `wire.CheckAddr` (or a new `CheckDialable` used on the candidate path),
   reject addresses whose IP is loopback, unspecified, link-local, private,
   or otherwise not global unicast, unless node policy explicitly opts in
   (a `AllowPrivatePeers`/`AllowLoopbackPeers` flag, defaulting off; tests
   that dial loopback already construct peers directly and can set it).
   Resolve host names with care, or only accept literal IPs on the
   peer-supplied path.
2. Prefer not to treat a peer-supplied `Contact` as a dial candidate until
   it has been independently corroborated (seen in more than one peer's
   verified list, or reachable on a back-connection), so one peer cannot
   plant a target by itself.
3. Fix `peers.Range` so loopback collapses to a single range key (e.g.
   `"loopback"`), matching the intent of `PerRange`, so a flood of
   `127.0.0.1:N` entries cannot each claim their own range.

Tests: a `pkg/wire` test that `CheckAddr`/`CheckDialable` rejects a loopback
and an RFC 1918 literal by default; a `pkg/p2p` test that a follow request
carrying a non-global `Contact` does not cause an outbound dial; a
`pkg/peers` test that N loopback addresses count against one range.

## Resolution

Fixed by BLUE-002 (decision D96). Addresses learned from peers and
registries go into the book only if they are host names or public
unicast IPs, and the connection manager's dialer checks the resolved IP
of every attempt before connecting, which also covers host names that
resolve to loopback and records from an older peer file. Policy
`p2p.allow_local_peers` turns the checks off for local networks; listed
`peers` are always dialled. Recommendation 2 (corroborate a contact) was
not taken: a public address one peer names is an ordinary candidate,
verified when dialled. Recommendation 3 (collapse the loopback range) was
not taken either: with loopback refused from other parties, D45's
assumption holds again, and collapsing it would cap local test networks.
Tests: `TestPublicIP`, `TestLearnable`, `TestPeerCannotPlantLocalDialTarget`,
`TestConfiguredLocalPeerIsDialled` in `pkg/p2p`.
