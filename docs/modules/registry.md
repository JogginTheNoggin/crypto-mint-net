# pkg/registry

## Summary

Reads address registries: public lists of node addresses for mint
policies, where a node with no connections finds its first candidates (the
design's "Address Registries"). The design leaves the format open; this
package defines a provisional one (decision D44). Everything read is a
candidate only: the node checks each address with a follow request before
peering with it.

## Format

```json
{
  "format": 1,
  "nodes": [
    {"addr": "203.0.113.5:7337", "follows": ["<issuer hex>/<series hex>/*/*"]}
  ]
}
```

`follows` uses the text form of `wire.Section`: issuer, series id, pool id
and serial, hashes in full hex, `*` for a wildcard from the right. Entries
with a bad address or no readable section are skipped. A document is at
most 1 MiB.

## API

- `Fetch(ctx, client, location)`: an http or https URL, or a file path
  (with or without `file://`). The client decides where it may connect:
  the node's (`p2p`, `registryClient`) refuses an address that is not
  public, after name resolution and on every redirect, unless the policy
  allows local peers (RED-043, D150).
- `Parse(data)`, `Encode(entries)` (for publishing a registry),
  `ErrFormat`.

## Not yet decided

The design leaves open whether registries are signed by their maintainer
and how the default list of registry locations is updated. There is no
default list yet: there is no project registry until release (milestone
8), so a node finds first peers only from its node policy's `peers` and
`registries`.
