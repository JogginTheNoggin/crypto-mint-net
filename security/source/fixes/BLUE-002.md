# BLUE-002: Public dial targets, single resends, KDF caps, no seed arguments

| Field | Value |
| --- | --- |
| Fixes | RED-001 (medium), RED-002 (low), RED-004 (low), RED-005 (medium); assesses RED-003 (info, won't fix) |
| Branch | `claude/project-thread-ggo8dt` |
| Base | `claude/m10-watch` @ `454cd3d` |
| Decisions | D96, D97, D98 |

## Summary

Four of Red's first five findings are fixed. A peer can no longer make
the node connect into its own network (RED-001). A missing-chunks
request can no longer make the node resend one chunk thousands of times
(RED-002). A wallet file can no longer make opening it allocate more
than 1 GiB (RED-004). `claim` and `pay` no longer take a seed as an
argument other users could read (RED-005). RED-003 is left as is, with
reasons in its file. No byte form, wire or consensus change.

## What changed

- **RED-001** (`pkg/p2p/addr.go`, `discovery.go`, `host.go`,
  `policy.go`, `pkg/nodepolicy`): every address learned from a peer or
  registry goes through `Host.learn`, which refuses literal IPs that are
  not public unicast. `candidateDialer` checks the resolved IP of every
  dial attempt of a book candidate, which also refuses host names that
  resolve locally and records from older peer files. Listed `peers` and
  the owner's `Connect` are not checked. Policy `p2p.allow_local_peers`
  (off) turns the checks off; the `pkg/p2p` test harness turns it on.
- **RED-002** (`pkg/p2p/discovery.go`, `handleMissing`): each distinct
  position resent once, bytes recorded as cost, a repeat counted with
  `tracking.Repeat`.
- **RED-004** (`pkg/wallet/keystore.go`): Argon2id at most 1 GiB and 8 GiB
  in passes times memory, PBKDF2 at most 10,000,000; `wallet.CheckKDF`.
- **RED-005** (`pkg/cli/node.go`, `main.go`): `seedFlags` reads
  `POON_SEED` or `-seed-file` (`-` for standard input); `-seed` is
  refused for `claim` and `pay`. `keygen` gains `-seed-file` and keeps
  `-seed` for test vectors (the GUI's tests use it).
- Docs: D96 to D98, `docs/running-a-node.md` (local networks need
  `allow_local_peers`; seeds from `POON_SEED`), `docs/modules/poon.md`,
  `nodepolicy.md`, `peers.md`. Finding files carry a Resolution section.

## Tests added

- `pkg/p2p`: `TestPublicIP`, `TestLearnable`,
  `TestPeerCannotPlantLocalDialTarget` (a follow request contact, a
  planted loopback record and `localhost:port` are never dialled; with
  local peers allowed the same contact is), `TestConfiguredLocalPeerIsDialled`,
  `TestMissingChunksResentOnce`.
- `pkg/wallet`: `TestKDFLimits` and new `TestCraftedKDFSettingsRefused`
  cases.
- `pkg/cli`: `TestSeedNotOnCommandLine`; existing claim and pay tests now
  pass the seed in a file.

## How Grey can check it

```sh
go test -race ./pkg/p2p ./pkg/wallet ./pkg/cli
go test -run 'PublicIP|Learnable|PlantLocal|ConfiguredLocal|MissingChunksResentOnce' -v ./pkg/p2p
```

By hand: two `poon node`s on one machine find each other through
discovery only with `"allow_local_peers": true`; a listed `-peer
127.0.0.1:…` connects either way. `poon claim -seed …` fails and names
`POON_SEED`.

For the GUI teams: `claim` and `pay` no longer accept `-seed`. `keygen
-seed` still works.
