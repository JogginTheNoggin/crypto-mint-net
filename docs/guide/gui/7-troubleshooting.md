# Troubleshooting and questions

## Summary

Most problems show up in **Node**, **Health check**, which says what is
wrong and how to fix it. This page covers the common ones, then
questions people ask.

## The screens say "No node is answering"

The node is not running for the node policy the screens use.

- **Node**, **Start node**. Wait for `rpc: listening`.
- If you set a **Node policy** or **Root folder** in **Settings**, check
  they match the node you started. The header shows which policy the
  screens use.
- If the node runs elsewhere (as a service), it must use the same root
  folder and node policy.

## Start node fails

Read the output under **Run the node**, then **Health check**.

- **No node policy**: write one first (**Node**, **Write a node policy**).
- **Address already in use**: another node, or another program, holds
  port 7732 or 7733. Stop it, or give this node policy other ports
  ([How to run several nodes](5-how-to-run-several-nodes.md)).
- **Open file limit too low**: **Followed pools** and **Health check** say
  so, with the number needed. Raise the limit for your user (on Linux,
  `ulimit -n`), then start again.

## The browser page will not open

- Run `poon-gui -no-open` in a terminal and open the address it prints,
  including the `?t=` part. That code signs you in.
- An old tab from an earlier run may have lost its sign-in. Open the
  address `poon-gui` printed this time.

## No peers

- Check **Peers**. A new node needs at least one peer address, or an
  address registry, to find the others.
- Add one with **Add, remove, label or connect a peer**.
- A firewall that blocks outgoing connections on port 7733 stops peers.

## My balance is 0 but I was paid

- **Wallet**, **Watch mints**: add the mint or pool the payment came from.
- Check the payment on **Ledger** with **Find** and its transaction id: it
  may still be pending.
- Check **Wallet file** at the top of **Wallet** names the right wallet.

## A payment shows as dropped

The node no longer holds it. **Wallet**, **Send dropped legs again**.

## The GPU does not show in Work, Devices

- The **note** column says why: usually no OpenCL driver.
- Install your card's driver with OpenCL, restart the node, press
  **Refresh**.
- A GPU grinds the SHA-256 family of hashes only; for other series the
  CPU is used.

## The bounty market says it is off

Turn it on in **Node policies**, **Set limits** (`bounty.post` to pay,
`bounty.take` to be paid). See [How to use the bounty market](5-how-to-use-the-bounty-market.md).

## Something needs a command the screens don't have

Use **Terminal**, or tick **Show the command behind each action** in
**Settings** to see and copy what each form runs. The
[headless guide](../headless/index.md) covers every command.

## Questions

**Is the GUI different from the headless tool?** No. Every button runs
the same command the headless tool would, through the same code, so they
always agree. You can switch between them with the same files.

**Does the GUI store my passphrase or seed?** No. A passphrase or seed is
typed into a masked field and passed straight to the wallet. Output that
contains a seed is shown once and not kept in **Jobs**.

**Can someone else on my network use my GUI?** No. It listens only on
your own computer, and the address it opens carries a one-time sign-in
code.

**Where is my data?** In the root folder, `~/.poon` by default. See
[Where your data lives](4-how-to-run-stop-and-update.md#where-your-data-lives).

**What happens if two ledgers disagree?** The node follows the heavier
one: the one with more grinding work behind it. See
[Concepts](../concepts.md).

**Can I look without changing anything?** Start `poon-gui -view-only`.

In the terminal: [Troubleshooting (headless)](../headless/7-troubleshooting.md).
