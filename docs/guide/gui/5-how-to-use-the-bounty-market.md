# How to use the bounty market

## Summary

The bounty market lets nodes trade grinding work: one node offers to pay
for layers on an entry, and a connected peer grinds them and is paid.
It is off until you turn it on. Everything is on the **Bounty market**
screen, after a switch in **Node policies**, **Set limits**.

## Turn it on

On **Node policies**, in **Set limits**:

- To **post offers** (pay others): set `bounty.post` to **on**, and set
  `bounty.max_amount` (the most for one offer) and `bounty.budget` (the
  most in all).
- To **take offers** (be paid): set `bounty.take` to **on**. Optionally
  set `bounty.max_strength`, the strongest layer your machine will work
  (see below).

Press **Save**. If the market is off, the **Bounty market** screen says
which setting to turn on. A line over the tables shows any setting that
cannot work yet, such as posting before a budget is set.

## Pay peers to grind (the payer)

1. You need an entry your node holds: the hash from **Build an entry**,
   or a pool's newest entry on **Ledger**.
2. **Post an offer**: **Entry** (or **Pool** or **Or a mint serial**),
   **Least strength (bits)**, **Units per layer**, **Layers** (how many)
   and **Open for (seconds)**. Press **Post**.
3. **Offers** lists your offers and the layers peers claimed on them,
   with unpaid claims counted.
4. Pay each accepted claim with **Pay** on its row (or **Pay a claim**,
   with **Offer id** and **Claim nonce**). It pays from the wallet named
   on the **Wallet** screen, and asks for its passphrase.
5. **Cancel** closes an offer early. Claims already accepted are still owed.

## Be paid to grind (the grinder)

1. Have a wallet and a fresh address (**Wallet**, **New address**).
2. **Ask peers for offers**, **Ask**. Offers peers sent appear in **Offers**.
3. Press **Work** on an offer's row. It fills **Work an offer**: check
   **Pay to address** and press **Work**. Your node takes a lease (it alone
   works the offer for a while), grinds, and claims each layer it finds.
4. When the payer pays, **Offers** shows it with the payment's
   transaction id. Add it to **Wallet**, **Watch mints** (or find it on
   **Ledger**) so **Balance** counts it.

### How strong an offer can I take?

Each bit of strength doubles the work. Run **Work**, **Benchmark**: its
`minute_strength` is about what your machine finds in a minute. Two bits
above that is about four minutes. Set `bounty.max_strength` (or
**Strongest offer (bits)** on **Ask** and **Work**) so you only take what
you can finish.

## Good to know

- Offers reach your direct peers only, and the payer pays by hand.
- A watch-only wallet cannot pay offers.
- Peers' offers are kept in memory: note a payment's transaction id
  before restarting the grinder's node.

In the terminal: [How to use the bounty market (headless)](../headless/5-how-to-use-the-bounty-market.md).
