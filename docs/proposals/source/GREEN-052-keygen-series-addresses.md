# GREEN-052: keygen gives the key's address in the node's own series

| Field | Value |
| --- | --- |
| Status | proposed (prototyped) |
| Based on | `main` @ `1a1c729` |
| Area | `pkg/cli` (`keygen`, `followedAddresses`, `cryptoCmd`), `docs/modules/poon.md` |
| Effort | small |
| Consensus | none. Output only: `keygen` adds fields, and `crypto` gains `-o table`. Addresses, the reserve series and every rule are unchanged |
| Decided by | Grey (Black may skip it) |

## Summary

D170 made the built-in reserve series what `poon init` follows by default.
Its hash is SHA-512/256. `poon keygen` still prints `address` under
`-hash sha256` by default.

Green followed a new node's path:

1. Ran `poon init`, then `poon keygen -alg slh-dsa-sha2-256s -seed-file seed`.
2. Claimed mint 40 with the same key.

The claim's output address was `122af1…`, but keygen's `address` was
`50520a…`. Both are correct. `address` is the key's address in a SHA-256
series, and the reserve series hashes the policy with SHA-512/256.

A newcomer reads keygen's `address` as "my address" and gives it to a
payer. A payment there is to an address no key of theirs can spend in the
reserve series. Nothing warned them. `poon wallet address` gets it right,
because it builds the address from the series, but `keygen` is what
`running-a-node.md` and the GUI's Make key form use first.

Two other things Green checked:

- **`poon crypto` printed JSON only.** It now lists `max_signatures_per_witness`
  (D170), the number a person compares across algorithms, which reads best
  as a table.
- **The rest of D170 worked as documented.**
  - `poon init` follows the reserve and prints its full id.
  - A claim signed with SLH-DSA-SHA2-256s took 0.8 s and was accepted under
    the 128 KiB ceiling.
  - `poon crypto` shows SLH-DSA-256s with 4 signatures per witness.

## Idea

- **`series_addresses`.** `poon keygen` reads the current node policy (it
  now takes `-root`, `-policy` and `-dir`, like other commands). When the
  policy lists series, keygen adds one entry per series:
  - `series`: the series id;
  - `builtin`: `"reserve"` when the full id is the reserve's;
  - `hash`: the series' hash;
  - `allowed`: whether the series lists the key's algorithm;
  - `address`: the key's address in that series.
- **`note`.** When `address` (from `-hash`) is the key's address in none of
  those series, a note says "address is for a sha256 series; this node's
  series use another hash, so give a payer the address in
  series_addresses (or pass -hash)".
- **No node policy, no change.** With no node policy, or one that cannot be
  read, keygen prints exactly what it printed before. The existing fields,
  `address_sha256` included, are unchanged, so scripts, vectors and the GUI
  read them as before.
- **`poon crypto -o table`.** It prints hashes, key algorithms and policy
  types as tables, through the shared `addOutput`.

## How it was tested

- **`TestKeygenSeriesAddresses`** checks these cases:
  - Under the reserve, `series_addresses` holds one entry, builtin
    `reserve`, hash `sha512-256` and allowed. Its address equals
    `keygen -hash sha512-256`, the plain `address` differs, and there is a
    note.
  - An ECDSA P-256 key, which the reserve does not list, is marked
    `allowed: false`.
  - `-hash sha512-256` gives no note.
  - No node policy gives neither field.
- **`TestCryptoTable`** checks the table and refuses `-o yaml`.
- Both fail on `main`.
- **Checked by hand:** mint 40, claimed with the SLH-DSA-256s key on the
  reserve series, is held at the address keygen now lists under
  `series_addresses`.
- **Checks run locally:** gofmt, go vet, staticcheck,
  `go test -race ./...`, and the nine GUI e2e scripts. Parity passes: the
  new `-root`, `-policy`, `-dir`, `-o` and `-wide` flags are already in its
  `EVERYWHERE` list.

## Ideas for later

- `running-a-node.md`'s first-key step could say to read
  `series_addresses`, or to use `poon wallet address`, which is
  series-aware.
- The GUI's Make key form could show `series_addresses` beside the key.
  That is GUI Grey's call.
- `keygen`'s `-hash` could default to the followed series' hash when they
  all share one. That changes what `address` means on such a node, so it is
  left as a question for Grey rather than built.
