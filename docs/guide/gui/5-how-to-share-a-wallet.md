# How to share a wallet

## Summary

A shared wallet needs k of n people to sign before coins move (for
example 2 of 3). Each person records the same shared policy, payments are
written to a co-signing file, each signer adds their signature, and the
last one submits it. All on **Wallet**.

## Set it up

1. Each signer gets their public key: **Wallet**, **New address**, and
   note the `public` key in the reply.
2. One signer, in **Record a shared policy**:
   - **Name**: for example `joint`;
   - **Signatures needed**: k, for example `2`;
   - **Keys**: `self` for their own new key, then each other signer's
     public key (`ALG:HEX` or just `HEX` for ed25519);
   - press **Record**. The reply shows the shared address.
3. Each other signer records the same policy in their own wallet, listing
   the keys in the same order, with `self` in their own place.
4. Pay coins to the shared address as to any address.

## Spend from it

1. One signer: **Pay**, with **Co-signing file** filled in (for example
   `pay.json`). Nothing is sent yet.
2. Pass the file to the next signer. They use **Co-sign a payment** with
   the file and their passphrase.
3. When enough have signed, anyone uses **Submit a co-signed payment**
   to send it.

A signer whose key is kept offline signs with **Sign with a key kept
elsewhere** instead ([How to use cold keys](3-how-to-use-cold-keys.md)).

In the terminal: [How to share a wallet (headless)](../headless/5-how-to-share-a-wallet.md).
