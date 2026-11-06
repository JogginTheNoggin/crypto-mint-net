# RED-106: A raw SLH-DSA key whose `PK.root` does not match its seeds is accepted, and its address cannot be spent

| Field | Value |
| --- | --- |
| Severity | Low |
| Status | fixed |
| Found | Source Round 35 Part B, on `main` @ `866a4ac` (D193) |
| Area | `pkg/policy/raw.go` (`NewRawSigner`, SLH-DSA case), `pkg/cli/main.go` (`keygen -raw`), `pkg/cli/sign.go` |
| Fixed by | `claude/project-thread-ggo8dt` @ f835882 (BLUE-036) |
| For Black | Not a rule. |

## What is wrong

For SLH-DSA, `NewRawSigner` only checks the length (`4n`) and hands the
bytes to `slhdsa.PrivateKey.UnmarshalBinary`, then takes the public key as
`SK`'s `PK.seed ∥ PK.root` **as given**. `PK.root` is derived from the other
three fields; nothing recomputes it. A key file with a damaged or mistyped
root (a copy-paste slip, a different key's root) is accepted, and
`poo keygen -raw` prints a public key and an address for it. Money sent to
that address cannot be spent: every signature the key makes fails
verification under the public key it was given. `poo sign` then adds those
invalid signatures to the payment file without checking them (RED-104), and
the failure shows up at the node.

Ed25519 (64-byte form) already checks its public half; ECDSA, secp256k1 and
Schnorr derive the public key from the scalar; ML-DSA from the seed. SLH-DSA
is the only one where the public half is trusted from the file.

## How I checked it

`pkg/policy` test (throwaway, removed; copy kept with Red's notes), SLH-DSA-
128s: a key from `slhdsa.GenerateKey`, one bit of its last byte (`PK.root`)
flipped:

```
genuine:          accepted; public key matches the real one: true;  its signature verifies: true
root one bit off: accepted; public key matches the real one: false; its signature verifies: false
```

## Suggested fix (Blue's call)

- After `UnmarshalBinary`, sign a fixed message and verify it under the public
  key (a few ms to about a second for SLH-DSA-128s here; the test above took
  0.8 s for both keys), and refuse on failure. Doing the same self-test in
  `NewRawSigner` for every algorithm makes this the one rule.
- Have `poo sign` verify each signature it adds against the policy's key
  before writing the file.

## Not tested

SLH-DSA-256s (the check is the same code path); a root that is a valid root
of another key.

## Resolution

Fixed in BLUE-036 (D194). `NewRawSigner` self-tests every key: a fixed
message must verify under the public key it gives. `SignWith` also checks
each signature it adds. Test: `TestBlue036RawKeySelfTested`.
