# BLUE-001: Seal new wallets with Argon2id

| Field | Value |
| --- | --- |
| Fixes | The "Key derivation" open item of [docs/security.md](../../../docs/security.md) (M8 review); no Red finding yet |
| Branch | `claude/project-thread-ggo8dt` |
| Base | `claude/m10-watch` @ `615f0bb` |
| Decision | D95 |

## Summary

The wallet seed was sealed under a PBKDF2-SHA256 key. PBKDF2 costs an
attacker who has the wallet file only computation, which GPUs make cheap.
New wallets now use Argon2id (RFC 9106: 3 passes, 64 MiB, 4 lanes), which
also costs memory for every guess. Old PBKDF2 wallets still open, and
`poo wallet passwd` moves them to Argon2id. While here, opening now
checks the derivation settings in the file against fixed limits, so a
damaged or crafted wallet file cannot make an unlock take gigabytes of
memory or hours of work.

## What changed

- `pkg/wallet/keystore.go`: `KDF` gains `memory` and `threads`; names
  `argon2id` and `pbkdf2-sha256`; `newKDF` builds settings with defaults;
  `check` enforces limits (Argon2id at most 4 GiB and 100 passes, at least
  8 KiB per lane; PBKDF2 1 to 100,000,000 iterations, no Argon2 fields).
- `pkg/wallet/wallet.go`: `Options.KDF` and `Options.Memory`;
  `Options.Iterations` is now the work factor of the chosen derivation.
  `Rekey` keeps Argon2id settings and upgrades PBKDF2 to Argon2id.
- Docs: D95 in `docs/decisions.md`, `docs/security.md` (open item closed),
  `docs/cryptography.md`, `docs/modules/wallet.md`.
- No byte form, wire or consensus change. Uses `golang.org/x/crypto`,
  already a dependency (D83); `go.mod` is unchanged.

## Tests added

In `pkg/wallet/keystore_test.go`:

- `TestNewWalletUsesArgon2id`: default options write Argon2id at the
  RFC 9106 settings, and the wallet opens with the right passphrase only.
- `TestPBKDF2WalletOpensAndRekeyUpgrades`: a PBKDF2 wallet opens, and a
  new passphrase moves it to Argon2id with the same seed.
- `TestRekeyKeepsArgon2idSettings`.
- `TestUnknownKDFRefused`: no file is written.
- `TestCraftedKDFSettingsRefused`: nine crafted settings (huge memory,
  huge passes or iterations, no lanes, too little memory, Argon2 fields on
  PBKDF2, unknown name, short salt) each fail with a settings error, not a
  passphrase error, within a second.

## How Grey can check it

```sh
go test -race ./pkg/wallet ./pkg/cli
go test -run 'Argon2id|PBKDF2|KDF' -v ./pkg/wallet
```

A wallet made by `poo wallet create` now shows `"name": "argon2id"` with
`"memory": 65536` in its `kdf` block.
