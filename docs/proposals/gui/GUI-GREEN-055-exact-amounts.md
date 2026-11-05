# GUI-GREEN-055: Show amounts exactly

| Field | Value |
| --- | --- |
| Status | adopted (Round 30 Part A, merged as is; the Source ask is queued at Source) |
| Based on | main @ 4e1c601 |
| Area | gui/web/static/render.js (parseJSON, scalar), views.js (showResult), views/node.js (series panels), views/watch.js (events), gui/e2e/qol.mjs, gui/e2e/views.mjs, docs/FEATURES.md, docs/gui.md |
| Effort | small |
| Decided by | (Grey) |

## Summary

The GUI showed some amounts **wrong**. poo prints amounts, balances and
totals as JSON numbers (Go `uint64`), and the GUI read every result with
`JSON.parse`. JavaScript numbers are exact only up to 2^53
(9,007,199,254,740,991), so any larger whole number was shown rounded.
This happens with no error. The output poo printed was right; only what
the GUI drew from it was wrong.

On the reserve series (10^14 units a mint, D170), 91 mints already pass
2^53, so a wallet's `total` or an address's `balance` could be off by a
few units. A series with more than 2^53 units in a mint shows the error
straight away. I checked this with a real `poo` on main @ 4e1c601:
`poo series new -mints 32 -pool-size 32 -supply 100000000000000001`
printed `"supply": 100000000000000001`, and the New series parameters
form showed **100000000000000000**.

This proposal adds `parseJSON`, which keeps every whole number exact,
and uses it wherever a result is drawn.

## Problem

- **Amounts are what people check.** A balance or a supply that is off by
  one unit says the GUI cannot be trusted with money, even when poo is
  right.
- **Source half-solved this already.** Pool weights and a series' total
  `units` are printed as decimal strings. `amount`, `balance`, `total`
  and a series' `supply` are still numbers. With the reserve as the
  default, these are the fields that grow large.
- **Nothing showed it.** No error, no warning. Copy output copies poo's
  exact text, so a person comparing the two would see a mismatch and not
  know which to believe.

## Idea

- **`parseJSON(text)` in render.js.** It is `JSON.parse`, except that a
  whole-number literal outside the safe range is read as a `BigInt`.
  - A fast path skips the scan when the text has no run of 16 digits, so
    ordinary results cost the same as before.
  - Otherwise one pass over the text skips strings (escapes included), so
    digits inside text are left alone. Each long integer is wrapped in a
    marked string, and a reviver turns it back into a `BigInt`.
  - Fractions, exponents and safe integers are JSON.parse's own.
- **`render`** prints a `BigInt` digit for digit, with the same `num`
  style as a number.
- **Where it is used:**
  - `showResult`, which covers every form and panel result: wallet
    balance, an address, series info and new, parts, and so on;
  - the Followed series and Variable series panels;
  - Watch's events.

  Code that does arithmetic on parsed values (counts, heights, the doctor
  report) keeps `JSON.parse`, so a `BigInt` never meets a number in a sum.

## For Source (not built here)

- **Print amounts as decimal strings**, as `weight` and `units` already
  are: `amount`, `balance`, a wallet's `total` and a series' `supply` in
  the JSON outputs and the RPC. Every JSON reader (scripts, jq, other
  languages) has the same 2^53 problem, not only the GUI. Orange named
  "decimal-string amounts" as needed before market scale in the reserve
  advice. This would change the RPC's JSON shape, so it is for Grey,
  Black and jerry to decide. `parseJSON` keeps the GUI right until then,
  and stays harmless afterwards.

## Cost and risk

About 30 lines in render.js. A result with no 16-digit run takes the
plain path. Code that reads specific fields is unchanged, except the two
series panels and Watch, which only print what they read. No rule,
consensus or policy change: the GUI shows exactly what poo printed.

## Prototype

This PR.
- views.mjs runs New series parameters with `-supply 100000000000000001`
  against the real poo and checks that the supply row reads exactly that.
  With `JSON.parse` it reads `100000000000000000`; I checked that the
  test fails without the fix.
- qol.mjs checks `parseJSON` on its own:
  - 2^53 + 1 and -(2^64 - 1) come back as exact `BigInt`s;
  - digits and escaped quotes inside a string are untouched;
  - a safe integer and a fraction stay numbers.
