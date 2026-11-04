# GREEN-051: Commands that take a seed take -alg

| Field | Value |
| --- | --- |
| Status | proposed (prototyped) |
| Based on | `main` @ `439b369` |
| Area | `pkg/cli` (`seedFlags`, `claimCmd`, `payCmd`, `issuerSigner`), `pkg/cli/main.go` usage, `docs/modules/poo.md` |
| Effort | small |
| Consensus | none: which key a command signs with, and refusals made before anything is sent. What a node accepts is unchanged. A series still allows an algorithm only by listing it in `key_algs` (D168) |
| Decided by | Grey (Black may skip it) |

## Summary

D168 added SLH-DSA-SHA2-128s. `poo keygen -alg slh-dsa-sha2-128s` makes the
key, a series can list it in `key_algs`, and the wallet can hold it.

The seed commands could not use it:

- `poo claim`
- `poo pay`
- `poo series sign`
- `poo series release`

They always built an Ed25519 key from the seed (`policy.NewEd25519Signer`
in `seedFlags.signer`, and `policy.AlgEd25519` in the claim and pay
addresses).

Green tried this as a user:

1. Made an SLH-DSA key with `poo keygen -alg slh-dsa-sha2-128s`.
2. Made a variable series with that key as the issuer, allowing
   `ed25519,slh-dsa-sha2-128s`.
3. Ran `poo series sign` with the same seed file.

What went wrong:

- **The issuer could not sign their own series.** `poo series sign` said
  "this key's 1-of-1 address is c067…, not the series' issuer 5e55…", and
  there was no way to say which key was meant. The same applies to
  `poo series release`.
- **An SLH-DSA key could not claim or pay.** `poo claim` and `poo pay`
  signed with the seed's Ed25519 key and paid its address, not the address
  `poo keygen -alg` printed.
- **A key the series does not allow was found only by the node.** The node
  answered "policy: key algorithm not allowed by the series", with no word
  of which algorithms the series allows.

All three are fixed on the branch. With the fix, Green ran the whole
journey on one node:

1. Made the SLH-DSA issuer key.
2. Signed the series.
3. Initialised and started the node.
4. Claimed a mint with the SLH-DSA key, and built it with work.
5. Paid 10 units from it.
6. Released part 2, and the node took it in place.

## Idea

- **`-alg` on every command that takes a seed.** `addSeed` adds `-alg NAME`
  (default `ed25519`, by name or number, as `poo keygen -alg` takes it).
  `seedFlags.signer` builds the key with `policy.NewSigner(alg, seed)`.
  `keygen` keeps its own `-alg`, and its seedFlags has none.
- **Addresses follow the key.** The claim and pay addresses use
  `signer.Alg()`.
- **`checkKeyAlg(series, signer)`.** Before anything is signed or sent,
  `claim` and `pay` refuse a key the series does not allow. The message is
  "series ID allows the key algorithms ed25519, not this key's
  slh-dsa-sha2-128s (-alg)".
- **The issuer check names the right `-alg`.** When the key is not the
  issuer's, `issuerSigner` tries the same seed under every other known
  algorithm and adds, for example, "; this seed's slh-dsa-sha2-128s key is
  the issuer: add -alg slh-dsa-sha2-128s". The refusal also names the
  key's algorithm.
  - The seed is not asked for again: `signer` keeps the bytes it read, so
    a seed typed at a prompt (`-seed-file -`) is read once.
- **Docs.** `poo help`, the usage errors and `docs/modules/poo.md` name
  `-alg` for claim, pay, series sign and series release.

## How it was tested

- **`TestSeedCommandsTakeAlg`** (`pkg/cli/keyalg_test.go`) covers:
  - the Ed25519 refusal and its hint;
  - signing and releasing with `-alg`;
  - a bad `-alg`;
  - a claim, a build and a pay with the SLH-DSA key on a running node;
  - that the holder is the SLH-DSA address.
- **`TestClaimRefusesAKeyTheSeriesDoesNotAllow`** covers the refusal before
  sending.
- Both fail on `main`.
- These all pass: gofmt, go vet, staticcheck, `go test -race ./...`, and
  the nine GUI e2e scripts.

## Also checked, no change needed

- **D168's work tries.** On a series with `min_layer_strength` 20, a bare
  `poo work` tried 2^22 = 2^(floor + 2), and its layer of strength 22
  switched.
- **In-place `limits -set`.**
  - `p2p.max_book` answered `in_place`.
  - `max_pending` answered `reloading`, as D168 lists it.
- **`add-release`.** On a series followed whole, it answered `in_place`.

## Ideas for later

- The GUI's claim and pay forms may want an algorithm choice to match.
  That is GUI Grey's call, and the parity e2e still passes, since a flag is
  not a subcommand.
