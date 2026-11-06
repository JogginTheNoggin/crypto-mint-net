# How to receive

## Summary

Give the payer an address from your wallet, or a payment request, then
watch the payment arrive. A payment is safe to count on once enough
weight is behind it. Everything here is on the **Wallet** screen.

## Before you start

- The node is running ([Start here](1-start-here.md)).
- You have a wallet. The **Wallet file** and **Series** fields at the top
  of **Wallet** pick which wallet every form uses; leave them empty for
  the usual one.

## Give an address

1. In **New address**, type a **Label** if you like (for example who it
   is for) and press **Give out an address**.
2. Send the `address` to the payer.

Use a new address for each payment. It costs nothing and keeps payments
apart.

## Or send a payment request

A request is a small file that says how much, what for, and where to
pay. The payer's wallet checks it before paying.

1. In **Request a payment**, fill in **Amount** and **What for**.
2. Fill in **Request file** with a file name to save it, or leave it
   empty to see it on screen.
3. Press **Make request** and send the file to the payer.

## See it arrive

- **What has arrived**: fill in **Requested address** (or **Or the request
  file**) and press **Check**. Each part of the payment shows as pending
  first, then selected once it is in the ledger, with the weight behind
  it. `settled` turns true once every part is in the ledger, ahead of any
  rival, and at your **Threshold**.
- If the payer told you the transaction ids, put them in **Transactions
  the payer named** and their mint numbers in **Their mint serials**, in
  the same order. The node then looks for those exactly.
- You can also paste a transaction id into **Find** on the **Ledger**
  screen to see where it stands ([How to browse the ledger](4-how-to-browse-the-ledger.md)).

## When is it safe?

A payment can be undone only if someone builds a heavier ledger without
it. The **weight behind it** is how much a rival would have to beat. For
a small amount, being selected is usually enough. For a large one, set
**Threshold** to the weight you want behind it and wait for `settled`.

## See it in your balance

**Balance** counts the mints your wallet watches. If a payment came from
a mint you don't watch yet, add it in **Watch mints**
([How to check your balance](3-how-to-check-your-balance.md)).

In the terminal: [How to receive (headless)](../headless/3-how-to-receive.md).
