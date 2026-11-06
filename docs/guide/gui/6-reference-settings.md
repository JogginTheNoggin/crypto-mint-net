# Every setting

## Summary

The GUI has a few settings of its own, on **Settings**. The node has
many more, kept in its node policy: the common ones are fields in the
forms, and every number and switch is on **Node policies**, **Set
limits**, with its range and default shown.

## The GUI's settings (Settings screen)

| Setting | What it does |
| --- | --- |
| Root folder (holds the node policies) | Where node policies and their data live. Empty: `~/.poon`, or the `POON_HOME` environment variable |
| Node policy | Which node setup the screens use. Empty: the current one |
| Theme | System, light or dark |
| Show the command behind each action | Shows the exact command each panel and form runs, with **Run in terminal** and **Copy**, to learn it or script it. Off by default |

Press **Save** for the first two. These are kept in your browser, for
this GUI's address.

## When starting poon-gui

| Option | What it does |
| --- | --- |
| `-app` | Open an app window instead of a browser tab |
| `-no-open` | Print the address instead of opening the browser |
| `-view-only` | Look but change nothing |
| `-listen 127.0.0.1:PORT` | Serve on this port (default: the last one used) |

Environment variable `POON_OPENCL=off` turns off GPU grinding.

## The node's settings

### Set when writing a node policy (Node, Write a node policy)

| Field | What it does |
| --- | --- |
| Follow the worked example series | Follow the small example series instead, for trying things out |
| Series files | Series to follow, from their files |
| RPC address | Where the node's control port listens (default `127.0.0.1:7732`; only this computer) |
| P2P address | Where peers connect (default `:7733`; `off` to accept none) |
| Peers | Peers to stay connected to (`host:port`) |
| Address registries | Lists of peer addresses to read |
| Advertise as | The address peers should dial, if they can reach you |
| Ask the router to forward the peer port | `auto`, `pcp`, `natpmp` or `upnp`; off by default ([inbound peers](5-how-to-accept-inbound-peers.md)) |
| Follow no series | Start with nothing followed |
| Replace the settings of an existing policy | Overwrite an existing policy's settings (its data is kept) |

### Limits (Node policies, Set limits)

**Settable limits** lists each limit with its range, its default, what
the policy sets and the value in effect. **Set limits** has a field for
each: fill in only the ones to change and press **Save**. A switch is a
choice of on or off. A running node applies the change at once where it
can, otherwise it reloads; the line above the result says which.

Common ones:

| Limit | What it controls |
| --- | --- |
| `bounty.post`, `bounty.take` | Turn the bounty market on for paying or for being paid |
| `bounty.max_amount`, `bounty.budget` | The most one offer, and all offers, may pay |
| `bounty.max_strength` | The strongest offer this machine works |
| `entry_cache_mib` | Memory for recent entries |
| `snapshot_every` | How often pool snapshots are saved, for faster starts |

The full list, with what each means, is in
[Every setting (headless)](../headless/6-reference-settings.md).

### Grinding defaults (Work, Schedule defaults)

**Device**, **Threads**, **Layer floor** and **Layer rate**: what every
grinding job uses unless it names its own.

In the terminal: [Every setting (headless)](../headless/6-reference-settings.md).
