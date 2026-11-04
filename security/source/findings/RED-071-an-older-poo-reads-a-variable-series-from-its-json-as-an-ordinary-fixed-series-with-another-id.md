# RED-071: An older poo reads a variable series from its JSON as an ordinary fixed series with another id

| Field | Value |
| --- | --- |
| Severity | Low |
| Status | open |
| Found | Source Round 24 Part B, on `main` @ `08404df` (D163), against `215d428` as the older poo |
| Area | `pkg/series` (`Params` JSON), `pkg/nodepolicy` (`Load`), `pkg/cli` (`series info`, `init -series`) |
| For Black | No rule changes. This is about how a series is handed around as JSON; the byte form is right. |

## What is wrong

D163 says "a node from before this change reads a variable series as
unreadable, never as invalid" (PoO section 16.7), and that holds for the
byte form: format version 2 is refused by an older `DecodeParams`.

The JSON form does not have that property. A series travels as JSON in
node.json (`series[].params`), in the files `poo series new` prints and
`poo init -series FILE` reads, and in tools. The older `series.Params` has no
`supply_type`, `part_size` or `max_parts`, and `json.Unmarshal` (used by
`nodepolicy.Load` and the series tools) ignores keys it does not know. So an
older poo reads the variable series' JSON as a **fixed** series with the same
mints, supply and pool size, and a different id.

## How I checked it

The variable series of D163's example (40 mints, pools of 4, parts of 8):

```
08404df  poo series info v.json   series_id 8dfdf9a3...  (variable)
215d428  poo series info v.json   series_id d6ebe25d...  (a fixed series of 40 mints)
215d428  poo doctor -dir n        "ok  series  2 accepted"   (n/node.json lists the variable series)
```

The older node accepts it without a word and would follow `d6ebe25d...`, a
series nobody else uses. The follow rule that names `8dfdf9a3...` is "not
known" there.

## What could happen

Nothing breaks on the network: `d6ebe25d...` is a different series id, so no
peer, entry or coin of the variable series is confused with it. But a person
on an older poo who is handed a variable series' JSON ("follow this series")
sees it accepted, can claim mints of the look-alike fixed series, and thinks
they hold the real thing. The rule that exists to prevent exactly that (an
unknown number makes the series unreadable) does not cover the JSON.

## Suggested fix (Blue's call)

An older binary cannot be changed, but what newer binaries write can be made
loud for it:

- Write a variable series where an older reader fails instead of ignoring it:
  in node.json under another key than `params`
  (`{"variable": {...}}`), so an older `Validate` finds an entry with no
  params and refuses the policy; and have `poo series new -variable` print a
  wrapper (`{"format": 2, "params": {...}}` is not enough, since the key is
  ignored), for instance with `"hash"` left out so an older `series.New`
  fails. Newer binaries read both forms.
- Print the `series_id` beside the JSON (`poo series new`, `poo series info`)
  and tell people to compare it, since a fixed series with another id is not
  the series.
- Say in `docs/variable-supply.md` and the encoding doc that the JSON is not
  safe for an older poo, and name the first version that reads format 2.

Also: `nodepolicy.Load` and `readSeriesFile` ignore unknown keys, which will
bite again with the next extension. A strict decode of `params` (unknown key
means unreadable) in the new binaries would make the next change safe.

## Not tested

A real older release (I used `215d428` as the older code); the GUI's series
view.
