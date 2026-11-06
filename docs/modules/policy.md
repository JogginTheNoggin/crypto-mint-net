# pkg/policy

## Summary

Mint policies: the rules that control an address. A policy is a type number
and params; its address is `H("PoO-address" ∥ encode(policy))`. A witness is
the policy plus the data that satisfies it, signing `"PoO-witness-v1" ∥ tid`
(decision D9). The package has a registry of policy types and key
algorithms. It ships three types: threshold (K of up to 16 keys, decision
D7), hashlock and k-of-policies (decision D82), and nine key algorithms:
Ed25519, ECDSA over P-256, P-384, P-521 and secp256k1, BIP-340 Schnorr,
and ML-DSA-44, -65 and -87 (decision D81). Every one is listed, with
sizes, in [../cryptography.md](../cryptography.md).

## API

- `Policy{Type, Params}`, `Encode`, `DecodePolicy`, `Address(h, p)`.
- `Witness{Policy, Data}`, `Encode`, `Size`, `DecodeWitness`;
  `Message(tid)`.
- `Rules`: what a series allows (implemented by `*series.Series`).
- `Registry` and `Default`; `Registry.Check(h, rules, w, address, tid)`
  checks in cheap-first order: size, address match, type allowed and known,
  algorithms allowed, then signatures.
- `Registry.CheckPolicy(rules, p)`: the same allowed checks on a policy
  alone, at every depth. `Types`, `Algs`, `TypeByName`, `AlgByName`.
- `Registry.CheckSpendable(rules, p)`, which `CheckPolicy` also runs:
  refuses (`ErrUnspendable`) a policy whose largest witness is over the
  series' `max_witness`, so every set of signers it allows can spend
  (worst case, D170). With no ceiling every policy passes. Under the
  default series' 128 KiB, 16-of-16 ML-DSA-87 and SLH-DSA-SHA2-128s fit,
  and SLH-DSA-SHA2-256s up to 4. It is checked where an address is made or
  paid, never when a witness is checked (RED-077, RED-079, D169, D170).
- `MaxK(alg, ceiling)`: the largest k of a k-of-k policy of one algorithm
  under a ceiling, params included. `poo crypto` prints it as
  `max_signatures_per_witness`.
- No network-wide size limit (D170): `DecodeWitness` needs no series and
  is bounded only by the bytes received; threshold data is read against
  its params, each signature at its key's exact size; the series' ceiling
  is checked first in `Check`.
- Threshold: `ThresholdParams`, `ThresholdData`, `SignThreshold`, and the
  shortcuts `SingleKey` and `SingleKeyWitness`.
- Keys: `Signer`, `NewSigner(alg, seed)` for any algorithm from a 32-byte
  seed, `NewRawSigner(alg, raw)` for a private key made elsewhere in the
  algorithm's own raw form (an Ed25519 seed, or seed and public key; an
  ECDSA or secp256k1 scalar; an ML-DSA seed; an SLH-DSA private key;
  D193), and `Ed25519Signer`.
- Hashlock: `NewHashlock(hash, preimage)`, `HashlockParams`,
  `HashlockData`.
- K-of-policies: `AllOf`, `AnyOf`, `KOfPoliciesParams`, `KOfPoliciesData`
  with `Part`s. `Composite` is the interface of a type made of policies;
  `Check` applies the series' rules to every policy inside one.

## Guarantees

- A valid witness has exactly one form: exactly K signatures in strictly
  increasing key order.
- Signature checks run only after every cheap check passes.
- `CheckCached` with a `Verified` list (each series keeps
  one, 16384 keys) skip the satisfaction function for a witness that
  already passed for the same policy, data and tid. Only passes are
  remembered, and the size, address and allowed checks run every time
  (ORANGE-002, D105).
- Every algorithm refuses the second encoding of a valid signature.
- A k-of-policies witness checks each listed policy at most once, and
  nesting is at most `MaxPolicyDepth` (4).
- `testdata/known.json` pins key derivation for every algorithm and the
  addresses of example policies under every hash.

## Extending

A new policy type or key algorithm implements the `Type` or `KeyAlg`
interface and registers under a new number in `Default`. A key algorithm
also gets a case in `NewSigner`. Declare its cost bound (how many
signature checks at most). Series opt in by listing the number.
