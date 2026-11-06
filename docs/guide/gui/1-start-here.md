# Start here: install, first run, your first wallet

## Summary

Download `poon-gui`, start it, and it opens in your browser. Then three
steps make a working node with a wallet: write a node policy, start the
node, and create a wallet. This page walks through them and gives a
short tour of the window.

## What you need

- A computer running Linux, macOS or Windows.
- A modern browser (Chrome, Chromium, Edge, Firefox or Safari).
- Nothing else: `poon-gui` is one program with no installer and nothing
  else to install. It contains the headless tool too.

## 1. Download

Download the `poon-gui` archive for your system from the project's
releases page. It is named `poon-gui-VERSION-OS-ARCH`: `.tar.gz` for
Linux and macOS, `.zip` for Windows (`amd64` is most PCs, `arm64` is Apple
silicon and ARM boards).

Check the download against `SHA256SUMS`, listed on the same page:

- Linux and macOS: `sha256sum -c SHA256SUMS --ignore-missing`
- Windows (PowerShell): `Get-FileHash poon-gui-*.zip` and compare it with
  the line in `SHA256SUMS`.

Unpack the archive anywhere you like.

## 2. Start it

Run `poon-gui` (double-click it, or run it from a terminal). It starts a
small server on your own computer only and opens it in your browser.
The address it opens carries a one-time sign-in code, so no other
program or person can use your GUI.

Useful options when you start it from a terminal:

| Option | What it does |
| --- | --- |
| `-app` | Opens it in an app window (Chrome, Chromium or Edge) instead of a tab |
| `-no-open` | Prints the address instead of opening a browser; open it yourself |
| `-view-only` | Lets you look but not change anything |
| `-listen 127.0.0.1:PORT` | Uses a port you choose (by default it reuses the last one) |

It comes back on the same port next time, so the browser remembers your
settings. Closing the browser tab does not stop it; stop it with Ctrl-C
in its terminal, or by ending the program.

## 3. A tour of the window

- **The menu on the left** lists the screens: Getting started, Node,
  Ledger, Watch, Explore, Work, Bounty market, Pools and payments,
  Wallet, Peers, Node policies, Series and keys, Jobs, Settings and
  Terminal. On a narrow window it becomes a drop-down at the top.
- **The header** shows which node policy you are using and the version.
- **Panels** show information and refresh by themselves; **forms** do
  something when you press their button. Anything that moves money or
  deletes something asks you first.
- **Terminal** is the headless tool, for anything the screens don't cover.
- **Jobs** lists everything the GUI has run, with its output.

Each screen is described in [Every screen](6-reference-screens.md).

## 4. Write a node policy

A node policy is your node's settings: what it follows, its addresses
and its limits. You need one before the node can start.

1. Open **Node**.
2. Find **Write a node policy**. You can leave every field empty: the
   node then follows the built-in series that every node follows out of
   the box.
3. To join other nodes, fill in **Peers** with one or more peer
   addresses (`host:port`). The peer port is 7733.
4. Press **Write policy**.

## 5. Start the node

1. On **Node**, in **Run the node**, press **Start node**.
2. The output shows `rpc: listening` when it is ready. **Followed
   pools** fills in after a few seconds.
3. **Health check** on the same screen says if anything needs fixing,
   with the fix next to each problem.

The node keeps running while you use the other screens. **Stop node**
stops it. See [How to run, stop and update](4-how-to-run-stop-and-update.md).

## 6. Create your first wallet

1. Open **Wallet**.
2. Scroll to **Create or restore a wallet** and press **Create**.
3. A masked field asks for a passphrase. Type a long one you will
   remember, and type it again when asked. The passphrase is passed
   straight to the wallet and never stored by the GUI.
4. Back up the seed at once: see
   [How to back up your wallet](3-how-to-back-up-your-wallet.md).

Leave **Wallet file** at the top empty to use `wallet.json` in the root
folder (`~/.poon`, or the folder set in [Settings](6-reference-settings.md)),
or type a file name to keep several wallets.

## 7. Get an address

In **Wallet**, **New address**, press **Give out an address**. Give the
`address` it shows to whoever pays you. A new one for each payment
keeps your payments apart. Then read [How to receive](3-how-to-receive.md).

## Next

- [Concepts](../concepts.md), if a word above was new.
- [How to browse the ledger](4-how-to-browse-the-ledger.md), to see what
  your node holds.
- [How to grind](5-how-to-grind.md), to help secure the ledger with your
  CPU or GPU.

In the terminal: [Start here (headless)](../headless/1-start-here.md).
