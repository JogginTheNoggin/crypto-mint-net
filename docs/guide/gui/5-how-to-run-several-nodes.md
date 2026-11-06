# How to run several nodes

## Summary

One computer can keep several node setups, each a node policy with its
own settings and data. The GUI works with one at a time: the one named in
**Settings**, or the current one. To run two nodes at once, give each
its own ports.

## Several setups, one at a time

1. **Node**, **Write a node policy** makes a new one. To make one beside
   the current one, set its name in **Settings**, **Node policy** first.
2. **Node policies**, **Use** makes a saved policy the current one.
3. **Settings**, **Node policy** points every screen at a named policy
   without changing which one is current.

The header shows which node policy the screens are using.

## Two nodes at once

Each running node needs its own control port and peer port.

1. Write the second node policy with **RPC address** `127.0.0.1:7742`
   and **P2P address** `:7743` (any free ports).
2. Start the first node as usual.
3. In **Settings**, set **Node policy** to the second one, then **Node**,
   **Start node**.
4. Switch **Settings** back and forth to look at each.

To connect them to each other, add one as a peer of the other
([How to manage peers](4-how-to-manage-peers.md)), for example
`127.0.0.1:7743`.

## A different root folder

**Settings**, **Root folder** keeps a whole separate set of node
policies in another folder, for example on another disk.

## Several GUIs

Start a second `poon-gui` with `-listen 127.0.0.1:PORT` on another port.
Each one keeps its own settings in the browser, since each has its own
address.

In the terminal: [How to run several nodes (headless)](../headless/5-how-to-run-several-nodes.md).
