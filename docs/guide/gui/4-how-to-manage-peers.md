# How to manage peers

## Summary

Peers are the other nodes yours talks to. Add them by address on
**Peers**, see who is connected, label, block or forget them, and export
your list to share it. Most of the time the node finds and keeps peers by
itself; these tools are for when you want a say.

## See your peers

On **Peers**, the top panel shows the connected peers and how many
addresses the peer book holds in each state. Each peer's version and
features are what that peer says about itself.

**Find peers** searches the whole peer book: tick **Categories**, or fill
in **Mint policy**, **Label holds** or **Address holds**, and press
**Find**.

## Add a peer

1. In **Add, remove, label or connect a peer**, choose **add** in
   **Action**.
2. Fill in **Peer address** (`host:port`; the usual port is 7733).
3. Tick **Also list it in node.json** to make the node always dial it,
   even after a restart.
4. Press **Apply**.

The same form removes a peer, gives it a **Label** you'll recognise, or
connects to it right now (**connect now**).

## Block, time out or forgive a peer

**Act on a peer** does the rest. Its **Action** is one of: **timeout** or
**block** for some **Seconds** (`-1` blocks until you unblock; **Whole
range** blocks the address's whole network range), **unblock**, **clear**
(what the node has noted about it), **template** (apply a tracking
template) or **delete** (forget the address). **Tracking** and **Tracking records** show what the
node has noted about peers that misbehaved.

## Chases

**Chases** lists heavier ledgers peers have told your node about, while
it fetches and checks them. Usually it empties by itself.

## Share your peer list

- **Export peers**: some or all categories, and your blocks, to a file
  (or the screen, with **Save as file**).
- **Import peers**: give the file; it shows what it would add first, and
  changes nothing until you press **Apply**. Importing never removes,
  unblocks or shortens anything. Tick **With blocks** to take the file's
  blocks too.

Peers can also come from address registries listed in the node policy
(**Address registries** in **Write a node policy**).

To let other nodes connect to yours from the internet, see
[How to accept inbound peers](5-how-to-accept-inbound-peers.md).

In the terminal: [How to manage peers (headless)](../headless/4-how-to-manage-peers.md).
