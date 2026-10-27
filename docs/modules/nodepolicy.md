# pkg/nodepolicy

## Summary

The node policy: every choice a full node makes for itself, as a
`node.json` file. It lists the known series (series information comes from
outside the network), which issuers are accepted, what the node follows (an
issuer, a series, some pools or some mints), K and the other limits, write
durability, the RPC address and the p2p settings. A `Manager` keeps several node policies side
by side with the design's save, archive and delete operations (decision
D25). Nothing here changes what is valid.

## node.json

```json
{
  "format": 1,
  "series": [{"params": {"hash": 1, "mints": 8, "...": "..."}, "issuer_witness": "hex, optional"}],
  "issuers": {"any": true, "accept": ["issuer address hex"]},
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
    "chunk_size": 1048576, "entry_limit": 512, "hash_limit": 256, "layer_limit": 65536,
    "no_notices": false,
    "check_seconds": 60,
    "registries": ["https://registry.example/poo.json"],
    "peers_per_policy": 4,
    "max_outbound": 8,
    "no_relay": false,
    "hop_limit": 8,
    "request_rate": 120,
    "ping_seconds": 120
  }
}
```

Every field but `format` may be left out; the defaults are the provisional
ones in [decisions.md](../decisions.md) (D12, D22, D24, D26). `k: -1` keeps
every branch. `rpc.listen: "off"` disables the interface. `p2p.listen:
"off"` accepts no connections (listed peers are still dialled);
`check_seconds: -1` turns off regular checks and `ping_seconds: -1` pings.
Without `advertise` the node is never offered to others as a contact. The p2p defaults are in
`p2p.DefaultConfig` (decision D37).

## API

- `Policy`, `Default`, `Validate`, `Load`, `Save` (atomic),
  `LedgerConfig`, `ListenAddr`, `P2PListenAddr` (`p2p.FromPolicy` turns
  the p2p section into host settings).
- `Accepted()` builds each listed series and applies the issuer rule,
  returning accepted series (with the verified issuer) and refused ones with
  reasons. One bad series never stops a node.
- `EncodeWitness` / `DecodeWitness`: the hex form of an issuer witness.
- `Manager{Root}`: `Save`, `Load`, `List`, `Current`, `Use`, `Archive`,
  `Unarchive(name, fresh)`, `Delete`, `Dir`.
- `Lock(dir)`: an exclusive lock on a data directory, released by the
  operating system if the process dies (advisory only on non-Unix systems).

## Extending

A new setting is a new optional field with a documented default. Bump
`Format` only for a change that old nodes would misread.
