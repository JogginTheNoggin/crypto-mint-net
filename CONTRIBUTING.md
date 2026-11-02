# Contributing

## Summary

Changes come in as pull requests against `main`, one milestone or one focused
change at a time. Every change keeps the build, `go vet`, `gofmt` and the full
test suite (with the race detector) green, adds tests for what it changes, and
updates the documentation it affects. Anything that touches consensus (what is
valid, how weight is computed, which ledger is selected, or any byte form)
must also be recorded in [docs/decisions.md](docs/decisions.md) and must keep
the test vectors stable unless the change is meant to alter them.

## Setting up

- Install Go 1.24 or later. There are no other dependencies.
- `go build ./...` builds everything; `go test ./...` runs the tests.

## Before you open a pull request

```sh
gofmt -l .                 # must print nothing
go vet ./...
go test -race ./...
go test ./pkg/vectors      # must pass unless you meant to change a byte form
```

CI runs the same steps on every push and pull request.

## How the code is organized

Read [docs/architecture.md](docs/architecture.md) first. In short, packages
form layers, and a package only imports packages below it:

```
cmd/poo
  rpc
    p2p
      wire
    node
      engine    store    nodepolicy
    ledger      mempool
      entry  layer  state
        tx
          series  policy
            hash  weight  encoding
```

Keep it that way. A new feature usually belongs in a new package rather than
in a larger existing one. Network and storage code must never be imported by
the consensus packages (`tx`, `entry`, `layer`, `state`, `ledger`) or by the
engine; the engine is the boundary, `node` is where storage meets it, and
`p2p` is where the network meets the node.

## Where a rule belongs

The design sorts every rule into three tiers, and the code follows it:

- **Consensus**: the same for every series and every node (validity, weight,
  selection). Lives in `tx`, `entry`, `layer`, `state`, `ledger`.
- **Series**: fixed per series (hash, supply, pool size, minimums, allowed
  types). Lives in `series` and is read through `*series.Series`.
- **Node policy**: may differ between nodes without anyone disagreeing about
  ownership (K, limits, what to follow, forwarding choices, storage). Lives
  in the node policy file (`pkg/nodepolicy`) and the configuration structs it
  fills (`ledger.Config`, `engine.Config`), with documented defaults.

When adding a rule, ask the design's two questions. If two nodes following the
same series disagreed on it, could they select different ledgers? If not, it
is node policy. If they could, is it the same for every series? If so, it is
consensus; otherwise it belongs to the series.

## Tests

- Unit tests sit next to the code (`*_test.go`). Prefer external test
  packages (`package foo_test`) so tests use the public API.
- Use `pkg/testutil` for keys, the example series and signed transactions.
- The ledger has a randomized test against a brute-force model
  (`pkg/ledger/property_test.go`). If you change selection or switching,
  extend it rather than only adding examples.
- Tests must be deterministic: seed any randomness.

## Style

- Standard Go style; run `gofmt`.
- Every package has a package comment saying what it is and which part of the
  design it implements. Exported names have doc comments.
- Use the design's terms (see [docs/glossary.md](docs/glossary.md)): series,
  mint, pool, portion, entry, layer, credit, weight, selected ledger, claim,
  stated weight, mint policy, node policy. Avoid inventing new terms; when one
  is unavoidable, define it in the glossary.
- Errors are sentinel values wrapped with context, so callers can use
  `errors.Is`.

## Documentation

- Update the module guide in `docs/modules/` for any package you change.
- Every document starts with a **Summary** section that says what it covers
  and what it decides. When you change a document significantly, update its
  summary.
- Record design decisions and deviations in `docs/decisions.md`, one entry
  each, with the reason.

## Commits and pull requests

- Small, focused commits with messages that say what and why.
- Fill in the pull request template: what changed, why, how it was tested, and
  whether it touches consensus.
- Never commit secrets, real keys or credentials.
