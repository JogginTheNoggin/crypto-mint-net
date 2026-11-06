# RED-066: A stale `rpc.addr` sends the full RPC token to whatever listens on that port

| Field | Value |
| --- | --- |
| Severity | Medium |
| Status | fixed |
| Found | Source Round 23 Part B, on `main` @ `215d428` (the dial is old; D161 adds `poon claim -parent`, `poon build -parent` and `poon policy limits -set`, which all send the full token) |
| Area | `pkg/rpc` (`Dial`, `ServeWith`), `pkg/cli` (`dialNode`, every command that writes) |
| For Black | No consensus rule is touched. |
| Fixed by | `claude/project-thread-ggo8dt` @ e8f71ff (BLUE-024) |

## What is wrong

`rpc.Dial(dir)` reads `rpc.addr` and `rpc.token` from the data directory and
sends `Authorization: Bearer <token>` to that address. It checks nothing
about who answers. The node removes `rpc.addr` only on a clean stop
(`defer os.Remove(addrPath)` in `ServeWith`). After a crash, `kill -9`, an
out-of-memory kill or a power loss, `rpc.addr` stays, and the address is the
node's fixed default `127.0.0.1:8645`.

Any other process on the machine, including another user's, can then bind
that port. The next `poon claim`, `pay`, `build`, `work`, `reload`,
`policy limits -set` or any other command that dials with the full token
hands it over. The node is down, so the command seems to fail or falls back
(`policy limits -set` falls back to editing node.json), and the user thinks
nothing happened.

The full token is the node's whole control: build and submit, follow and
unfollow, import node policy with `-apply`, reload, grind jobs, peers.

## How I checked it

A node with its RPC on `127.0.0.1:18645` killed with `kill -9`, so
`rpc.addr` stayed. A script that only listens on that port and prints the
request, then:

```
$ poon policy limits -dir n23 -set ties=7
(the command finishes by editing node.json: nothing seems wrong)

POST /v1/policy/limits HTTP/1.1
Host: 127.0.0.1:18645
Authorization: Bearer 2abeaee516385bcb831a6dae931a443383a42be7754d530ad91c0f6ebc0e7ab1
```

The same bytes are in `n23/rpc.token`. The token file does not change on
restart (`LoadOrCreateToken`), so the captured token also opens the node once
it runs again, from any process that can reach `127.0.0.1`.

A listener that is not the node also chooses what the client prints: error
text from the reply goes to the terminal as the node's own words (the same
class as RED-053).

## What could happen

- On a shared machine (a server, a lab, a build host), a local user who
  watches for the node to die binds its port and collects the token of the
  next person who runs a poon command. With the token they can later change
  the node's policy and registries (`policy import -apply`), or stop and
  start its grinding.
- The window is not rare: RED-063 showed a panic at start; an OOM kill, a
  crash and a power loss are all unclean stops.

## Suggested fix (Blue's call)

Pick one or both:

1. Before sending the token, check that a node really runs in `dir`: the
   node holds the data directory lock (`nodepolicy.Lock`) the whole time, so
   a client that finds the lock free knows `rpc.addr` is stale, deletes or
   ignores it, and reports `ErrNotRunning`. `lockPolicy` already uses the
   same check for the offline path.
2. Prove that the server knows the token without sending it: a first
   unauthenticated `GET /v1/hello?nonce=N` answered with
   `HMAC(token, N)`, which the client checks before it sends the bearer
   token. A stranger on the port cannot answer it.

Also write `rpc.addr` with the node's process id, and have the client check
that the process is alive.

## Not tested

Windows (the lock and port rules differ). Whether `poon-gui` dials the same
way (it only runs `poon`, so I expect it does).

## Resolution

Fixed (D162), with Red's second option: the server proves that it knows the
token before the client sends it.

- `rpc.Dial` and `DialView` first send `GET /v1/hello?nonce=N`, with 32
  random bytes and no token. They send the bearer token only if the reply
  holds HMAC-SHA256(token, `poon-rpc-hello`, the address in `rpc.addr`, N),
  for the full or the view token (`pkg/rpc/hello.go`).
- A stranger on the port knows neither token. Red's listener therefore gets
  only the hello, and `poon policy limits -set` falls back to node.json as
  before, without the token having left. The node at another address
  signs that address, so a relay does not pass either.
- The failure is a `*NotProvedError`. It wraps `ErrNotRunning` and either
  the dial error or `ErrNoProof`, and it prints nothing the stranger sent
  (the address is escaped).
- `poon doctor` reports "something that is not this node answers at the
  address in rpc.addr" or "rpc.addr is left over".
- Not done:
  - Red's lock check. Probing the lock could make a node that is starting
    at the same moment fail to take it, and some platforms have no lock.
  - The process id in `rpc.addr`. The proof already covers what it would.
- A node of an older poon has no `/hello`, so restart the node after an
  upgrade.
- Tests: `TestDialSendsNoTokenToAStranger` covers a 404, a wrong MAC and an
  oversized reply. It checks that no request carried an Authorization
  header and that terminal codes are not printed. `TestDialProvesTheNode`
  covers both tokens, another address, a wrong token and a bad nonce.
