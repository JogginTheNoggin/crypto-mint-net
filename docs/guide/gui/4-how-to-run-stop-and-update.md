# How to run, stop and update

## Summary

Start and stop the node on **Node** with **Start node** and **Stop
node**. To update, stop the node and `poon-gui`, unpack the new release
over the old program, and start it again: your data and settings are kept
in their own folder.

## Start and stop

- **Node**, **Run the node**, **Start node** starts the node. Its output
  shows below; `rpc: listening` means it is ready.
- **Stop node** stops it cleanly. Wait for `node stopped (exit 0)`.
- The node keeps running while you use other screens. It stops when
  `poon-gui` stops.
- **Check every record again on this start** (the box under the button)
  makes the start check every stored ledger from the beginning instead
  of from its snapshot. It is slower; use it
  if **Health check** suggests it.

## Is it healthy?

**Health check** on **Node** checks the node policy, files, series, the
control port and peers, and shows what to fix next to each problem.
**Followed pools** shows the node's version beside the GUI's and warns if
they differ.

## Where your data lives

Everything is in the root folder: `~/.poon` (or `%USERPROFILE%\.poon` on
Windows), or the folder named in [Settings](6-reference-settings.md).
Each node policy has its own folder in it, with `node.json` (its
settings), its stored ledgers, and `wallet.json` if you use the default
wallet. Program files and data are separate, so replacing the program
never touches your data.

## Update

1. Download and check the new `poon-gui` archive
   ([Start here](1-start-here.md#1-download)).
2. On **Node**, press **Stop node**, then stop `poon-gui`.
3. Replace the old program with the new one.
4. Start `poon-gui` and press **Start node**.

The version shows in the header. Peers see your version too.

## Keep it running unattended

The GUI is for when you are at the computer. To keep a node running all
the time, run the headless node as a service; see
[How to run, stop and update (headless)](../headless/4-how-to-run-stop-and-update.md).
You can still open `poon-gui` on the same computer to look at it: the
screens talk to whichever node is running for that node policy.

In the terminal: [How to run, stop and update (headless)](../headless/4-how-to-run-stop-and-update.md).
