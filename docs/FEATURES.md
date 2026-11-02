# Features

This is a plain list of what the project offers. It is kept up to date
every round and will later feed the full docs and the website. Each
feature is listed once, in its own entry.

There are two sections:

- **Source** is the headless node and the `poo` command. Everything
  starts there.
- **GUI** (`poo-gui`) is a visual layer on the same code. Every GUI
  feature runs a `poo` command line through `cli.Main`, so anything the
  GUI can do, `poo` can do too (docs/gui.md).

Each section is kept by its own team's Grey: Source/Grey keeps Source
and GUI/Grey keeps GUI. When a feature lands on main, its Grey adds it
in that Part A.

## Source (headless node and `poo`)

Source/Grey keeps this section. Until it is filled in, see
docs/running-a-node.md and `poo help`.

## GUI (`poo-gui`)

### Foundation

- **Same code as headless.** Every action is a `poo` command line run
  through `cli.Main`. The GUI module may import only `pkg/cli`, and a test
  enforces this (D84 to D89).
- **Shows its command.** Every panel and form shows the exact command
  line it runs, with a button to run it in the terminal or copy it to a
  shell.
- **Built-in terminal.** It is identical to `poo` in a shell. It splits a
  line the way the platform's shell would, keeps history, and supports
  Ctrl-C and Ctrl-D (end of input).
- **Local web app.** One Go binary serves on loopback only and opens the
  system browser, or an app window with `-app`. It needs no CGO and no
  JavaScript build, and it runs on every platform the headless tool
  builds for.
- **Secure sign-in.** A browser signs in once through a one-time address.
  A session cookie is required, cross-origin requests are refused, and every page has
  a strict content security policy.
- **View-only mode.** `poo-gui -view-only` runs every command in the core's
  view-only mode, so anything that changes state is refused by `poo`
  itself.
- **Secret prompts.** A passphrase or seed field is masked exactly when
  `poo` reads a secret. A second prompt, such as "again", is asked for
  again.
- **Jobs.** Every command run is a job, with live output, Stop, Run again
  and bounded scrollback. Long-running jobs, such as the node, watch and
  grind, keep running when you switch views.
- **Long input.** A pasted entry of any size reaches `poo`: it is sent in
  parts, and end of input waits until the whole paste has arrived.
- **Settings.** `-root` and `-policy` are set once and added by the views
  to their commands.

### Views

- **Getting started.** First steps, each one a command line to run.
- **Node.** Start and stop the node, its status, follow and unfollow
  (including `-replace`), reload and init. It also shows a notice when a
  node.json change is waiting for a reload, with a Reload now button.
- **Doctor checks.** `poo doctor`'s report, where each problem offers its fix command
  exactly as `poo` printed it.
- **Watch.** Live events, newest first, with the filter flags, pause,
  counts and each event in full.
- **Work (grinding).** Devices, jobs and benchmarks. A live run shows the
  plan, the rate and each layer or entry found. Start and stop jobs.
- **Grind schedule.** The schedule is shown as a table with Remove on each
  job, guarded against stale changes. You can add a job, and set the
  defaults: device, threads, layer floor and layer rate. Hints show the
  node's limits.
- **Explore.** Pools, the ledger page by page, pending, unspent, a
  transaction, and an address (with paging and transaction filters). Ids
  and addresses link to each other.
- **Pools and payments.** Status, holdings, payment, build an entry or
  add a layer (optionally printing the hex), and submit an entry or layer
  as hex or from a file.
- **Wallet.** Create, balance, addresses, address, pay, request, receive,
  status, watch, claim, shared policy, co-sign, submit, resend, follow,
  passphrase change and seed. One series choice is shared by every wallet
  command.
- **Peers.** Peers, peer actions, tracking and chases.
- **Node policies.** List, use, archive, unarchive and delete.
- **Series and keys.** Crypto choices, keygen, new series (with an
  example) and series info.
- **Simulate.** Runs `poo sim` scenarios.

### Quality checks

- **Flag parity.** CI fails if a flag that `poo CMD -h` lists is not sent
  by any form, unless the flag is listed with a reason. It also checks
  each field's stated default against the default `poo -h` prints.
- **End-to-end tests.** Nine browser scripts (gui/e2e) run in CI against
  real nodes.
- **Performance re-checks.** GUI/Orange measures every round: a 5-minute
  soak, page load, Watch and Work at high rates, and large results
  (docs/proposals/gui/README.md).

### Coming next

- **Node policy export and import.** A form to export the node's policies
  to one file, and to import a shared file with a preview of duplicates
  and conflicts. It builds on Source's `poo` command once that is on
  main.
