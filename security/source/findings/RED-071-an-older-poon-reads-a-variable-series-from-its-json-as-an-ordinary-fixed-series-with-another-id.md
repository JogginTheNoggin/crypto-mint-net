# RED-071: An older poon reads a variable series from its JSON as an ordinary fixed series with another id

| Field | Value |
| --- | --- |
| Severity | Low |
| Status | fixed |
| Found | Source Round 24 Part B, on `main` @ `08404df` (D163), against `215d428` as the older poon |
| Area | `pkg/series` (`Params` JSON), `pkg/nodepolicy` (`Load`), `pkg/cli` (`series info`, `init -series`) |
| For Black | No rule changes. This is about how a series is handed around as JSON; the byte form is right. |
| Fixed by | `claude/project-thread-ggo8dt` @ `c05286b` (BLUE-025) |

## What is wrong

D163 says "a node from before this change reads a variable series as
unreadable, never as invalid" (PoON section 16.7), and that holds for the
byte form: format version 2 is refused by an older `DecodeParams`.

The JSON form does not have that property. A series travels as JSON in
node.json (`series[].params`), in the files `poon series new` prints and
`poon init -series FILE` reads, and in tools. The older `series.Params` has no
`supply_type`, `part_size` or `max_parts`, and `json.Unmarshal` (used by
`nodepolicy.Load` and the series tools) ignores keys it does not know. So an
older poon reads the variable series' JSON as a **fixed** series with the same
mints, supply and pool size, and a different id.

## How I checked it

The variable series of D163's example (40 mints, pools of 4, parts of 8):

```
08404df  poon series info v.json   series_id 8dfdf9a3...  (variable)
215d428  poon series info v.json   series_id d6ebe25d...  (a fixed series of 40 mints)
215d428  poon doctor -dir n        "ok  series  2 accepted"   (n/node.json lists the variable series)
```

The older node accepts it without a word and would follow `d6ebe25d...`, a
series nobody else uses. The follow rule that names `8dfdf9a3...` is "not
known" there.

## What could happen

Nothing breaks on the network: `d6ebe25d...` is a different series id, so no
peer, entry or coin of the variable series is confused with it. But a person
on an older poon who is handed a variable series' JSON ("follow this series")
sees it accepted, can claim mints of the look-alike fixed series, and thinks
they hold the real thing. The rule that exists to prevent exactly that (an
unknown number makes the series unreadable) does not cover the JSON.

## Suggested fix (Blue's call)

An older binary cannot be changed, but what newer binaries write can be made
loud for it:

- Write a variable series where an older reader fails instead of ignoring it:
  in node.json under another key than `params`
  (`{"variable": {...}}`), so an older `Validate` finds an entry with no
  params and refuses the policy; and have `poon series new -variable` print a
  wrapper (`{"format": 2, "params": {...}}` is not enough, since the key is
  ignored), for instance with `"hash"` left out so an older `series.New`
  fails. Newer binaries read both forms.
- Print the `series_id` beside the JSON (`poon series new`, `poon series info`)
  and tell people to compare it, since a fixed series with another id is not
  the series.
- Say in `docs/variable-supply.md` and the encoding doc that the JSON is not
  safe for an older poon, and name the first version that reads format 2.

Also: `nodepolicy.Load` and `readSeriesFile` ignore unknown keys, which will
bite again with the next extension. A strict decode of `params` (unknown key
means unreadable) in the new binaries would make the next change safe.

## Not tested

A real older release (I used `215d428` as the older code); the GUI's series
view.

## Resolution

Fixed (D165). I followed Red's first and last points.

- A variable series' JSON names its hash under `variable_hash` and leaves
  `hash` out (`Params.MarshalJSON`, `pkg/series/json.go`). This covers
  every place the JSON travels: node.json, `poon series new` and the files
  `poon init -series` reads.
- A poon from before D163 then finds hash 0 and calls the series
  unreadable. Checked with the 215d428 binary on a file from this branch:
  - `poon series info v.json` fails with `unreadable at this node: hash:
    unknown hash function number: 0`.
  - `poon doctor` on a node.json listing it says "0 accepted, 1 not",
    where it said "1 accepted" before.
- A fixed series' JSON is unchanged.
- `UnmarshalJSON` reads both forms (D163's first form, with `hash`,
  still reads). It refuses a key it does not know (`ErrUnknownKey`), and
  `variable_hash` on a fixed series, so the next extension is unreadable
  instead of misread.
- `poon series info` already prints `series_id`. `docs/variable-supply.md`
  now says to compare it.
- Not done: a wrapper around `poon series new`'s output. Leaving out
  `hash` already makes an older reader fail, and a wrapper would change
  the file that `poon init -series` reads.
- Test: `TestVariableJSONIsUnreadableToOlderReaders` (pkg/series).
