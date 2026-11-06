# GREEN-060: The cold-key steps work with the key never on the node

| Field | Value |
| --- | --- |
| Status | proposed (prototyped) |
| Based on | `main` @ `866a4ac` |
| Area | `pkg/wallet` (`ClaimUnsigned`, `witnessCount` in `SignWith` and `Finish`), `pkg/cli` (`poo keygen -out`, `poo wallet claim -to-policy -out`, `claim` in `poo sign -show`), `gui/e2e/parity.mjs` (two LEFT_OUT lines), docs (running-a-node, poo, wallet) |
| Effort | small to medium |
| Consensus | none: a claim made this way is the same transaction `Wallet.Claim` makes, with its one witness for its one output, checked as every claim is (`tx.CheckStateless`) |
| Decided by | Grey; Black to confirm no rule changed; White may note the unsigned file below; Red may want to look at the key file |

## Summary

BASE.md asked Green to try the cold-key steps in
docs/running-a-node.md as a new user. Green did, on one machine with a
running node and a separate directory standing in for the key's
machine (`POO_HOME` pointing nowhere). Two steps did not work as
written:

1. **The first command failed.** `poo keygen -seed-file key.hex` said
   "open key.hex: no such file or directory". Nothing tells a new user
   how to make `key.hex`. The only way was to run `poo keygen`, copy
   `seed` from its output into a file, and `chmod 600` it. That puts the
   key on screen and in the terminal's scrollback, which is exactly what
   a cold key is meant to avoid.
2. **The claim needed the key on the node.** The docs say "a claim is
   made where the key is, with `poo claim -seed-file`". But `poo claim`
   dials the node, so it runs only where both the key and a node are.
   The watch-only wallet refuses `wallet claim`, and `poo sign` signs
   only payments. A mint could not be claimed to a cold key without
   bringing the key to a networked machine. D193's test does exactly
   that ("as a claim made elsewhere would").

Everything after the claim worked as written: `wallet watch`, `wallet
pay -out`, `poo sign` with no node, `wallet submit`, the change back to
the cold address, and `-show` before signing.

## Idea

- **`poo keygen -out FILE`.** It makes a new random key and writes the
  seed in hex to a new file, mode 0600, opened with `O_EXCL`, so an
  existing key file, perhaps the only copy, is never replaced. It prints
  the usual JSON with `key_file` and a `key_note` and without `seed`. It
  refuses to go with `-seed`, `-seed-file` or `-raw`.
- **`poo wallet claim -serial N -to-policy NAME -out FILE`.**
  - `Wallet.ClaimUnsigned` prepares the claim to the wallet policy's
    address, with an empty witness for its one output, and writes it as
    an `Unsigned` file whose one leg lists that policy.
  - It records the claim as a `Sent` with state `unsigned`, so `wallet
    submit` takes the signed file, and watches the mint.
  - It works in any wallet, so a shared policy can claim too. It is how
    a watch-only wallet claims.
  - A plain `wallet claim` in a watch-only wallet now says how: "poo
    wallet claim -serial 5 -to-policy NAME -out claim.json, poo sign
    claim.json -key-file KEY, poo wallet submit claim.json".
  - The flag is `-to-policy`, because `-policy` already names the node
    policy on every wallet command.
- **Signing a claim.** A claim needs one witness per output, not per
  input (`tx.CheckForm`). `SignWith` and `UnsignedLeg.Finish` therefore
  count `witnessCount(t)`: inputs for a payment, outputs for a claim.
  Nothing else changes for payments. `poo sign -show` marks a claim leg
  with `"claim": true`, since it shows `inputs: 0`.
- **Docs.**
  - The cold-key block in running-a-node.md now runs from `keygen -out`
    through a cold claim to payment.
  - The poo.md rows for keygen and sign, and wallet.md.
- **GUI parity.** Two LEFT_OUT lines: `keygen -out`, and `wallet claim
  -to-policy` and `-out`. Like `-raw` and `-watch-only`, they stay in
  the terminal for now.

## For Black

The signed claim is `tx.Tx{Mint, Outputs: [{policy address, supply}],
Witnesses: [one]}`, the same shape `Wallet.Claim` and `poo claim` build.
The node checks it with `CheckStateless` like any claim. No rule
changed. `witnessCount` only lets the off-node signer fill in the
witness a claim already needs.

## For White

The `Unsigned` file format is unchanged: a claim leg is a leg with one
`inputs` entry. A `poo sign` from before this change refuses such a
file with "unsigned leg has the wrong number of inputs" and writes
nothing, which is a clean refusal. The wallet file gains no field,
since the claim's `Sent` uses the existing fields.

## For Red

`keygen -out` writes the seed to a new 0600 file and never replaces
one. Red may want to try a symlink at the target path. `O_EXCL` refuses
an existing path, a dangling symlink included.

## How it was tested

- **`TestColdClaimWithoutTheKeyOnTheNode`** (pkg/cli):
  - The key: `keygen -out` writes a 0600 file and prints no seed, and
    the file holds the printed key. It refuses to replace the file, and
    `-out` does not combine with `-seed-file`.
  - The claim: a plain watch-only claim says how, `-to-policy` without
    `-out` is refused, and the claim is prepared.
  - Signing: `sign -show` marks the claim and its output is the policy
    address, and `sign` adds the one signature.
  - The result: once submitted and worked in, the cold address holds
    100 and the watch-only balance is 100.

  It fails on `main`, where `-out` and `-to-policy` do not exist.
- **`TestColdKeyPayment`** (D193) still passes.
- **By hand:** the whole updated block in running-a-node.md, with the
  "key machine" a separate directory and `POO_HOME=/nonexistent`. The
  claim was selected, then the payment of 30, with change back.
- **Checks run locally:**
  - gofmt, go vet and staticcheck;
  - `go test -race ./...`;
  - the nine GUI e2e scripts.

## Ideas for later

- `poo sign -show` could print the policy address each claim or payment
  output belongs to, by name, when it matches the key's own address.
- The GUI could offer the cold claim next to watch-only wallets once
  they leave the terminal.
