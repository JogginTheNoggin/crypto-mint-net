# ORANGE-024: Add a variable series id without copying the whole set

| Field | Value |
| --- | --- |
| Status | accepted by GUI Grey (R33A); routed to Source Grey for pkg/series |
| Based on | main @ 5f80582 |
| Area | `addVariableID` and `IsVariableID` in pkg/series/released.go (Source). The GUI sees it through `poon doctor` (Health check), `poon node` (Start node) and every in-process command that builds the followed series |
| Effort | small |
| Decided by | |

## Summary

Every process that builds a variable series adds the series' id to a
process-wide set (`variableIDs`, ORANGE-035), so a transaction's bytes
can be read. To keep reads free of locks, `addVariableID` builds a new
map with every id so far, plus the new one, and swaps it in. Building N
variable series therefore copies about N²/2 ids. At 4,096 series, the
most a node can follow, that is 8.4 million map inserts, about one
second of CPU each time a process loads the node policy.

The GUI shows this in two places:

- **Node view:** the Health check runs `poon doctor`, which took 1.7 to
  2.0 s at 4,096 variable series, most of it in this copy. The view
  takes as long to settle as its slowest panel.
- **Start node:** `poon node` took 1.5 to 1.7 s to answer, against 0.38
  to 0.43 s with the set grown in place.

A local prototype (a `sync.Map`, so reads stay lock-free) gave these
results, back to back on the same home:

| At 4,096 variable series | main @ 5f80582 | prototype |
| --- | --- | --- |
| `poon node`, until the RPC answers | 1.5 to 1.7 s | 0.38 to 0.43 s |
| `poon doctor -o json` | 1.7 to 2.0 s | 0.68 to 0.80 s |
| Node view settled (3 runs each) | 2.0 to 2.5 s | 1.08 to 1.36 s |
| Node view busy | 1.07 to 1.82 s | 0.74 to 1.0 s |

The code is Source's, so this proposal goes to Source. It changes how
the set is stored, not what it holds.

## Problem

```go
m := make(map[hash.Hash]struct{}, 1)
for k := range *old { m[k] = struct{}{} }
m[id] = struct{}{}
variableIDs.Store(&m)
```

Each new id copies every id added before it, into a map that starts at
size 1 and rehashes as it grows. A CPU profile of `poon doctor` at 4,096
variable series:

| Where | Share of 1.77 s |
| --- | --- |
| `nodepolicy.Policy.Accepted` | 68% |
| of which `series.addVariableID` | 51% (most of it map growth and rehashing) |
| of which issuer signatures (`CheckIssuer`) | 17% |

The copy was chosen in ORANGE-035 for a set that holds a few series,
where it is cheap. The worst case only shows at the size limit, and
pkg/p2p allows 4,096 series.

Inside poon-gui the set lasts for the process, and an id already present
returns early. So only the first command after poon-gui starts pays in
full. Every `poon` started on its own, `poon node` included, pays each
time.

## Measurements

These were taken on the probe home that follows 4,096 signed variable
series (8,192 pools; see ORANGE-019), with the real `poon` and
`poon-gui`. The prototype was built from main @ 5f80582 with the change
below and nothing else. It is not committed, because pkg/series belongs
to Source.

- **`poon node`:** time from start until `rpc.addr` appears, two runs of
  each build alternately: 1,701 and 1,494 ms on main, 425 and 375 ms
  with the prototype.
- **Other commands at the same time:** `poon series followed` took
  154–192 ms against 167–172 ms, and `poon follow` 242–247 ms against
  234–266 ms. These build the series in the node, so they do not change.
- **Node view:** Chromium through Playwright opened the Node view on a
  fresh poon-gui each time, alternating builds, three runs each. "Settled"
  means every panel has answered.
- **Decoding:** `go test ./pkg/tx -bench Decode` gave 378–395 ns/op on
  main and 377–396 ns/op with the prototype. The set is empty there, so
  this shows that the empty fast path is kept.

## Idea

Grow the set in place, keeping lock-free reads, for example:

```go
var (
	variableIDs sync.Map // hash.Hash -> struct{}
	variableAny atomic.Bool
)

func addVariableID(id hash.Hash) {
	variableIDs.LoadOrStore(id, struct{}{})
	variableAny.Store(true)
}

func IsVariableID(id hash.Hash) bool {
	if !variableAny.Load() {
		return false
	}
	_, ok := variableIDs.Load(id)
	return ok
}
```

`sync.Map` is designed for keys that are written once and read many
times, and a load of a key already promoted takes no lock. Source may
prefer another shape, for example keeping the copy but adding ids in
one batch when a policy loads. The point is to make loading 4,096
series take linear time, not quadratic.

## Cost and risk

- About 20 lines in one file, and `go test ./pkg/series ./pkg/tx`
  passes with the prototype.
- **Risk:** none to consensus. The set holds the same ids, and
  `IsVariableID` gives the same answers. Only the way it is stored
  changes. Black should still see it, because `tx` decoding reads it.
- **Compatibility (White):** nothing on disk or on the wire changes.

## Prototype

The diff above, measured locally. It is not on this branch, because it
is Source's code.

## Modularity

Nothing to challenge.

## Consensus

None: same set, same answers.
