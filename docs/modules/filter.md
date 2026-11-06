# pkg/filter

## Summary

One way to say which items a viewer or watcher wants. A `Filter` selects
by series, pool, mint serial, address, address policy, transaction id and
kind of event. The node's views, its live watch stream, the RPC and the
command line all take the same value, so every front end, the GUI's
included, selects items the same way (decision D90).

## Rules

- An empty field matches everything; an empty filter matches everything.
- Fields are joined with **and**, values within a field with **or**:
  `-serial 5,6 -kind tx` is "a transaction event of mint 5 or 6".
- **series**: the series id.
- **pool**: a pool index of the series.
- **serial**: a mint serial. An item matches when it touches the mint: a
  transaction spending or paying a portion of it, or an entry or layer of
  the mint's pool.
- **address** and **policy** together are one field: an item matches when
  it pays or spends from one of the addresses, or from the address a
  policy makes under the item's series hash. A policy is given in hex of
  its byte form, so one filter can name a policy across series with
  different hashes.
- **tid**: a transaction id. An entry matches when it holds the
  transaction.
- **kind**: `tx`, `entry`, `layer`, `switch`, `selected`, `reverted`,
  `dropped`, `grind` (grind jobs, M11). Views have no kind: the RPC views ignore it, and the view
  commands refuse `-kind`, which only `poon watch` takes.

`MatchScope(series, pool, first, last)` answers whether anything in a
pool holding serials `first..last` could match, so views skip whole pools
before looking at their items.

## Forms

| Form | Shape |
| --- | --- |
| Go | `filter.Filter{Series, Pools, Serials, Addresses, Policies, TIDs, Kinds}` |
| Query parameters (RPC) | `series`, `pool`, `serial`, `address`, `policy`, `tid`, `kind`; each comma-separated, repeatable. `Values` and `FromValues` convert |
| Command-line flags | `-series`, `-pool`, `-serial`, `-address`, `-address-policy`, `-tid`, `-kind`; each comma-separated, repeatable. `Flags(fs)` adds them and returns a function that reads the filter after parsing; `ItemFlags(fs)` adds all but `-kind`, for the views, which hold items rather than events (GREEN-041) |

The flag for policies is `-address-policy` because `-policy` already
chooses the node policy on every command. A bad value fails with
`ErrBad` and names the field.

## API

- `Filter.Match(Subject)`, `MatchScope`, `WantsKind`, `Empty`.
- `Subject`: what an item offers to be matched. A subject's empty field
  matches no filter value of that field, except `Kind`, which is not
  checked when empty.
- `Builder.Add(name, values)` collects fields; `FromValues` and `Flags`
  both use it.

## Tests

`filter_test.go`: `TestMatch` covers the and/or rule, every field,
policy matching and `MatchScope`; `TestValuesAndFlags` the round trip
through query parameters and flags, and refusal of bad values.

## Extending

A new field needs a slice in `Filter`, a value in `Subject`, a line in
`Match` (and `MatchScope` if it narrows pools), a name in `Values`,
`FromValues` and `Builder.add`, and a flag in `Flags`. Then set it in
`pkg/watch`'s `subjectOf` so views and the stream see it.
