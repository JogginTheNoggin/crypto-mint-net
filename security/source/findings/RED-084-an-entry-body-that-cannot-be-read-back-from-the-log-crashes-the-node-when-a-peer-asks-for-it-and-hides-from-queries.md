# RED-084: An entry body that cannot be read back from the log crashes the node when a peer asks for it, and is hidden from queries

| Field | Value |
| --- | --- |
| Severity | High (a remote peer crashes the node; the trigger is a damaged or changed log) |
| Status | open |
| Found | Source Round 28 Part B, on `main` @ `705639a` (D172) |
| Area | `pkg/p2p` (`serve.go`: `item`, `parts`; `chase.go`), `pkg/ledger` (`Pool.Entry`, `Rivals`), `pkg/watch` (`query.go`), `pkg/engine` (`Payment`) |
| For Black | No consensus rule is touched. No verdict reads a body, as D172 says; this is about what the callers do when a read fails. |
| Fixed by | |

## What is wrong

D172 drops an entry's body from memory once its record is in the log, and
`Pool.Entry(h)` reads it back. A read can now fail where it never could
before: the record is damaged, the file was truncated or replaced, the disk
returns an error. `Entry` then returns `(nil, false)`, and D172 says this
"hides the body from that query only". But the callers that existed before
D172 never expected `false` for a held entry:

- `p2p/serve.go:162` (`item`, used by `entries`) and `:284` (`parts`) do
  `e, _ := l.Entry(h)` and build `wire.EntryItem{Entry: e}`. `it.Size()`
  then calls `Entry.EncodeTo` on a nil pointer and **panics in the host's
  work loop**. A panic in that goroutine ends the process.
- `watch/query.go:126` and `:367` do `en, _ := p.Entry(eh)` and use
  `len(en.Txs)` / `txsOf(s, en)`: a nil dereference in the RPC handler
  (the HTTP server recovers it as a 500, but under the engine's lock).
- `ledger.Rivals` skips an entry whose body it cannot read, `Engine.Payment`
  returns `Selected` with no `Tx`, and the chase walk in `p2p/chase.go:510`
  stops early. None of these says the answer is incomplete, so a receiver
  asking "any rival to this payment?" gets "none" when the node simply could
  not read the rival's entry.

The node does not notice the damage in any other way: the skeleton in memory
is complete, selection and switching are unaffected, `bodies.errors` rises
in `poo status`, and `poo doctor` says nothing.

## How I checked it

`pkg/p2p` test harness (throwaway, removed). Node `a` holds 6 entries, its
body cache is set to 0 and the bodies released (`InMemory: 0`, `Located:
6`). Then its pool log is changed under it (one byte flipped every 50 bytes)
and a peer `b` connects and syncs:

```
panic: runtime error: invalid memory address or nil pointer dereference
entry.(*Entry).EncodeTo(0x0, ...)         entry/entry.go:99
entry.(*Entry).Size                       entry/entry.go:93
wire.EntryItem.Size                       wire/messages.go:412
p2p.replier.entries                       p2p/serve.go:220
p2p.replier.build                         p2p/serve.go:190
p2p.(*Host).serve.func1                   p2p/serve.go:62
engine.(*Engine).Ledger                   engine/engine.go:1215
p2p.(*Host).workLoop                      p2p/inbox.go:114
```

With a single record damaged the result is the same as soon as a peer asks
for that entry. A flipped byte in a layer record only (the first try, one
byte in the middle) did not reach an entry and there was no error. Reads
from disk and compaction beside them held: 6 readers, the cache bound
switched between 0, 1 KiB and 1 GiB, 60 compactions, `-race`: 28,116 reads,
0 misses, 0 errors.

## What could happen

- A node whose log is damaged after start (bit rot, a failing disk, a
  restore or `logrotate copytruncate` over a running node, a log trimmed by
  hand) is crashed by **any** peer that asks for the unreadable entry, as
  often as the node restarts, since the peers ask again after it reconnects.
  Before D172 a node could only fail this way at start, where the log is cut
  at the first bad record.
- A wallet or receiver reading `payment`/`rivals` can be told "no rivals".
- The damage is invisible until the crash.

## Suggested fix (Blue's call)

- A failed read of a held entry is "not held right now" for the caller, never
  a nil entry: `item`/`parts`/`entries` reply `not held` or end the reply
  before it (as `tooLarge` does), without encoding; `watch` skips the entry
  and says in the reply how many it could not read (`unreadable: n`);
  `Payment` and `Rivals` say the answer is incomplete.
- Make `Pool.Entry` hard to misuse: return an error, so `e, _ :=` cannot
  compile silently, or check every caller (grep `\.Entry(`).
- Recover from a damaged body: the node can fetch the entry again from a peer
  (its hash is known) and append it as a new record, so `bodies.errors` falls
  to 0 by itself. Until then `poo doctor` warns ("N entry bodies cannot be
  read from the log") and `poo status` shows it.
- Defence in depth: recover from a panic in a peer's message handler by
  closing that peer, so any nil left in a serving path cannot stop the node.
- Tests: damage one entry record under a running node with a cold cache; a
  peer's `entries` and `parts` requests do not panic; `watch` and `Payment`
  say the answer is incomplete.

## Not tested

A truncated file and a log replaced by another pool's log (the unit test
`TestPoolBodiesChecks` covers the hash check on a record of another entry);
start-up memory against the log's size (Orange measures it); the cost of
reading and hashing a large body under the engine lock when the cache is cold.
