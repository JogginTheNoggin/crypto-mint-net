# pkg/policy

## Summary

Mint policies: the rules that control an address. A policy is a type number
and params; its address is `H("PoO-address" ∥ encode(policy))`. A witness is
the policy plus the data that satisfies it, signing `"PoO-witness-v1" ∥ tid`
(decision D9). The package has a registry of policy types and key
algorithms. It ships the threshold type (K of up to 16 keys, decision D7)
and Ed25519.

## API

- `Policy{Type, Params}`, `Encode`, `DecodePolicy`, `Address(h, p)`.
- `Witness{Policy, Data}`, `Encode`, `Size`, `DecodeWitness`;
  `Message(tid)`.
- `Rules`: what a series allows (implemented by `*series.Series`).
- `Registry` and `Default`; `Registry.Check(h, rules, w, address, tid)`
  checks in cheap-first order: size, address match, type allowed and known,
  algorithms allowed, then signatures.
- Threshold: `ThresholdParams`, `ThresholdData`, `Ed25519Signer`,
  `SignThreshold`, and the shortcuts `SingleKey` and `SingleKeyWitness`.

## Guarantees

- A valid witness has exactly one form: exactly K signatures in strictly
  increasing key order.
- Signature checks run only after every cheap check passes.

## Extending

A new policy type or key algorithm implements the `Type` or `KeyAlg`
interface and registers under a new number. Declare its cost bound (how many
signature checks at most). Series opt in by listing the number.
