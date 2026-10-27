# RED-005: `poo claim`, `poo pay` and `poo keygen` take the raw seed on the command line

| Field | Value |
| --- | --- |
| Severity | medium |
| Status | open |
| Found on | `claude/m10-watch` @ 2811f88 |
| Component | pkg/cli/node.go (claimCmd, payCmd), pkg/cli/main.go (keygen), pkg/cli/node.go (signerFrom) |
| Surface | CLI / files |
| Fixed by | — |

## Summary

`poo claim`, `poo pay` and `poo keygen` read a 32-byte wallet seed — the one
secret that controls every key derived from it — as a plain command-line
flag:

```go
// pkg/cli/node.go
seed := fs.String("seed", "", "the claiming key's seed in hex")   // claimCmd
seed := fs.String("seed", "", "the paying key's seed in hex")     // payCmd
...
signer, err := signerFrom(*seed)   // the raw hex seed, straight off argv
```

```go
// pkg/cli/main.go
seedHex := fs.String("seed", "", "32-byte seed in hex (random if empty)")  // keygen
```

There is no stdin / environment / file alternative on these commands — the
seed must be put on the command line to use them. This is in contrast to
`poo wallet`, which was deliberately built to keep a *sealed* seed and to
read its passphrase only from `$POO_PASSPHRASE`, `-passphrase-file` or
standard input (pkg/cli/wallet.go). The claim/pay/keygen path bypasses all
of that and handles the unsealed seed in the clear.

A secret on a command line leaks several ways:

- **Shell history.** `~/.bash_history` / `~/.zsh_history` persist the full
  command, seed included, to disk.
- **Process listing.** While the command runs, any local user can read the
  full argument vector from `ps -ef`, `/proc/<pid>/cmdline`, or process
  accounting. On a shared or multi-tenant host this discloses the seed to
  other users.
- **The GUI terminal.** Per the project's hard rule the GUI runs the same
  CLI; a built-in terminal that echoes or logs commands would capture the
  seed string too.

## Reproduction

1. On a multi-user host, user A runs
   `poo pay -seed <64 hex> -serial 0 -to <addr> -amount 1`.
2. While it runs (it dials the node and fetches holdings, so it is not
   instantaneous), user B runs `cat /proc/$(pgrep -f '^poo pay')/cmdline |
   tr '\0' ' '` and reads the seed.
3. Separately, `fc -l` / `history` shows user A's own shell has recorded the
   seed on disk.

## Impact

Disclosure of the seed is total compromise of that key: the holder of the
seed can claim and spend everything the address controls. The condition is a
local position (a co-tenant reading `ps`/`/proc`, or later access to the
shell history file), so this is not remotely exploitable — hence medium
rather than critical. On a genuinely single-user machine the exposure is
"only" the persisted shell history; on a shared host it is a live secret
readable by every other local user, which is the "local secret leaking"
case the finding guide rates High. Rated medium as the middle of that range;
Blue/Grey may raise it if shared-host use is expected.

## Recommendation

- Read the seed the way `poo wallet` reads its passphrase: from
  `$POO_SEED` / standard input / a `-seed-file`, never a flag. Remove the
  `-seed` string flag from `claim`, `pay` and `keygen`, or keep it only
  behind an explicit `-seed-unsafe` with a stderr warning.
- Better, let `claim`/`pay` use the sealed wallet (like `poo wallet pay`),
  so a raw seed is never needed for everyday spending.
- Document in `docs/running-a-node.md` that a seed must never be passed as
  an argument.

Test: a `pkg/cli` test that `claim`/`pay` take the seed from stdin/env/file
and that passing it as a flag is refused (or warns); confirm `keygen` with a
fixed seed still works through the new input path for reproducible vectors.
