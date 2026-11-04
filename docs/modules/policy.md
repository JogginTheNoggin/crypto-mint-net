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
  refuses (`ErrUnspendable`) a policy whose smallest witness cannot be
  spent. That is a witness whose data is over `MaxDataSize`, whose params
  are over `MaxParamsSize`, or that is over the series' `max_witness`.
  For example, at most 14 ML-DSA-87 or 8 SLH-DSA-SHA2-128s signatures fit.
  It is checked where an address is made or paid, never when a witness is
  checked, and it refuses nothing a valid witness could have (RED-077,
  RED-079, D169). `poo crypto` prints `max_signatures_per_witness` for
  each algorithm.
- Threshold: `ThresholdParams`, `ThresholdData`, `SignThreshold`, and the
  shortcuts `SingleKey` and `SingleKeyWitness`.
- Keys: `Signer`, `NewSigner(alg, seed)` for any algorithm from a 32-byte
  seed, and `Ed25519Signer`.
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
