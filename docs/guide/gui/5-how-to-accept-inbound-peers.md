# How to accept inbound peers

## Summary

A node behind a home router works fine: it connects out to its peers.
What it cannot do is accept connections from others. To help the network
by being reachable, forward the peer port in your router, or let the node
ask the router to do it.

## Do I need this?

No, to use the network. Yes, if you want other nodes to be able to reach
yours, which makes the network stronger.

## Option 1: forward the port by hand

1. In your router's settings, forward TCP port 7733 to this computer.
2. Tell the node the address others should dial: your public address and
   the port, for example `203.0.113.5:7733`.
   - New node: fill in **Advertise as** in **Node**, **Write a node
     policy**.
   - Existing node: open its `node.json` (see
     [Where your data lives](4-how-to-run-stop-and-update.md#where-your-data-lives)),
     set `"advertise": "203.0.113.5:7733"` in the `p2p` section, and press
     **Reload** in **Reload the node policy** on **Node**.

## Option 2: let the node ask the router

1. New node: in **Node**, **Write a node policy**, set **Ask the router to
   forward the peer port** to `auto` (it tries PCP, then NAT-PMP, then
   UPnP).
2. Existing node: add `"port_map": "auto"` to the `p2p` section of its
   `node.json` and press **Reload** in **Reload the node policy**.

Don't use **Replace the settings of an existing policy** for this: it
replaces all of the policy's settings with the form's, including peers.

The form warns you: with this on, peers learn your network's public
address.

## Did it work?

**Followed pools** on **Node** has a line for the router port mapping,
and an alert if it failed. **Health check** shows a `port-map` line.

In the terminal: [How to accept inbound peers (headless)](../headless/5-how-to-accept-inbound-peers.md).
