# pkg/daemon

## Summary

The one place that runs a full node: it opens the node in a data
directory, starts its p2p host, its local interface and its watch hub, and
stops them together. `poo node` calls it, and so would any graphical
front end, so both run exactly the same node (D84). A running node can
**reload**: apply a changed node policy, such as a new follow list,
without restarting the process (decision D92).

## API

```go
err := daemon.Run(ctx, dir, daemon.Options{
	Log:    os.Stderr,           // node log lines, written one line at a time
	Ready:  func(addr string){}, // called when the interface is up, again after every reload
	Reload: reloadCh,            // a value here makes the node reload
})
```

`Run` returns when `ctx` is done (nil) or the node fails to start or
reload (the error). `poo node` maps SIGHUP to `Reload`. The local
interface also reloads the node on `POST /v1/reload` and after
`POST /v1/follow`.

A reload is refused while `node.json` does not load and validate: the
node logs an error and keeps running on the policy it has, and
`POST /v1/reload` answers 422 `invalid` with the reason, so `poo reload`
fails and says why (GREEN-011). Without this a mistake in the file would
stop the node for good at the reload. The check and the reopen are not
atomic: a file spoiled between them still stops the node, as before.

## Grinding

The daemon runs one [grind](grind.md) `Manager` per node (M11, D109). It
takes its defaults from `node.json`'s `grind` section, sends its events
to the watch hub as kind `grind`, and starts the section's jobs by
itself (`Manager.Schedule`, which `PUT /v1/grind/schedule` calls again
when the section changes, with no reload). It is closed before the hub, on a reload and at a stop, so every
job ends and says so before the stream does; the policy's jobs start
again after a reload. `pkg/daemon/grind_test.go` covers it.

## What a reload does

1. The watch hub closes, so every watch stream ends with an `end` line.
2. The interface and the p2p host shut down; the node is closed and its
   stores flushed.
3. The node is opened again with the node policy now on disk, which
   replays the stores of the pools it now follows.
4. The host, interface and a new hub start; `rpc.addr` is written again
   (the port may change when the policy's listen address has port 0) and
   `Ready` is called.

Stores of pools no longer followed stay on disk, so following them again
later replays them instead of fetching them. Events between steps 1 and 4
are not replayed to watchers (see [watch.md](watch.md)). A policy that
fails to load stops the node with the error, as a bad policy would at
start-up; `poo follow` validates before saving, so this needs a hand
edit of `node.json`.

## Tests

`daemon_test.go` (`TestFollowAppliesLive`) runs a node, follows another
pool through the interface, and checks that the running node took it.
`pkg/cli`'s `TestViewFollowAndWatch` drives a reload from the command
line while a watcher is connected.
