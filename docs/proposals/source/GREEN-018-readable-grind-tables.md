# GREEN-018: Readable tables for nested objects and rates

| Field | Value |
| --- | --- |
| Status | proposed (prototyped) |
| Based on | `claude/m10-watch` @ `29b08ce` (code `a92c2ca`) |
| Area | `pkg/cli/table.go` |
| Effort | small |
| Decided by | Grey |

## Summary

`poo grind bench -o table` and `poo grind jobs -o table` printed `{6}`
where the plan should be, and rates such as `27737159.883412345`. The
generic table now spreads a nested object of up to eight scalar fields
into dotted columns (`PLAN.DEVICE`, `PLAN.THREADS`, ...), counts an array
of scalars as a scalar, and keeps two decimals of a fractional number.
`-wide` and the JSON form are unchanged. Prototyped, with a test.

## Problem

The table from GREEN-003 spread a nested object into columns only when it
had at most four scalar fields. The grinder's plan has six (device, kind,
grinder, hash, threads, reason), so every grind table showed `{6}`, and a
job's spec showed `{5}`. A spec holding `pools: [1,2]` was not flat
either, because any array made an object "not flat". Rates are float64,
and the table printed every digit.

## Idea

- `flat` accepts arrays of scalars, which print joined (`1,2`).
- A nested flat object spreads into columns up to `maxFlatKeys` = 8.
- A JSON number with more than two decimals is rounded to two in table
  mode, unless `-wide` is given. Whole numbers, which include every
  amount, are never touched.

After the change, `poo grind bench -seconds 0.2 -o table` prints:

```
PLAN.DEVICE  PLAN.KIND  PLAN.GRINDER  PLAN.HASH  PLAN.THREADS  PLAN.REASON                                     TRIES    SECONDS  RATE
cpu          cpu        cpu-midstate  sha256     3             asked for: the cpu-midstate grinder for sha256  5578752  0.20     27737159.88
```

## Testing

`TestTableNestedAndRates` in `pkg/cli/table_test.go` checks the dotted
columns, the joined pools, the rounding, and that `-wide` keeps every
digit. The existing table tests pass unchanged.

## Risks

A wider object now makes a wider table. Eight keys is a guess, and a
later reply with a bigger nested object is still counted.
