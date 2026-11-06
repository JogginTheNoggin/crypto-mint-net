# Security review (release 0.1.0)

## Summary

This is the milestone 8 review of the node before its first release: what
an attacker can reach, what was checked, what was found and fixed, and what
remains open. The node's attack surface is its TCP port (any peer can send
it bytes), its address registries, and, on the owner's machine, the local
RPC and the files in the data directory and wallet. The review found and
fixed four things:

- Inbound connections could fill every peer slot and shut out the peers
  the node chooses, an eclipse. Inbound now has its own cap (D75).
- The Go toolchain (1.24.7) was out of support, with 37 reachable standard
  library vulnerabilities. The module now uses a supported, patched one,
  and `govulncheck` reports none (D77).
- `staticcheck` found dead code and one test that checked the same thing
  twice. They are removed and fixed.
- The full simulation re-run found that a node catching up after a
  partition could prune the heavier ledger entry by entry as it arrived,
  and stay on the lighter one for good. The newest entry is now kept while
  it is a leaf (D79).

Everything else checked held: decoders are bounded and fuzzed, amounts
cannot overflow, signatures are strict, secrets stay in owner-only files,
and the RPC needs a token and listens on loopback. The open items are
mostly the design's own open questions: the economics of pre-grinding
(measured in [simulation.md](simulation.md)), which receivers handle with
their threshold, and the provisional peer defaults. Nobody outside the
project has reviewed the code yet. That is the most important next step
before real value depends on it.

## Threat model

| Who | Can | Must not be able to |
| --- | --- | --- |
| Any peer | Connect, send any bytes, lie, withhold, flood | Crash the node, use unbounded memory or CPU, make it accept an invalid transaction, entry or layer, cut it off from honest peers |
| A builder with work | Grind layers, build rival entries | Spend what it does not own; reverse a payment without out-working the network on that pool (the design's security assumption) |
| A registry operator | Serve any document | Do more than suggest addresses; the node verifies every peer itself (D40) |
| A local user without the token | Reach loopback ports | Use the RPC |
| Someone with the wallet file | Read it | Learn the seed without the passphrase |

Out of scope: an attacker with the owner's account on the machine, and a
compromised Go toolchain. Release archives are reproducible (D78) so the
second can at least be detected.

## What was checked

### Untrusted bytes

- **Frames** are capped at 16 MiB before anything is allocated
  (`wire.ReadFrame`), and the hello must arrive within its timeout. An
  entry larger than a frame comes in parts on request; what is held grows
  only with the bytes that arrive, and the item is decoded and its hash
  checked once whole (D117). No item count is capped (C1, C2): memory
  stays in proportion to the bytes received.
- **Lengths and counts** inside every object go through
  `encoding.Reader.Count` and `VarBytes`, which refuse a count larger than
  the remaining bytes could hold. A small message cannot make the node
  allocate a large list.
- **Every decoder is fuzzed** for panics and for a single encoding per
  value (D73). CI runs each target on every change.
- **Amounts** are summed with overflow checks (`bits.Add64`) on both the
  output and input sides of a transaction.
- **Weights** are arbitrary precision (D10), and strength is at most 256.
- **Signatures** of every algorithm have one accepted encoding: Ed25519
  and Schnorr parse strictly, and ECDSA refuses the upper-half s.
  `FuzzVerify` gives every verifier arbitrary keys and signatures.
  Threshold witnesses need exactly K signatures
  in strictly increasing key order, so a witness cannot repeat a key or
  cost more than K checks. A k-of-policies witness checks each listed
  policy at most once, nested at most 4 deep. Every primitive is listed in
  [cryptography.md](cryptography.md).
- **Ids** do not cover witnesses, so a re-signed transaction keeps its id
  and cannot be used to make two entries look different.

### Resources a peer can use

- Per-peer send queues are bounded, and a peer that falls behind is
  disconnected. Writes have a deadline.
- The pending pool, the seen list, the inbox queues and discovery requests
  per minute are all capped (D12, D62).
- Bad data costs a peer flags, kept by its address, and enough flags
  block it (D50, D55).
- **Found:** `MaxPeers` capped inbound and outbound connections together,
  so 32 inbound connections that answer the follow request left no room
  to dial out. Fixed by D75: inbound is capped at `MaxPeers` less
  `MaxOutbound` by default. `TestInboundCannotFillOutboundSlots` pins it.

### The owner's machine

- **RPC:** listens on loopback unless the policy sets `allow_remote`
  (D26). Every request needs the bearer token, compared in constant time.
  The token file is created with mode 0600. Bodies are capped, and the
  server has header, read and idle timeouts. A web page cannot send the
  `Authorization` header to it without the token.
- **Files:** the data directory, policies, peer book, tracking file, logs
  and wallet are written with mode 0600 in 0700 directories, through a
  temporary file and a rename. The only 0644 files are the payment
  requests and unsigned transactions `poon wallet` writes when asked, which
  hold nothing secret.
- **Wallet:** the seed is sealed with AES-256-GCM under a key from
  Argon2id (64 MiB, 3 passes, 16-byte random salt, random nonce; D95,
  older wallets PBKDF2-SHA256 at 600,000 iterations) and bound to its use
  with associated data (D64). The passphrase comes from an environment variable, a file or standard input, never a flag.
- **View token and view-only mode (D100, D101):** `rpc.view-token` (mode
  0600) opens `GET` and `HEAD` only; `poon -view-only` and
  `POON_VIEW_ONLY=1` use it and refuse every command that writes. A typed
  passphrase is read with echo off.
- **Views and the watch stream (M10):** behind the same token. Every
  view is bounded per request (D94). A watcher that reads slowly loses
  events, counted in a `missed` line, and never holds up the node (D91).
  `POST /v1/follow` validates the changed policy before saving it, and a
  reload reopens the node through the same checks as a start-up (D92).
- **Registries:** fetched with a 15-second timeout and read up to a size
  limit. An entry is only a suggestion (D40, D44).
- **Storage:** nothing on disk is trusted. The log is replayed through the
  engine on start-up, and a corrupt tail is cut (D21).

### Tools

| Tool | Result |
| --- | --- |
| `go vet ./...` | Clean |
| `staticcheck ./...` | 6 findings (dead code, a duplicated test condition, a needless guard), all fixed |
| `govulncheck ./...` | 37 reachable standard library vulnerabilities on Go 1.24.7; none on Go 1.27.3 (D77) |
| `go test -race ./...` | Clean |
| Fuzzing | Every decoder, in CI on every change, and 2 minutes each before release: no failures |
| Simulation | Every scenario at 16 nodes, and load, partitions and attacks at 24 to 128 nodes: one bug found and fixed (D79) |

## Open items

- **External review.** Everything here was checked by the people who wrote
  the code. Before the network carries real value, the consensus packages
  (`tx`, `entry`, `layer`, `state`, `ledger`) and `wire` should be reviewed
  by someone else.
- **Pre-grinding** is the design's accepted trade-off: an attacker with a
  head start can reverse a payment whose receiver waited for too little
  weight. [simulation.md](simulation.md) measures it. Receivers choose a
  threshold for the value at stake.
- **Peer defaults** (D62) are provisional. The simulator does not model
  ranking, so they have not been measured under attack.
- **Transport privacy.** Connections are plain TCP (D30). Peers see what a
  node follows and sends. Nothing secret crosses the wire, but an observer
  can link a node to its payments.
- **Dependencies (D83).** Milestone 9 added BLAKE2b, BLAKE3 and
  secp256k1 from four maintained modules outside the standard library.
  They are pinned in `go.sum` and covered by `govulncheck`, which reports
  one module-level advisory, for `x/crypto/openpgp`, a package the node
  does not import. An external review should include how they are used.
- **Timelocks.** The k-of-policies type can express the claim side of an
  atomic swap but not its refund, since consensus has no clock (D82).
- **Key derivation.** Closed by D95: new wallets seal the seed with
  Argon2id (64 MiB, 3 passes), which resists guessing on GPUs far better
  than PBKDF2, and `poon wallet passwd` upgrades older wallets.

## Reporting

See [SECURITY.md](../SECURITY.md).
