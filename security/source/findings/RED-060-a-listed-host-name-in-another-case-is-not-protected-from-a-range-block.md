# RED-060: A listed host name in another case is not protected from a range block

| Field | Value |
| --- | --- |
| Severity | Low |
| Status | fixed |
| Found | Source Round 21 Part B, on `main` @ `8662766` (D157) |
| Area | `pkg/peerport/import.go` (`Import`, `listedRange`), `pkg/peers` (`Range`) |
| For Black | No consensus rule is touched. |
| Fixed by | `claude/project-thread-ggo8dt` @ 282571b (BLUE-022) |

## What is wrong

D157 and the `peerport` header say a file "never blocks an address the
node policy lists (p2p.peers)". For a range block `Import` checks
`listedRange[peers.Range(listed)]`, but `peers.Range` of a host name is
`"host:" + host` with the host as written, and a file's range is the
lower case the exporter wrote (`tracking` keys host names in lower
case). A listed `Boot.Example.com:8646` has the range
`host:Boot.Example.com`, so a file's `host:boot.example.com` is not
"a range holding a listed address".

Also:

- A listed host name is never matched to the IP it resolves to, so an
  address block or a `v4:` range block that covers it is not seen.
- A trailing dot (`boot.example.com.`) or a port alone does not match the
  listed key either (these two do no harm: the tracker does not key a
  block under them).

## How I checked it

`peerport.Import` with `Options{Blocks: true, Ranges: true}`, `Listed:
["Boot.Example.com:8646", "[2001:db8::1]:8646", "198.51.100.7:8646"]`, a
real tracker:

```
skipped 198.51.100.7:9999 (address block): this node policy lists it
skipped range v4:198.51: holds an address this node policy lists
BLOCKED range host:boot.example.com until ...        <- not skipped
boot.example.com:8646 blocked now: true
```

IP literals and the mapped IPv6 form are all protected.

## What could happen

An owner who wrote a bootstrap name with capitals imports a shared block
list that carries `host:` ranges, and the node refuses its own listed
peer, which is the thing the rule is there to prevent. A listed host name
whose IP a list blocks the same way.

## Suggested fix (Blue's call)

Compare host names in lower case without a trailing dot on both sides
(`Range` itself can lower-case the host), and say in the plan that a
listed host name is checked by name only.

## Not tested

Resolving the listed names to compare IPs (the preview should not
resolve names from a file).

## Resolution

Fixed (D158).

- `peers.Range` of a host name is now the name in lower case without the
  root's trailing dot, so `Boot.Example.com.:8646` and a file's
  `host:boot.example.com` are one range. The tracker already keyed host
  names in lower case.
- The plan notes that "a listed host name is protected by its name only:
  a block on the address it resolves to is not seen here". The preview
  resolves nothing, as with D156's fit check.
- Test: `TestListedHostNameInCapitalsIsProtected` (pkg/peerport) and
  `TestRangeOfAHostName` (pkg/peers).
