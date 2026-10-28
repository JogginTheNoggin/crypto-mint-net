# GREEN-009: `-serial` for `build` and `work`

| Field | Value |
| --- | --- |
| Status | adopted |
| Based on | `claude/m10-watch` @ `c71f774` |
| Area | `pkg/cli` (`build`, `work`) |
| Effort | small |
| Decided by | Grey, Source Round 3: `poo build -serial` and `poo work -serial` |

## Summary

`poo claim` and `poo pay` take `-serial N`, but `poo build` and `poo work`
only take `-pool K`. A user has to work out which pool holds their mint
before they can settle a payment. With this change, `build` and `work` also
take `-serial N` and use the pool that holds that mint. Prototyped, with a
test.

## Problem

The worked example's first steps are `poo claim -serial 5` and then
`poo build -pool 1`. Users have to know that the pool size is 4 and do the
arithmetic themselves. If they get it wrong, the command acts on a pool
that does not hold their transaction, and nothing in the reply points at
the real mistake.

## Idea

A shared `poolFlags` (`-pool`, `-serial`) with `resolve(fs, series)`:
- `-serial` alone picks `series.PoolOf(serial)`.
- Both flags must agree: `-serial 5 -pool 0` fails with "-serial 5 is in
  pool 1, not -pool 0".
- A serial past the series' mints is refused.
- Neither flag keeps today's default, pool 0.

`fs.Visit` tells the flags apart from their defaults, so `-serial 0` works.

## Cost and risk

About 40 lines, with no change to any reply or RPC. The GUI gets the same
flags, since it runs the same code.

## Prototype

On `claude/source-green`: `poolFlags` and `addPool` in `pkg/cli/node.go`,
used by `buildCmd` and `workCmd`; `pkg/cli/pool_test.go`; the usage lines
and the `poo.md` rows. It was checked by hand on a live node: `claim
-serial 5`, then `build -serial 5 -work 4096`, built and layered pool 1.
