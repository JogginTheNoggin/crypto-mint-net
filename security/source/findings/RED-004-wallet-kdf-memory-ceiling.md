# RED-004: A crafted wallet file can still force a ~4 GiB allocation on open

| Field | Value |
| --- | --- |
| Severity | low |
| Status | fixed |
| Found on | `claude/m10-watch` @ 2811f88 (the BLUE-001 merge) |
| Component | pkg/wallet/keystore.go (KDF.check), pkg/wallet/wallet.go (Open), pkg/cli/wallet.go (`-wallet`) |
| Surface | wallet / files |
| Fixed by | `claude/project-thread-ggo8dt` @ a733b18 (BLUE-002) |

## Summary

BLUE-001 added `KDF.check` to stop "a damaged or crafted file [from making]
opening it take hours or exhaust memory". The bounds it sets are a real
improvement, but the Argon2id memory ceiling is high enough that a crafted
wallet file can still exhaust memory on a modest host:

```go
// pkg/wallet/keystore.go
const (
    maxIterations  = 100_000_000
    maxArgonPasses = 100
    maxArgonMemory = 4 << 20 // KiB, 4 GiB
)

case KDFArgon2id:
    if k.Iterations < 1 || k.Iterations > maxArgonPasses || k.Threads < 1 ||
        k.Memory < 8*uint32(k.Threads) || k.Memory > maxArgonMemory {
        return errors.New("wallet: bad key derivation settings")
    }
```

A wallet file may legally name `memory = 4 GiB` (and `passes = 100`). The
`KDF` struct is read straight from the file's JSON, and `KDF.key` passes
`k.Memory` to `argon2.IDKey`, which allocates that much memory as a single
block. So opening such a file (any operation that unseals the seed, once a
passphrase is supplied) immediately tries to allocate ~4 GiB. On a machine
with less RAM than that — common for a node — the process OOMs or the host
starts swapping, before the passphrase is even checked. `passes = 100` over
4 GiB is also minutes of CPU, not the "hours" the bound rules out but still
far beyond a legitimate wallet's ~64 MiB / 3 passes.

`poon wallet -wallet <path>` opens whatever file the path names, so this
applies whenever a user opens a wallet file they did not create themselves
(one received, restored from a backup of unknown provenance, or placed by
another program).

## Reproduction

1. Create a valid wallet, then edit its JSON `kdf` block to
   `{"name":"argon2id","iterations":100,"memory":4194304,"threads":1,
   "salt":"…"}` (memory in KiB = 4 GiB). The settings pass `check`.
2. Run any `poon wallet` command that unseals the seed against that file with
   any passphrase.
3. The process attempts a single ~4 GiB allocation in `argon2.IDKey` before
   the passphrase is verified; on a host with less memory it OOMs.

A `pkg/wallet` test can assert that `KDF{Name: KDFArgon2id, Iterations: 100,
Memory: 4<<20, Threads: 1, Salt: …}` passes `check()` today — i.e. the
ceiling admits a 4 GiB cost — which the fix should lower.

## Impact

Low, and partly by design — the wallet file is normally the owner's own
(the threat model treats "someone with the wallet file" as able to read it,
not to attack the opener). The residual risk is a user opening a wallet file
from someone else: a single crafted field turns `poon wallet` into a local
OOM / long stall. No secret leaks and nothing is forged; it is a
denial-of-service against the person opening the file.

## Recommendation

- Lower `maxArgonMemory` to something a node host can always absorb (e.g.
  1 GiB), or make the ceiling a documented node-policy/CLI setting with a
  conservative default, so an unusually large but legitimate wallet can opt
  in while the default cannot OOM.
- Consider also bounding `passes × memory` (a cost product) rather than each
  axis alone, so `100 passes × 1 GiB` is not admissible either.
- Optionally, when opening a wallet whose KDF cost exceeds a soft threshold,
  require an explicit `--allow-expensive-kdf` style confirmation.

Test: a `pkg/wallet` test that `check()` rejects settings above the new
ceiling (and the cost product), and that the defaults still pass.

## Resolution

Fixed by BLUE-002 (decision D97). Argon2id memory is capped at 1 GiB and
passes times memory at 8 GiB; PBKDF2 at 10,000,000 iterations. A policy
setting and an "expensive KDF" confirmation were not added: the limits
are far above any setting a person would choose and below what a modest
host can absorb. Tests: `TestKDFLimits` and new cases in
`TestCraftedKDFSettingsRefused` (`pkg/wallet`).
