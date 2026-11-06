# How to use cold keys

## Summary

A cold key is a private key that never touches the computer running
the node. The node's wallet is watch-only: it sees balances and prepares
payments, and you sign each payment on the machine that holds the key,
offline if you like. **Wallet**, **Keep the key off this machine** lists
the seven steps with a button to each form.

## Why

If the node's computer is compromised, a watch-only wallet gives away
nothing that can spend. The key stays on a machine you trust, or one that
is never online.

## On the key's machine: make the key

Use `poon-gui` or the headless tool there (it needs no node and no
network).

1. Open **Series and keys**, **Make a key**.
2. Tick **Save the new key to a file** and give a path. The key goes to
   that file only (readable only by you) and is never shown.
3. Press **Make key** and note the `public_key` and the key algorithm
   (`ed25519` by default).

Already have a key from another tool? Tick **Or a key made by another
tool** instead: the key is typed into a masked field and never shown, and
you get its public key and address. Back up the key file: it is the only
copy.

## On the node's machine: a watch-only wallet

1. In **Wallet**, type a file name in **Wallet file**, for example
   `cold.json`.
2. **Create or restore a wallet**: tick **Watch-only** and press
   **Create**. No passphrase is needed, as there is no key in it.
3. **Record a shared policy**: **Name** `cold`, **Signatures needed** `1`,
   **Keys** `ed25519:PUBLIC_KEY` (the algorithm, a colon, and the public
   key you noted). Press **Record**. Its address is where your coins go.
4. Get paid to that address, or claim a mint to it: **Claim a mint** with
   **To a wallet policy** set to `cold` and a **Claim file**. The claim is
   signed like a payment, below.

## Every payment: prepare, sign, submit

1. **Here**: **Pay** as usual, and fill in **Co-signing file**, for example
   `pay.json`. Nothing is sent yet.
2. Carry `pay.json` to the key's machine (a USB stick, for example).
3. **There**: **Wallet**, **Sign with a key kept elsewhere**. Fill in
   **Co-signing file**. Either tick **The key is in a file** and give
   **Key file**, or leave it unticked and type the key into the masked
   field when asked.
   - Tick **Show only** first and press **Sign**: it says what the payment
     pays, and signs nothing. Read it.
   - Untick **Show only** and press **Sign**. The line under the result
     says what the key added and whether more signatures are needed.
4. Carry the file back.
5. **Here**: **Submit a co-signed payment**, with the file, sends it.

The signing machine can be fully offline: turn on command lines in
[Settings](6-reference-settings.md), copy the **Sign** line, and run it
there with the headless tool.

In the terminal: [How to use cold keys (headless)](../headless/3-how-to-use-cold-keys.md).
