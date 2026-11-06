# GREEN-056: Port mapping status says why, where and when

| Field | Value |
| --- | --- |
| Status | proposed (prototyped) |
| Based on | `main` @ `bf23ddb` |
| Area | `pkg/p2p` (`PortMapStatus`, `portMapLoop`, `holdMapping`), `pkg/cli` (`portMapCheck` in doctor), docs (running-a-node, rpc) |
| Effort | small |
| Consensus | none: what status and doctor show of the port mapping, and how soon a lapsed mapping is asked for again. Port mapping only decides whether other nodes can dial this one (D182) |
| Decided by | Grey (Black may skip it) |

## Summary

Green ran D182 as a node runner, on a machine whose router speaks none
of the three methods:

1. `poon init -port-map bogus` is refused with the list of modes, and
   `-port-map auto` writes `p2p.port_map`.
2. With `-p2p off`, status shows `skipped` and doctor says ok.
3. With peers on, every method fails. The node logs it, status shows
   `failed` with the full error, and doctor warns with a fix.
4. Adding `"port_map": "auto"` to a running node's `node.json` and
   running `poon reload` started the mapping. The new
   `snapshots.due` (ORANGE-040) shows in status.

All of that behaved as D182 describes. Four loose ends showed up:

1. **Skipped did not say which reason.** Doctor said "p2p.advertise is
   set by hand ... or the node accepts no peers (p2p.listen is off)".
   The node knows which one applies.
2. **Failed did not say which router was asked.** The fix says "set
   p2p.port_map_gateway if the gateway is not found", but status never
   named the gateway the node found. It appeared only inside a socket
   address in the error text.
3. **"The node keeps trying" did not say when.** The waits double from
   1 minute to 30. A runner who has just turned on UPnP in the router
   could not tell whether to wait or reload, and doctor did not mention
   that `poon reload` asks again at once. Green checked that it does.
4. **The retry wait never reset.** After a mapping had worked for days,
   then lapsed (a router reboot), the next try waited as long as the
   failures before it had built up, up to 30 minutes, rather than 1.

## Idea

- **`PortMapStatus`** gains three fields, all left out when empty:
  - `gateway`: `p2p.port_map_gateway`, or else the default route's
    gateway as last found. It is found again on each try, as
    `portmap.Client` finds it.
  - `reason`: for `skipped`, either "p2p.advertise is set by hand, which
    always wins" or "the node accepts no peers (p2p.listen is off)"
    (`PortMapReasonAdvertise`, `PortMapReasonNoListen`).
  - `next_try`: when a failed, unconfirmed, not-public or lapsed
    mapping is asked for again. It is zero while a mapping is held or
    asked for.
- **Retry reset.** `holdMapping` also reports whether the mapping was
  confirmed and advertised. When it was, the wait starts over at
  1 minute.
- **Doctor:**
  - `skipped` names the one reason.
  - `failed` reads "the router gave no mapping (asked the router at
    192.0.2.1): ...; it asks again in 57s", or says "no gateway was
    found", and the fix adds "poon reload tries again at once".
  - `unconfirmed` and `not_public` add when the node asks again.
  - Against an older node that sends no `reason`, the old text stays.
- **Docs.** running-a-node.md says how to turn the mapping on for a
  node set up already, and explains `gateway`, `next_try`, `reason` and
  the reload. The `/status` row in rpc.md lists the new fields.

## How it was tested

- **`TestPortMapStatusSaysWhyWhereWhen`** (pkg/p2p) covers:
  - the advertise reason when a mapping is skipped;
  - `gateway` and a `next_try` an hour out after a failure.
- **`TestPortMapRetryResetsAfterAHeldMapping`** (pkg/p2p):
  - Setup: four failures build the wait to 800 ms, a mapping is then
    held, and its renewal fails.
  - Expected: the next try is set within 400 ms.
  - It fails when the reset is taken out. A helper in its own export
    file sets the first and longest waits separately.
- **`TestPortMapCheckSaysWhyWhereWhen`** (pkg/cli) checks the skipped,
  failed and no-gateway lines.
- **By hand:**
  - Status gave `gateway: 192.0.2.1` and `next_try` a minute out.
  - Doctor printed the lines above.
  - `poon reload` moved `since` and `next_try` forward at once.
- **Checks run locally:**
  - gofmt, go vet and staticcheck;
  - `go test -race ./...`;
  - the nine GUI e2e scripts.

## Ideas for later

- **Turning the mapping on.** For a node set up already, it is a hand
  edit of `node.json`, and the same holds for `p2p.advertise`. A small
  `poon p2p set port_map=auto` (or `-set` on an existing command) could
  cover the p2p settings that `poon policy limits` does not, since those
  are not bounded numbers.
- **The GUI's Node view** could show `gateway` and `next_try` next to the
  port-map state. That is GUI team work.
