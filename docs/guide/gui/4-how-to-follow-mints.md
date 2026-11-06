# How to follow mints

## Summary

Your node keeps the ledgers of the series, pools and mints it follows,
and nothing else. By default it follows the built-in series. On **Node**
you can follow more, follow less, or follow only what your wallet needs.

## See what the node follows

On **Node**:

- **Followed series**: each series, with its name when it is built in,
  whether its supply is fixed or can grow, and its pools and mints.
- **Followed pools**: totals per series, refreshed every few seconds.
- **What the node follows**: the follow list in the node policy.
- **Each pool**: one row per pool; press **Show** (it can be long).

## Follow more

1. In **Follow more**, fill in what to add:
   - **Series id**, with **Pools** or **Mints** to follow only part of it;
   - or **Issuer id** to follow every series an issuer signs.
2. Press **Follow**. A running node starts following at once.

Tick **Replace what the series follows with these** to swap the list for
that series instead of adding to it. Tick **Only write the policy (no
reload)** to change the file without touching the running node.

## Follow less

**Follow less** takes the same fields. Press **Unfollow**. The node stops
keeping those ledgers.

## Follow only what your wallet needs (light mode)

On **Wallet**, **Follow only the watched mints** sets the node to follow
just the pools of the mints the wallet watches. This uses far less disk
and bandwidth, but your node then knows nothing about other pools.

## A series that can grow

Some series release more mints over time. **Variable series** on **Node**
shows the parts released so far. When a new release appears, give it to
the node with **Add a release** (a file path, or pick the file).

## After editing node.json by hand

If you changed the node policy file yourself, **Reload the node policy**
makes the running node apply it. **Followed pools** shows a notice with
**Reload now** when a change is waiting.

In the terminal: [How to follow mints (headless)](../headless/4-how-to-follow-mints.md).
