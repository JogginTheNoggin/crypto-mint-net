# GREEN-007: A read-only control scope in the core

| Field | Value |
| --- | --- |
| Status | proposed |
| Based on | `claude/m10-watch` @ `5ea6828` |
| Area | `pkg/rpc` (view token, prototyped), `pkg/cli` (read-only mode, proposed) |
| Effort | small for the token, medium for the CLI mode |
| Decided by | |

## Summary

This gives the core a "view only" scope, so that a monitor, a dashboard,
or a GUI started in view mode can read everything and change nothing.
The scope comes in two parts. The first is a second RPC token,
`rpc.view-token`, that opens only the GET endpoints. It is prototyped on
the branch, with tests. The second is a read-only mode in `pkg/cli` that
refuses every command that changes state and talks to the node with the
view token. That part is a proposal only. Both parts live in the headless
core, so `poo` and the GUI enforce the same rule with the same code. The
idea comes from GUI-005 (GUI/Red), which notes that the GUI's one secret
is all-or-nothing control of the node.

## Problem

Every RPC client today holds `rpc.token`, and `rpc.token` can submit
transactions, add layers, change what the node follows, reload the
policy, and block or delete peers. Three uses only need to look:

- a Prometheus scrape of `/v1/metrics` (GREEN-002)
- a status dashboard
- a GUI left open to watch the node

They still get full control, so a leak of any of their credentials is a
full compromise. GUI-005 makes the same point for the GUI. Its suggested
read-only mode cannot be enforced in `gui/` alone. The GUI runs `pkg/cli`,
and `pkg/cli` reads `rpc.token` and the wallet files directly, so the
boundary has to be in the core.

## Idea

**1. View token (prototyped).** The node makes `rpc.view-token` beside
`rpc.token` on first start, also mode 0600. `Handler.ServeHTTP` accepts
it for GET and HEAD requests only. Any other method gets 403 with the new
code `forbidden`, and a wrong token still gets 401. `rpc.DialView(dir)`
gives a client that holds the view token. The rule follows the HTTP
method, not a list of endpoints, so a new endpoint is covered as soon as
it is written: a GET must never change state, which is already the
convention here. `/v1/watch` is a GET, so the view token can stream
events. The doctor's `files` check covers the new file.

**2. Read-only CLI mode (proposed).** `cli.Run` takes an option, for
example `Stdio.ReadOnly`, or a `-read-only` flag before the command. When
it is set:

- `pkg/cli` dials with `rpc.DialView`. It never opens `rpc.token`, so
  even a bug in a command cannot write through the RPC.
- A table in `pkg/cli` marks each command and wallet subcommand as
  `read` or `write`. A write command fails at once with "read-only mode".
  A test checks that every command in the usage text is in the table, the
  same way GREEN-001's test checks the command list.
- Commands that write local files without the RPC are refused too:
  `init`, `policy use/archive/delete`, the wallet commands that change
  the wallet, and `follow`/`unfollow`, which also edit the policy.
- Read commands that only read local files stay allowed: `poo policy
  list` and `poo wallet` balances.

The GUI could then offer a view mode that runs `cli.Run` with
`ReadOnly: true` (GUI/Grey's call). Its secret would then be worth only
what the view token is worth.

**3. Later, if wanted.** Named scopes in the node policy (`rpc.tokens:
[{file, scope: read|pay|admin}]`), so that, for example, a payment service
can submit transactions without managing peers. This proposal does not
include that, because nobody needs it yet.

## Cost and risk

- **Token.** About 60 lines in `pkg/rpc`, standard library only. It adds
  one file to the data directory. It changes no existing behavior:
  `rpc.token` works exactly as before. One risk is a future GET handler
  that changes state. A review rule, "a GET never changes state", in
  `docs/modules/rpc.md` and AGENTS.md would guard against that, and
  Source/Red could add a check that every POST route answers 403 to the
  view token. The view token is a new secret, but it is weaker than the
  existing one and lives under the same file mode.
- **CLI mode.** Medium. The command table has to be kept up to date, and
  the test above enforces that. It touches no consensus code.

## Prototype

On `claude/source-green`:

- `pkg/rpc/rpc.go`: the `ViewTokenFile` and `CodeForbidden` constants,
  `LoadOrCreateViewToken`, `SetViewToken`, and the check in `ServeHTTP`
- `pkg/rpc/server.go`: makes the token and sets it
- `pkg/rpc/client.go`: `DialView`
- `pkg/rpc/viewtoken_test.go`
- `pkg/cli/doctor.go`: the `files` check covers the view token
- `docs/modules/rpc.md`: describes both tokens

The test covers four cases. The view token reads `/status` and
`/metrics`. It gets `forbidden` on `POST /tx` and `POST /reload`. The full
token still submits. A wrong token gets `unauthorized`.

The read-only CLI mode is not built. If Grey adopts part 2, Green can
build it next.
