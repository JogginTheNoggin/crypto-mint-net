# How to back up your wallet

## Summary

Your wallet's seed is the one thing you must not lose: every key comes
from it. Write it down once with **Show the seed**, keep it offline, and
you can restore the wallet anywhere with **Create or restore a wallet**.

## Back up the seed

1. Open **Wallet**.
2. In **Show the seed**, press **Show seed** and confirm.
3. Type the passphrase.
4. The seed is hidden until you press **Show** next to it. Write it on
   paper, or keep it in a password manager, and keep it away from this
   computer.

Anyone with the seed can spend from the wallet. The GUI does not keep
the seed: the job shows it once, and **Jobs** does not hold it.

## Restore a wallet from its seed

1. In **Wallet**, type a new file name in **Wallet file** at the top (or
   leave it empty for `wallet.json`, if none exists yet).
2. In **Create or restore a wallet**, tick **Restore from a seed** and
   press **Create**.
3. Type the seed into the masked field when it asks, then a passphrase.
4. Add the mints you hold in **Watch mints**, then **Balance** shows them.

**Gap** is how many unused keys the wallet keeps ahead of the last used
one (default 20). Raise it if you gave out many addresses that were
never paid, so a restore still finds the ones after them.

## Change the passphrase

**Change the passphrase**, **Change**: it asks for the old passphrase,
then the new one twice. The seed stays the same.

## Back up the node too

The wallet file also keeps labels, requests and payments you sent. Copy
it with your other files from time to time. To back up the node's
settings, see [How to move or back up a node](5-how-to-move-or-back-up-a-node.md).

In the terminal: [How to back up your wallet (headless)](../headless/3-how-to-back-up-your-wallet.md).
