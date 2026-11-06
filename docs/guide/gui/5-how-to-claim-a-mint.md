# How to claim a mint

## Summary

A mint is a fixed amount of units that nobody owns until someone claims
it. Claiming writes a claim transaction paying the mint to your wallet;
once it is in the ledger with weight behind it, the units are yours.
**Wallet**, **Claim a mint** does it in one step.

## Claim a mint to your wallet

1. Find an unclaimed mint. On **Ledger**, open a pool: its **Mints**
   column is the range of mint numbers it holds. **Pools and payments**,
   **Holdings of a mint**, with a **Mint serial**, shows whether a mint
   already has holders.
2. On **Wallet**, in **Claim a mint**, fill in **Mint serial** and press
   **Claim**, then confirm and type the passphrase.
3. The claim is pending until it goes into an entry. To speed that up,
   give it weight yourself: **Pools and payments**, **Build an entry** for
   the mint's pool, or start a grinding job
   ([How to grind](5-how-to-grind.md)).
4. Once it is selected, **Balance** shows the mint's units. Claiming also
   watches the mint for you.

Who owns a mint is decided like everything else: by the heaviest ledger.
If someone else claimed the same mint in a heavier ledger, theirs wins.

## Claim to a cold key

With a watch-only wallet, fill in **To a wallet policy** (the policy's
name) and **Claim file**. The claim is written to the file unsigned; sign
it on the key's machine and submit it, as in
[How to use cold keys](3-how-to-use-cold-keys.md).

## Compete for a claimed mint (rival claims)

If a mint was claimed in an entry you think should not win, you can claim
it on a branch from before that claim and add weight until your branch is
heavier. This costs real grinding work, and only wins if your branch ends
up heavier.

1. **Pools and payments**, **Build a rival claim**: **Mint serial**, and
   **Parent** (for example the entry before the claim). Type the seed of
   the key that should own it into the masked field when asked.
2. It prints the entry's hash. Add weight to it with **Add a layer**
   (**Entry hash**) or a grinding job on that entry.
3. **Ledger** shows when the node switches to your branch.

In the terminal: [How to claim a mint (headless)](../headless/5-how-to-claim-a-mint.md).
