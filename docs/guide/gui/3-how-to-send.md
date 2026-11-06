# How to send

## Summary

On **Wallet**, **Pay** sends units to an address or pays a payment
request. The GUI asks you to confirm, the wallet asks for its passphrase,
and **Payments sent** shows where each payment stands.

## Pay an address

1. Open **Wallet** and check the **Wallet file** at the top.
2. In **Pay**, fill in **To address** and **Amount**.
3. Optional:
   - **Memo**: a note for your own records.
   - **Only from mints**: pay only from these mint numbers.
   - **Change address**: where the rest goes. By default the wallet
     uses a fresh address of its own.
4. Press **Pay** and confirm.
5. Type the wallet's passphrase into the masked field when it asks.

The reply lists each part of the payment with its transaction id. A
payment drawn from several mints has one part (a leg) per mint.

## Pay a request

If you were sent a request file, fill in **Or a request file** with its
path instead of the address and amount, and press **Pay**. The wallet
checks the request before paying.

## Follow what you sent

- **Payments sent**: press **Show** to see every payment, each part as
  pending, selected (with the weight behind it), dropped or void.
- **Dropped** means the node no longer holds that part, for example after
  a restart before it was built into an entry. **Send dropped legs again**
  sends it again.
- **Void** means another transaction spent the same coins first; that
  part will not go through.

## Leave Co-signing file empty unless you need it

**Co-signing file** is only for a wallet that cannot sign on its own: a
shared wallet ([How to share a wallet](5-how-to-share-a-wallet.md)) or a
watch-only wallet with a cold key ([How to use cold keys](3-how-to-use-cold-keys.md)).
If you leave it empty, the confirm reminds you, and the wallet refuses a
payment that needs another signature without one.

In the terminal: [How to send (headless)](../headless/3-how-to-send.md).
